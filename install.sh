#!/usr/bin/env bash
# Fleet Console — install dependencies and start the dashboard.
set -euo pipefail
root="$(cd "$(dirname "$0")" && pwd)"
cd "$root"

if ! command -v node >/dev/null 2>&1; then
  echo "Node.js 20+ is required: https://nodejs.org"
  exit 1
fi

if ! command -v npm >/dev/null 2>&1; then
  echo "npm is required (it ships with Node.js)."
  exit 1
fi

echo ">> npm install"
npm install

if [[ ! -f dist/fleet-agent.exe ]] && command -v go >/dev/null 2>&1; then
  echo ">> building Windows agent"
  bash scripts/build-agent.sh || true
fi
if [[ ! -f dist/fleet-console.exe ]] && command -v go >/dev/null 2>&1; then
  echo ">> building Windows console host"
  bash -lc 'cd host && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w -H windowsgui" -o ../dist/fleet-console.exe .' || true
fi

echo ">> starting Fleet Console on :43123"
exec npm run dev
