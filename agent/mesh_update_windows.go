//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cursor/fleet-agent/mesh"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

type meshUpdateRequest struct {
	CommandID  string `json:"commandId"`
	Release    string `json:"release"`
	DataDir    string `json:"dataDir"`
	Executable string `json:"executable"`
	HealthURL  string `json:"healthUrl"`
	CreatedAt  int64  `json:"createdAt"`
}
type meshUpdateResult struct {
	CommandID string      `json:"commandId"`
	Release   string      `json:"release"`
	Result    mesh.Result `json:"result"`
}

func saveMeshJSON(path string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "update-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
func copyMeshBinary(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0700)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if err == nil {
		err = out.Sync()
	}
	closeErr := out.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func stageMeshUpdate(ctx context.Context, n *mesh.Node, c mesh.Command, dir, exe string) mesh.Result {
	failed := func(err error) mesh.Result { return mesh.Result{Status: "error", Message: err.Error()} }
	if _, err := os.Stat(filepath.Join(dir, "service-installed")); err != nil {
		return failed(errors.New("self-update requires the installed mesh service"))
	}
	id, err := n.FetchRelease(ctx, c.Payload["release"])
	if err != nil {
		return failed(err)
	}
	manifest, err := n.Releases.Manifest(id)
	if err != nil {
		return failed(err)
	}
	if manifest.SHA256 == n.BinaryHash {
		return mesh.Result{Status: "ok", Message: "This release is already installed"}
	}
	var installed struct {
		Sequence uint64 `json:"sequence"`
	}
	if b, err := os.ReadFile(filepath.Join(dir, "installed-release.json")); err == nil {
		if json.Unmarshal(b, &installed) != nil {
			return failed(errors.New("installed release record is corrupt"))
		}
		if manifest.Sequence <= installed.Sequence {
			return failed(errors.New("release sequence must advance; downgrade refused"))
		}
	}
	updates := filepath.Join(dir, "updates")
	if err = os.MkdirAll(updates, 0700); err != nil {
		return failed(err)
	}
	healthURL := "http://127.0.0.1:43123/api/v2/health"
	if u, err := url.Parse(n.Self.DashboardURL); err == nil {
		healthURL = "http://127.0.0.1:" + u.Port() + "/api/v2/health"
	}
	req := meshUpdateRequest{CommandID: c.ID, Release: id, DataDir: dir, Executable: exe, HealthURL: healthURL, CreatedAt: time.Now().UnixMilli()}
	if err = saveMeshJSON(filepath.Join(updates, "request.json"), req); err != nil {
		return failed(err)
	}
	return mesh.Result{Status: "staged", Message: "Verified release staged; awaiting restart and health check"}
}

func configureMeshUpdates(n *mesh.Node, dir, exe string) {
	requestPath := filepath.Join(dir, "updates", "request.json")
	if b, err := os.ReadFile(requestPath); err == nil {
		var request meshUpdateRequest
		if json.Unmarshal(b, &request) == nil && time.Now().UnixMilli()-request.CreatedAt < int64(15*time.Minute/time.Millisecond) {
			if c, _, err := n.Store.Command(request.CommandID); err == nil && c.Target == n.Self.ID && c.Kind == "self_update" {
				if r, err := n.Store.Receipt(c.ID); err == nil && r.Status == "running" {
					n.PendingUpdate = c.ID
				}
			}
		}
	}
	n.OnStagedUpdate = func(c mesh.Command) error {
		helper := filepath.Join(dir, "updates", "helper-"+c.ID+".exe")
		if err := copyMeshBinary(exe, helper); err != nil {
			return err
		}
		cmd := exec.Command(helper, "--mesh", "apply-update", "--request", requestPath)
		cmd.SysProcAttr = &windows.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
		return cmd.Start()
	}
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-n.Done():
				return
			case <-ticker.C:
			}
			requestBytes, err := os.ReadFile(requestPath)
			if err != nil {
				continue
			}
			var req meshUpdateRequest
			if json.Unmarshal(requestBytes, &req) != nil {
				continue
			}
			c, _, err := n.Store.Command(req.CommandID)
			if err != nil || c.Target != n.Self.ID || c.Kind != "self_update" {
				continue
			}
			if old, err := n.Store.Receipt(c.ID); err == nil && (old.Status == "ok" || old.Status == "error") {
				_ = os.Remove(requestPath)
				_ = os.Remove(filepath.Join(dir, "updates", "receipt.json"))
				continue
			}
			b, err := os.ReadFile(filepath.Join(dir, "updates", "receipt.json"))
			var receipt meshUpdateResult
			if err != nil || json.Unmarshal(b, &receipt) != nil || req.CommandID != receipt.CommandID || req.Release != receipt.Release {
				if time.Now().UnixMilli()-req.CreatedAt > int64(15*time.Minute/time.Millisecond) {
					_ = n.Store.Complete(c, mesh.Result{Status: "unknown", Message: "Update helper did not finish; inspect the service and installed binary before retrying"})
					_ = os.Remove(requestPath)
				}
				continue
			}
			if err = n.Store.Complete(c, receipt.Result); err != nil {
				logf("update receipt persistence: %v", err)
				continue
			}
			_ = os.Remove(requestPath)
			_ = os.Remove(filepath.Join(dir, "updates", "receipt.json"))
		}
	}()
}

func serviceTransition(service *mgr.Service, start bool) error {
	if start {
		if err := service.Start(); err != nil && err != windows.ERROR_SERVICE_ALREADY_RUNNING {
			return err
		}
	} else {
		if _, err := service.Control(svc.Stop); err != nil && err != windows.ERROR_SERVICE_NOT_ACTIVE {
			return err
		}
	}
	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		state, err := service.Query()
		if err != nil {
			return err
		}
		if start && state.State == svc.Running || !start && state.State == svc.Stopped {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return errors.New("service transition timed out")
}

// The callbacks permit fault injection without touching real Windows services.
// Rollback never replaces a binary until the failed service is stopped.
func meshSwapBinary(staged, installed, expected string, stop, start, health func() error) (bool, error) {
	if err := stop(); err != nil {
		return false, err
	}
	backup := installed + ".previous"
	if err := copyMeshBinary(installed, backup); err != nil {
		_ = start()
		return false, err
	}
	err := copyMeshBinary(staged, installed)
	if err == nil {
		hash, hashErr := mesh.FileHash(installed)
		if hashErr != nil || hash != expected {
			err = errors.New("installed checksum mismatch")
		}
	}
	if err == nil {
		err = start()
	}
	if err == nil {
		err = health()
	}
	if err == nil {
		return false, nil
	}
	if stopErr := stop(); stopErr != nil {
		return false, fmt.Errorf("update failed: %v; could not stop failed service: %w", err, stopErr)
	}
	if restoreErr := copyMeshBinary(backup, installed); restoreErr != nil {
		return false, fmt.Errorf("update failed: %v; rollback copy failed: %w", err, restoreErr)
	}
	if restartErr := start(); restartErr != nil {
		return true, fmt.Errorf("previous binary restored after %v; restart failed: %w", err, restartErr)
	}
	return true, err
}

func applyMeshUpdate(requestPath string) error {
	if requestPath == "" {
		return errors.New("update request required")
	}
	b, err := os.ReadFile(requestPath)
	if err != nil {
		return err
	}
	var req meshUpdateRequest
	if json.Unmarshal(b, &req) != nil {
		return errors.New("invalid update request")
	}
	root, err := filepath.Abs(req.DataDir)
	if err != nil {
		return err
	}
	expected := filepath.Join(root, "updates", "request.json")
	actual, _ := filepath.Abs(requestPath)
	if !strings.EqualFold(expected, actual) {
		return errors.New("update request must reside in the node's protected updates directory")
	}
	cache, err := mesh.OpenReleaseCache(root)
	if err != nil {
		return err
	}
	manifest, err := cache.Manifest(req.Release)
	if err != nil {
		return err
	}
	staged, err := cache.Binary(req.Release)
	if err != nil {
		return err
	}
	manager, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer manager.Disconnect()
	service, err := manager.OpenService(meshServiceName)
	if err != nil {
		return err
	}
	defer service.Close()
	config, err := service.Config()
	if err != nil {
		return err
	}
	if !strings.HasPrefix(strings.ToLower(config.BinaryPathName), strings.ToLower(`"`+req.Executable+`"`)) {
		return errors.New("update destination differs from installed service binary")
	}
	result := mesh.Result{Status: "ok", Message: "Installed " + manifest.Version + " (" + manifest.SHA256 + ")", Via: "signed release"}
	rolledBack, applyErr := meshSwapBinary(staged, req.Executable, manifest.SHA256, func() error { return serviceTransition(service, false) }, func() error { return serviceTransition(service, true) }, func() error {
		client := http.Client{Timeout: 2 * time.Second}
		healthy := false
		deadline := time.Now().Add(45 * time.Second)
		for time.Now().Before(deadline) {
			res, err := client.Get(req.HealthURL)
			if err == nil {
				var health struct {
					OK  bool   `json:"ok"`
					SHA string `json:"sha256"`
				}
				err = json.NewDecoder(io.LimitReader(res.Body, 65536)).Decode(&health)
				res.Body.Close()
				if err == nil && health.OK && health.SHA == manifest.SHA256 {
					healthy = true
					break
				}
			}
			time.Sleep(time.Second)
		}
		if !healthy {
			return errors.New("new release did not pass its health check")
		}
		return nil
	})
	if applyErr != nil {
		result = mesh.Result{Status: "error", Message: fmt.Sprintf("Update failed: %v; rollback restored=%v", applyErr, rolledBack), Via: "signed release rollback"}
		if err = saveMeshJSON(filepath.Join(root, "updates", "receipt.json"), meshUpdateResult{req.CommandID, req.Release, result}); err != nil {
			return err
		}
		return applyErr
	}
	if err = saveMeshJSON(filepath.Join(root, "installed-release.json"), manifest); err != nil {
		return err
	}
	return saveMeshJSON(filepath.Join(root, "updates", "receipt.json"), meshUpdateResult{req.CommandID, req.Release, result})
}
