# Client updates

The console's Update action fetches the selected repository and branch into a dedicated bare Git cache. Every fetch must succeed before its commit can be used; the agent reads release files directly from that commit rather than a mutable working checkout. Operator checkouts and local edits are not reset.

If Git is unavailable or cannot fetch, public GitHub repositories can fall back to the GitHub commit API. The requested branch is resolved with a cache-bypassing request, and all three release files are downloaded from the returned commit SHA. Custom repositories and branches are respected; unsupported or inaccessible repositories fail explicitly instead of using the default repository.

The release must contain `dist/fleet-agent.exe`, `dist/install.ps1`, and `dist/install.cmd`. The updater checks that the binary is a complete Windows executable for the current architecture and records its SHA256. The staged job message identifies the commit. Release publishers must rebuild and commit `dist/fleet-agent.exe` when changing agent source; fetching the latest commit cannot turn an outdated checked-in executable into a new build.

For installed boot agents, a separate **Fleet Console Update** task runs as SYSTEM after the old resident agent exits. It invokes the installer from the fetched commit using the existing pairing, skips redundant Git setup, and refreshes the executable, supervisor, startup task, and user tray. It verifies the installed executable against the staged checksum. Installer failure restores the previous agent executable and attempts to restart monitoring. The update never selects ResetPairing.

The new agent sends a final receipt identifying the installed commit and checksum, or the apply failure. Delivery retries until the manager acknowledges it. `update.log`, `install.log`, and `supervisor.log` in the agent's data directory contain local diagnostics. The job initially says **Staged commit**; **Installed commit** is the final installation receipt. If installation cannot restart the agent, the client will remain offline and its local logs need inspection.

Existing clients need this updater installed once, either through their existing working Update action or by rerunning the updated installer with the new executable. Updating repository source alone does not change an already running client. Legacy foreground agents update their executable using the restart helper; rerun the current installer to migrate them to boot startup and receive full installer/tray updates.

Validation covers actual Git repositories advancing between updates, wrong-origin/cache reuse, missing branches, invalid executables, pinned GitHub fallback, PowerShell apply and rollback with task operations mocked, pairing preservation, and delivery retries. Task registration/restart as SYSTEM still requires a live installed-client check; the test suite does not stop or replace real scheduled services on the developer machine.
