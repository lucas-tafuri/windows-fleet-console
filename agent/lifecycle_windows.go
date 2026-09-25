//go:build windows

package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

func ensureInstalled() (relocated bool, err error) {
	self, err := os.Executable()
	if err != nil {
		return false, err
	}
	self, _ = filepath.Abs(self)
	dest := filepath.Join(dataDir, "fleet-agent.exe")
	exePath = dest

	if !sameFile(self, dest) {
		if copyErr := copyFile(self, dest); copyErr != nil {
			exePath = self
			regErr := registerLogon(self)
			return false, fmt.Errorf("copy to %s: %w (running from current path); startup: %v", dest, copyErr, regErr)
		}
		regErr := registerLogon(dest)
		if installOnly {
			return true, regErr
		}
		if startErr := startDetached(dest); startErr != nil {
			exePath = dest
			return false, startErr
		}
		return true, regErr
	}

	return false, registerLogon(dest)
}

func registerLogon(exe string) error {
	// Boot installation owns startup; do not recreate per-user logon entries.
	if _, err := os.Stat(filepath.Join(dataDir, "boot-installed")); err == nil {
		return nil
	}
	runner, runErr := writeRunnerCmd(exe)
	startupErr := writeStartupCmd(exe)

	var taskErr error
	if runErr == nil {
		cmd := exec.Command("schtasks", "/create", "/tn", taskName, "/tr", `"`+runner+`"`, "/sc", "onlogon", "/f")
		cmd.SysProcAttr = &windows.SysProcAttr{HideWindow: true}
		out, err := cmd.CombinedOutput()
		if err != nil {
			taskErr = fmt.Errorf("schtasks: %v (%s)", err, strings.TrimSpace(string(out)))
		}
	} else {
		taskErr = runErr
	}

	if taskErr != nil && startupErr != nil {
		return fmt.Errorf("%v; startup folder: %w", taskErr, startupErr)
	}
	return nil
}

func writeRunnerCmd(exe string) (string, error) {
	path := filepath.Join(dataDir, "run-agent.cmd")
	return path, os.WriteFile(path, []byte(launcherCmd(exe)), 0o644)
}

func writeStartupCmd(exe string) error {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return fmt.Errorf("APPDATA unset")
	}
	dir := filepath.Join(appData, `Microsoft\Windows\Start Menu\Programs\Startup`)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	script := filepath.Join(dir, "FleetConsole.cmd")
	return os.WriteFile(script, []byte(launcherCmd(exe)), 0o644)
}

func launcherCmd(exe string) string {
	return fmt.Sprintf("@echo off\r\nstart \"\" \"%s\" --data-dir \"%s\"\r\n", exe, dataDir)
}

func httpOnlyFlag() string {
	if agentCfg.HTTPOnly {
		return " --http-only"
	}
	return ""
}

func startDetached(exe string) error {
	args := []string{"--server", agentCfg.Server, "--token", agentCfg.Token, "--data-dir", dataDir}
	if background {
		args = append(args, "--background")
	}
	if agentCfg.HTTPOnly {
		args = append(args, "--http-only")
	}
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &windows.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
	}
	return cmd.Start()
}

func selfUpdate(jobID, repo, branch string) JobResult {
	if repo == "" {
		repo = agentCfg.Repo
	}
	if repo == "" {
		repo = defaultRepo
	}
	if branch == "" {
		branch = agentCfg.Branch
	}
	if branch == "" {
		branch = defaultBranch
	}
	agentCfg.Repo = repo
	agentCfg.Branch = branch
	saveConfig(agentCfg)

	var logs []string
	via := ""
	srcExe := ""

	root := findGitRoot(filepath.Dir(mustExe()))
	owned := filepath.Join(dataDir, "repo")
	if root == "" {
		root = owned
	}

	if gitOK() {
		if !isGit(root) {
			_ = os.RemoveAll(root)
			out, err := runCmd("git", "clone", "--branch", branch, "--single-branch", repo, root)
			logs = append(logs, out)
			if err != nil {
				logs = append(logs, err.Error())
			} else {
				via = "git clone"
			}
		}
		if isGit(root) {
			ours := sameFile(root, owned)
			fetch, _ := runCmd("git", "-C", root, "fetch", "origin", branch)
			logs = append(logs, fetch)
			var pull string
			var err error
			if ours {
				pull, err = runCmd("git", "-C", root, "reset", "--hard", "origin/"+branch)
			} else {
				pull, err = runCmd("git", "-C", root, "pull", "--ff-only", "origin", branch)
			}
			logs = append(logs, pull)
			if err != nil {
				logs = append(logs, err.Error())
			} else {
				if via == "" {
					via = "git pull"
				}
				cand := filepath.Join(root, "dist", "fleet-agent.exe")
				if _, statErr := os.Stat(cand); statErr == nil {
					srcExe = cand
				}
			}
		}
	}

	dest := filepath.Join(dataDir, "fleet-agent.exe")
	if exePath != "" {
		dest = exePath
	}
	newPath := dest + ".new"

	if srcExe == "" {
		if err := downloadFile(defaultExeURL, newPath); err != nil {
			return JobResult{
				JobID:   jobID,
				Status:  "error",
				Via:     firstNonEmpty(via, "none"),
				Message: "git pull/clone failed and GitHub download failed",
				Output:  clip(strings.Join(logs, "\n") + "\n" + err.Error()),
			}
		}
		if via == "" {
			via = "github download"
		} else {
			via += " + github download"
		}
	} else if !sameFile(srcExe, dest) {
		if err := copyFile(srcExe, newPath); err != nil {
			return JobResult{
				JobID:   jobID,
				Status:  "error",
				Via:     via,
				Message: "Could not stage the new agent binary",
				Output:  clip(strings.Join(logs, "\n") + "\n" + err.Error()),
			}
		}
	} else {
		_ = copyFile(srcExe, newPath)
	}

	_ = registerLogon(dest)
	restartRequested = true
	if via == "" {
		via = "git pull"
	}
	return JobResult{
		JobID:   jobID,
		Status:  "ok",
		Via:     via,
		Message: "Updated; agent is restarting",
		Output:  clip(strings.Join(logs, "\n")),
	}
}

func spawnRestart(c *Client) {
	// The boot task owns this process and applies staged updates after it exits.
	if background {
		return
	}
	dest := filepath.Join(dataDir, "fleet-agent.exe")
	if exePath != "" {
		dest = exePath
	}
	helper := filepath.Join(dataDir, "restart-agent.cmd")
	mode := ""
	if background {
		mode = " --background"
	}
	body := fmt.Sprintf("timeout /t 2 /nobreak >nul\r\nif exist \"%s.new\" move /y \"%s.new\" \"%s\" >nul\r\nstart \"\" \"%s\" --data-dir \"%s\"%s\r\n",
		dest, dest, dest, dest, dataDir, mode)
	_ = os.WriteFile(helper, []byte("@echo off\r\n"+body), 0o644)
	cmd := exec.Command("cmd", "/c", helper)
	cmd.SysProcAttr = &windows.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
	}
	_ = cmd.Start()
}

func gitOK() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

func isGit(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && st.IsDir()
}

func findGitRoot(start string) string {
	dir, _ := filepath.Abs(start)
	for i := 0; i < 8; i++ {
		if isGit(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func mustExe() string {
	self, err := os.Executable()
	if err != nil {
		return exePath
	}
	return self
}

func sameFile(a, b string) bool {
	absA, _ := filepath.Abs(a)
	absB, _ := filepath.Abs(b)
	return strings.EqualFold(filepath.Clean(absA), filepath.Clean(absB))
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	_ = os.Remove(dst)
	return os.Rename(tmp, dst)
}

func downloadFile(url, dest string) error {
	client := &http.Client{Timeout: 120 * time.Second}
	res, err := client.Get(url)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("download %s: HTTP %d", url, res.StatusCode)
	}
	tmp := dest + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, res.Body); err != nil {
		out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	_ = os.Remove(dest)
	return os.Rename(tmp, dest)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" && v != " + github download" {
			return v
		}
	}
	return ""
}
