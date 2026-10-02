# Recovering damaged fleet data

An HTTP 500 with a JSON parsing error can mean `data/fleet.json` is damaged. Leading zero bytes are not valid JSON. Do not delete the file or start a new fleet to hide the error: this loses the saved fleet token, approvals, and history.

On the server, run the inspection tool from the installed repository:

```powershell
node .\scripts\repair-fleet-store.mjs .\data
```

It reports file validity, sizes, zero-byte counts, and machine/job counts without printing credentials. Inspection changes nothing. If `FLEET_DATA_DIR` is configured, pass that actual directory instead of `.\data`.

If a valid recovery copy exists, stop the console host and its dashboard process first. Then name that copy explicitly, for example:

```powershell
node .\scripts\repair-fleet-store.mjs .\data --restore fleet.json.bak
```

The tool refuses invalid recovery files or replacement of a valid primary. It preserves the damaged primary under a unique `fleet.json.damaged-*` filename, flushes the restored copy to disk, and replaces the primary. Restart the host afterward. The recovered state contains only what existed when that copy was saved. Retain the damaged original and recovery files until the fleet has been checked.

Older versions did not create a backup. A leftover `fleet.json.tmp` or a file from a separate server backup may be usable; the inspection tool can check these after they are copied into the data folder under a `fleet*` filename. If no intact copy exists, the overwritten bytes cannot be reconstructed by this tool. Keep the original for further recovery rather than resetting automatically.

Future saves use unique temporary files, flush their contents before replacement, and retain the previous complete state as `fleet.json.bak`. These safeguards reduce the risk from interrupted writes, but do not replace external backups. They cannot retroactively create a backup of already damaged data. Rebuild and restart the updated host to use them.
