# Software versions

On **Software**, select clients and choose **Check catalog** to refresh every listed title. Each cell shows all matching installation names and installed versions, including side-by-side releases, plus the last successful check time.

Checks read `DisplayName` and `DisplayVersion` from machine-wide and signed-in-user uninstall registries, including both 32-bit and 64-bit locations. Winget and Get-Package provide fallbacks. The catalog's display-name match is retained even when a Winget ID is configured.

Update the console and client agents, then run a new check to populate versions. Existing results from older agents remain readable and show **Version unavailable** until refreshed. If an installation has no version metadata, the console says so; it does not guess from the product name or release year.

Failed checks and failed install/uninstall jobs preserve the last successful inventory. After installing or uninstalling software, run another check to refresh the versions. Background agents perform software checks in the signed-in user's session, so a user must be signed in.
