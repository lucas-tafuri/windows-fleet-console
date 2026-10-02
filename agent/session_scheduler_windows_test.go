//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestSessionFallbackOnlyRetriesDeniedNonElevatedLaunch(t *testing.T) {
	for _, kind := range []string{"map_drive", "unmap_drive", "maintain_drives", "check", "launch", "install", "uninstall"} {
		for _, err := range []error{nil, windows.ERROR_ACCESS_DENIED, windows.ERROR_FILE_NOT_FOUND, windows.ERROR_PRIVILEGE_NOT_HELD} {
			want := err == windows.ERROR_ACCESS_DENIED && kind != "install" && kind != "uninstall"
			if got := useScheduledSessionWorker(kind, err); got != want {
				t.Fatalf("%s, %v: fallback = %v", kind, err, got)
			}
		}
	}
}

// Execute the actual PowerShell with a fake COM boundary. No installed tasks or
// real services are modified; the assertions check the identity/session contract.
func TestScheduledSessionScript(t *testing.T) {
	for _, mode := range []string{"success", "launch-denied", "worker-failed", "registration-denied"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			script := filepath.Join(dir, "probe.ps1")
			if err := os.WriteFile(script, []byte(scheduledSessionMock+scheduledSessionScript), 0600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script)
			cmd.SysProcAttr = &windows.SysProcAttr{HideWindow: true}
			cmd.Env = append(os.Environ(), "FLEET_SESSION_TEST="+mode, "FLEET_SESSION_CLEANUP="+filepath.Join(dir, "deleted"), "FLEET_SESSION_SID=S-1-5-21-111-222-333-1001", "FLEET_SESSION_ID=7", "FLEET_SESSION_WORKER=C:\\Protected Folder\\fleet-session-worker.exe", "FLEET_SESSION_INPUT=C:\\Protected Folder\\job.json")
			out, err := cmd.CombinedOutput()
			if (err == nil) != (mode == "success") {
				t.Fatalf("unexpected status %v: %s", err, out)
			}
			if mode != "success" {
				message := map[string]string{"launch-denied": "test launch denied", "worker-failed": "0x00000005", "registration-denied": "test registration denied"}[mode]
				if !strings.Contains(string(out), message) {
					t.Fatalf("missing diagnostic %s: %s", message, out)
				}
			}
			_, err = os.Stat(filepath.Join(dir, "deleted"))
			if (err == nil) != (mode != "registration-denied") {
				t.Fatal(fmt.Sprintf("unexpected cleanup: %v", err))
			}
		})
	}
}

const scheduledSessionMock = `
$ErrorActionPreference = 'Stop'
function Start-Sleep { param($Milliseconds) }
$script:action = [pscustomobject]@{Path=''; Arguments=''; WorkingDirectory=''}
$actions = New-Object PSObject
$actions | Add-Member ScriptMethod Create { param($type) if ($type -ne 0) { throw 'wrong action' }; return $script:action }
$script:definition = [pscustomobject]@{
    Principal=[pscustomobject]@{UserId=''; LogonType=-1; RunLevel=-1}
    Settings=[pscustomobject]@{Enabled=$false; AllowDemandStart=$false; DisallowStartIfOnBatteries=$true; StopIfGoingOnBatteries=$true; ExecutionTimeLimit=''}
    Actions=$actions
}
$script:task = [pscustomobject]@{State=3; LastTaskResult=0}
$script:task | Add-Member ScriptMethod RunEx {
    param($parameters, $flags, $sessionId, $user)
    if ($flags -ne 4 -or $sessionId -ne 7 -or $null -ne $user) { throw 'wrong session selection' }
    if ($env:FLEET_SESSION_TEST -eq 'launch-denied') { throw 'test launch denied' }
    if ($env:FLEET_SESSION_TEST -eq 'worker-failed') { $this.LastTaskResult=5 }
}
$script:task | Add-Member ScriptMethod Stop { param($flags) throw 'must not stop a completed task' }
$script:folder = New-Object PSObject
$script:folder | Add-Member ScriptMethod RegisterTaskDefinition {
    param($name, $definition, $flags, $sid, $password, $logonType, $sddl)
    if ($env:FLEET_SESSION_TEST -eq 'registration-denied') { throw 'test registration denied' }
    if ($name -notmatch '^Fleet Console Session [a-f0-9]{32}$') { throw 'unsafe task name' }
    $script:taskName=$name
    if ($flags -ne 18 -or $null -ne $password -or $logonType -ne 3) { throw 'wrong registration' }
    if ($sddl -ne 'D:P(A;;GA;;;SY)(A;;GA;;;BA)') { throw 'unsafe task ACL' }
    if ($sid -ne $env:FLEET_SESSION_SID -or $definition.Principal.UserId -ne $sid -or $definition.Principal.RunLevel -ne 0 -or $definition.Principal.LogonType -ne 3) { throw 'wrong identity or elevation' }
    if ($definition.Settings.DisallowStartIfOnBatteries -or $definition.Settings.StopIfGoingOnBatteries -or $definition.Settings.ExecutionTimeLimit -ne 'PT10M') { throw 'wrong task settings' }
    if ($script:action.Path -ne $env:FLEET_SESSION_WORKER -or $script:action.Arguments -ne ('--session-job "' + $env:FLEET_SESSION_INPUT + '"')) { throw 'wrong worker command' }
    return $script:task
}
$script:folder | Add-Member ScriptMethod DeleteTask {
    param($name, $flags)
    if ($name -ne $script:taskName) { throw 'deleted another task' }
    [IO.File]::WriteAllText($env:FLEET_SESSION_CLEANUP, 'ok')
}
$script:service = New-Object PSObject
$script:service | Add-Member ScriptMethod Connect {}
$script:service | Add-Member ScriptMethod GetFolder { param($path) return $script:folder }
$script:service | Add-Member ScriptMethod NewTask { param($flags) return $script:definition }
function New-Object { param($ComObject) if ($ComObject -ne 'Schedule.Service') { throw 'unexpected COM object' }; return $script:service }
`
