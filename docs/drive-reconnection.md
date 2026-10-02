# Automatic drive reconnection

Drive mappings created through the updated agent are remembered per Windows user. The agent checks them at startup and every minute after the previous check finishes, verifies that the drive root is accessible, and reconnects unavailable shares using the saved credentials. Checks run independently of the console connection and resume after Windows restarts once that user signs in. Share outages are retried; conflicting drive letters are left unchanged. Unmapping through the console removes the saved mapping and stops retries.

Settings and credentials are encrypted with Windows DPAPI in `%LOCALAPPDATA%\FleetConsole\managed-drives.dat`. If an initial connection fails, its settings remain saved for automatic retry. Update clients and submit **Map drive** once for existing mappings to enable monitoring. A drive check confirms access to its root, not write permissions or the health of every file.

Mapping and maintenance run under the normal signed-in user token. Before sign-in, or when multiple remote sessions make the user ambiguous, the agent waits and retries. Settings belong to the user who requested the mapping, so another user's login does not receive their credentials. The normal background agent continues reporting to the console while maintenance runs.
