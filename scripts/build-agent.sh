#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root/agent"
mkdir -p "$root/dist"
export CGO_ENABLED=0
export GOOS=windows
export GOARCH=amd64
go build -trimpath -ldflags="-s -w" -o "$root/dist/fleet-agent.exe" .
echo "wrote $root/dist/fleet-agent.exe"
if [[ -f "$root/dist/install.ps1" ]]; then
  echo "client install script: $root/dist/install.ps1 (and install.cmd)"
fi
