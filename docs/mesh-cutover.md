# Maintenance cutover and Windows rehearsal

## Preparation

1. Build the mesh release and run automated tests. Keep the old installers, binaries, host build, and original database for rollback.
2. Choose a maintenance window. Stop submitting legacy jobs and wait for running work to report. Record unfinished work; imported pending/running records become historical unknown outcomes.
3. Locate the actual database, including any `FLEET_DATA_DIR` override. Keep backups administrator-protected because the old file may contain passwords.
4. Stop the old host before the final snapshot/import. Never dispatch from both systems to the same PC.

## Three-PC rehearsal

Use three designated or disposable Windows PCs; use read-only software checks for connectivity tests.

1. Create the fleet on A and import the legacy export. Reconcile inventory/catalog counts, omitted demo PCs, redacted credentials, and historical pending jobs.
2. Join B through A and C through B, comparing installer fingerprints before approval. All three must show the same fleet ID.
3. Confirm historical aliases. If a legacy ID was not saved locally, explicitly bind it with `/api/v2/alias` before treating that history as migrated.
4. Submit a read-only check to B from A. Confirm the final target receipt and at least two additional copies of its command/result.
5. Stop A. Open B locally and C from another LAN device. Submit a check from B to C.
6. Stop C. B must still monitor/check itself and show its new records as local-only.
7. Restart A/C; verify B's records synchronize without repeating its check.
8. Isolate B with test firewall rules. Run distinct checks in both groups, reconnect, and verify the combined history.
9. Reboot B before sign-in. Verify monitoring from A and the no-user error for desktop work. After sign-in, test the correct user profile, drive reconnection, and ambiguous RDP-session handling.
10. Queue an offline check with a short expiry and verify it does not execute after expiry. An ordinary immediate command must never run just because an unavailable PC reconnects.
11. Use a test signed release with a failing health check. Verify real SCM rollback, previous-binary restart, and a replicated failed-update receipt.

The local fault harness does not establish real reboot, user-session, firewall, or SCM behavior. Record PC names, versions, timestamps, and outcomes before production cutover.

## Reopen work

The installer backs up existing binaries/pairing, exports and disables legacy tasks, backs up/removes old Startup launchers, and installs the mesh service. Managed-drive files remain in their user profiles. Import on one PC; peers receive its historical records.

Keep legacy dispatch disabled until three nodes have synchronized, or all nodes for a smaller fleet. PCs not yet enrolled remain historical/unreachable. Verify each tray dashboard's fleet identity and recent synchronization before reopening work.

## Rollback

Before reopening commands, stop mesh services and restore the old installation/tasks from the cutover backups. Confirm the intended database before restoring it.

After mesh commands have run, preserve their databases/receipts and reconcile performed actions first. Do not restore an old executable queue. Resolve or remove legacy pending work before enabling old dispatch; unknown outcomes require inspecting the PC.

Installer failures deliberately leave old dispatch disabled and preserve identity/data/backups. Automatic fallback could revive an old queue. Repair the installation or perform a reviewed rollback; resetting pairing is not a generic recovery step.
