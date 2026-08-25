# Fleet Console

A lightweight control plane for Windows PCs you own. The web UI shows live status (online, under load, frozen, offline) and runs bulk jobs: install/uninstall/check software, map/unmap drives, clean Downloads, empty the Recycle Bin, and launch a program.

The dashboard is one Node process. Each PC runs a small Go agent that **phones home** (no inbound ports on the machines).

**Public repo:** [github.com/lucas-tafuri/windows-fleet-console](https://github.com/lucas-tafuri/windows-fleet-console)

## Run the console

On the machine that will host the dashboard (any OS with Node 20+):

```bash
git clone https://github.com/lucas-tafuri/windows-fleet-console.git
cd windows-fleet-console
./install.sh
```

That installs npm dependencies and starts the console on [http://127.0.0.1:43123](http://127.0.0.1:43123). Until a real agent connects, three simulated PCs are shown so every action can be tried.

```bash
npm run build
npm start
```

The console must be reachable from your Windows PCs (LAN IP, VPN, or a VPS). Set `FLEET_PUBLIC_URL` to that address so Enroll copies the right command.

Optional env (unset is fine — first run generates a fleet token and stays unlocked):

| Variable | Purpose |
|---|---|
| `PORT` | Bind port (default `43123`) |
| `HOST` | Bind address (default `0.0.0.0`) |
| `FLEET_TOKEN` | Shared secret agents must present |
| `DASHBOARD_PIN` | Optional PIN gate for the web UI |
| `FLEET_PUBLIC_URL` | Public URL printed on the Enroll page |
| `FLEET_TLS` | Set `1` if the console is behind HTTPS |

State lives in `data/fleet.json` (atomic writes, no database).

## Enroll a Windows PC

On each PC, run the install script (Git via winget if missing, agent download, logon registration, start). From the Enroll page copy the one-liner, or:

```powershell
iwr -UseBasicParsing https://raw.githubusercontent.com/lucas-tafuri/windows-fleet-console/main/dist/install.ps1 -OutFile install.ps1
powershell -ExecutionPolicy Bypass -File .\install.ps1 -Server http://YOUR_CONSOLE:43123 -Token YOUR_TOKEN
```

`dist/install.cmd` sits next to `fleet-agent.exe` — double-click it and paste the server URL and token when asked.

The agent is copied to `%LOCALAPPDATA%\FleetConsole` and starts at Windows logon (scheduled task if allowed, otherwise a Startup-folder shortcut). A missing scheduled task is a warning, not a failed install — if the script prints `SUCCESS` and `fleet-agent.exe` is in that folder, the PC is enrolled.

### Update every PC

In the console, select machines and click **Update**. Each agent `git pull`s [this repo](https://github.com/lucas-tafuri/windows-fleet-console) (or clones it / downloads `dist/fleet-agent.exe` if Git is missing), replaces the binary, and restarts.

To rebuild the agent yourself (Go on any OS):

```bash
npm run build:agent
```

## Fallbacks

| Area | Preferred | Then |
|---|---|---|
| Agent transport | WebSocket `/api/agent/ws` | HTTP poll `POST /api/agent/poll` |
| Browser live updates | WebSocket `/api/ui/ws` | REST poll every 2s |
| Software | winget | Uninstall registry / Get-Package (detect). Install/uninstall require winget |
| Drives | `net use` | `WNetAddConnection2` / `WNetCancelConnection2` |
| Recycle Bin | `SHEmptyRecycleBin` | Shell.Application / `Clear-RecycleBin` |
| Launch | `ShellExecute` | `cmd /c start`, PATH and Program Files search |
| Status | CPU + RAM + hung windows | Memory only, or **Limited** if counters fail. Frozen is never inferred from high CPU alone |

## Status rules

- **Offline** — no heartbeat for 20 seconds
- **Frozen** — agent alive and an interactive window is hung
- **Under load** — CPU ≥ 85% or memory ≥ 90%
- **Limited** — connected, but metrics APIs failed
- **Online** — none of the above

## What this is not

Not remote desktop, not a file manager, not a stealth implant. The agent is a named process plus an optional scheduled task, for machines you enroll yourself.
