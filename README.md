# PrettyDamnFleet Mesh

A Windows fleet manager that runs on **every PC**. Each node serves the dashboard, stores fleet history, exchanges records with peers, and executes its own jobs. There is no permanent server or voting majority required for routine work.

- A surviving PC can manage itself and reachable peers.
- Disconnected groups work independently and exchange history when reconnected.
- Every PC has a local dashboard; phones and other LAN devices can open any peer.
- Jobs execute only after the target commits acceptance. Replication alone never executes an immediate command.
- Offline delivery is explicit and has an expiration time.
- Interrupted side effects are marked **outcome unknown**, never silently repeated.

Targets up to 50 Windows x64 PCs on one trusted LAN. Browser access is intentionally unlocked HTTP. Peer traffic uses mutually authenticated TLS with individual machine keys. Any person who can reach a dashboard can run commands and approve peers.

## Build

Build prerequisites: Node 24 and Go 1.26 or newer. Installed PCs need neither Node nor Git.

```powershell
npm ci
npm run contracts
npm run typecheck
npm run build:node
```

If Go is outside PATH, set `$env:GO` to its full path. The output is `dist/fleet-node.exe`, including the embedded React dashboard. `scripts/build-mesh.ps1` also runs the Go tests and accepts a release version.

Old host/agent source remains for reuse, regression tests, and migration. Legacy instructions are archived in [docs/legacy-server.md](docs/legacy-server.md). Do not use the old client updater to install a mesh release.

## Install the first PC

Copy `fleet-node.exe`, `install-mesh.ps1`, and `install-mesh.cmd` together. Run in elevated PowerShell:

```powershell
.\install-mesh.ps1 -Mode Create
```

The installer creates the `PrettyDamnFleetMesh` Windows service and a separate user tray task. Open **http://127.0.0.1:43123**. Other LAN devices use that PC's LAN address and port `43123`.

Create is explicit and one-time. Failed discovery never creates another fleet.

## Join other PCs

Open **Enroll** on an existing peer. Copy its installer command, including peer address, peer fingerprint, and fleet fingerprint. Run as administrator on the new PC, compare its fingerprint with the approval prompt, and approve it.

Each PC installs the same executable. Once records replicate, the first PC can shut down without disabling the others. Saved addresses survive restarts; discovery learns changed addresses only after an authenticated probe.

Installer firewall rules allow these ports only on the local subnet, on Private/Domain profiles:

- TCP `43123`: browser dashboard and `/api/v2`.
- TCP `43125`: authenticated peer protocol.
- UDP `43124`: discovery using actual interface subnet masks.

If discovery is blocked, joining through an explicit peer still works. Keep one saved peer address reachable until discovery can refresh changed addresses.

## Run work

The fleet overview opens in a compact list with a remembered card-view option. Open a machine's **Details** for its session, resource usage, mapped drives, synchronization, dashboard link and recent activity. Historical machines are available under **Include history** and cannot be selected for new work.

**Tools** collects read-only system, disk, network, process, service and Windows-error reports from reachable, compatible nodes. Reports run in the service context even before sign-in, have a 45-second timeout, and are bounded below the 64 KiB result limit. Process reports list the 40 largest working sets (CPU is lifetime seconds); service reports list up to 100 running/automatic services; error reports list up to 20 errors from the past day. They do not execute browser-provided scripts or store SSH credentials. Reports support filtering and JSON downloads, and interrupted read-only collection can resume safely.

**Jobs** supports search, machine/status filters, expandable results, and result export. Reports and exports can contain internal machine details and Windows event text; dashboard access remains the same intentionally unlocked LAN access described above.

Select PCs and use software, drive, cleanup, launch, or update actions. Default commands attempt live delivery. Enable **Queue for unavailable PCs** to permit later delivery, with a default 24-hour deadline.

The target journals acceptance, start, and result separately. A lost acknowledgment is resolved using the original execution ID. If a PC crashes during a side effect, inspect it before choosing **Review and retry**; retry creates a new linked execution.

Desktop work still runs in the signed-in user's Windows session. Before sign-in, or with ambiguous remote sessions, these jobs return an explicit session error. Monitoring, synchronization, and existing per-user drive maintenance remain independent.

**Unreachable** means unreachable from this dashboard's node, not proof that a PC is off. Cached observations remain stale. A local record is not a backup: additional acknowledged copies are shown separately from current peer reachability.

## Migrate

Follow the [maintenance cutover procedure](docs/mesh-cutover.md). Import preserves historical inventory, catalog, software observations, and outcomes. It excludes demo PCs and credentials and never replays old queued/running work.

```powershell
.\install-mesh.ps1 -Mode Create -LegacyData 'C:\old-console\data\fleet.json'
```

Other PCs join the new fleet. Saved legacy machine IDs become aliases when available. The installer backs up/disables old startup tasks and launchers before enabling the mesh. Per-user managed-drive credentials remain in their original profiles.

## Backup and recovery

State is in `%ProgramData%\FleetConsole\mesh`. Protect this directory. Stop the service before maintenance CLI commands so one process owns the identity:

```powershell
Stop-Service PrettyDamnFleetMesh
& "$env:ProgramData\FleetConsole\fleet-node.exe" backup --output 'D:\protected-backups\fleet.db'
Start-Service PrettyDamnFleetMesh
```

Backups use SQLite's consistent snapshot mechanism. Never copy a live database without its WAL, synchronize it through a shared folder, or clone an identity onto another PC.

Restore into a new, empty directory:

```powershell
.\fleet-node.exe restore --input 'D:\protected-backups\fleet.db' --data-dir 'C:\FleetRecovery'
```

Restore validates signatures and creates a **new identity**, with no executable inbox restored. Join it through a surviving peer before issuing commands. If every node is lost, keep restored history as an archive and explicitly create a replacement fleet; there is no automatic trust reset or revival of an old execution writer.

## Signed updates

Install a base64 Ed25519 publisher public key using `-ReleasePublicKey`. Without one, normal work remains available and self-update returns a clear error.

See [release publishing and recovery](docs/mesh-updates.md). Signed manifests identify the exact binary, checksum, size, protocol range, and increasing release number. Peers can relay cached releases; targets verify independently. A separate helper replaces the binary, checks service health, and restores the old executable on failure.

Update a small group first, confirm final receipts, then continue. Update your current dashboard's PC last. Staging is never presented as successful installation.

## Verification

```powershell
npm run test:mesh
node --test tests/mesh-installer.test.mjs tests/store.test.mjs tests/store-recovery.test.mjs tests/software.test.mjs
$env:FLEET_SCALE_TEST='1'
Push-Location agent
go test ./mesh -run TestFiftyNodesTenMiBConvergence -v -count=1 -timeout 180s
Pop-Location
```

Tests use isolated databases, actual local TLS connections, and fake executors. They cover partitions, founder loss, single survivors, lost acknowledgments, replay prevention, crash boundaries, expiry, import, corruption, signing, and rollback callbacks. They do not stop real services or execute cleanup jobs.

The 50-node test transfers a 10 MiB backlog after founder loss with a 60-second convergence budget. Real boot-before-sign-in, session selection, LAN firewall behavior, and three-PC cutover require the Windows rehearsal checklist.

See [validation results and remaining release gates](docs/mesh-validation.md) for the checks actually performed and the limits of the local harness.

For frontend development, build/start a node and run `npm run dev`; Vite proxies `/api` to it. `lib/mesh-contracts.ts` is generated from Go. See [protocol and consistency](docs/mesh-protocol.md) for guarantees and limits.
