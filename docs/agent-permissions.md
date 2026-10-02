# Client action permissions

Install the client with `dist\install.ps1` from an administrator PowerShell window. The installer registers **Fleet Console Agent** as a LocalSystem startup task. Background user-session dispatch requires that identity; launching `--background` under an ordinary or administrator account is insufficient.

The agent enables and verifies the SYSTEM task's existing `SeTcbPrivilege`, `SeAssignPrimaryTokenPrivilege`, and `SeIncreaseQuotaPrivilege` before querying or starting a user session. It does not grant these privileges to other accounts.

- Checks, drive mappings, cleanup, and application launches use the signed-in user's normal token and profile.
- Install and uninstall use that user's elevated administrator token when available. A standard user without a linked administrator token receives an explicit error; sign in with an administrator account and retry. Account membership and UAC settings are not changed.
- Only application launch requests use the visible desktop. Other jobs use a noninteractive desktop in the selected user's session.
- The executable and input request are read-only to the ordinary user. Results are written into a separate writable output directory.

After updating agents, retry failed jobs. Errors identify the failed stage (user token, worker directory, environment, process launch, or result) and include the Windows error. If the identity check fails, rerun the installer as administrator to repair startup. If application-control software blocks the worker, review its policy for `fleet-session-worker.exe`; the agent does not disable that policy.

These changes require the updated client binary. They cannot repair task configuration on an unreachable client. One unambiguous signed-in user is still required; the console session takes precedence over Remote Desktop sessions.
