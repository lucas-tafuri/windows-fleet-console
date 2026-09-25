# PrettyDamnFleet

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
| `FLEET_TOKEN` | Initial shared secret for a new installation |
| `DASHBOARD_PIN` | Optional PIN gate for the web UI |
| `FLEET_PUBLIC_URL` | Public URL printed on the Enroll page |
| `FLEET_TLS` | Set `1` if the console is behind HTTPS |

State lives in `data/fleet.json` (atomic writes, no database).

## Enroll a Windows PC

On each PC, run the install script **as administrator** (Git via winget if missing, agent download, boot registration, start). From the Enroll page copy the one-liner, or:

```powershell
iwr -UseBasicParsing https://raw.githubusercontent.com/lucas-tafuri/windows-fleet-console/main/dist/install.ps1 -OutFile install.ps1
powershell -ExecutionPolicy Bypass -File .\install.ps1 -Server http://YOUR_CONSOLE:43123 -Token YOUR_TOKEN
```

`dist/install.cmd` sits next to `fleet-agent.exe` — double-click it and paste the server URL and token when asked.

The agent is installed in `%ProgramData%\FleetConsole`. Its saved server URL, token, and machine identity survive restarts and binary updates. The installer migrates pairing from the old `%LOCALAPPDATA%\FleetConsole` installation when available. Reusing approval only applies to the saved server; explicitly choosing a different server requires its token or approval.

On the **console machine**, run `dist\install-host.ps1` as administrator once. Both installers create Windows startup tasks running in the background before sign-in, with automatic recovery after failures. Clients retry the saved server indefinitely, so startup order does not matter. Keep the server address stable (DNS name or DHCP reservation).

The host keeps `data/fleet.json` in the installed repository; retain this directory when replacing or moving the application. `FLEET_DATA_DIR` can select another persistent location. The existing saved fleet token takes precedence over `FLEET_TOKEN`, which only seeds a new installation. An unreadable or corrupt state file now produces an error instead of silently resetting approvals.

Boot tasks run as Local System so they can start before sign-in. Once someone signs in, software, drive mapping, Downloads cleanup, Recycle Bin, and launch jobs run through a temporary worker using that user's Windows session and profile. The local console session takes precedence; otherwise a single active Remote Desktop session is used. If no user is signed in, or multiple remote users make the target ambiguous, these jobs return a clear error. Pairing and monitoring continue without a signed-in user. Foreground hung-window detection is unavailable in the background session.

Background startup does not display a tray icon; open the dashboard in a browser. Protect the host installation directory from modification by untrusted users because its startup task runs with system privileges. Rerun the host installer after updating the checkout to refresh its installed launcher; it retains fleet data.

### GPU usage

Fleet rows and mobile cards include a GPU meter. It reports the busiest GPU engine across adapters, summing processes sharing that engine and clamping to 0�100%. Unsupported drivers, missing counters, and failed samples display **�**, not a fabricated zero. Sampling runs independently of heartbeats.

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
