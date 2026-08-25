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

echo ">> starting Fleet Console on :43123"
exec npm run dev
