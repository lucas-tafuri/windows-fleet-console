# Signed releases and update recovery

## Publisher keys

Keep the private key outside the repository and off fleet PCs:

```powershell
Push-Location agent
go run ./cmd/release -new-key 'D:\private\fleet-release.key'
Pop-Location
```

The output is the base64 public key. Install it with `install-mesh.ps1 -ReleasePublicKey VALUE`. Protected `mesh/release-keys.json` contains only public keys. Changing trust requires an explicit local administrative action; a dashboard or peer cannot add publisher keys.

## Build and publish

```powershell
.\scripts\build-mesh.ps1 -Version 2.0.1
Push-Location agent
go run ./cmd/release -key 'D:\private\fleet-release.key' -binary ../dist/fleet-node.exe -version 2.0.1 -sequence 1 -url 'https://YOUR-RELEASE-HOST/2.0.1/fleet-node.exe' -out ../dist/release.json
Pop-Location
```

Increase the release sequence each time. Publish the manifest and exact executable at immutable HTTPS URLs. The tool validates Windows x64 format and prints the manifest hash. A local development build is not a signed production release.

## Cache and install

Enter the manifest HTTPS URL in the Update action, or cache it locally with the service stopped:

```powershell
fleet-node.exe cache-release --input 'https://YOUR-RELEASE-HOST/2.0.1/release.json'
```

Use the printed hash to update from a peer cache while offline. Targets search reachable peers, verify the signature, protocol range, size, and checksum, and store a local copy. Cached executables are checked again before serving and applying. Only enrolled peers can read the cache.

Self-update requires the installed service. Foreground development nodes reject it while preserving normal work. The package contains the executable and embedded dashboard, not replacement pairing or fleet databases. Incompatible protocols and non-increasing release numbers are rejected; v2 does not perform destructive database migrations.

## Apply and recover

The node journals the job, stages a protected request, and starts a separate hidden helper. The helper verifies the staged release and configured service destination, stops the service, backs up the old executable, replaces it, and checks the new process's executable hash through the loopback health endpoint.

If replacement/startup/health fails, the failed service must stop before the helper restores the previous binary and restarts it. The final receipt is stored locally and replicated. A helper that does not finish within 15 minutes becomes an unknown outcome, without automatic retry. The job worker remains occupied while the helper runs; monitoring and communication continue until the restart.

Update a small group first, check final receipts, then continue in batches. Update the current dashboard PC last and keep alternative peer URLs available. Downloaded/staged is never treated as installed.

Tests cover signing, cache tampering, untrusted publishers, incompatible protocols, failed health, checksum failure, and rollback callbacks. Actual SCM restart/rollback remains part of the three-PC rehearsal.
