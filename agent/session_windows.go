//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var sessionPrivilegesOnce sync.Once
var sessionPrivilegesErr error

// Enable only privileges already assigned to the installed SYSTEM task.
func ensureSessionPrivileges() error {
	sessionPrivilegesOnce.Do(func() {
		var token windows.Token
		if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY|windows.TOKEN_ADJUST_PRIVILEGES, &token); err != nil {
			sessionPrivilegesErr = err
			return
		}
		defer token.Close()
		user, err := token.GetTokenUser()
		if err != nil {
			sessionPrivilegesErr = err
			return
		}
		if !user.User.Sid.IsWellKnown(windows.WinLocalSystemSid) {
			sessionPrivilegesErr = fmt.Errorf("background agent must run as LocalSystem; rerun dist\\install.ps1 as administrator to repair the Fleet Console Agent startup task")
			return
		}
		for _, name := range []string{"SeTcbPrivilege", "SeAssignPrimaryTokenPrivilege", "SeIncreaseQuotaPrivilege"} {
			var luid windows.LUID
			p, _ := windows.UTF16PtrFromString(name)
			if err := windows.LookupPrivilegeValue(nil, p, &luid); err != nil {
				sessionPrivilegesErr = err
				return
			}
			state := windows.Tokenprivileges{PrivilegeCount: 1}
			state.Privileges[0] = windows.LUIDAndAttributes{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED}
			if err := windows.AdjustTokenPrivileges(token, false, &state, 0, nil, nil); err != nil {
				sessionPrivilegesErr = fmt.Errorf("enable %s: %w", name, err)
				return
			}
			if err := verifyTokenPrivilege(token, luid); err != nil {
				sessionPrivilegesErr = fmt.Errorf("%s unavailable: %w; repair the SYSTEM startup task or review its local security policy", name, err)
				return
			}
		}
	})
	return sessionPrivilegesErr
}

func verifyTokenPrivilege(token windows.Token, want windows.LUID) error {
	var size uint32
	err := windows.GetTokenInformation(token, windows.TokenPrivileges, nil, 0, &size)
	if err != windows.ERROR_INSUFFICIENT_BUFFER {
		return fmt.Errorf("read privilege size: %v", err)
	}
	buffer := make([]byte, size)
	if err := windows.GetTokenInformation(token, windows.TokenPrivileges, &buffer[0], size, &size); err != nil {
		return err
	}
	privileges := (*windows.Tokenprivileges)(unsafe.Pointer(&buffer[0]))
	for _, privilege := range privileges.AllPrivileges() {
		if privilege.Luid == want && privilege.Attributes&windows.SE_PRIVILEGE_ENABLED != 0 {
			return nil
		}
	}
	return windows.ERROR_NOT_ALL_ASSIGNED
}

func packageJobNeedsAdmin(kind string) bool { return kind == "install" || kind == "uninstall" }

func interactiveToken() (windows.Token, error) {
	if err := ensureSessionPrivileges(); err != nil {
		return 0, err
	}
	var sessions *windows.WTS_SESSION_INFO
	var count uint32
	if err := windows.WTSEnumerateSessions(0, 0, 1, &sessions, &count); err != nil {
		return 0, err
	}
	defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(sessions)))
	sessionID, err := selectInteractiveSession(unsafe.Slice(sessions, count), windows.WTSGetActiveConsoleSessionId())
	if err != nil {
		return 0, err
	}
	var token windows.Token
	if err := windows.WTSQueryUserToken(sessionID, &token); err != nil {
		return 0, fmt.Errorf("query signed-in user token for session %d: %w; verify the agent startup task runs as SYSTEM", sessionID, err)
	}
	return token, nil
}

func selectInteractiveSession(sessions []windows.WTS_SESSION_INFO, console uint32) (uint32, error) {
	var active []uint32
	for _, session := range sessions {
		if session.State == windows.WTSActive && session.SessionID != 0 {
			if session.SessionID == console {
				active = []uint32{console}
				break
			}
			active = append(active, session.SessionID)
		}
	}
	if len(active) != 1 {
		return 0, fmt.Errorf("one active user session is required (found %d); sign in on the PC and retry", len(active))
	}
	return active[0], nil
}

func interactiveUser() string {
	token, err := interactiveToken()
	if err != nil {
		return ""
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return ""
	}
	name, _, _, err := user.User.Sid.LookupAccount("")
	if err != nil {
		return ""
	}
	return name
}

func runInteractiveJob(job AssignedJob) JobResult {
	result, err := dispatchInteractiveJob(job)
	if err != nil {
		return JobResult{JobID: job.ID, Status: "error", Message: "User-session action failed: " + err.Error()}
	}
	return result
}

func dispatchInteractiveJob(job AssignedJob) (result JobResult, err error) {
	stage := "select user session"
	defer func() {
		if err != nil {
			err = fmt.Errorf("%s: %w", stage, err)
		}
	}()
	token, err := interactiveToken()
	if err != nil {
		return result, err
	}
	defer token.Close()
	if packageJobNeedsAdmin(job.Kind) && !token.IsElevated() {
		stage = "obtain administrator rights for " + job.Kind
		linked, linkErr := token.GetLinkedToken()
		if linkErr != nil {
			return result, fmt.Errorf("the signed-in user has no administrator token; sign in with an administrator account and retry (%w)", linkErr)
		}
		defer linked.Close()
		if !linked.IsElevated() {
			return result, fmt.Errorf("administrator rights are required for %s", job.Kind)
		}
		token = linked
	}
	stage = "read user identity"
	user, err := token.GetTokenUser()
	if err != nil {
		return result, err
	}

	// The worker and request are read-only to the ordinary user, especially when
	// the job runs elevated. Only the separate output directory is user-writable.
	stage = "create protected worker directory"
	dir, err := os.MkdirTemp(dataDir, "session-job-")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(dir) // MkdirTemp creates this exact child of the configured data directory.
	sd, err := windows.SecurityDescriptorFromString(sessionJobSDDL(user.User.Sid.String(), false))
	if err != nil {
		return result, err
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return result, err
	}
	if err = windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		return result, err
	}
	stage = "grant result-directory access"
	outputDir := filepath.Join(dir, "output")
	if err = os.Mkdir(outputDir, 0700); err != nil {
		return result, err
	}
	outputSD, err := windows.SecurityDescriptorFromString(sessionJobSDDL(user.User.Sid.String(), true))
	if err != nil {
		return result, err
	}
	outputACL, _, err := outputSD.DACL()
	if err != nil {
		return result, err
	}
	if err = windows.SetNamedSecurityInfo(outputDir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, outputACL, nil); err != nil {
		return result, err
	}
	stage = "write job request"
	input := filepath.Join(dir, "job.json")
	raw, err := json.Marshal(job)
	if err != nil {
		return result, err
	}
	if err = os.WriteFile(input, raw, 0600); err != nil {
		return result, err
	}

	var env *uint16
	stage = "create user environment"
	if err = windows.CreateEnvironmentBlock(&env, token, false); err != nil {
		return result, err
	}
	defer windows.DestroyEnvironmentBlock(env)
	exe, err := os.Executable()
	if err != nil {
		return result, err
	}
	// This is a short-lived job worker, not another resident agent. Give it a
	// distinct process name so Task Manager reflects that distinction.
	worker := filepath.Join(dir, "fleet-session-worker.exe")
	stage = "copy session worker"
	if err = copyFile(exe, worker); err != nil {
		return result, err
	}
	exe = worker
	app, _ := windows.UTF16PtrFromString(exe)
	command, _ := windows.UTF16PtrFromString(windows.ComposeCommandLine([]string{exe, "--session-job", input}))
	// Non-UI jobs do not need access to the interactive window station.
	desktopName := ""
	if job.Kind == "launch" {
		desktopName = `winsta0\default`
	}
	desktop, _ := windows.UTF16PtrFromString(desktopName)
	cwd, _ := windows.UTF16PtrFromString(dir)
	si := windows.StartupInfo{Desktop: desktop}
	si.Cb = uint32(unsafe.Sizeof(si))
	var pi windows.ProcessInformation
	stage = "start user-session worker"
	if err = windows.CreateProcessAsUser(token, app, command, nil, nil, false, windows.CREATE_UNICODE_ENVIRONMENT|windows.CREATE_NO_WINDOW, env, cwd, &si, &pi); err != nil {
		return result, err
	}
	windows.CloseHandle(pi.Thread)
	defer windows.CloseHandle(pi.Process)
	stage = "wait for session worker"
	status, err := windows.WaitForSingleObject(pi.Process, 10*60*1000)
	if err != nil || status != windows.WAIT_OBJECT_0 {
		kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.FormatUint(uint64(pi.ProcessId), 10))
		kill.SysProcAttr = &windows.SysProcAttr{HideWindow: true}
		_ = kill.Run()
		return result, fmt.Errorf("session worker timed out or could not be monitored")
	}
	stage = "read session worker result"
	var exitCode uint32
	if err = windows.GetExitCodeProcess(pi.Process, &exitCode); err != nil {
		return result, err
	}
	if exitCode != 0 {
		return result, fmt.Errorf("worker exited with code 0x%08X; verify application-control policy permits fleet-session-worker.exe", exitCode)
	}
	file, err := os.Open(filepath.Join(outputDir, "result.json"))
	if err != nil {
		return result, fmt.Errorf("session worker returned no result: %w", err)
	}
	defer file.Close()
	if err = json.NewDecoder(io.LimitReader(file, 2*1024*1024)).Decode(&result); err != nil {
		return result, err
	}
	if result.JobID != job.ID || (result.Status != "ok" && result.Status != "error") {
		return JobResult{}, fmt.Errorf("invalid session worker result")
	}
	return result, nil
}

func sessionJobSDDL(sid string, writable bool) string {
	rights := "GRGX"
	if writable {
		rights = "FA"
	}
	return "D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;" + rights + ";;;" + sid + ")"
}
