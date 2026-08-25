# Fleet Console - Windows client install (ASCII only; Windows PowerShell 5.1)
# Logs: %TEMP%\fleet-console-install.log
#       %LOCALAPPDATA%\FleetConsole\install.log
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

function Write-Step {
  param([string]$Message)
  Write-Host ""
  Write-Host (">> " + $Message) -ForegroundColor Yellow
}

function Save-LogCopy {
  try {
    New-Item -ItemType Directory -Force -Path $installRoot | Out-Null
    if (Test-Path $logTemp) { Copy-Item $logTemp $logLocal -Force }
  } catch {}
}

try {
  New-Item -ItemType Directory -Force -Path $installRoot | Out-Null
  $stamp = Get-Date -Format o
  Set-Content -Path $logTemp -Encoding ASCII -Value ("=== Fleet Console install " + $stamp + " ===")
  try { Start-Transcript -Path $logTemp -Append -Force | Out-Null } catch {}

  Write-Host ""
  Write-Host "Fleet Console client installer" -ForegroundColor Cyan
  Write-Host ("Log file: " + $logTemp)
  Write-Host ("Also:     " + $logLocal)
  Write-Host ("Folder:   " + $here)
  Write-Host ""

  if (-not $Server) {
    $Server = Read-Host "Fleet Console URL (example: http://192.168.1.10:43123)"
  }
  if (-not $Token) {
    $Token = Read-Host "Fleet token (from the Enroll page)"
  }
  $Server = ([string]$Server).Trim().TrimEnd("/")
  $Token = ([string]$Token).Trim()
  if (-not $Server -or -not $Token) {
    throw "Server URL and token are required. Run again and paste both."
  }
  Write-Host ("Server: " + $Server)

  $haveGit = $false
  if (Get-Command git -ErrorAction SilentlyContinue) {
    $haveGit = $true
    Write-Step "Git is already installed"
  } else {
    Write-Step "Git not found - trying winget (this can take a minute)"
    if (Get-Command winget -ErrorAction SilentlyContinue) {
      & winget install -e --id Git.Git --accept-package-agreements --accept-source-agreements --silent
      $machinePath = [Environment]::GetEnvironmentVariable("Path", "Machine")
      $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
      $env:Path = $machinePath + ";" + $userPath
      $gitProg = Join-Path $env:ProgramFiles "Git\cmd"
      if (Test-Path $gitProg) {
        $env:Path = $gitProg + ";" + $env:Path
      }
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
  Write-Host ("Agent: " + $exe)

  if ($haveGit) {
    Write-Step "Fetching the fleet repo (optional, for later Update)"
    $oldEap = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    $oldNative = $null
    if (Test-Path variable:PSNativeCommandUseErrorActionPreference) {
      $oldNative = $PSNativeCommandUseErrorActionPreference
      $PSNativeCommandUseErrorActionPreference = $false
    }
    try {
      if (Test-Path (Join-Path $repoDir ".git")) {
        Write-Host "Updating existing checkout..."
        git -C $repoDir pull --ff-only origin main
      } else {
        if (Test-Path $repoDir) { Remove-Item -Recurse -Force $repoDir }
        Write-Host ("Cloning " + $Repo)
        git clone --depth 1 --branch main $Repo $repoDir
      }
      if ($LASTEXITCODE -and $LASTEXITCODE -ne 0) {
        Write-Host ("Git finished with exit " + $LASTEXITCODE + ". Continuing; the agent does not need this clone.")
      } else {
        Write-Host "Repo ready."
      }
    } catch {
      Write-Host ("Git clone/pull skipped: " + $_.Exception.Message)
      Write-Host "Continuing; the agent does not need this clone to run."
    } finally {
      $ErrorActionPreference = $oldEap
      if ($null -ne $oldNative) { $PSNativeCommandUseErrorActionPreference = $oldNative }
    }
  }

  $argList = @("--server", $Server, "--token", $Token, "--data-dir", $installRoot)
  if ($HttpOnly) { $argList += "--http-only" }

  Write-Step "Starting the agent (copies itself to LocalAppData and registers at logon)"
  Write-Host ("Command: " + $exe + " " + ($argList -join " "))
  & $exe @argList
  Write-Host ("Agent first-launch exit code: " + $LASTEXITCODE)

  Start-Sleep -Seconds 2
  $running = Get-Process -Name "fleet-agent" -ErrorAction SilentlyContinue
  $installed = Join-Path $installRoot "fleet-agent.exe"
  $startupCmd = Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs\Startup\FleetConsole.cmd"
  $runnerCmd = Join-Path $installRoot "run-agent.cmd"
  $taskOk = $false
  # Query via cmd.exe so a missing task never becomes a terminating PowerShell error.
  # (schtasks prints "The system cannot find the file specified" when the task is absent.)
  cmd.exe /c 'schtasks /query /tn "Fleet Console Agent" 1>nul 2>nul'
  if ($LASTEXITCODE -eq 0) { $taskOk = $true }

  Write-Host ""
  if ($running -or (Test-Path $installed)) {
    Write-Host "SUCCESS - the agent is installed." -ForegroundColor Green
    if ($running) {
      Write-Host "fleet-agent.exe is running now."
    } else {
      Write-Host "The exe is in place. If you do not see it in Task Manager, start:"
      Write-Host ("  " + $installed)
    }
  } else {
    Write-Host "The agent exe was not found in LocalAppData." -ForegroundColor Yellow
  }
  Write-Host ("  install folder : " + $installRoot)
  if (Test-Path $installed) { Write-Host ("  installed exe  : " + $installed) }
  Write-Host ("  server         : " + $Server)
  if ($taskOk) {
    Write-Host "  logon task     : Fleet Console Agent"
  } elseif (Test-Path $startupCmd) {
    Write-Host ("  logon startup  : " + $startupCmd)
  } elseif (Test-Path $runnerCmd) {
    Write-Host ("  runner         : " + $runnerCmd)
  } else {
    Write-Host "  logon          : not confirmed (you can still start the exe by hand)"
  }
  Write-Host "It should start again the next time this user signs in."
}
catch {
  $exitCode = 1
  Write-Host ""
  Write-Host "INSTALL FAILED" -ForegroundColor Red
  Write-Host $_.Exception.Message
  if ($_.ScriptStackTrace) { Write-Host $_.ScriptStackTrace }
  try { Add-Content -Path $logTemp -Value ("FAILED: " + $_.Exception.Message) } catch {}
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
