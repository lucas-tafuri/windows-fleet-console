# Client action permissions

Install the client with `dist\install.ps1` from an administrator PowerShell window. The installer registers **Fleet Console Agent** as a LocalSystem startup task. Background user-session dispatch requires that identity; launching `--background` under an ordinary or administrator account is insufficient.

The agent enables and verifies the SYSTEM task's existing `SeTcbPrivilege`, `SeAssignPrimaryTokenPrivilege`, and `SeIncreaseQuotaPrivilege` before querying or starting a user session. It does not grant these privileges to other accounts.

- Checks, drive mappings, cleanup, and application launches use the signed-in user's normal token and profile.
- Install and uninstall use that user's elevated administrator token when available. A standard user without a linked administrator token receives an explicit error; sign in with an administrator account and retry. Account membership and UAC settings are not changed.
- All workers explicitly select `winsta0\default` in the signed-in user's session. Console jobs use `CREATE_NO_WINDOW` so drive and maintenance work does not open a console window. Application launch requests can open their requested program normally.
- The executable and input request are read-only to the ordinary user. Results are written into a separate writable output directory.

Worker and request permissions are applied explicitly after the files are written or copied. A launch-stage access-denied error includes the selected user SID and desktop; if it persists with the updated binary, check Windows AppLocker/WDAC or endpoint-protection events for the temporary worker path and the user's desktop access. This error occurs before attempting the share connection, so changing share credentials does not resolve it.

After updating agents, retry failed jobs. Errors identify the failed stage (user token, worker directory, environment, process launch, or result) and include the Windows error. If the identity check fails, rerun the installer as administrator to repair startup. If application-control software blocks the worker, review its policy for `fleet-session-worker.exe`; the agent does not disable that policy.

These changes require the updated client binary. They cannot repair task configuration on an unreachable client. One unambiguous signed-in user is still required; the console session takes precedence over Remote Desktop sessions.

The installer also registers **Fleet Console Tray** for interactive user logon. It runs `fleet-tray.exe --tray-only` with limited rights and a separate mutex per Windows session. This companion displays the icon and opens the manager; it does not monitor, execute jobs, or write pairing. Hiding the icon leaves the SYSTEM agent running. Rerun the updated client installer to register the tray task on existing installations.
