//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestOnlyPackageChangesUseAdministratorToken(t *testing.T) {
	for _, kind := range []string{"check", "map_drive", "unmap_drive", "clean_downloads", "empty_recycle", "launch"} {
		if packageJobNeedsAdmin(kind) {
			t.Fatalf("%s must retain normal user context", kind)
		}
	}
	for _, kind := range []string{"install", "uninstall"} {
		if !packageJobNeedsAdmin(kind) {
			t.Fatalf("%s needs administrator context", kind)
		}
	}
}

func TestWorkerFilesAllowExecutionAndProtectRequest(t *testing.T) {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		t.Fatal(err)
	}
	defer token.Close()
	if token.IsElevated() {
		t.Skip("request write protection must be checked under a normal user token")
	}
	user, err := token.GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	sid := user.User.Sid.String()
	dir := t.TempDir()
	worker := filepath.Join(dir, "fleet-session-worker.exe")
	input := filepath.Join(dir, "job.json")
	output := filepath.Join(dir, "output")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := copyFile(self, worker); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(input, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(output, 0700); err != nil {
		t.Fatal(err)
	}
	// Restore owner access before Go removes its own exact temporary directory.
	defer func() {
		_ = filepath.Walk(dir, func(path string, _ os.FileInfo, err error) error {
			if err == nil {
				return setSessionJobACL(path, sid, true)
			}
			return err
		})
	}()
	for _, path := range []string{dir, worker, input} {
		if err := setSessionJobACL(path, sid, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := setSessionJobACL(output, sid, true); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(worker, "-test.run=^TestWorkerPermissionProbe$")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "FLEET_PERMISSION_PROBE="+dir)
	cmd.SysProcAttr = &windows.SysProcAttr{HideWindow: true}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("launch copied worker: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(output, "probe-ok")); err != nil {
		t.Fatal(err)
	}
}

func TestWorkerPermissionProbe(t *testing.T) {
	dir := os.Getenv("FLEET_PERMISSION_PROBE")
	if dir == "" {
		return
	}
	input := filepath.Join(dir, "job.json")
	if _, err := os.ReadFile(input); err != nil {
		t.Fatal(err)
	}
	if file, err := os.OpenFile(input, os.O_WRONLY, 0); err == nil {
		file.Close()
		t.Fatal("worker can write protected job request")
	}
	if err := os.WriteFile(filepath.Join(dir, "output", "probe-ok"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestWorkerACLSeparatesExecutableFromWritableResults(t *testing.T) {
	sid := "S-1-5-21-123-456-789-1001"
	for _, writable := range []bool{false, true} {
		sddl := sessionJobSDDL(sid, writable)
		if _, err := windows.SecurityDescriptorFromString(sddl); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(sddl, ";;;WD)") || strings.Contains(sddl, ";;;BU)") {
			t.Fatal("unrelated users must not access job files")
		}
		grant := "(A;OICI;GRGX;;;" + sid + ")"
		if writable {
			grant = "(A;OICI;FA;;;" + sid + ")"
		}
		if !strings.Contains(sddl, grant) {
			t.Fatalf("unexpected worker permissions: %s", sddl)
		}
	}
}

func TestUnassignedPrivilegeIsRejected(t *testing.T) {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		t.Fatal(err)
	}
	defer token.Close()
	if err := verifyTokenPrivilege(token, windows.LUID{LowPart: 0xffffffff, HighPart: -1}); err != windows.ERROR_NOT_ALL_ASSIGNED {
		t.Fatalf("unassigned privilege must not appear enabled: %v", err)
	}
}

func TestSessionSelection(t *testing.T) {
	rows := []windows.WTS_SESSION_INFO{
		{SessionID: 0, State: windows.WTSActive},
		{SessionID: 2, State: windows.WTSActive},
		{SessionID: 3, State: windows.WTSActive},
	}
	if id, err := selectInteractiveSession(rows, 2); err != nil || id != 2 {
		t.Fatalf("console user not selected: %d %v", id, err)
	}
	if _, err := selectInteractiveSession(rows, 99); err == nil {
		t.Fatal("ambiguous remote sessions must not guess a user")
	}
	if _, err := selectInteractiveSession(nil, 99); err == nil {
		t.Fatal("no user must not run as SYSTEM")
	}
	if id, err := selectInteractiveSession(rows[:2], 99); err != nil || id != 2 {
		t.Fatal("single remote user should be selected")
	}
}
