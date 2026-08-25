# Fleet Console - host install (ASCII only; Windows PowerShell 5.1)
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
  Write-Host "Fleet Console host installer" -ForegroundColor Cyan
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

  Write-Step "npm install"
  Push-Location $root
  npm install
  Pop-Location

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

  Write-Step "Starting Fleet Console tray host"
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
  Start-Process -FilePath $exe
  Write-Host "Started. Look for the Fleet Console icon in the notification area."
  Write-Host "Dashboard: http://127.0.0.1:43123"
  Write-Host "SUCCESS" -ForegroundColor Green
} catch {
  Write-Host "INSTALL FAILED" -ForegroundColor Red
  Write-Host $_.Exception.Message
  if (-not $NoPause) { Read-Host "Press Enter to close this window" }
  exit 1
}

if (-not $NoPause) { Read-Host "Press Enter to close this window" }
