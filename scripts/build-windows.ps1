# Build Windows tray binaries (agent + console host).
$ErrorActionPreference = "Stop"
$root = Resolve-Path (Join-Path $PSScriptRoot "..")
$dist = Join-Path $root "dist"
New-Item -ItemType Directory -Force -Path $dist | Out-Null

$env:CGO_ENABLED = "0"
$env:GOOS = "windows"
$env:GOARCH = "amd64"
$ldflags = "-s -w -H windowsgui"

Push-Location (Join-Path $root "agent")
go build -trimpath -ldflags $ldflags -o (Join-Path $dist "fleet-agent.exe") .
if ($LASTEXITCODE -ne 0) { throw "agent build failed" }
Pop-Location
Write-Host "wrote dist\fleet-agent.exe"

Push-Location (Join-Path $root "host")
go build -trimpath -ldflags $ldflags -o (Join-Path $dist "fleet-console.exe") .
if ($LASTEXITCODE -ne 0) { throw "host build failed" }
Pop-Location
Write-Host "wrote dist\fleet-console.exe"
