//go:build windows

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDriveSettingsSurviveReloadEncrypted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "managed-drives.dat")
	want := []savedDrive{{"Z", `\\server\share with spaces`, "domain\\user", "secret-password"}}
	if err := saveDrives(path, want); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(want[0].Password)) {
		t.Fatal("plaintext password saved")
	}
	got, err := loadDrives(path)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("reload = %v, %v", got, err)
	}
	if err := saveDrives(path, nil); err != nil {
		t.Fatal(err)
	}
	got, err = loadDrives(path)
	if err != nil || len(got) != 0 {
		t.Fatalf("removed mapping returned: %v, %v", got, err)
	}
	if err := os.WriteFile(path, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadDrives(path); err == nil {
		t.Fatal("corruption was silently accepted")
	}
}

func TestReconnectDrive(t *testing.T) {
	for _, tc := range []struct {
		name                                       string
		conflict, healthy, connectFails, recovered bool
		wantCalls                                  int
		wantErr                                    bool
	}{
		{"healthy", false, true, false, true, 0, false},
		{"missing after reboot", false, false, false, true, 1, false},
		{"network offline", false, false, true, false, 1, true},
		{"still inaccessible", false, false, false, false, 1, true},
		{"conflicting letter", true, false, false, false, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := savedDrive{"Z", `\\server\share`, "user", "password"}
			calls := 0
			err := reconcileDrive(d, func(savedDrive) error {
				if tc.conflict {
					return errors.New("conflict")
				}
				return nil
			}, func(string) bool {
				if calls == 0 {
					return tc.healthy
				}
				return tc.recovered
			}, func(local, remote, user, pass string) error {
				calls++
				if local != "Z:" || remote != d.UNC || user != d.User || pass != d.Password {
					t.Fatal("lost reconnect settings")
				}
				if tc.connectFails {
					return errors.New("offline")
				}
				return nil
			})
			if calls != tc.wantCalls || (err != nil) != tc.wantErr {
				t.Fatalf("calls=%d, err=%v", calls, err)
			}
		})
	}
}

func TestDriveLetterValidation(t *testing.T) {
	for _, bad := range []string{"", "ZZ", "C:\\", "';exit", "1"} {
		if _, err := driveLetter(bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
	if got, err := driveLetter(" z: "); got != "Z" || err != nil {
		t.Fatalf("got %q, %v", got, err)
	}
}
