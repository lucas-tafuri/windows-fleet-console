#Requires -RunAsAdministrator
# PrettyDamnFleet v2: one installer for every Windows x64 node.
[CmdletBinding()]
param(
  [ValidateSet('Create','Join','Update')][string]$Mode,
  [string]$Peer,
  [string]$Fingerprint,
  [string]$Fleet,
  [string]$Source = (Join-Path $PSScriptRoot 'fleet-node.exe'),
  [string]$InstallRoot = (Join-Path $env:ProgramData 'FleetConsole'),
  [string]$LegacyRoot = (Join-Path $env:ProgramData 'FleetConsole'),
  [string]$LegacyData,
  [string]$ReleasePublicKey,
  [string]$Advertise,
  [int]$UiPort = 43123,
  [int]$PeerPort = 43125,
  [int]$DiscoveryPort = 43124
)
$ErrorActionPreference = 'Stop'
$serviceName = 'PrettyDamnFleetMesh'
$root = [IO.Path]::GetFullPath($InstallRoot)
$legacyRootPath = [IO.Path]::GetFullPath($LegacyRoot).TrimEnd('\')
$nodeData = Join-Path $root 'mesh'
$binary = Join-Path $root 'fleet-node.exe'
$sourceFile = (Resolve-Path -LiteralPath $Source).Path
if ($root -eq [IO.Path]::GetPathRoot($root)) { throw 'InstallRoot must be a dedicated application directory' }
foreach ($port in @($UiPort, $PeerPort, $DiscoveryPort)) { if ($port -lt 1 -or $port -gt 65535) { throw 'Invalid port' } }
if (@($UiPort, $PeerPort, $DiscoveryPort | Select-Object -Unique).Count -ne 3) { throw 'Choose distinct ports' }
if (!$Mode) {
  Write-Host '1. Create a new fleet (first PC only)'
  Write-Host '2. Join an existing fleet'
  Write-Host '3. Update this mesh installation'
  switch (Read-Host 'Choose 1, 2 or 3') { '1' {$Mode='Create'} '2' {$Mode='Join'} '3' {$Mode='Update'} default { throw 'No installation mode chosen' } }
}
if ($Mode -eq 'Join') {
  if (!$Peer) { $Peer = Read-Host 'Peer HTTPS address from its Enroll page' }
  if (!$Fingerprint) { $Fingerprint = Read-Host 'Peer fingerprint from its Enroll page' }
  if (!$Fleet) { $Fleet = Read-Host 'Fleet fingerprint from its Enroll page' }
  if ($Fingerprint -notmatch '^[a-f0-9]{64}$' -or $Fleet -notmatch '^[a-f0-9]{64}$') { throw 'Exact fingerprints are required; discovery never grants trust' }
}
if ($Mode -eq 'Create' -and (Test-Path -LiteralPath (Join-Path $nodeData 'mesh.db'))) { throw 'Fleet data already exists. Choose Update, or inspect/recover it explicitly.' }
if ($Mode -eq 'Update' -and !(Test-Path -LiteralPath (Join-Path $nodeData 'mesh.db'))) { throw 'No mesh installation found' }
# Check the local release before stopping any existing installation.
$stream = [IO.File]::OpenRead($sourceFile)
try { if ($stream.ReadByte() -ne 77 -or $stream.ReadByte() -ne 90 -or $stream.Length -lt 1024) { throw 'Source is not a Windows executable' } } finally { $stream.Dispose() }
# A pipeline makes Windows PowerShell wait for the windowless executable and
# populate LASTEXITCODE before the installer checks its result.
& $sourceFile --mesh version | Out-Host
if ($LASTEXITCODE -ne 0) { throw 'The source executable did not pass its version check' }
if ($ReleasePublicKey) { if ([Convert]::FromBase64String($ReleasePublicKey.Trim()).Length -ne 32) { throw 'Publisher key must be a base64 Ed25519 public key' } }
if ($LegacyData) { $LegacyData = (Resolve-Path -LiteralPath $LegacyData).Path; $null = Get-Content -Raw -LiteralPath $LegacyData | ConvertFrom-Json }

New-Item -ItemType Directory -Force -Path $root, $nodeData | Out-Null
& icacls.exe $root '/inheritance:r' '/grant:r' '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' '*S-1-5-32-545:(OI)(CI)RX' | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Could not protect installation directory' }
& icacls.exe $nodeData '/inheritance:r' '/grant:r' '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Could not protect private mesh data' }
$backupDir = Join-Path $nodeData ('cutover-' + (Get-Date -Format 'yyyyMMdd-HHmmss'))
New-Item -ItemType Directory -Path $backupDir | Out-Null
$wasRunning = $false
$existingService = Get-Service -Name $serviceName -ErrorAction SilentlyContinue
if ($existingService) { $wasRunning = $existingService.Status -eq 'Running'; Stop-Service -Name $serviceName -ErrorAction Stop; $existingService.WaitForStatus('Stopped', [TimeSpan]::FromSeconds(45)) }
$oldTasks = @()
foreach ($taskName in @('Fleet Console Agent','Fleet Console Host','Fleet Console Tray','Fleet Console Update')) {
  $task = Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
  if ($task) {
    $oldTasks += [pscustomobject]@{ Name=$taskName; Enabled=$task.Settings.Enabled }
    Export-ScheduledTask -TaskName $taskName | Set-Content -LiteralPath (Join-Path $backupDir ($taskName + '.xml')) -Encoding Unicode
    Disable-ScheduledTask -TaskName $taskName | Out-Null
    Stop-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
  }
}
if ($LegacyData) { Copy-Item -LiteralPath $LegacyData -Destination (Join-Path $backupDir 'fleet.json') }
$legacyConfig = Join-Path $root 'config.json'
if (Test-Path -LiteralPath $legacyConfig) { Copy-Item -LiteralPath $legacyConfig -Destination (Join-Path $backupDir 'config.json') }
if (![String]::Equals($legacyRootPath, $root.TrimEnd('\'), [StringComparison]::OrdinalIgnoreCase)) {
  $oldConfigPath = Join-Path $legacyRootPath 'config.json'
  if (Test-Path -LiteralPath $oldConfigPath) {
    Copy-Item -LiteralPath $oldConfigPath -Destination (Join-Path $backupDir 'legacy-config.json')
    $oldConfig = Get-Content -Raw -LiteralPath $oldConfigPath | ConvertFrom-Json
    if ($oldConfig.machineId) {
      # Carry only the historical alias into the new installation, never its token.
      @{ machineId=[string]$oldConfig.machineId } | ConvertTo-Json | Set-Content -LiteralPath $legacyConfig -Encoding ASCII
    }
  }
}
if (Test-Path -LiteralPath $binary) { Copy-Item -LiteralPath $binary -Destination (Join-Path $backupDir 'fleet-node.exe') }
# Stop only legacy binaries belonging to this installation, not unrelated tools.
Get-CimInstance Win32_Process | Where-Object {
  $_.Name -in @('fleet-agent.exe','fleet-tray.exe') -and $_.ExecutablePath -and (
    ([IO.Path]::GetFullPath($_.ExecutablePath)).StartsWith($root.TrimEnd('\') + '\', [StringComparison]::OrdinalIgnoreCase) -or
    ([IO.Path]::GetFullPath($_.ExecutablePath)).StartsWith($legacyRootPath + '\', [StringComparison]::OrdinalIgnoreCase)
  )
} | ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
$startupBackups = @()
foreach ($profile in Get-CimInstance Win32_UserProfile | Where-Object { !$_.Special -and $_.LocalPath }) {
  $startup = Join-Path $profile.LocalPath 'AppData\Roaming\Microsoft\Windows\Start Menu\Programs\Startup'
  foreach ($name in @('FleetConsole.cmd','FleetConsoleHost.cmd')) {
    $launcher = Join-Path $startup $name
    if (Test-Path -LiteralPath $launcher) {
      $saved = Join-Path $backupDir ([Guid]::NewGuid().ToString('N') + '-' + $name)
      Copy-Item -LiteralPath $launcher -Destination $saved
      $startupBackups += [pscustomobject]@{ Original=$launcher; Backup=$saved }
      Remove-Item -LiteralPath $launcher
    }
  }
}
@{ tasks=$oldTasks; launchers=$startupBackups; legacyData=$LegacyData } | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $backupDir 'cutover.json') -Encoding UTF8
try {
  # The tray shares the service executable and would otherwise lock replacement.
  Stop-ScheduledTask -TaskName 'PrettyDamnFleet Mesh Tray' -ErrorAction SilentlyContinue
  Get-CimInstance Win32_Process | Where-Object {
    $_.ExecutablePath -and [String]::Equals($_.ExecutablePath, $binary, [StringComparison]::OrdinalIgnoreCase) -and
    $_.CommandLine -match '(?:^|\s)tray(?:\s|$)'
  } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction Stop }
  if (![String]::Equals($sourceFile, $binary, [StringComparison]::OrdinalIgnoreCase)) { Copy-Item -LiteralPath $sourceFile -Destination $binary -Force }
  $common = @('--data-dir', $nodeData, '--ui-port', "$UiPort", '--peer-port', "$PeerPort", '--discovery-port', "$DiscoveryPort")
  if ($Advertise) { $common += @('--advertise', $Advertise) }
  if ($ReleasePublicKey) { ConvertTo-Json -InputObject @($ReleasePublicKey.Trim()) -Compress | Set-Content -LiteralPath (Join-Path $nodeData 'release-keys.json') -Encoding ASCII }
  if ($Mode -eq 'Create') { & $binary create @common | Out-Host; if ($LASTEXITCODE -ne 0) { throw 'Fleet creation failed' } }
  if ($Mode -eq 'Join') { & $binary join @common --peer $Peer --fingerprint $Fingerprint --fleet $Fleet | Out-Host; if ($LASTEXITCODE -ne 0) { throw 'Enrollment failed; saved identity was retained for retry' } }
  if ($LegacyData) { & $binary import --data-dir $nodeData --input $LegacyData | Out-Host; if ($LASTEXITCODE -ne 0) { throw 'Legacy import failed; source is unchanged' } }
  $binPath = '"' + $binary + '" run --service --data-dir "' + $nodeData + '" --ui-port ' + $UiPort + ' --peer-port ' + $PeerPort + ' --discovery-port ' + $DiscoveryPort
  if ($Advertise) { if ($Advertise -notmatch '^[a-zA-Z0-9.:-]+$') { throw 'Invalid advertised address' }; $binPath += ' --advertise ' + $Advertise }
  if ($existingService) { & sc.exe config $serviceName binPath= $binPath start= auto | Out-Null } else { New-Service -Name $serviceName -BinaryPathName $binPath -DisplayName 'PrettyDamnFleet Mesh' -StartupType Automatic -Description 'Local fleet dashboard, peer replication and Windows job executor' | Out-Null }
  if ($existingService -and $LASTEXITCODE -ne 0) { throw 'Could not configure mesh service' }
  & sc.exe failure $serviceName reset= 86400 actions= restart/5000/restart/15000/restart/30000 | Out-Null
  if ($LASTEXITCODE -ne 0) { throw 'Could not configure service recovery' }
  foreach ($rule in @(@('PrettyDamnFleet Mesh Dashboard','TCP',$UiPort),@('PrettyDamnFleet Mesh Peers','TCP',$PeerPort),@('PrettyDamnFleet Mesh Discovery','UDP',$DiscoveryPort))) {
    Get-NetFirewallRule -DisplayName $rule[0] -ErrorAction SilentlyContinue | Remove-NetFirewallRule
    New-NetFirewallRule -DisplayName $rule[0] -Direction Inbound -Action Allow -Program $binary -Protocol $rule[1] -LocalPort $rule[2] -Profile Private,Domain -RemoteAddress LocalSubnet | Out-Null
  }
  $trayAction = New-ScheduledTaskAction -Execute $binary -Argument ('tray --ui-port ' + $UiPort)
  $trayPrincipal = New-ScheduledTaskPrincipal -GroupId 'S-1-5-32-545' -RunLevel Limited
  # Explorer may not have created the notification area when the logon trigger fires.
  $trayTrigger = New-ScheduledTaskTrigger -AtLogOn
  $trayTrigger.Delay = 'PT30S'
  $traySettings = New-ScheduledTaskSettingsSet -ExecutionTimeLimit ([TimeSpan]::Zero) -MultipleInstances Parallel -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1) -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries
  Register-ScheduledTask -TaskName 'PrettyDamnFleet Mesh Tray' -Action $trayAction -Trigger $trayTrigger -Principal $trayPrincipal -Settings $traySettings -Force | Out-Null
  Set-Content -LiteralPath (Join-Path $nodeData 'service-installed') -Value $serviceName -Encoding ASCII
  Start-Service -Name $serviceName
  $healthy = $false
  for ($attempt=0; $attempt -lt 30; $attempt++) {
    try { $health=Invoke-RestMethod -Uri ('http://127.0.0.1:'+$UiPort+'/api/v2/health') -TimeoutSec 2; if ($health.ok) { $healthy=$true; break } } catch {}
    Start-Sleep -Seconds 1
  }
  if (!$healthy) { throw 'Installed node failed its health check' }
  Start-ScheduledTask -TaskName 'PrettyDamnFleet Mesh Tray' -ErrorAction SilentlyContinue
  Write-Host ('Ready: http://127.0.0.1:' + $UiPort)
  Write-Host ('Cutover backup: ' + $backupDir)
  Write-Host 'Verify at least three synchronized nodes (or all nodes in a smaller fleet) before retiring the old server.'
} catch {
  $failure = $_
  Stop-Service -Name $serviceName -ErrorAction SilentlyContinue
  $previous = Join-Path $backupDir 'fleet-node.exe'
  if (Test-Path -LiteralPath $previous) { Copy-Item -LiteralPath $previous -Destination $binary -Force; if ($wasRunning) { Start-Service -Name $serviceName -ErrorAction SilentlyContinue } }
  Start-ScheduledTask -TaskName 'PrettyDamnFleet Mesh Tray' -ErrorAction SilentlyContinue
  Write-Host ('Installation did not finish. Backups and enrollment are retained at ' + $backupDir)
  Write-Host 'Legacy dispatch remains disabled. Reconcile jobs before manually restoring legacy tasks.'
  throw $failure
}
