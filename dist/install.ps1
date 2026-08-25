# Fleet Console — Windows client install
# Installs Git if missing, fetches the agent, registers logon startup, and starts it.
#
#   powershell -ExecutionPolicy Bypass -File install.ps1 -Server http://HOST:43123 -Token TOKEN
#   or double-click install.cmd

param(
  [string]$Server = $env:FLEET_SERVER,
  [string]$Token = $env:FLEET_TOKEN,
  [switch]$HttpOnly,
  [string]$Repo = "https://github.com/lucas-tafuri/windows-fleet-console.git"
)

$ErrorActionPreference = "Stop"
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$here = Split-Path -Parent $MyInvocation.MyCommand.Path
if (-not $here) { $here = Get-Location }
$exeUrl = "https://github.com/lucas-tafuri/windows-fleet-console/raw/main/dist/fleet-agent.exe"
$installRoot = Join-Path $env:LOCALAPPDATA "FleetConsole"
$repoDir = Join-Path $installRoot "repo"

function Write-Step($msg) {
  Write-Host ">> $msg" -ForegroundColor Yellow
}

function Refresh-Path {
  $env:Path = [Environment]::GetEnvironmentVariable("Path", "Machine") + ";" +
    [Environment]::GetEnvironmentVariable("Path", "User")
}

function Ensure-Git {
  if (Get-Command git -ErrorAction SilentlyContinue) { return $true }
  Write-Step "Git not found — installing with winget"
  if (-not (Get-Command winget -ErrorAction SilentlyContinue)) {
    Write-Host "winget is missing. Agent updates will download the exe from GitHub instead."
    return $false
  }
  winget install -e --id Git.Git --accept-package-agreements --accept-source-agreements --silent
  Refresh-Path
  $gitProg = Join-Path $env:ProgramFiles "Git\cmd"
  if (Test-Path $gitProg) { $env:Path = "$gitProg;$env:Path" }
  return [bool](Get-Command git -ErrorAction SilentlyContinue)
}

function Ensure-AgentExe {
  $local = Join-Path $here "fleet-agent.exe"
  if (Test-Path $local) { return $local }
  Write-Step "Downloading fleet-agent.exe"
  New-Item -ItemType Directory -Force -Path $here | Out-Null
  Invoke-WebRequest -UseBasicParsing -Uri $exeUrl -OutFile $local
  return $local
}

if (-not $Server) { $Server = Read-Host "Fleet Console URL (example: http://192.168.1.10:43123)" }
if (-not $Token) { $Token = Read-Host "Fleet token (from the Enroll page)" }
$Server = $Server.Trim().TrimEnd("/")
$Token = $Token.Trim()
if (-not $Server -or -not $Token) {
  throw "Server URL and token are required."
}

New-Item -ItemType Directory -Force -Path $installRoot | Out-Null
$haveGit = Ensure-Git
$exe = Ensure-AgentExe

if ($haveGit) {
  Write-Step "Checking out the fleet repo (needed for Update / git pull)"
  if (Test-Path (Join-Path $repoDir ".git")) {
    git -C $repoDir fetch origin 2>$null
    git -C $repoDir pull --ff-only origin main 2>$null
  } else {
    if (Test-Path $repoDir) { Remove-Item -Recurse -Force $repoDir }
    git clone --depth 1 --branch main $Repo $repoDir
  }
}

$argList = @("--server", $Server, "--token", $Token, "--data-dir", $installRoot)
if ($HttpOnly) { $argList += "--http-only" }

Write-Step "Starting agent (copies itself to $installRoot and registers at Windows logon)"
Start-Process -FilePath $exe -ArgumentList $argList -WorkingDirectory $installRoot

Write-Host ""
Write-Host "Client is running." -ForegroundColor Green
Write-Host "  exe     $exe"
Write-Host "  install $installRoot"
Write-Host "  server  $Server"
Write-Host "It will start again at the next sign-in."
