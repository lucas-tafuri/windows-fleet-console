//go:build windows

package main

import (
	"golang.org/x/sys/windows"
	"testing"
)

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
