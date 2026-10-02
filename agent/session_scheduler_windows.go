//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func runScheduledSessionWorker(token windows.Token, sid, worker, input string) error {
	var sessionID, size uint32
	if err := windows.GetTokenInformation(token, windows.TokenSessionId, (*byte)(unsafe.Pointer(&sessionID)), uint32(unsafe.Sizeof(sessionID)), &size); err != nil {
		return fmt.Errorf("read target session: %w", err)
	}
	if sessionID == 0 {
		return fmt.Errorf("refusing a session-zero user task")
	}
	script := filepath.Join(filepath.Dir(input), "start-worker.ps1")
	if err := os.WriteFile(script, []byte(scheduledSessionScript), 0600); err != nil {
		return err
	}
	// The script is inherited read-only by the user, like the executable/request.
	if err := setSessionJobACL(script, sid, false); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 11*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script)
	cmd.SysProcAttr = &windows.SysProcAttr{HideWindow: true}
	cmd.Env = append(os.Environ(), "FLEET_SESSION_SID="+sid, "FLEET_SESSION_ID="+strconv.FormatUint(uint64(sessionID), 10), "FLEET_SESSION_WORKER="+worker, "FLEET_SESSION_INPUT="+input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// No trigger, saved password, or elevated principal. An explicit session ID
// prevents reconnects from mapping drives in a different logon of the same user.
const scheduledSessionScript = `
$ErrorActionPreference = 'Stop'
function Invoke-FleetSessionWorker($service, [string]$sid, [int]$sessionId, [string]$worker, [string]$inputFile) {
    if ($sessionId -le 0) { throw 'An interactive session is required' }
    $folder = $service.GetFolder('\')
    $definition = $service.NewTask(0)
    $definition.Principal.UserId = $sid
    $definition.Principal.LogonType = 3 # TASK_LOGON_INTERACTIVE_TOKEN
    $definition.Principal.RunLevel = 0 # TASK_RUNLEVEL_LUA: same drive namespace as Explorer
    $definition.Settings.Enabled = $true
    $definition.Settings.AllowDemandStart = $true
    $definition.Settings.DisallowStartIfOnBatteries = $false
    $definition.Settings.StopIfGoingOnBatteries = $false
    $definition.Settings.ExecutionTimeLimit = 'PT10M'
    $action = $definition.Actions.Create(0)
    $action.Path = $worker
    $action.Arguments = '--session-job "' + $inputFile + '"'
    $action.WorkingDirectory = [IO.Path]::GetDirectoryName($inputFile)
    $name = 'Fleet Console Session ' + [Guid]::NewGuid().ToString('N')
    $task = $null
    try {
        # CREATE | DONT_ADD_PRINCIPAL_ACE: only SYSTEM/admins may alter this task.
        $task = $folder.RegisterTaskDefinition($name, $definition, 18, $sid, $null, 3, 'D:P(A;;GA;;;SY)(A;;GA;;;BA)')
        $null = $task.RunEx($null, 4, $sessionId, $null) # TASK_RUN_USE_SESSION_ID
        $deadline = [DateTime]::UtcNow.AddMinutes(10)
        do {
            Start-Sleep -Milliseconds 200
            $state = $task.State
            $result = $task.LastTaskResult
            if ($state -ne 2 -and $state -ne 4 -and $result -ne 0x41303) {
                if ($result -ne 0) { throw ('Worker task failed: 0x{0:X8}' -f [long]$result) }
                return
            }
        } while ([DateTime]::UtcNow -lt $deadline)
        throw 'Worker task timed out after 10 minutes'
    } finally {
        if ($null -ne $task) {
            try {
                if ($task.State -eq 2 -or $task.State -eq 4) { $task.Stop(0) }
            } finally { $folder.DeleteTask($name, 0) }
        }
    }
}
try {
    $service = New-Object -ComObject 'Schedule.Service'
    $service.Connect()
    Invoke-FleetSessionWorker $service $env:FLEET_SESSION_SID ([int]$env:FLEET_SESSION_ID) $env:FLEET_SESSION_WORKER $env:FLEET_SESSION_INPUT
} catch {
    [Console]::Error.WriteLine($_.Exception.Message)
    exit 1
}
`
