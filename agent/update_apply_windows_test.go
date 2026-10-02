//go:build windows

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestUpdateInstallerAppliesVerifiesAndRollsBack(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failed installer"}[fail], func(t *testing.T) {
			stage, root := t.TempDir(), t.TempDir()
			previous, replacement := []byte("old-client"), []byte("new-client")
			os.WriteFile(filepath.Join(root, "fleet-agent.exe"), previous, 0600)
			pairing := []byte(`{"token":"keep-token","machineId":"keep-id"}`)
			os.WriteFile(filepath.Join(root, "config.json"), pairing, 0600)
			os.WriteFile(filepath.Join(stage, "fleet-agent.exe"), replacement, 0600)
			hash := sha256.Sum256(replacement)
			request := updateRequest{JobID: "apply-test", PID: 2147483000, Root: root, Revision: "fresh-commit", SHA256: hex.EncodeToString(hash[:]), Repo: "test-source"}
			raw, _ := json.Marshal(request)
			os.WriteFile(filepath.Join(stage, "request.json"), raw, 0600)
			installer := `param([switch]$NoPause,[string]$Repo,[string]$DataDir,[switch]$SkipRepoSync)
if (-not $SkipRepoSync) { exit 9 }
if ($env:FLEET_TOKEN -ne 'keep-token') { exit 10 }
Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'fleet-agent.exe') -Destination (Join-Path $DataDir 'fleet-agent.exe') -Force
`
			if fail {
				installer += "exit 7\n"
			} else {
				installer += "exit 0\n"
			}
			os.WriteFile(filepath.Join(stage, "install.ps1"), []byte(installer), 0600)
			os.WriteFile(filepath.Join(stage, "apply.ps1"), []byte(applyUpdateScript), 0600)
			// Replace task/process operations so the real apply script cannot stop
			// any installed services during a local regression test.
			wrapper := `function Start-ScheduledTask { param($TaskName,$ErrorAction) }
function Stop-ScheduledTask { param($TaskName,$ErrorAction) }
function Get-CimInstance { param($ClassName,$Filter) @() }
& (Join-Path $PSScriptRoot 'apply.ps1')
`
			wrapperPath := filepath.Join(stage, "test-apply.ps1")
			os.WriteFile(wrapperPath, []byte(wrapper), 0600)
			if out, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", wrapperPath).CombinedOutput(); err != nil {
				t.Fatalf("apply update: %v\n%s", err, out)
			}
			var receipt JobResult
			raw, err := os.ReadFile(filepath.Join(root, "update-result.json"))
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(bytesWithoutBOM(raw), &receipt); err != nil {
				t.Fatal(err)
			}
			installed, _ := os.ReadFile(filepath.Join(root, "fleet-agent.exe"))
			if fail {
				if receipt.Status != "error" || string(installed) != string(previous) {
					t.Fatalf("rollback failed: %+v; binary %s", receipt, installed)
				}
			} else if receipt.Status != "ok" || string(installed) != string(replacement) {
				t.Fatalf("apply failed: %+v; binary %s", receipt, installed)
			}
			retained, _ := os.ReadFile(filepath.Join(root, "config.json"))
			if string(retained) != string(pairing) {
				t.Fatal("pairing modified")
			}
		})
	}
}

func bytesWithoutBOM(raw []byte) []byte {
	if len(raw) >= 3 && raw[0] == 0xef && raw[1] == 0xbb && raw[2] == 0xbf {
		return raw[3:]
	}
	return raw
}

func TestResultDeliveryRetriesUntilServerAcknowledges(t *testing.T) {
	previous := dataDir
	dataDir = t.TempDir()
	defer func() { dataDir = previous }()
	failed := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var hb Heartbeat
		if err := json.NewDecoder(r.Body).Decode(&hb); err != nil {
			t.Error(err)
		}
		if len(hb.Results) != 1 || hb.Results[0].JobID != "result" {
			t.Errorf("result missing: %+v", hb.Results)
		}
		if failed {
			w.WriteHeader(500)
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	c := &Client{Server: server.URL}
	c.addResult(JobResult{JobID: "result", Status: "ok", Message: "update staged"})
	if c.pollOnceNoRestart() == nil || len(c.pending) != 1 {
		t.Fatal("failed result delivery lost the result")
	}
	failed = false
	if err := c.pollOnceNoRestart(); err != nil {
		t.Fatal(err)
	}
	if len(c.pending) != 0 {
		t.Fatal("successful acknowledgment did not clear the result")
	}
}
