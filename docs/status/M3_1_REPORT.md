# M3.1 implementation report — authenticated encrypted Shell

Date: 2026-08-18

M3.1 establishes the internal encrypted Shell session boundary without
unlocking public `connect` or `exec`. The proof uses the platform OpenSSH
client, a local RouteCandidate, a temporary local `sshd`, and synthetic keys;
it does not use a private deployment.

## Outcome

- A target Device signs a bounded, versioned list of SSH host public keys for
  one exact Fabric/Device/Shell Capability tuple with its protected Ed25519
  Device identity.
- Verification derives the immutable Device ID from the catalog public key,
  validates the exact tuple and signature, and renders only the signed keys to
  an isolated OpenSSH known-hosts file.
- The OpenSSH executor starts `ssh` directly without a shell, ignores external
  SSH configuration, requires strict host-key checking, and disables host-key
  learning, proxy commands, connection sharing, forwarding, and interactive
  password prompts.
- The daemon-owned subject identity, canonical target, authorization expiry,
  route, executor, binding version/signature, and host-key fingerprints are
  pinned into one Session. Reconnect creates a fresh Session and re-verifies.
- While an executor runs, the coordinator periodically re-resolves the pinned
  target for its trusted subject. Revoked/missing authorization, unavailable
  targets, invalid identity bindings, or binding changes cancel the Session.
- SQLite stores Device public identities and signed Capability bindings in the
  same transaction as a snapshot. A durable high-water mark rejects binding
  rollback and same-version key changes across restart, recovery, and snapshot
  pruning.
- Internal Session faults have stable safe stages and do not copy OpenSSH
  process errors or caller cancellation details into durable diagnostics.

## Security decisions

The transport and encryption protocol remain SSH. PF Remote supplies the
missing trusted association between an immutable PF target and the SSH server
host key; it does not implement a new Shell, tunnel, cipher, or OS
authorization layer. The Device identity private key is not reused as an SSH
host key.

Route address, port, username, credential source, and relay metadata remain
outside TargetReference. A route that reaches the wrong SSH server fails strict
host-key verification before the remote command is accepted. Host-key rotation
uses a higher Device-signed binding version; a list of at most four keys permits
an explicit overlap window.

The public `connect`, `exec`, and `open` actions remain `NOT_IMPLEMENTED` until
M3.4 defines their protected local streaming and interaction contract.

## Acceptance evidence

### Binding, state, and session tests

- Domain-separated canonical signing, tuple tampering, public-key-derived
  Device identity, invalid encodings, key limits, duplicate rejection,
  canonical sorting, fingerprints, and known-hosts injection resistance.
- Atomic binding round trip, corrupt binding fail-closed recovery, binding
  version rollback after restart, same-version different-key rejection,
  high-water survival after more than five snapshots are pruned, and a
  v2-to-v3 database migration/reopen proof.
- Trusted subject propagation, canonical-target consistency, unavailable and
  non-Shell denial, expired authorization before start and during execution,
  observed and polled revocation cancellation, reconnect Session freshness,
  immutable pin copies, structured target-auth faults, and cancellation-cause
  redaction.

### Real OpenSSH vertical proof

`TestRealOpenSSHEndToEndAuthenticatesPinnedHostKeyAndEncryptsCommand` creates
ephemeral Device, SSH host, and SSH user keys, starts a loopback-only temporary
Windows `sshd`, and executes a fixed command through the real system OpenSSH
client and a byte-capturing TCP relay.

The test proves:

- the Device-signed correct host key completes and returns the sentinel output;
- the captured relay bytes do not contain the Shell command sentinel;
- a different signed host key cannot reach the Shell;
- the relay parses clear SSH packets through `SSH_MSG_NEWKEYS`, mutates the
  first encrypted server packet, and observes transport failure with no command
  output;
- all temporary keys, server process state, and known-hosts files are bounded
  to the test and cleaned up.

The security test passed five consecutive uncached runs. On hosts without the
Windows OpenSSH client, `ssh-keygen`, or `sshd`, the test explicitly skips;
therefore the Windows security job must record an actual execution, not merely
a green suite containing a skip.

### Repository checks and reviews

- `go vet ./...` passed.
- `go test -count=5 -run TestRealOpenSSHEndToEnd ./internal/executor/openssh`
  passed all five executions.
- `scripts/check.ps1` passed private-data hygiene, execution governance, all Go
  tests, Go vet/build, daemon/CLI smoke, 9 Windows Center tests, and the WinUI
  build with zero warnings and zero errors.
- Independent reviews `PFREMOTE-M3-REVIEW-20260818-02`,
  `PFREMOTE-M3-REREVIEW-20260818-03`, and
  `PFREMOTE-M3-POLLER-REVIEW-20260818-04` drove subject binding, rollback,
  expiry, deterministic post-NEWKEYS tampering, safe faults, and live
  revocation closure. The final two reviews found no P0-P2.

The race detector was unavailable because this Windows Go environment has CGO
disabled. No compiler, global toolchain, production dependency, or host
configuration was added to bypass that limitation.

## Git evidence

- `e75b091` — Shell session, binding, state, threat, and ADR contracts.
- `93f63e7` — Device-signed binding, route/session/OpenSSH executor, persistent
  rollback protection, and negative/integration tests.
- `c8124b4` — local protected-endpoint integration test isolation, so a running
  development daemon cannot collide with repository checks.

## Remaining boundary

M3.1 is an internal local vertical slice. M3.2 adds the Gateway-primary FRP
route adapter; M3.3 adds independent Tailscale/LAN candidates and diagnostics;
M3.4 exposes identity-bound public `connect` and `exec`. Non-Windows platforms
currently have unit-level contract coverage but not this real `sshd` vertical
proof.

No Computer Use, GUI automation, remote target, private credential, private
mapping, or private production deployment was accessed or modified.
