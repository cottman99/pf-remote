# M2 implementation report — 2026-08-18

This report records repository evidence for the identity, catalog, and Grant
milestone. A roadmap item is marked complete only after its success, failure,
and recovery behavior is exercised without a private deployment.

## M2.1 Device identity and protected storage

### Outcome

- Device identities use Go's standard Ed25519 implementation.
- The public Device ID is deterministic from the public key; private seed
  material is not exposed through the package API.
- Windows storage uses current-user DPAPI with UI forbidden and without
  machine-wide scope.
- Unix-like storage uses a mode-0700 directory and mode-0600 regular file.
- First creation publishes a fully written same-directory staging file without
  replacing a concurrently created identity.
- Corrupt, mismatched, unknown-version, symlinked, or weakly protected state
  fails closed instead of silently generating a new Device identity.

### Evidence

- Identity unit tests cover stable reload, signing and verification, corrupt
  state, public/private mismatch, immutable accessors, invalid inputs, and
  concurrent creators.
- The Windows DPAPI test proves that protected output does not expose the
  synthetic plaintext and round-trips under the current user.
- The Unix test package cross-compiles for `linux/amd64`; its platform test
  rejects widened file permissions.
- A real `pfremoted` process was started with an isolated synthetic config
  root. It remained alive and created a `pfremote.device-identity/v1` record
  using `windows-dpapi-current-user`, with the expected Device ID shape and
  nonempty public and protected key fields. The process was stopped after the
  bounded smoke test.
- `scripts/check.ps1` passed after the implementation with zero Windows Center
  build warnings or errors.

### Toolchain limitation

The concurrent-creator test passes normally, but Go's race build was not
available on this host because `CGO_ENABLED` is disabled and no C compiler is
installed. No global compiler or system configuration was added for this task.

## M2.2 Owner initialization, activation, revocation, and version sync

### Outcome

- One self-signed Owner Device initializes the Fabric exactly once.
- New Devices prove possession of their protected Ed25519 identity before a
  short-lived device and user code are issued.
- Owner approvals and revocations and Device version sync requests are signed
  with domain-separated canonical messages and single-use request IDs.
- Device codes have 256 random bits. Human user codes have 40 random bits,
  expire after ten minutes, and receive approval-attempt and polling limits.
- Successful polling consumes the codes and activates only the proved public
  identity. Revocation immediately blocks version sync and increments both
  directory and Grant versions.
- Remote plaintext enrollment requests are rejected; loopback development and
  TLS requests remain available.

### Evidence

- Enrollment tests exercise initialization, complete activation, pending and
  early polling, slow-down recovery, code expiry, user-code rate limiting,
  request replay, entropy failure, unsupported versions, identity mismatch,
  Owner self-revocation protection, version sync, and revoked-device denial.
- A full Gateway HTTP test covers every M2.2 endpoint with real Ed25519
  signatures. Additional HTTP tests cover external plaintext rejection,
  unknown JSON fields, standard error correlation IDs, and version errors.
- Enrollment business-logic statement coverage is 82.7 percent; Gateway
  statement coverage is 84.6 percent.
- M2.2 initially used in-memory control-plane state. M2.3 now persists the same
  signed wire behavior and replay controls transactionally.

## M2.3 SQLite catalog, Grant, and control state

### Outcome

- Catalog data is stored in strict, normalized Device, Capability, and Grant
  tables linked to an immutable snapshot record.
- A transaction inserts staging rows, marks the snapshot committed, and moves
  the singleton active pointer last. The newest five committed snapshots are
  retained for last-valid recovery.
- Startup runs quick and foreign-key integrity checks and validates every loaded
  object and reference. It recovers a prior valid committed snapshot when the
  active logical snapshot is invalid, and fails closed when none remains.
- Catalog listing and resolution expose only Capabilities with an active Grant
  for the current Device; unauthorized Capabilities cannot be enumerated by
  canonical ID or alias.
- Enrollment state persists public keys, code hashes, expiry/polling metadata,
  rate state, monotonic versions, revocation, and replay IDs. It contains no
  private keys or raw activation codes; replay history has explicit per-role
  and per-Device bounds.
- The direct pure-Go SQLite dependency and BSD-3-Clause license were reviewed;
  the pinned dependency graph does not require CGO.

### Evidence

- State tests cover commit/reopen round trips, failed-transaction rollback,
  corrupt-active fallback, five-snapshot retention, reference validation,
  startup pragmas and integrity checks, and control-state replacement.
- Enrollment restart tests carry an approval through restart, preserve signed
  replay rejection and revocation through later restarts, prove raw codes are
  absent from the serialized payload, reject corrupt state, and verify that a
  save failure makes the live manager unhealthy.
- Catalog tests prove active-Grant filtering and non-enumerability of an
  unauthorized canonical target.
- A real daemon using an isolated synthetic configuration root returned the
  same three authorized targets before and after process restart and created
  both protected identity and SQLite state artifacts. The bounded processes
  were stopped after the smoke test.
- `scripts/check.ps1` passed including private-data hygiene, all Go tests and
  vet checks, three executable builds, CLI smoke, and the WinUI Center build
  with zero warnings or errors. A `linux/amd64` cross-build also passed.

## M2.4 Seven-day cached authorization and expiry UI

### Outcome

- Cached authority ends at the earlier of snapshot capture plus seven days and
  an optional Grant expiry. The exact boundary is expired; there is no grace
  period and restart does not extend it.
- Expired targets disappear from listing and fail both alias and canonical
  resolution, which also prevents new context-envelope generation.
- A missing or revoked subject Device is denied even when a stale active Grant
  row still exists in the snapshot.
- A SQLite high-water clock survives restart and prevents an already observed
  later time from being rolled back to extend cached authority. Clock-state
  persistence failure expires authority.
- Catalog and target JSON include active/expired state, absolute UTC expiry,
  and nonnegative remaining seconds.
- Windows Center presents a localized active state, a final-24-hour warning,
  an expired error, and per-target effective expiry. Dynamic status uses a UI
  Automation live region; target actions remain semantic buttons.
- Backend exception strings and local paths are no longer echoed into Center
  status text.

### Evidence

- Go tests exercise one second before expiry, the exact boundary, Grant-shorter
  and snapshot-shorter validity, multiple Grants, denial by alias and canonical
  reference, expired `doctor` state, and the persistent clock high-water mark
  across database reopen and clock rollback.
- A real daemon using isolated synthetic state returned three active authorized
  targets and one fixed absolute cache boundary through protected Named Pipe
  calls before and after process restart. Both bounded processes were stopped.
- Nine MSTest/MTP tests cover active, 24-hour warning, server-expired, local
  boundary-expired, localized target expiry, clean list-item naming, and the
  warning/expiry refresh schedule. The
  reviewed MSTest 4.3.2 test dependency is MIT licensed.
- A real unpackaged self-contained Center executable was launched without
  Computer Use. Windows UI Automation verified the localized window and cache
  status, three distinct visible target cards, six authorization elements, and
  three enabled keyboard-focusable target buttons. Essential bounds were
  nonzero, inside the window, and non-overlapping. This is runtime semantic and
  layout evidence, not a claim of pixel-perfect screenshot review.
- The final `scripts/check.ps1` run passed private-data hygiene, all Go tests and
  vet checks, nine Center tests, all three Go executable builds, CLI smoke, and
  the WinUI x64 build with zero warnings or errors. The final `linux/amd64`
  cross-build and whitespace audit also passed.

## Post-review closure — 2026-08-18

An independent M2 review found five cross-layer gaps that package-local tests
did not expose. They are closed as follows:

- `pfremote` is now a protected-local-API client. Center's development adapter
  still launches the CLI, but both processes consume `pfremoted` state and fail
  closed when the daemon is unavailable; neither constructs synthetic authority.
- `pfremoted` reloads the latest valid snapshot for each action. Persistent
  Gateway active/revoked transitions atomically update enrollment state,
  monotonic control versions, and the shared revocation overlay.
- Catalog authorization rejects a revoked target Device as well as a missing or
  revoked subject Device, including alias and canonical resolution.
- Center schedules daemon refreshes at the 24-hour warning and expiry boundaries,
  clamps long waits to six hours, and retries expired or unavailable state.
- Device activation permits one pending request per Device and 256 globally.
  Expired activations are pruned; recent unregistered replay buckets are capped
  at 4,096 and age out after 24 hours without a pending activation.

### Closure evidence

- A real-state integration test completes signed Owner initialization and Device
  activation, lists one target through the daemon provider, revokes that Device
  through the persistent Gateway manager, then proves the next daemon list and
  canonical inspect deny it without moving `captured_at`.
- Local API client/server tests cover versioned results and fail-closed provider
  errors. The repository check now starts an isolated daemon and exercises the
  real CLI-to-daemon path instead of accepting an in-process synthetic smoke.
- Two real CLI processes 1.2 seconds apart returned the identical absolute cache
  boundary `2026-08-25T01:17:56.2204161Z`; the earlier reviewed implementation
  moved the boundary on every invocation.
- The final full `scripts/check.ps1` run passed private-data hygiene, all Go
  tests and vet checks, nine Center tests, the isolated CLI/daemon smoke, all
  executable builds, and WinUI x64 with zero warnings or errors. A fresh
  `linux/amd64` cross-build also passed.
- A rebuilt unpackaged Center and its workspace daemon were launched locally
  without Computer Use. Read-only UI Automation observed the same fixed expiry,
  three target cards, and three enabled, keyboard-focusable, positive-bounds
  copy buttons. The verified instances were left running. No private deployment
  was accessed.
