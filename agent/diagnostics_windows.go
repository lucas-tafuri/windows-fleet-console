//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

// Only fixed, read-only scripts are allowed. Browser input is never interpolated
// into PowerShell, and reports deliberately omit command lines and environment.
var diagnosticScripts = map[string]string{
	"system":    `$os=Get-CimInstance Win32_OperatingSystem; $pc=Get-CimInstance Win32_ComputerSystem; $cpu=Get-CimInstance Win32_Processor | Select-Object -First 1; [pscustomobject]@{Computer=$env:COMPUTERNAME; Windows=$os.Caption; Build=$os.BuildNumber; Manufacturer=$pc.Manufacturer; Model=$pc.Model; Processor=$cpu.Name; LogicalProcessors=$pc.NumberOfLogicalProcessors; MemoryGB=[math]::Round($pc.TotalPhysicalMemory/1GB,1); UptimeHours=[math]::Round(((Get-Date)-$os.LastBootUpTime).TotalHours,1); LastBoot=$os.LastBootUpTime.ToUniversalTime().ToString('o')}`,
	"disks":     `Get-CimInstance Win32_LogicalDisk -Filter 'DriveType=3' | Sort-Object DeviceID | ForEach-Object { [pscustomobject]@{Drive=$_.DeviceID; Label=$_.VolumeName; FileSystem=$_.FileSystem; SizeGB=[math]::Round($_.Size/1GB,1); FreeGB=[math]::Round($_.FreeSpace/1GB,1); FreePercent=$(if($_.Size){[math]::Round(100*$_.FreeSpace/$_.Size,1)}else{0}); Attention=$(if($_.Size -and $_.FreeSpace/$_.Size -lt 0.1){'Low disk space'}else{'OK'})} }`,
	"network":   `Get-NetIPConfiguration | Where-Object {$_.IPv4Address} | ForEach-Object { [pscustomobject]@{Adapter=$_.InterfaceAlias; IPv4=($_.IPv4Address.IPAddress -join ', '); Gateway=($_.IPv4DefaultGateway.NextHop -join ', '); DNS=($_.DNSServer.ServerAddresses -join ', '); Profile=[string]$_.NetProfile.NetworkCategory; Status=$_.NetAdapter.Status} }`,
	"processes": `Get-Process | Sort-Object WorkingSet64 -Descending | Select-Object -First 40 | ForEach-Object { [pscustomobject]@{Process=$_.ProcessName; PID=$_.Id; MemoryMB=[math]::Round($_.WorkingSet64/1MB,1); CPUSeconds=[math]::Round($_.CPU,1); Threads=$_.Threads.Count; Session=$_.SessionId} }`,
	"services":  `Get-CimInstance Win32_Service | Where-Object {$_.StartMode -eq 'Auto' -or $_.State -eq 'Running'} | Sort-Object State,DisplayName | Select-Object -First 100 | ForEach-Object { [pscustomobject]@{Service=$_.Name; DisplayName=$_.DisplayName; State=$_.State; StartMode=$_.StartMode; PID=$_.ProcessId} }`,
	"events":    `$events=Get-WinEvent -FilterHashtable @{LogName='System','Application'; Level=1,2; StartTime=(Get-Date).AddDays(-1)} -MaxEvents 20 -ErrorAction SilentlyContinue -ErrorVariable eventError; if($eventError -and $eventError[0].FullyQualifiedErrorId -notlike 'NoMatchingEventsFound*'){throw $eventError[0]}; $events | ForEach-Object { $message=[string]$_.Message; if($message.Length -gt 800){$message=$message.Substring(0,800)+'...'}; [pscustomobject]@{Time=$_.TimeCreated.ToUniversalTime().ToString('o'); Log=$_.LogName; Source=$_.ProviderName; EventID=$_.Id; Message=$message} }`,
}

func diagnosticReport(jobID, profile string) JobResult {
	script, ok := diagnosticScripts[profile]
	if !ok {
		return JobResult{JobID: jobID, Status: "error", Message: "Unknown diagnostic report"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	script = "$ErrorActionPreference='Stop'; [Console]::OutputEncoding=[Text.UTF8Encoding]::new(); $rows=@(& { " + script + " }); ConvertTo-Json -Depth 5 -Compress -InputObject @{profile='" + profile + "'; capturedAt=[DateTime]::UtcNow.ToString('o'); rows=$rows}"
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.SysProcAttr = &windows.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return JobResult{JobID: jobID, Status: "error", Message: "Report timed out after 45 seconds", Via: "Windows diagnostics"}
	}
	if err != nil {
		return JobResult{JobID: jobID, Status: "error", Message: "Windows could not collect this report", Output: clip(string(out)), Via: "Windows diagnostics"}
	}
	output := strings.TrimSpace(strings.TrimPrefix(string(out), "\ufeff"))
	var report struct {
		Rows []json.RawMessage `json:"rows"`
	}
	if len(output) > 60*1024 || json.Unmarshal([]byte(output), &report) != nil {
		return JobResult{JobID: jobID, Status: "error", Message: "Report exceeded its limit or returned invalid data", Via: "Windows diagnostics"}
	}
	return JobResult{JobID: jobID, Status: "ok", Message: fmt.Sprintf("%s report collected · %d entries", profile, len(report.Rows)), Output: output, Via: "Windows diagnostics"}
}
