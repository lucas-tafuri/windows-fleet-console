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
	"unsafe"

	"golang.org/x/sys/windows"
)

func interactiveToken() (windows.Token, error) {
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
		return 0, err
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

func dispatchInteractiveJob(job AssignedJob) (JobResult, error) {
	var result JobResult
	token, err := interactiveToken()
	if err != nil {
		return result, err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return result, err
	}

	// Limit job credentials and results to SYSTEM, administrators, and this user.
	dir, err := os.MkdirTemp(dataDir, "session-job-")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(dir) // MkdirTemp creates this exact child of the configured data directory.
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;FA;;;" + user.User.Sid.String() + ")")
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
	input := filepath.Join(dir, "job.json")
	raw, err := json.Marshal(job)
	if err != nil {
		return result, err
	}
	if err = os.WriteFile(input, raw, 0600); err != nil {
		return result, err
	}

	var env *uint16
	if err = windows.CreateEnvironmentBlock(&env, token, false); err != nil {
		return result, err
	}
	defer windows.DestroyEnvironmentBlock(env)
	exe, err := os.Executable()
	if err != nil {
		return result, err
	}
	app, _ := windows.UTF16PtrFromString(exe)
	command, _ := windows.UTF16PtrFromString(windows.ComposeCommandLine([]string{exe, "--session-job", input}))
	desktop, _ := windows.UTF16PtrFromString(`winsta0\default`)
	cwd, _ := windows.UTF16PtrFromString(dir)
	si := windows.StartupInfo{Desktop: desktop}
	si.Cb = uint32(unsafe.Sizeof(si))
	var pi windows.ProcessInformation
	if err = windows.CreateProcessAsUser(token, app, command, nil, nil, false, windows.CREATE_UNICODE_ENVIRONMENT|windows.CREATE_NO_WINDOW, env, cwd, &si, &pi); err != nil {
		return result, err
	}
	windows.CloseHandle(pi.Thread)
	defer windows.CloseHandle(pi.Process)
	status, err := windows.WaitForSingleObject(pi.Process, 10*60*1000)
	if err != nil || status != windows.WAIT_OBJECT_0 {
		kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.FormatUint(uint64(pi.ProcessId), 10))
		kill.SysProcAttr = &windows.SysProcAttr{HideWindow: true}
		_ = kill.Run()
		return result, fmt.Errorf("session worker timed out or could not be monitored")
	}
	file, err := os.Open(filepath.Join(dir, "result.json"))
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
