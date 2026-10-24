[CmdletBinding()]
param([string]$Go = 'go', [string]$Version = '2.0.0-dev')
$ErrorActionPreference='Stop'
if ($Version -notmatch '^[a-zA-Z0-9._+-]+$') { throw 'Invalid version' }
$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
  & npm.cmd ci
  if ($LASTEXITCODE -ne 0) { throw 'Dependency installation failed' }
  & npm.cmd run typecheck
  if ($LASTEXITCODE -ne 0) { throw 'Type checking failed' }
  & npm.cmd run build
  if ($LASTEXITCODE -ne 0) { throw 'Dashboard build failed' }
  Push-Location (Join-Path $root 'agent')
  try {
    & $Go run ./cmd/contracts ../lib/mesh-contracts.ts
    if ($LASTEXITCODE -ne 0) { throw 'Contract generation failed' }
    & $Go test ./... -count=1 -timeout 180s
    if ($LASTEXITCODE -ne 0) { throw 'Go tests failed' }
    $env:CGO_ENABLED='0'; $env:GOOS='windows'; $env:GOARCH='amd64'
    # The tray shares this executable with the service; never allocate a console.
    & $Go build -trimpath -ldflags "-s -w -H windowsgui -X main.meshVersion=$Version" -o ../dist/fleet-node.exe .
    if ($LASTEXITCODE -ne 0) { throw 'Node build failed' }
  } finally { Pop-Location }
  Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $root 'dist/fleet-node.exe')
} finally { Pop-Location }
