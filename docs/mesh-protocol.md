# Mesh protocol and consistency

## Identity and trust

Each installation has Ed25519 signing and NaCl box encryption keys. The signing-key hash is the node ID. Private material uses Windows DPAPI; directory ACLs grant SYSTEM, administrators, and the creating account access. There is no reusable shared fleet token.

The founder's key determines the fleet ID. Its signed genesis admits itself. Later admissions are signed by an admitted peer, carrying public keys, name, addresses, and an optional historical alias. A joining PC pins the selected peer fingerprint, proves key possession through TLS, and waits for dashboard approval.

TLS pins self-signed node certificates. Current membership is checked on every request, including reused connections. Removal permanently tombstones an identity. Disconnected peers use last-known membership until communication returns. Historical signatures remain verifiable after removal; removed command origins cannot gain new target acceptance.

This is a trusted-node protocol, not Byzantine consensus. It cannot provide immediate revocation across partitions or defend against an enrolled machine manufacturing malicious admission histories. Unlocked HTTP dashboards intentionally permit LAN users to control the fleet. Approved Host, matching Origin, JSON content type, and an application header protect against ambient requests from unrelated websites, not deliberate LAN clients.

## Replication

SQLite WAL with `synchronous=FULL` commits events and projections together. An event includes fleet ID, origin key, contiguous origin sequence, Lamport counter, causal dependencies, schema version, body, content hash, and signature. Every origin event references its predecessor and admission; receipts reference their commands.

Events exchange in Lamport/origin/sequence order. Gaps, missing dependencies, signature changes, and conflicting sequence reuse reject the whole batch. Per-origin cursors therefore represent contiguous committed history. Bounded pull/push batches resume from those cursors. Large output blobs exchange separately and are hash-verified.

V2 retains event metadata and execution deduplication indefinitely. Output is bounded to 64 KiB per execution and expires 90 days after the receipt, without renewing retention on transfer. There is no distributed tombstone garbage collection. Frequent telemetry is direct and ephemeral; inventory is persisted once per minute.

Catalog edits use Lamport order and stable origin/sequence tie-breaking. Deletion wins permanently for an item ID; re-adding creates a new ID. Losing edits remain historical. Inventory and receipts are writable only by their owner identity. Terminal execution receipts never regress. History visibility and dashboard preferences are local.

## Execution

A submission ID and target derive a fleet-scoped execution ID. The origin stores a keyed digest of the whole request to reject changed retry contents without storing plaintext credentials. Resubmission on another peer is rejected when the job is already known; query the original receipt instead.

Command events are inert history. Only target acceptance inserts local inbox work. Acceptance and its signed receipt commit together. The serial worker records `running` before Windows actions and commits the result before reporting success.

After restart, accepted/unstarted work resumes. A running side effect becomes `unknown`; only read-only checks retry automatically. Update helpers have separate durable requests/receipts and bounded timeouts. Backup restoration never revives the writer identity or execution inbox.

Immediate delivery requires a live authenticated acceptance request. Replication never turns it into deferred execution. Explicitly queued work may arrive through any peer, with expiry checked at acceptance and before execution. Clock jumps or more than 30 seconds of disagreement with a reachable peer hold queued work. Correct Windows time before acknowledging a local clock-jump hold; active peer disagreement cannot be dismissed.

External side effects are not claimed to be exactly-once. A crash can happen between starting the action and recording the result. Unknown outcomes preserve that uncertainty; an explicit retry creates a new linked execution.

## Interfaces

Browser APIs use `/api/v2`:

- `GET /fleet`, `/peers`, `/diagnostics`: local snapshots, stale observations, reachability, and copies.
- `GET /events`: server-sent snapshots; clients fall back to REST polling.
- `POST /jobs`: generated `SubmitRequest`, online by default or queued with absolute expiry.
- `GET /jobs`, `DELETE /jobs`: history and local hiding, never execution-record deletion.
- `GET /receipts/{commandId}`: locally known target-authored receipt.
- `POST /retry`: new submission ID and prior command ID; target-encrypted credentials stay encrypted.
- `GET/POST/DELETE /software`: catalog operations.
- `GET /enroll`, `POST /join/{nodeId}`: enrollment details and approval/denial.
- `POST /remove/{nodeId}`, `POST /alias`: removal and historical identity binding.
- `POST /clock/acknowledge`: corrected local clock acknowledgment.
- `GET /health`: identity, version, and running executable hash.

The independent `/peer/v2` protocol provides hello, pull, push, explicit acceptance, enrollment, identity collision probing, output blobs, and release-cache reads. Hello advertises job kinds. Unknown command kinds can replicate as history but receive an unsupported result on an older executor. Unsupported event schemas fail synchronization explicitly rather than being skipped.

Discovery verifies learned addresses against admitted keys. Two instances proving possession of the same identity are quarantined when detected. A partition cannot reveal an unseen clone; the supported restore process creates a fresh identity to avoid this condition.

## Durability

No quorum is required. Replica counts record durable acknowledgments, not a promise those PCs remain reachable. A target receipt is distinct from the origin's delivery attempt. A single survivor can work but has no off-machine backup for new records.

The intended limit is 50 nodes. Reused TLS connections, per-peer jittered backoff, bounded requests, and independent metrics/execution limit coordination overhead. External package sources and publishers are needed only for uncached downloads, not for the fleet's own control plane.
