# PrettyDamnFleet - host install (ASCII only; Windows PowerShell 5.1)
# Installs Node deps if needed, builds the tray host when Go is present,
# registers logon startup, and starts the dashboard in the tray.
#
#   powershell -ExecutionPolicy Bypass -File dist\install-host.ps1

param(
  [switch]$NoPause
)

$ErrorActionPreference = "Stop"
$here = Split-Path -Parent $MyInvocation.MyCommand.Path
if (-not $here) { $here = (Get-Location).Path }
$root = Split-Path -Parent $here
if (-not (Test-Path (Join-Path $root "server.mjs"))) {
  $root = (Get-Location).Path
}

function Refresh-Path {
  $env:Path = [System.Environment]::GetEnvironmentVariable("Path", "Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path", "User")
}

function Write-Step([string]$Message) {
  Write-Host ""
  Write-Host (">> " + $Message) -ForegroundColor Yellow
}

try {
  $admin = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
  if (-not $admin.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw "Run this installer as administrator to enable startup before Windows sign-in."
  }
  Write-Host "PrettyDamnFleet host installer" -ForegroundColor Cyan
  Write-Host ("Root: " + $root)

  Refresh-Path
  if (-not (Get-Command node -ErrorAction SilentlyContinue)) {
    Write-Step "Installing Node.js LTS"
    winget install --id OpenJS.NodeJS.LTS -e --accept-package-agreements --accept-source-agreements
    Refresh-Path
  }
  if (-not (Get-Command node -ErrorAction SilentlyContinue)) {
    throw "Node.js is required. Install Node 20+ and run this script again."
  }
  Write-Host ("Node " + (node -v))

  # Stop our task before replacing binaries or dependencies during upgrades.
  $task = Get-ScheduledTask -TaskName 'Fleet Console Host' -ErrorAction SilentlyContinue
  if ($task) { Stop-ScheduledTask -TaskName 'Fleet Console Host'; Start-Sleep -Seconds 2 }
  Write-Step "npm install"
  Push-Location $root
  try { npm install; if ($LASTEXITCODE -ne 0) { throw "npm install failed" } } finally { Pop-Location }

  $exe = Join-Path $root "dist\fleet-console.exe"
  if (Get-Command go -ErrorAction SilentlyContinue) {
    Write-Step "Building tray host"
    powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $root "scripts\build-windows.ps1")
  } elseif (-not (Test-Path $exe)) {
    throw "Go is not installed and dist\fleet-console.exe is missing. Install Go and retry."
  }

  if (-not (Test-Path $exe)) {
    throw "dist\fleet-console.exe was not built."
  }

  Write-Step "Starting PrettyDamnFleet tray host"
  $running = Get-CimInstance Win32_Process -Filter "Name='fleet-console.exe'" -ErrorAction SilentlyContinue
  if ($running) {
    Write-Host "Stopping existing fleet-console.exe..."
    Get-Process fleet-console -ErrorAction SilentlyContinue | Stop-Process -Force
    Start-Sleep -Seconds 1
  }
  $listen = Get-NetTCPConnection -LocalPort 43123 -State Listen -ErrorAction SilentlyContinue
  if ($listen) {
    Write-Host "Stopping process on port 43123..."
    $listen | ForEach-Object {
      try { Stop-Process -Id $_.OwningProcess -Force -ErrorAction SilentlyContinue } catch {}
    }
    Start-Sleep -Seconds 1
  }
  New-Item -ItemType Directory -Force -Path (Join-Path $root 'data') | Out-Null
  Set-Content -LiteralPath (Join-Path $root 'data\boot-installed') -Value '1'
  # Run a stable installed copy so git can replace dist binaries during updates.
  $runtimeDir = Join-Path $env:ProgramData 'FleetConsoleHost'
  New-Item -ItemType Directory -Force -Path $runtimeDir | Out-Null
  $runtimeExe = Join-Path $runtimeDir 'fleet-console.exe'
  Copy-Item -LiteralPath $exe -Destination $runtimeExe -Force
  $action = New-ScheduledTaskAction -Execute $runtimeExe -Argument ('--background --root "' + $root + '"') -WorkingDirectory $root
  $trigger = New-ScheduledTaskTrigger -AtStartup
  $principal = New-ScheduledTaskPrincipal -UserId 'SYSTEM' -LogonType ServiceAccount -RunLevel Highest
  $settings = New-ScheduledTaskSettingsSet -StartWhenAvailable -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -ExecutionTimeLimit ([TimeSpan]::Zero) -RestartCount 999 -RestartInterval (New-TimeSpan -Minutes 1) -MultipleInstances IgnoreNew
  Register-ScheduledTask -TaskName 'Fleet Console Host' -Action $action -Trigger $trigger -Principal $principal -Settings $settings -Force | Out-Null
  $oldStartup = Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs\Startup\FleetConsoleHost.cmd'
  if (Test-Path $oldStartup) { Remove-Item -LiteralPath $oldStartup }
  if (-not (Get-NetFirewallRule -DisplayName 'PrettyDamnFleet HTTP' -ErrorAction SilentlyContinue)) {
    New-NetFirewallRule -DisplayName 'PrettyDamnFleet HTTP' -Direction Inbound -Action Allow -Protocol TCP -LocalPort 43123 -Profile Private,Domain | Out-Null
  }
  if (-not (Get-NetFirewallRule -DisplayName 'PrettyDamnFleet Discovery' -ErrorAction SilentlyContinue)) {
    New-NetFirewallRule -DisplayName 'PrettyDamnFleet Discovery' -Direction Inbound -Action Allow -Protocol UDP -LocalPort 43124 -Profile Private,Domain | Out-Null
  }
  Start-ScheduledTask -TaskName 'Fleet Console Host'
  Write-Host "Started in the background. It will start automatically before Windows sign-in."
  Write-Host "Dashboard: http://127.0.0.1:43123"
  Write-Host "SUCCESS" -ForegroundColor Green
} catch {
  Write-Host "INSTALL FAILED" -ForegroundColor Red
  Write-Host $_.Exception.Message
  if (-not $NoPause) { Read-Host "Press Enter to close this window" }
  exit 1
}

if (-not $NoPause) { Read-Host "Press Enter to close this window" }
