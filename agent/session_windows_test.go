//go:build windows

package main

import (
	"golang.org/x/sys/windows"
	"strings"
	"testing"
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
