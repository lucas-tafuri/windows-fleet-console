# Fleet Console — Windows client install
# Logs to %TEMP%\fleet-console-install.log and %LOCALAPPDATA%\FleetConsole\install.log
# The window stays open until you press Enter.
#
#   powershell -ExecutionPolicy Bypass -File install.ps1 -Server http://HOST:43123 -Token TOKEN
#   or double-click install.cmd

param(
  [string]$Server = $env:FLEET_SERVER,
  [string]$Token = $env:FLEET_TOKEN,
  [switch]$HttpOnly,
  [switch]$NoPause,
  [string]$Repo = "https://github.com/lucas-tafuri/windows-fleet-console.git"
)

$ErrorActionPreference = "Stop"
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$logTemp = Join-Path $env:TEMP "fleet-console-install.log"
$installRoot = Join-Path $env:LOCALAPPDATA "FleetConsole"
$logLocal = Join-Path $installRoot "install.log"
$repoDir = Join-Path $installRoot "repo"
$exeUrl = "https://github.com/lucas-tafuri/windows-fleet-console/raw/main/dist/fleet-agent.exe"
$here = Split-Path -Parent $MyInvocation.MyCommand.Path
if (-not $here) { $here = (Get-Location).Path }
$exitCode = 0

function Write-Step($msg) {
  Write-Host ""
  Write-Host ">> $msg" -ForegroundColor Yellow
}

function Save-LogCopy {
  try {
    New-Item -ItemType Directory -Force -Path $installRoot | Out-Null
    if (Test-Path $logTemp) { Copy-Item $logTemp $logLocal -Force }
  } catch {}
}

try {
  New-Item -ItemType Directory -Force -Path $installRoot | Out-Null
  "=== Fleet Console install $(Get-Date -Format o) ===" | Set-Content -Path $logTemp -Encoding UTF8
  try { Start-Transcript -Path $logTemp -Append -Force | Out-Null } catch {}

  Write-Host ""
  Write-Host "Fleet Console client installer" -ForegroundColor Cyan
  Write-Host "Log file: $logTemp"
  Write-Host "Also:     $logLocal"
  Write-Host "Folder:   $here"
  Write-Host ""

  if (-not $Server) {
    $Server = Read-Host "Fleet Console URL (example: http://192.168.1.10:43123)"
  }
  if (-not $Token) {
    $Token = Read-Host "Fleet token (from the Enroll page)"
  }
  $Server = "$Server".Trim().TrimEnd("/")
  $Token = "$Token".Trim()
  if (-not $Server -or -not $Token) {
    throw "Server URL and token are required. Run again and paste both."
  }
  Write-Host "Server: $Server"

  $haveGit = $false
  if (Get-Command git -ErrorAction SilentlyContinue) {
    $haveGit = $true
    Write-Step "Git is already installed"
  } else {
    Write-Step "Git not found — trying winget (this can take a minute)"
    if (Get-Command winget -ErrorAction SilentlyContinue) {
      & winget install -e --id Git.Git --accept-package-agreements --accept-source-agreements --silent
      $env:Path = [Environment]::GetEnvironmentVariable("Path", "Machine") + ";" +
        [Environment]::GetEnvironmentVariable("Path", "User")
      $gitProg = Join-Path ${env:ProgramFiles} "Git\cmd"
      if (Test-Path $gitProg) { $env:Path = "$gitProg;$env:Path" }
      $haveGit = [bool](Get-Command git -ErrorAction SilentlyContinue)
    }
    if (-not $haveGit) {
      Write-Host "Git is not available. Updates will download the exe from GitHub instead."
    }
  }

  $exe = Join-Path $here "fleet-agent.exe"
  if (-not (Test-Path $exe)) {
    Write-Step "Downloading fleet-agent.exe from GitHub"
    Invoke-WebRequest -UseBasicParsing -Uri $exeUrl -OutFile $exe
  } else {
    Write-Step "Using existing fleet-agent.exe"
  }
  if (-not (Test-Path $exe)) {
    throw "fleet-agent.exe is missing and download failed."
  }
  Write-Host "Agent: $exe"

  if ($haveGit) {
    Write-Step "Fetching the fleet repo (for later Update / git pull)"
    if (Test-Path (Join-Path $repoDir ".git")) {
      & git -C $repoDir pull --ff-only origin main 2>&1 | ForEach-Object { Write-Host "   $_" }
    } else {
      if (Test-Path $repoDir) { Remove-Item -Recurse -Force $repoDir }
      & git clone --depth 1 --branch main $Repo $repoDir 2>&1 | ForEach-Object { Write-Host "   $_" }
    }
    if ($LASTEXITCODE -and $LASTEXITCODE -ne 0) {
      Write-Host "Git clone/pull failed (exit $LASTEXITCODE). Continuing with the agent anyway."
    }
  }

  $argList = @("--server", $Server, "--token", $Token, "--data-dir", $installRoot)
  if ($HttpOnly) { $argList += "--http-only" }

  Write-Step "Starting the agent (copies itself to LocalAppData and registers at logon)"
  Write-Host "Command: $exe $($argList -join ' ')"
  & $exe @argList
  Write-Host "Agent first-launch exit code: $LASTEXITCODE"

  Start-Sleep -Seconds 2
  $running = Get-Process -Name "fleet-agent" -ErrorAction SilentlyContinue
  $installed = Join-Path $installRoot "fleet-agent.exe"
  $task = schtasks /query /tn "Fleet Console Agent" 2>$null

  Write-Host ""
  if ($running) {
    Write-Host "SUCCESS — fleet-agent is running." -ForegroundColor Green
  } else {
    Write-Host "The agent process is not visible yet." -ForegroundColor Yellow
    Write-Host "Check Task Manager for fleet-agent.exe, or run the exe again."
  }
  Write-Host "  install folder : $installRoot"
  if (Test-Path $installed) { Write-Host "  installed exe  : $installed" }
  Write-Host "  server         : $Server"
  if ($task) {
    Write-Host "  logon task     : Fleet Console Agent (registered)"
  } else {
    Write-Host "  logon task     : not listed yet (Startup folder fallback may still apply)"
  }
  Write-Host "It should start again the next time this user signs in."
}
catch {
  $exitCode = 1
  Write-Host ""
  Write-Host "INSTALL FAILED" -ForegroundColor Red
  Write-Host $_.Exception.Message
  if ($_.ScriptStackTrace) { Write-Host $_.ScriptStackTrace }
  Add-Content -Path $logTemp -Value "FAILED: $($_.Exception.Message)" -ErrorAction SilentlyContinue
}
finally {
  try { Stop-Transcript | Out-Null } catch {}
  Save-LogCopy
  Write-Host ""
  Write-Host "======== LOG ========" -ForegroundColor Cyan
  Write-Host $logTemp
  Write-Host $logLocal
  Write-Host "====================="
  if (-not $NoPause) {
    Write-Host ""
    Read-Host "Press Enter to close this window"
  }
}

exit $exitCode
