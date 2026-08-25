//go:build windows

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cursor/windows-fleet-console/wintray"
	"golang.org/x/sys/windows"
)

const (
	taskName     = "Fleet Console Host"
	trayOpenID   = 1
	trayUpdateID = 2
	trayCloseID  = 3
	dashboardURL = "http://127.0.0.1:43123"
)

var (
	rootDir  string
	logFile  *os.File
	mu       sync.Mutex
	nodeCmd  *exec.Cmd
	updating bool
)

func main() {
	rootDir = findRoot()
	_ = os.MkdirAll(filepath.Join(rootDir, "data"), 0o755)
	setupLog()
	logf("Fleet Console host root=%s", rootDir)

	if err := registerLogon(); err != nil {
		logf("logon registration: %v", err)
	}

	if err := startNode(); err != nil {
		logf("start dashboard: %v", err)
	}

	err := wintray.Run(wintray.Config{
		Tooltip: "Fleet Console",
		Items: []wintray.Item{
			{ID: trayOpenID, Title: "Open"},
			{ID: trayUpdateID, Title: "Update"},
			{ID: trayCloseID, Title: "Close"},
		},
		OnCommand: func(id uint32) {
			switch id {
			case trayOpenID:
				openDashboard()
			case trayUpdateID:
				go runUpdate()
			case trayCloseID:
				stopNode()
				wintray.Quit()
			}
		},
	})
	stopNode()
	if err != nil {
		logf("tray: %v", err)
		os.Exit(1)
	}
}

func findRoot() string {
	if v := strings.TrimSpace(os.Getenv("FLEET_ROOT")); v != "" {
		return v
	}
	self, err := os.Executable()
	if err != nil {
		cwd, _ := os.Getwd()
		return cwd
	}
	dir := filepath.Dir(self)
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "server.mjs")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	cwd, _ := os.Getwd()
	return cwd
}

func setupLog() {
	f, err := os.OpenFile(filepath.Join(rootDir, "data", "console.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	logFile = f
	os.Stdout = f
	os.Stderr = f
}

func logf(format string, args ...any) {
	line := time.Now().Format("2006-01-02 15:04:05") + " " + fmt.Sprintf(format, args...) + "\n"
	if logFile != nil {
		_, _ = io.WriteString(logFile, line)
	}
}

func nodePath() string {
	if p, err := exec.LookPath("node"); err == nil {
		return p
	}
	candidates := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "nodejs", "node.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "nodejs", "node.exe"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return "node"
}

func startNode() error {
	mu.Lock()
	defer mu.Unlock()
	if nodeCmd != nil && nodeCmd.Process != nil {
		return nil
	}
	cmd := exec.Command(nodePath(), "server.mjs")
	cmd.Dir = rootDir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &windows.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP,
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	nodeCmd = cmd
	logf("dashboard pid %d", cmd.Process.Pid)
	go func() {
		_ = cmd.Wait()
		mu.Lock()
		if nodeCmd == cmd {
			nodeCmd = nil
		}
		mu.Unlock()
		logf("dashboard exited")
	}()
	return nil
}

func stopNode() {
	mu.Lock()
	cmd := nodeCmd
	nodeCmd = nil
	mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return
	}
	pid := strconv.Itoa(cmd.Process.Pid)
	kill := exec.Command("taskkill", "/T", "/F", "/PID", pid)
	kill.SysProcAttr = &windows.SysProcAttr{HideWindow: true}
	_ = kill.Run()
	logf("stopped dashboard pid %s", pid)
}

func restartNode() {
	stopNode()
	time.Sleep(400 * time.Millisecond)
	if err := startNode(); err != nil {
		logf("restart dashboard: %v", err)
	}
}

func runUpdate() {
	mu.Lock()
	if updating {
		mu.Unlock()
		return
	}
	updating = true
	mu.Unlock()
	defer func() {
		mu.Lock()
		updating = false
		mu.Unlock()
	}()

	logf("update: git pull")
	out, err := runInRoot("git", "pull", "--ff-only")
	logf("%s", strings.TrimSpace(out))
	if err != nil {
		logf("git pull: %v", err)
	}
	logf("update: npm install")
	out, err = runInRoot("npm", "install")
	logf("%s", strings.TrimSpace(out))
	if err != nil {
		logf("npm install: %v", err)
		return
	}
	restartNode()
	logf("update complete")
}

func runInRoot(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = rootDir
	cmd.SysProcAttr = &windows.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func openDashboard() {
	cmd := exec.Command("cmd", "/c", "start", "", dashboardURL)
	cmd.SysProcAttr = &windows.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		logf("open dashboard: %v", err)
	}
}

func registerLogon() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	self, _ = filepath.Abs(self)
	startupErr := writeStartupCmd(self)
	runner := filepath.Join(rootDir, "data", "run-console.cmd")
	body := fmt.Sprintf("@echo off\r\nstart \"\" \"%s\"\r\n", self)
	runErr := os.WriteFile(runner, []byte(body), 0o644)

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
	if taskErr != nil {
		logf("logon task skipped: %v (startup folder registered)", taskErr)
	}
	return nil
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
	script := filepath.Join(dir, "FleetConsoleHost.cmd")
	body := fmt.Sprintf("@echo off\r\nstart \"\" \"%s\"\r\n", exe)
	return os.WriteFile(script, []byte(body), 0o644)
}
