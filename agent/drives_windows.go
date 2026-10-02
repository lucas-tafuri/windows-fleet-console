//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type savedDrive struct {
	Letter   string `json:"letter"`
	UNC      string `json:"unc"`
	User     string `json:"user,omitempty"`
	Password string `json:"password,omitempty"`
}

func driveLetter(letter string) (string, error) {
	letter = strings.TrimSuffix(strings.ToUpper(strings.TrimSpace(letter)), ":")
	if len(letter) != 1 || letter[0] < 'A' || letter[0] > 'Z' {
		return "", fmt.Errorf("a drive letter from A to Z is required")
	}
	return letter, nil
}

func driveStatePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "FleetConsole", "managed-drives.dat"), nil
}

// DPAPI binds the entire file, including share credentials, to this user.
func cryptDrives(raw []byte, encrypt bool) ([]byte, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty drive settings")
	}
	in := windows.DataBlob{Size: uint32(len(raw)), Data: &raw[0]}
	var out windows.DataBlob
	var err error
	if encrypt {
		err = windows.CryptProtectData(&in, nil, nil, 0, nil, 1, &out)
	} else {
		err = windows.CryptUnprotectData(&in, nil, nil, 0, nil, 1, &out)
	}
	if err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}

func loadDrives(path string) ([]savedDrive, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	raw, err = cryptDrives(raw, false)
	if err != nil {
		return nil, err
	}
	var drives []savedDrive
	err = json.Unmarshal(raw, &drives)
	return drives, err
}

func saveDrives(path string, drives []savedDrive) error {
	raw, err := json.Marshal(drives)
	if err != nil {
		return err
	}
	raw, err = cryptDrives(raw, true)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "drives-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(raw); err != nil {
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

func driveError(id string, err error) JobResult {
	return JobResult{JobID: id, Status: "error", Message: err.Error()}
}

func mapDrive(id, letter, unc, user, pass string) JobResult {
	letter, err := driveLetter(letter)
	if err != nil {
		return driveError(id, err)
	}
	unc = strings.TrimSpace(unc)
	if !strings.HasPrefix(unc, `\\`) || len(strings.Split(strings.Trim(unc, `\`), `\`)) < 2 {
		return driveError(id, fmt.Errorf("a UNC share path is required"))
	}
	path, err := driveStatePath()
	if err != nil {
		return driveError(id, err)
	}
	drives, err := loadDrives(path)
	if err != nil {
		return driveError(id, fmt.Errorf("read saved drive settings: %w", err))
	}
	d := savedDrive{letter, unc, user, pass}
	// Refuse a letter owned by another share or a local device before remembering it.
	if err = checkDriveTarget(d); err != nil {
		return driveError(id, err)
	}
	found := false
	for i := range drives {
		if drives[i].Letter == letter {
			drives[i] = d
			found = true
		}
	}
	if !found {
		drives = append(drives, d)
	}
	if err = saveDrives(path, drives); err != nil {
		return driveError(id, fmt.Errorf("save drive settings: %w", err))
	}
	if err = ensureDrive(d); err != nil {
		return driveError(id, fmt.Errorf("%s: saved for automatic retry; %w", letter, err))
	}
	return JobResult{JobID: id, Status: "ok", Message: "Mapped " + letter + ": to " + unc + "; automatic reconnection enabled"}
}

func unmapDrive(id, letter string) JobResult {
	letter, err := driveLetter(letter)
	if err != nil {
		return driveError(id, err)
	}
	path, err := driveStatePath()
	if err != nil {
		return driveError(id, err)
	}
	drives, err := loadDrives(path)
	if err != nil {
		return driveError(id, err)
	}
	kept := make([]savedDrive, 0, len(drives))
	for _, d := range drives {
		if d.Letter != letter {
			kept = append(kept, d)
		}
	}
	// Persist the opt-out first, even if the disconnected mapping cannot be removed.
	if err = saveDrives(path, kept); err != nil {
		return driveError(id, err)
	}
	if err = wnetCancel(letter + ":"); err != nil && err != syscall.Errno(2250) {
		return driveError(id, fmt.Errorf("automatic reconnection stopped; unmap failed: %w", err))
	}
	return JobResult{JobID: id, Status: "ok", Message: "Unmapped " + letter + ":; automatic reconnection stopped"}
}

func maintainDrives(id string) JobResult {
	path, err := driveStatePath()
	if err != nil {
		return driveError(id, err)
	}
	drives, err := loadDrives(path)
	if err != nil {
		return driveError(id, fmt.Errorf("read saved drive settings: %w", err))
	}
	var failures []string
	for _, d := range drives {
		if err := ensureDrive(d); err != nil {
			failures = append(failures, d.Letter+": "+err.Error())
		}
	}
	if len(failures) > 0 {
		return driveError(id, fmt.Errorf("%s", strings.Join(failures, "; ")))
	}
	return JobResult{JobID: id, Status: "ok", Message: "Managed drives checked"}
}

func mappedTarget(letter string) (string, error) {
	local, _ := windows.UTF16PtrFromString(letter + ":")
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	r, _, _ := modMpr.NewProc("WNetGetConnectionW").Call(uintptr(unsafe.Pointer(local)), uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&size)))
	// ERROR_CONNECTION_UNAVAIL still supplies the remembered remote path.
	if r == 0 || r == 1201 {
		return windows.UTF16ToString(buffer), nil
	}
	if r == 2250 {
		return "", nil
	}
	return "", syscall.Errno(r)
}

func checkDriveTarget(d savedDrive) error {
	if _, err := driveLetter(d.Letter); err != nil {
		return err
	}
	target, err := mappedTarget(d.Letter)
	if err != nil {
		return err
	}
	if target != "" {
		if normalizeUNC(target) != normalizeUNC(d.UNC) {
			return fmt.Errorf("drive letter is already used by another share; left unchanged")
		}
		return nil
	}
	bits, _, _ := procGetLogicalDrives.Call()
	if bits&(1<<uint(d.Letter[0]-'A')) != 0 {
		return fmt.Errorf("drive letter is used by another device; left unchanged")
	}
	return nil
}

func ensureDrive(d savedDrive) error {
	return reconcileDrive(d, checkDriveTarget, driveAccessible, reconnectDrive)
}

func reconnectDrive(local, remote, user, pass string) error {
	target, err := mappedTarget(strings.TrimSuffix(local, ":"))
	if err != nil {
		return err
	}
	if target != "" {
		if normalizeUNC(target) != normalizeUNC(remote) {
			return fmt.Errorf("drive target changed; left unchanged")
		}
		name, _ := windows.UTF16PtrFromString(local)
		// Remove a stale connection only when Windows can do so without closing
		// open files. Never force-disconnect applications during a health check.
		r, _, _ := procWNetCancelConnection2W.Call(uintptr(unsafe.Pointer(name)), 0, 0)
		if r != 0 && r != 2250 {
			return syscall.Errno(r)
		}
	}
	return wnetAdd(local, remote, user, pass)
}

func reconcileDrive(d savedDrive, check func(savedDrive) error, accessible func(string) bool, connect func(string, string, string, string) error) error {
	if err := check(d); err != nil {
		return err
	}
	if accessible(d.Letter) {
		return nil
	}
	// Repair only the intended connection, without forcibly closing open files.
	if err := connect(d.Letter+":", d.UNC, d.User, d.Password); err != nil {
		return fmt.Errorf("reconnect failed: %w", err)
	}
	if !accessible(d.Letter) {
		return fmt.Errorf("share is still unavailable; will retry")
	}
	return nil
}

func driveAccessible(letter string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// A separate process bounds Windows network filesystem timeouts.
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "if (Test-Path -LiteralPath '"+letter+":\\' -PathType Container) { exit 0 }; exit 1")
	cmd.SysProcAttr = &windows.SysProcAttr{HideWindow: true}
	return cmd.Run() == nil
}
