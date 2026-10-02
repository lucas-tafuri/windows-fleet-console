//go:build windows

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

func bootInstallDir() string {
	base := os.Getenv("ProgramData")
	if base == "" {
		return ""
	}
	dir := filepath.Join(base, "FleetConsole")
	if _, err := os.Stat(filepath.Join(dir, "boot-installed")); err == nil {
		return dir
	}
	return ""
}

func registerLogon(exe string) error {
	// Boot installation owns startup; do not recreate per-user logon entries.
	if bootInstallDir() != "" {
		return nil
	}
	if _, err := os.Stat(filepath.Join(dataDir, "boot-installed")); err == nil {
		return nil
	}
	runner, runErr := writeRunnerCmd(exe)

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

	if taskErr != nil {
		if startupErr := writeStartupCmd(exe); startupErr != nil {
			return fmt.Errorf("%v; startup folder: %w", taskErr, startupErr)
		}
		return nil
	}
	// The Startup folder is only a fallback, not a second startup trigger.
	if appData := os.Getenv("APPDATA"); appData != "" {
		_ = os.Remove(filepath.Join(appData, `Microsoft\Windows\Start Menu\Programs\Startup\FleetConsole.cmd`))
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
	args := []string{"--wait-for-instance", "--data-dir", dataDir}
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

func spawnRestart(c *Client) error {
	if background {
		out, err := runCmd("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "Start-ScheduledTask -TaskName 'Fleet Console Update' -ErrorAction Stop")
		if err != nil {
			return fmt.Errorf("start update task: %w; %s", err, out)
		}
		return nil
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
	if err := os.WriteFile(helper, []byte("@echo off\r\n"+body), 0o644); err != nil {
		return err
	}
	cmd := exec.Command("cmd", "/c", helper)
	cmd.SysProcAttr = &windows.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
	}
	return cmd.Start()
}

func gitOK() bool {
	_, err := exec.LookPath("git")
	return err == nil
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
