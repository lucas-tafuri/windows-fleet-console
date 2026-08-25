//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

type snapshot struct {
	Hostname       string
	User           string
	OS             string
	CPU            *float64
	Memory         *float64
	Frozen         bool
	MetricsLimited bool
	LastInputAgeMs *int64
	MappedDrives   []MappedDrive
}

var (
	modKernel32 = windows.NewLazySystemDLL("kernel32.dll")
	modUser32   = windows.NewLazySystemDLL("user32.dll")
	modShell32  = windows.NewLazySystemDLL("shell32.dll")
	modMpr      = windows.NewLazySystemDLL("mpr.dll")

	procGetSystemTimes         = modKernel32.NewProc("GetSystemTimes")
	procGlobalMemoryStatusEx   = modKernel32.NewProc("GlobalMemoryStatusEx")
	procGetTickCount           = modKernel32.NewProc("GetTickCount")
	procGetLogicalDrives       = modKernel32.NewProc("GetLogicalDrives")
	procGetDriveTypeW          = modKernel32.NewProc("GetDriveTypeW")
	procGetLastInputInfo       = modUser32.NewProc("GetLastInputInfo")
	procGetForegroundWindow    = modUser32.NewProc("GetForegroundWindow")
	procIsHungAppWindow        = modUser32.NewProc("IsHungAppWindow")
	procSHEmptyRecycleBinW     = modShell32.NewProc("SHEmptyRecycleBinW")
	procShellExecuteW          = modShell32.NewProc("ShellExecuteW")
	procWNetAddConnection2W    = modMpr.NewProc("WNetAddConnection2W")
	procWNetCancelConnection2W = modMpr.NewProc("WNetCancelConnection2W")
)

var (
	prevIdle, prevKernel, prevUser uint64
	prevCPUValid                   bool
	prevCPU100                     bool
	prevInputAge                   int64
	hungStreak                     int
)

type filetime struct {
	low  uint32
	high uint32
}

func ftToUint(ft filetime) uint64 {
	return (uint64(ft.high) << 32) | uint64(ft.low)
}

func cpuPercent() (float64, error) {
	var idle, kernel, user filetime
	r, _, err := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idle)),
		uintptr(unsafe.Pointer(&kernel)),
		uintptr(unsafe.Pointer(&user)),
	)
	if r == 0 {
		return 0, err
	}
	i, k, u := ftToUint(idle), ftToUint(kernel), ftToUint(user)
	if !prevCPUValid {
		prevIdle, prevKernel, prevUser, prevCPUValid = i, k, u, true
		return 0, errors.New("warming cpu sample")
	}
	idleDelta := i - prevIdle
	total := (k - prevKernel) + (u - prevUser)
	prevIdle, prevKernel, prevUser = i, k, u
	if total == 0 {
		return 0, nil
	}
	busy := 1 - float64(idleDelta)/float64(total)
	if busy < 0 {
		busy = 0
	}
	if busy > 1 {
		busy = 1
	}
	return busy * 100, nil
}

type memStatusEx struct {
	length     uint32
	memoryLoad uint32
	totalPhys  uint64
	availPhys  uint64
	_          [5]uint64
}

func memPercent() (float64, error) {
	var st memStatusEx
	st.length = uint32(unsafe.Sizeof(st))
	r, _, err := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&st)))
	if r == 0 {
		return 0, err
	}
	return float64(st.memoryLoad), nil
}

type lastInputInfo struct {
	cbSize uint32
	time   uint32
}

func lastInputAge() (int64, error) {
	var info lastInputInfo
	info.cbSize = uint32(unsafe.Sizeof(info))
	r, _, err := procGetLastInputInfo.Call(uintptr(unsafe.Pointer(&info)))
	if r == 0 {
		return 0, err
	}
	tick, _, _ := procGetTickCount.Call()
	age := int64(uint32(tick) - info.time)
	if age < 0 {
		age = 0
	}
	return age, nil
}

const (
	driveRemote = 4
)

func hungForeground() (bool, error) {
	if err := procGetForegroundWindow.Find(); err != nil {
		return false, err
	}
	if err := procIsHungAppWindow.Find(); err != nil {
		return false, err
	}
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return false, nil
	}
	r, _, _ := procIsHungAppWindow.Call(hwnd)
	return r != 0, nil
}

func collectSnapshot() snapshot {
	host, _ := os.Hostname()
	user := os.Getenv("USERNAME")
	osName := productName()
	s := snapshot{Hostname: host, User: user, OS: osName, MappedDrives: listDrives()}

	cpu, cpuErr := cpuPercent()
	mem, memErr := memPercent()
	if cpuErr == nil {
		s.CPU = &cpu
	}
	if memErr == nil {
		s.Memory = &mem
	}
	if cpuErr != nil && memErr != nil {
		s.MetricsLimited = true
	}

	age, ageErr := lastInputAge()
	if ageErr == nil {
		s.LastInputAgeMs = &age
	}

	hung, hungErr := hungForeground()
	if hungErr == nil {
		if hung {
			hungStreak++
		} else {
			hungStreak = 0
		}
		s.Frozen = hungStreak >= 2
	} else {
		hungStreak = 0
		cpu100 := s.CPU != nil && *s.CPU >= 99.5
		inputStuck := ageErr == nil && prevInputAge != 0 && age == prevInputAge
		s.Frozen = prevCPU100 && cpu100 && inputStuck
		prevCPU100 = cpu100
		if ageErr == nil {
			prevInputAge = age
		}
	}
	return s
}

func productName() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return "Windows"
	}
	defer k.Close()
	name, _, err := k.GetStringValue("ProductName")
	if err != nil || name == "" {
		return "Windows"
	}
	return name
}

func runCmd(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &windows.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func wingetOK() bool {
	_, err := exec.LookPath("winget")
	return err == nil
}

func checkPackage(jobID, pkg string) JobResult {
	if pkg == "" {
		return JobResult{JobID: jobID, Status: "error", Message: "package id or name is required"}
	}
	if wingetOK() {
		out, err := runCmd("winget", "list", "--id", pkg, "--exact", "--disable-interactivity")
		if err == nil && wingetFound(out, pkg) {
			return JobResult{JobID: jobID, Status: "ok", Via: "winget", Message: pkg + " is installed", Output: clip(out)}
		}
		out2, err2 := runCmd("winget", "list", "--name", pkg, "--disable-interactivity")
		if err2 == nil && wingetFound(out2, pkg) {
			return JobResult{JobID: jobID, Status: "ok", Via: "winget", Message: pkg + " is installed", Output: clip(out2)}
		}
		if found, via, detail := registryHas(pkg); found {
			return JobResult{JobID: jobID, Status: "ok", Via: via, Message: pkg + " is installed", Output: detail}
		}
		return JobResult{JobID: jobID, Status: "ok", Via: "winget", Message: pkg + " is not installed", Output: clip(out + "\n" + out2)}
	}
	if found, via, detail := registryHas(pkg); found {
		return JobResult{JobID: jobID, Status: "ok", Via: via, Message: pkg + " is installed", Output: detail}
	}
	if found, detail := getPackage(pkg); found {
		return JobResult{JobID: jobID, Status: "ok", Via: "Get-Package", Message: pkg + " is installed", Output: detail}
	}
	return JobResult{JobID: jobID, Status: "ok", Via: "registry", Message: pkg + " is not installed", Output: "No uninstall registry entry or Get-Package match."}
}

func installPackage(jobID, pkg string) JobResult {
	if pkg == "" {
		return JobResult{JobID: jobID, Status: "error", Message: "package id or name is required"}
	}
	if !wingetOK() {
		found, via, _ := registryHas(pkg)
		msg := "winget is not installed on this PC; cannot install"
		if found {
			msg += "; already detected via " + via
		}
		return JobResult{JobID: jobID, Status: "error", Via: "none", Message: msg}
	}
	out, err := runCmd("winget", "install", "--id", pkg, "--exact", "--silent", "--disable-interactivity", "--accept-package-agreements", "--accept-source-agreements")
	if err == nil {
		return JobResult{JobID: jobID, Status: "ok", Via: "winget", Message: "Installed " + pkg, Output: clip(out)}
	}
	out2, err2 := runCmd("winget", "install", "--name", pkg, "--silent", "--disable-interactivity", "--accept-package-agreements", "--accept-source-agreements")
	if err2 == nil {
		return JobResult{JobID: jobID, Status: "ok", Via: "winget", Message: "Installed " + pkg, Output: clip(out2)}
	}
	return JobResult{JobID: jobID, Status: "error", Via: "winget", Message: "Install failed for " + pkg, Output: clip(out + "\n" + out2)}
}

func uninstallPackage(jobID, pkg string) JobResult {
	if pkg == "" {
		return JobResult{JobID: jobID, Status: "error", Message: "package id or name is required"}
	}
	if !wingetOK() {
		return JobResult{JobID: jobID, Status: "error", Via: "none", Message: "winget is not installed on this PC; cannot uninstall"}
	}
	out, err := runCmd("winget", "uninstall", "--id", pkg, "--exact", "--silent", "--disable-interactivity")
	if err == nil {
		return JobResult{JobID: jobID, Status: "ok", Via: "winget", Message: "Uninstalled " + pkg, Output: clip(out)}
	}
	out2, err2 := runCmd("winget", "uninstall", "--name", pkg, "--silent", "--disable-interactivity")
	if err2 == nil {
		return JobResult{JobID: jobID, Status: "ok", Via: "winget", Message: "Uninstalled " + pkg, Output: clip(out2)}
	}
	return JobResult{JobID: jobID, Status: "error", Via: "winget", Message: "Uninstall failed for " + pkg, Output: clip(out + "\n" + out2)}
}

func wingetFound(out, pkg string) bool {
	low := strings.ToLower(out)
	if strings.Contains(low, "no installed package") || strings.Contains(low, "no package found") {
		return false
	}
	return strings.Contains(low, strings.ToLower(pkg))
}

func registryHas(pkg string) (bool, string, string) {
	needle := strings.ToLower(pkg)
	roots := []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`},
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`},
		{registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`},
	}
	for _, r := range roots {
		k, err := registry.OpenKey(r.root, r.path, registry.ENUMERATE_SUB_KEYS)
		if err != nil {
			continue
		}
		names, _ := k.ReadSubKeyNames(0)
		for _, name := range names {
			sk, err := registry.OpenKey(k, name, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			dn, _, _ := sk.GetStringValue("DisplayName")
			sk.Close()
			if dn == "" {
				continue
			}
			if strings.Contains(strings.ToLower(dn), needle) || strings.EqualFold(name, pkg) {
				k.Close()
				return true, "registry", dn
			}
		}
		k.Close()
	}
	return false, "registry", ""
}

func getPackage(pkg string) (bool, string) {
	out, err := runCmd("powershell", "-NoProfile", "-Command",
		"Get-Package -Name '"+strings.ReplaceAll(pkg, "'", "''")+"' -ErrorAction SilentlyContinue | Format-List")
	if err != nil || strings.TrimSpace(out) == "" {
		return false, ""
	}
	return true, clip(out)
}

func mapDrive(jobID, letter, unc, user, pass string) JobResult {
	if letter == "" || unc == "" {
		return JobResult{JobID: jobID, Status: "error", Message: "drive letter and UNC path are required"}
	}
	letter = strings.TrimSuffix(letter, ":")
	spec := letter + ":"
	args := []string{"use", spec, unc, "/persistent:no"}
	if user != "" {
		args = append(args, "/user:"+user)
		if pass != "" {
			args = append(args, pass)
		}
	}
	out, err := runCmd("net", args...)
	if err == nil {
		return JobResult{JobID: jobID, Status: "ok", Via: "net use", Message: "Mapped " + spec + " to " + unc, Output: clip(out)}
	}
	if err2 := wnetAdd(spec, unc, user, pass); err2 == nil {
		return JobResult{JobID: jobID, Status: "ok", Via: "WNetAddConnection2", Message: "Mapped " + spec + " to " + unc, Output: clip(out)}
	} else {
		return JobResult{JobID: jobID, Status: "error", Via: "net use", Message: "Map failed for " + spec, Output: clip(out + "\n" + err2.Error())}
	}
}

func unmapDrive(jobID, letter string) JobResult {
	if letter == "" {
		return JobResult{JobID: jobID, Status: "error", Message: "drive letter is required"}
	}
	letter = strings.TrimSuffix(letter, ":")
	spec := letter + ":"
	out, err := runCmd("net", "use", spec, "/delete", "/y")
	if err == nil {
		return JobResult{JobID: jobID, Status: "ok", Via: "net use", Message: "Unmapped " + spec, Output: clip(out)}
	}
	if err2 := wnetCancel(spec); err2 == nil {
		return JobResult{JobID: jobID, Status: "ok", Via: "WNetCancelConnection2", Message: "Unmapped " + spec, Output: clip(out)}
	} else {
		return JobResult{JobID: jobID, Status: "error", Via: "net use", Message: "Unmap failed for " + spec, Output: clip(out + "\n" + err2.Error())}
	}
}

type netResource struct {
	scope       uint32
	typ         uint32
	displayType uint32
	usage       uint32
	localName   *uint16
	remoteName  *uint16
	comment     *uint16
	provider    *uint16
}

func wnetAdd(local, remote, user, pass string) error {
	ln, _ := windows.UTF16PtrFromString(local)
	rn, _ := windows.UTF16PtrFromString(remote)
	nr := netResource{typ: 1, localName: ln, remoteName: rn}
	var u, p *uint16
	if user != "" {
		u, _ = windows.UTF16PtrFromString(user)
	}
	if pass != "" {
		p, _ = windows.UTF16PtrFromString(pass)
	}
	r, _, err := procWNetAddConnection2W.Call(uintptr(unsafe.Pointer(&nr)), uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(u)), 0)
	if r != 0 {
		return err
	}
	return nil
}

func wnetCancel(local string) error {
	ln, _ := windows.UTF16PtrFromString(local)
	r, _, err := procWNetCancelConnection2W.Call(uintptr(unsafe.Pointer(ln)), 0, 1)
	if r != 0 {
		return err
	}
	return nil
}

func listDrives() []MappedDrive {
	out, err := runCmd("net", "use")
	if err == nil {
		drives := parseNetUse(out)
		if len(drives) > 0 {
			return drives
		}
	}
	return logicalRemote()
}

func parseNetUse(out string) []MappedDrive {
	var drives []MappedDrive
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		var letter, unc string
		for _, f := range fields {
			if len(f) == 2 && f[1] == ':' {
				letter = string(f[0])
			}
			if strings.HasPrefix(f, `\\`) {
				unc = f
			}
		}
		if letter != "" && unc != "" {
			drives = append(drives, MappedDrive{Letter: letter, Path: unc})
		}
	}
	return drives
}

func logicalRemote() []MappedDrive {
	bits, _, _ := procGetLogicalDrives.Call()
	var drives []MappedDrive
	for i := 0; i < 26; i++ {
		if bits&(1<<uint(i)) == 0 {
			continue
		}
		root := string(rune('A'+i)) + `:\`
		p, _ := windows.UTF16PtrFromString(root)
		t, _, _ := procGetDriveTypeW.Call(uintptr(unsafe.Pointer(p)))
		if t == driveRemote {
			drives = append(drives, MappedDrive{Letter: string(rune('A' + i)), Path: root})
		}
	}
	return drives
}

func cleanDownloads(jobID string) JobResult {
	home, err := os.UserHomeDir()
	if err != nil {
		return JobResult{JobID: jobID, Status: "error", Message: "cannot resolve home directory"}
	}
	dir := filepath.Join(home, "Downloads")
	deleted, skipped, failed := 0, 0, 0
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			failed++
			return nil
		}
		if path == dir {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if err := os.Remove(path); err != nil {
			if isLocked(err) {
				skipped++
			} else {
				failed++
			}
			return nil
		}
		deleted++
		return nil
	})
	return JobResult{
		JobID:   jobID,
		Status:  "ok",
		Via:     "filesystem",
		Message: fmt.Sprintf("Deleted %d files, skipped %d locked, %d failed", deleted, skipped, failed),
		Output:  dir,
	}
}

func isLocked(err error) bool {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno == 32 || errno == 5
	}
	return strings.Contains(strings.ToLower(err.Error()), "used by another process")
}

const (
	sherbNoconfirm = 0x1
	sherbNoprog    = 0x2
	sherbNosound   = 0x4
)

func emptyRecycle(jobID string) JobResult {
	r, _, err := procSHEmptyRecycleBinW.Call(0, 0, sherbNoconfirm|sherbNoprog|sherbNosound)
	if r == 0 {
		return JobResult{JobID: jobID, Status: "ok", Via: "SHEmptyRecycleBin", Message: "Recycle Bin emptied", Output: "S_OK"}
	}
	out, err2 := runCmd("powershell", "-NoProfile", "-Command",
		"$shell = New-Object -ComObject Shell.Application; $bin = $shell.NameSpace(10); if ($bin) { $bin.Items() | ForEach-Object { Remove-Item -LiteralPath $_.Path -Recurse -Force -ErrorAction SilentlyContinue } }")
	if err2 == nil {
		return JobResult{JobID: jobID, Status: "ok", Via: "Shell.Application", Message: "Recycle Bin emptied", Output: clip(out)}
	}
	out3, err3 := runCmd("powershell", "-NoProfile", "-Command", "Clear-RecycleBin -Force -ErrorAction SilentlyContinue")
	if err3 == nil {
		return JobResult{JobID: jobID, Status: "ok", Via: "Clear-RecycleBin", Message: "Recycle Bin emptied", Output: clip(out3)}
	}
	return JobResult{
		JobID:   jobID,
		Status:  "error",
		Via:     "SHEmptyRecycleBin",
		Message: "Could not empty Recycle Bin",
		Output:  clip(fmt.Sprintf("%v\n%v\n%s", err, err2, out3)),
	}
}

func launchProgram(jobID, target, args string) JobResult {
	if target == "" {
		return JobResult{JobID: jobID, Status: "error", Message: "program path or name is required"}
	}
	resolved := resolveTarget(target)
	if err := shellExec(resolved, args); err == nil {
		return JobResult{JobID: jobID, Status: "ok", Via: "ShellExecute", Message: "Launched " + resolved, Output: resolved}
	} else {
		cmd := exec.Command(resolved)
		if args != "" {
			cmd = exec.Command("cmd", "/c", "start", "", resolved, args)
		} else {
			cmd = exec.Command("cmd", "/c", "start", "", resolved)
		}
		cmd.SysProcAttr = &windows.SysProcAttr{HideWindow: false}
		if err2 := cmd.Start(); err2 == nil {
			return JobResult{JobID: jobID, Status: "ok", Via: "cmd start", Message: "Launched " + resolved, Output: resolved}
		} else {
			return JobResult{JobID: jobID, Status: "error", Via: "ShellExecute", Message: "Launch failed for " + target, Output: err.Error() + "\n" + err2.Error()}
		}
	}
}

func resolveTarget(target string) string {
	if filepath.IsAbs(target) || strings.ContainsAny(target, `/\`) {
		return target
	}
	if p, err := exec.LookPath(target); err == nil {
		return p
	}
	roots := []string{
		os.Getenv("ProgramFiles"),
		os.Getenv("ProgramFiles(x86)"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs"),
	}
	want := strings.ToLower(target)
	for _, root := range roots {
		if root == "" {
			continue
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			p := filepath.Join(root, e.Name())
			if !e.IsDir() {
				if strings.EqualFold(e.Name(), target) {
					return p
				}
				continue
			}
			inner, _ := os.ReadDir(p)
			for _, f := range inner {
				if strings.EqualFold(f.Name(), target) || strings.ToLower(f.Name()) == want {
					return filepath.Join(p, f.Name())
				}
			}
		}
	}
	return target
}

func shellExec(file, params string) error {
	verb, _ := windows.UTF16PtrFromString("open")
	f, _ := windows.UTF16PtrFromString(file)
	var p *uint16
	if params != "" {
		p, _ = windows.UTF16PtrFromString(params)
	}
	r, _, err := procShellExecuteW.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(f)), uintptr(unsafe.Pointer(p)), 0, 1)
	if r <= 32 {
		return err
	}
	return nil
}

func clip(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 4000 {
		return s[:4000] + "…"
	}
	return s
}
