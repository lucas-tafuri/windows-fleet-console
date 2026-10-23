# Implementation validation and release gate

Validated on 2026-10-23 in the development workspace. This is an implementation candidate, not a completed production cutover.

## Completed checks

- Go unit and integration tests, including the existing Windows executor tests; `go vet ./...`.
- Real local TLS connections covering founder loss, single-survivor commands, lost acknowledgments, explicit approval, fingerprint validation, and removal.
- Durable journal tests covering deduplication, transactional rejection, restart recovery, unknown side effects, deferred expiry, clock holds, six-node partition reconciliation, corruption, and history-only restore.
- Signed release verification and updater rollback callbacks using temporary files and fake service controls.
- A 50-node local simulation with the founder stopped: 49 survivors converged a 10 MiB inventory backlog in 20.94 seconds, within the 60-second test budget. This measures same-host simulated nodes, not physical LAN throughput or simultaneous full-mesh traffic.
- TypeScript checking, ESLint, Vite production build, and the embedded Windows x64 executable build.
- Four Node tests covering legacy storage/recovery/software and installer syntax/firewall scope.
- Browser smoke check of the embedded executable on loopback: machine monitoring, SSE updates, local-only durability message, action selection, and explicit queue/UTC expiry controls. No Windows action was submitted.

The smoke process used isolated temporary data and was stopped afterward. No installed services, startup tasks, firewall rules, or existing fleet state were changed by validation.

## Required before production release

Complete and record the [three-PC maintenance rehearsal](mesh-cutover.md), including actual boot without sign-in, user/session selection, managed-drive reconnect, firewall reachability, former-host loss, and real SCM update rollback.

Also exercise real disk exhaustion and process termination at journal boundaries, restored disk images/duplicate identities, interrupted installation, and representative 50-PC LAN traffic. The automated write-failure test uses SQLite write rejection; it is not a physical disk-full test. The 20-second unreachable threshold is implemented, but a real-LAN failure-indication timing measurement remains outstanding.

The installer is supplied and parsed, not executed as part of this validation. Signed release keys must be configured for deployment; no production publisher private key is included. Commands must remain paused until migration counts, aliases, replicated history, and rollback checks pass.
