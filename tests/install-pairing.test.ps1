# Run with Windows PowerShell 5.1; exercises installer branches without installing.
$ErrorActionPreference = 'Stop'
$installer = Join-Path $PSScriptRoot '..\dist\install.ps1'
$tokens = $null
$parseErrors = $null
$ast = [Management.Automation.Language.Parser]::ParseFile($installer, [ref]$tokens, [ref]$parseErrors)
if ($parseErrors.Count) { throw ($parseErrors | Out-String) }
$branches = @($ast.FindAll({ param($node) $node -is [Management.Automation.Language.IfStatementAst] }, $true))
$load = $branches | Where-Object { $_.Extent.Text.StartsWith('if ($ResetPairing)') -and $_.Extent.Text.Contains('$savedPath') } | Select-Object -First 1
$clear = $branches | Where-Object { $_.Extent.Text.StartsWith('if ($ResetPairing)') -and $_.Extent.Text.Contains('$pairingFile') } | Select-Object -First 1
if (-not $load -or -not $clear) { throw 'Missing pairing branches' }
$loadPairing = [scriptblock]::Create($load.Extent.Text)
$clearPairing = [scriptblock]::Create($clear.Extent.Text)
function Write-Step { param($Message) }
function Assert($condition, $message) { if (-not $condition) { throw $message } }
$testRoot = Join-Path ([IO.Path]::GetTempPath()) ('fleet-pairing-test-' + [guid]::NewGuid().ToString())
try {
  $installRoot = Join-Path $testRoot 'installed'
  $legacyRoot = Join-Path $testRoot 'legacy'
  New-Item -ItemType Directory -Path $installRoot, $legacyRoot | Out-Null
  $config = '{"server":"http://old-manager:43123","token":"old-token","httpOnly":true}'
  Set-Content -LiteralPath (Join-Path $legacyRoot 'config.json') -Value $config
  Set-Content -LiteralPath (Join-Path $legacyRoot 'machine-id') -Value 'old-id'
  $ResetPairing = $false
  $Server = ''; $Token = ''; $HttpOnly = $false
  . $loadPairing
  Assert ($Server -eq 'http://old-manager:43123' -and $Token -eq 'old-token' -and $HttpOnly) 'Normal update must preserve pairing'
  $Server = 'http://other-manager:43123'; $Token = ''
  . $loadPairing
  Assert ($Token -eq '') 'Changing managers must not reuse the old token'

  # Reset bypasses even corrupt saved settings, and inherited/explicit values.
  Set-Content -LiteralPath (Join-Path $installRoot 'config.json') -Value 'broken'
  $ResetPairing = $true
  $Server = 'http://inherited-manager:43123'; $Token = 'inherited-token'
  . $loadPairing
  Assert ($Server -eq '' -and $Token -eq '') 'Reset must force discovery and approval'
  Assert ((Get-Content -Raw -LiteralPath (Join-Path $installRoot 'config.json')).Trim() -eq 'broken') 'Do not remove pairing before successful approval'

  foreach ($root in @($installRoot, $legacyRoot)) {
    Set-Content -LiteralPath (Join-Path $root 'managed-drives.dat') -Value 'keep-drive-settings'
  }
  . $clearPairing
  foreach ($root in @($installRoot, $legacyRoot)) {
    Assert (-not (Test-Path -LiteralPath (Join-Path $root 'config.json'))) 'Old token must be removed'
    Assert (-not (Test-Path -LiteralPath (Join-Path $root 'machine-id'))) 'Old identity must be removed'
    Assert (Test-Path -LiteralPath (Join-Path $root 'managed-drives.dat')) 'Drive settings must remain'
  }
  Write-Host 'PASS: preserved pairing, manager change, rediscovery, and scoped reset'
} finally {
  $resolved = [IO.Path]::GetFullPath($testRoot)
  $temp = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\')
  if ([IO.Path]::GetDirectoryName($resolved) -ne $temp -or [IO.Path]::GetFileName($resolved) -notlike 'fleet-pairing-test-*') { throw 'Unsafe test cleanup path' }
  if (Test-Path -LiteralPath $resolved) { Remove-Item -LiteralPath $resolved -Recurse -Force }
}
