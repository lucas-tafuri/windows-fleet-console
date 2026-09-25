# PrettyDamnFleet - Windows client install (ASCII only; Windows PowerShell 5.1)
# Logs: %TEMP%\fleet-console-install.log
#       %ProgramData%\FleetConsole\install.log
#
#   powershell -ExecutionPolicy Bypass -File install.ps1
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
$installRoot = Join-Path $env:ProgramData "FleetConsole"
$legacyRoot = Join-Path $env:LOCALAPPDATA "FleetConsole"
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

function Test-FleetUrl {
  param([string]$Url)
  if (-not $Url) { return $false }
  $u = ([string]$Url).Trim().TrimEnd("/")
  try {
    $r = Invoke-WebRequest -UseBasicParsing -Uri ($u + "/api/discover") -TimeoutSec 2
    if ($r.StatusCode -eq 200 -and $r.Content -match '"ok"') { return $true }
  } catch {}
  return $false
}

function Get-LanIPv4 {
  $list = @()
  try {
    $addrs = Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue |
      Where-Object { $_.IPAddress -notmatch '^(127\.|169\.254\.)' }
    foreach ($a in $addrs) { $list += $a }
  } catch {}
  return $list
}

function Find-FleetByUdp {
  $port = 43124
  $payload = [Text.Encoding]::ASCII.GetBytes("FLEETDISC")
  $targets = New-Object System.Collections.Generic.List[string]
  [void]$targets.Add("255.255.255.255")
  foreach ($a in Get-LanIPv4) {
    if ($a.PrefixLength -eq 24) {
      $p = $a.IPAddress.Split(".")
      [void]$targets.Add(($p[0] + "." + $p[1] + "." + $p[2] + ".255"))
    }
  }
  $udp = New-Object System.Net.Sockets.UdpClient
  $udp.EnableBroadcast = $true
  $udp.Client.ReceiveTimeout = 1500
  try {
    $deadline = (Get-Date).AddSeconds(8)
    while ((Get-Date) -lt $deadline) {
      foreach ($ip in $targets) {
        try {
          $ep = New-Object System.Net.IPEndPoint ([Net.IPAddress]::Parse($ip), $port)
          [void]$udp.Send($payload, $payload.Length, $ep)
        } catch {}
      }
      try {
        $from = New-Object System.Net.IPEndPoint ([Net.IPAddress]::Any, 0)
        $bytes = $udp.Receive([ref]$from)
        $text = [Text.Encoding]::ASCII.GetString($bytes).Trim()
        if ($text.StartsWith("FLEETHTTP ")) {
          $url = $text.Substring(10).Trim().TrimEnd("/")
          if (Test-FleetUrl $url) { return $url }
        }
      } catch {}
    }
  } finally {
    $udp.Close()
  }
  return $null
}

function Find-FleetByScan {
  $ips = New-Object System.Collections.Generic.List[string]
  foreach ($a in Get-LanIPv4) {
    [void]$ips.Add($a.IPAddress)
    $p = $a.IPAddress.Split(".")
    if ($p.Count -eq 4 -and $a.PrefixLength -eq 24) {
      $base = $p[0] + "." + $p[1] + "." + $p[2]
      foreach ($last in @(1, 2, 10, 20, 50, 80, 81, 100, 200, 254)) {
        [void]$ips.Add($base + "." + $last)
      }
      try {
        $hostNum = [int]$p[3]
        for ($i = -8; $i -le 8; $i++) {
          $n = $hostNum + $i
          if ($n -ge 1 -and $n -le 254) { [void]$ips.Add($base + "." + $n) }
        }
      } catch {}
    }
  }
  try {
    $gw = (Get-NetRoute -DestinationPrefix "0.0.0.0/0" -ErrorAction SilentlyContinue | Select-Object -First 1).NextHop
    if ($gw) { [void]$ips.Add($gw) }
  } catch {}
  try {
    $arp = arp -a 2>$null
    foreach ($line in $arp) {
      if ($line -match "(\d+\.\d+\.\d+\.\d+)") { [void]$ips.Add($Matches[1]) }
    }
  } catch {}
  $seen = @{}
  foreach ($ip in $ips) {
    if ($seen.ContainsKey($ip)) { continue }
    $seen[$ip] = $true
    $url = "http://" + $ip + ":43123"
    if (Test-FleetUrl $url) { return $url }
  }
  return $null
}

function Find-FleetConsole {
  Write-Host "Searching the LAN for PrettyDamnFleet..."
  $found = Find-FleetByUdp
  if ($found) {
    Write-Host ("Found via broadcast: " + $found)
    return $found
  }
  Write-Host "No UDP reply. Probing nearby hosts on port 43123..."
  $found = Find-FleetByScan
  if ($found) {
    Write-Host ("Found via scan: " + $found)
    return $found
  }
  throw "Could not find PrettyDamnFleet on the LAN. Open the dashboard on the host PC, allow the firewall, or pass -Server http://HOST:43123"
}

function Request-FleetApproval {
  param([string]$Base)
  $osName = "Windows"
  try {
    $osName = [string](Get-CimInstance -ClassName Win32_OperatingSystem -ErrorAction Stop).Caption
  } catch {}
  $payload = @{
    hostname = $env:COMPUTERNAME
    user     = $env:USERNAME
    os       = $osName
  } | ConvertTo-Json -Compress
  Write-Host ("Requesting approval as " + $env:COMPUTERNAME + " (" + $env:USERNAME + ")")
  Write-Host "Approve this PC on the PrettyDamnFleet dashboard."
  $created = Invoke-RestMethod -Method Post -Uri ($Base + "/api/join") -ContentType "application/json" -Body $payload
  if (-not $created.id) { throw "Host did not accept the join request." }
  $deadline = (Get-Date).AddMinutes(10)
  while ((Get-Date) -lt $deadline) {
    Start-Sleep -Seconds 2
    $st = Invoke-RestMethod -Uri ($Base + "/api/join/" + $created.id)
    if ($st.status -eq "approved") {
      if (-not $st.token) { throw "Host approved but sent no token." }
      Write-Host "Approved."
      return $st.token
    }
    if ($st.status -eq "denied") { throw "The host denied this PC." }
    if ($st.status -eq "expired") { throw "Join request expired. Run the installer again." }
  }
  throw "Timed out waiting for the host to Approve this PC."
}

try {
  $admin = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
  if (-not $admin.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw "Run this installer as administrator to enable startup before Windows sign-in."
  }
  New-Item -ItemType Directory -Force -Path $installRoot | Out-Null
  # Preserve existing pairing when upgrading the old per-user installation.
  foreach ($name in @("config.json", "machine-id")) {
    $dest = Join-Path $installRoot $name
    $old = Join-Path $legacyRoot $name
    if (-not (Test-Path $dest) -and (Test-Path $old)) { Copy-Item -LiteralPath $old -Destination $dest }
  }
  $savedPath = Join-Path $installRoot "config.json"
  if (Test-Path $savedPath) {
    $saved = Get-Content -Raw -LiteralPath $savedPath | ConvertFrom-Json
    if (-not $Server) { $Server = $saved.server }
    if (-not $Token -and $Server.TrimEnd('/') -eq ([string]$saved.server).TrimEnd('/')) { $Token = $saved.token }
    if ($saved.httpOnly) { $HttpOnly = $true }
  }
  $stamp = Get-Date -Format o
  Set-Content -Path $logTemp -Encoding ASCII -Value ("=== PrettyDamnFleet install " + $stamp + " ===")
  try { Start-Transcript -Path $logTemp -Append -Force | Out-Null } catch {}

  Write-Host ""
  Write-Host "PrettyDamnFleet client installer" -ForegroundColor Cyan
  Write-Host ("Log file: " + $logTemp)
  Write-Host ("Also:     " + $logLocal)
  Write-Host ("Folder:   " + $here)
  Write-Host ""

  if (-not $Server) {
    $Server = Find-FleetConsole
  } else {
    $Server = ([string]$Server).Trim().TrimEnd("/")
    if (-not (Test-FleetUrl $Server)) {
      Write-Host ("Warning: " + $Server + " did not answer /api/discover. Continuing anyway.")
    }
  }
  if (-not $Token) {
    $Token = Request-FleetApproval $Server
  }
  $Server = ([string]$Server).Trim().TrimEnd("/")
  $Token = ([string]$Token).Trim()
  if (-not $Server -or -not $Token) {
    throw "Could not get a console URL and token. Approve the PC on the host dashboard, or pass -Server and -Token."
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

  $existingTask = Get-ScheduledTask -TaskName 'Fleet Console Agent' -ErrorAction SilentlyContinue
  if ($existingTask) { Stop-ScheduledTask -TaskName 'Fleet Console Agent'; Start-Sleep -Seconds 2 }
  # Older installs can remain in another user's profile after migration to boot
  # startup. Remove only Fleet's known launcher and stop only its installed exe.
  $legacyRoots = @($legacyRoot)
  $profiles = Get-CimInstance Win32_UserProfile | Where-Object { -not $_.Special -and $_.LocalPath }
  foreach ($profile in $profiles) {
    $legacyRoots += Join-Path $profile.LocalPath 'AppData\Local\FleetConsole'
    $launcher = Join-Path $profile.LocalPath 'AppData\Roaming\Microsoft\Windows\Start Menu\Programs\Startup\FleetConsole.cmd'
    if ((Test-Path -LiteralPath $launcher) -and ((Get-Content -Raw -LiteralPath $launcher) -match 'fleet-agent\.exe')) {
      Remove-Item -LiteralPath $launcher
    }
  }
  $legacyExecutables = @($legacyRoots | ForEach-Object { Join-Path $_ 'fleet-agent.exe' })
  Get-CimInstance Win32_Process -Filter "Name='fleet-agent.exe'" | Where-Object {
    ($_.ExecutablePath -eq (Join-Path $installRoot 'fleet-agent.exe') -or
    $_.ExecutablePath -in $legacyExecutables) -and $_.CommandLine -notmatch '--session-job\b'
  } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force }
  Set-Content -LiteralPath (Join-Path $installRoot 'boot-installed') -Value '1'
  $argList = @("--install-only","--server", $Server, "--token", $Token, "--data-dir", $installRoot)
  if ($HttpOnly) { $argList += "--http-only" }

  Write-Step "Starting the agent (preserves pairing and registers Windows boot startup)"
  Write-Host "Saving the existing pairing and installing the agent."
  & $exe @argList
  $agentExit = $LASTEXITCODE
  Write-Host ("Agent install exit code: " + $agentExit)

  if ($agentExit -ne 0) { throw "Agent installation failed." }
  $installed = Join-Path $installRoot 'fleet-agent.exe'
  # A persistent supervisor owns restart/update so a successful self-update exit
  # cannot leave the scheduled task stopped until the next boot.
  $runner = Join-Path $installRoot 'run-agent.ps1'
  @'
$ErrorActionPreference = 'Stop'
$exe = Join-Path $PSScriptRoot 'fleet-agent.exe'
while ($true) {
  try {
    if (Test-Path -LiteralPath ($exe + '.new')) {
      Move-Item -LiteralPath ($exe + '.new') -Destination $exe -Force
    }
    Start-Process -FilePath $exe -ArgumentList ('--background --data-dir "' + $PSScriptRoot + '"') -WindowStyle Hidden -Wait
  } catch {
    Add-Content -LiteralPath (Join-Path $PSScriptRoot 'supervisor.log') -Value $_
  }
  Start-Sleep -Seconds 3
}
'@ | Set-Content -LiteralPath $runner -Encoding UTF8
  $action = New-ScheduledTaskAction -Execute "$env:SystemRoot\System32\WindowsPowerShell\v1.0\powershell.exe" -Argument ('-NoProfile -NonInteractive -ExecutionPolicy Bypass -File "' + $runner + '"')
  $trigger = New-ScheduledTaskTrigger -AtStartup
  $principal = New-ScheduledTaskPrincipal -UserId 'SYSTEM' -LogonType ServiceAccount -RunLevel Highest
  $settings = New-ScheduledTaskSettingsSet -StartWhenAvailable -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -ExecutionTimeLimit ([TimeSpan]::Zero) -RestartCount 999 -RestartInterval (New-TimeSpan -Minutes 1) -MultipleInstances IgnoreNew
  Register-ScheduledTask -TaskName 'Fleet Console Agent' -Action $action -Trigger $trigger -Principal $principal -Settings $settings -Force | Out-Null
  $oldStartup = Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs\Startup\FleetConsole.cmd'
  if (Test-Path $oldStartup) { Remove-Item -LiteralPath $oldStartup }
  Start-ScheduledTask -TaskName 'Fleet Console Agent'
  Start-Sleep -Seconds 2
  $running = @(Get-CimInstance Win32_Process -Filter "Name='fleet-agent.exe'" | Where-Object { $_.CommandLine -notmatch '--session-job\b' })
  if ($running.Count -gt 1) { throw "Multiple resident agents are still running. Close older manually launched copies and rerun this installer." }
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
    throw "The agent exe was not found in ProgramData."
  }
  Write-Host ("  install folder : " + $installRoot)
  if (Test-Path $installed) { Write-Host ("  installed exe  : " + $installed) }
  Write-Host ("  server         : " + $Server)
  if ($taskOk) {
    Write-Host "  boot task      : Fleet Console Agent"
  } elseif (Test-Path $startupCmd) {
    Write-Host ("  logon startup  : " + $startupCmd)
  } elseif (Test-Path $runnerCmd) {
    Write-Host ("  runner         : " + $runnerCmd)
  } else {
    Write-Host "  logon          : not confirmed (you can still start the exe by hand)"
  }
  Write-Host "It will start at Windows boot, before sign-in, and reconnect automatically."
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
