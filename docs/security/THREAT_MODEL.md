# Threat model

## Assets

- Owner and Device identities.
- Capability grants and catalog integrity.
- Shell/Desktop session confidentiality and integrity.
- Stable target/alias bindings.
- Recovery material and revocation authority.
- Diagnostic privacy.

## Trust boundaries

- The Owner administrates the Fabric.
- Every Device has its own Ed25519 identity and can be revoked independently.
- The Gateway is trusted for authorization metadata and availability, but not
  for plaintext Shell/Desktop session content.
- Route providers and relays are untrusted for content confidentiality.
- Target operating-system accounts remain authoritative for actions after a
  PF Remote connection is authorized.

## Primary threats and controls

### Leaked package or copied target

Installers and targets contain no bearer secrets. Activation uses short-lived
device codes; long-lived private identity material is generated and protected on
the device.

### Stolen or retired device

The Owner revokes that Device identity. Catalog, resolve, route lease, Gateway,
and optional web-emergency access all consume the same revocation state. Cached
authorization is bounded and visibly reports its expiry.

### Activation-code guessing or replay

Device codes contain 256 random bits. Human user codes contain 40 random bits,
expire after ten minutes, and invalid approval attempts are rate-limited.
Gateway state uses code hashes rather than raw codes. Signed single-use request
IDs bind Device activation, Owner approval/revocation, and version sync to the
expected identity. Those hashes, version counters, rate state, and replay IDs
are transactionally persisted across restart. A persistence failure stops later
enrollment mutations until restart. Remote plaintext control-plane calls are
rejected.

Only one activation may be pending per Device and no more than 256 may be
pending globally. Expired activations are pruned. Recent unregistered Device
replay history is capped at 4,096 buckets and expires after 24 hours when no
activation remains, preventing unauthenticated key generation from growing the
durable control payload without bound.

### Partial or corrupt local state update

Catalog refreshes insert normalized Device, Capability, and Grant rows and move
the active-snapshot pointer in one SQLite transaction. A failed write leaves the
previous active snapshot in place. Startup integrity and reference validation
skip a corrupt logical candidate and recover the newest retained valid snapshot;
if none remains, startup fails closed.

### Offline cache extension or clock rollback

The cache boundary is fixed at snapshot capture plus seven days and is shortened
by an earlier Grant expiry. Restart and failed refresh do not renew it. PF Remote
persists the greatest locally observed time and uses that value after a backward
clock change, so previously observed expiry cannot be undone by restart. Failure
to persist the time observation expires authority rather than granting a grace
period.

### Malicious or compromised relay

Shell/Desktop content uses end-to-end encryption and authenticates the target.
The relay can observe limited connection metadata and disrupt availability, but
must not receive session keys or remote passwords.

For Shell, the target Device signs a Capability-scoped list of SSH host public
keys with its independently enrolled Device identity. The controller verifies
that signature and immutable target tuple before giving the keys to OpenSSH as
isolated strict known-hosts input. Trust-on-first-use, route-learned keys, and
`accept-new` do not satisfy target authentication.
Binding versions have a durable per-target high-water mark; snapshot recovery
fails closed instead of rolling back to a rotated-out signed key.

Gateway FRP access requires a single-use, Device-signed route-lease request for
the exact canonical target. A lease is bounded by the Grant deadline, and the
broker must remove target-side state no later than lease expiry. The controller
uses an STCP visitor bound only to loopback, protects and deletes its temporary
configuration, owns the child process, and releases the lease on every exit.
FRP endpoint metadata and lease secrets never become target identity or SSH
trust; strict Device-signed host-key verification still rejects a wrong relay
destination before Shell content is sent.

### Confused target

Canonical references bind immutable Fabric, Device, and Capability IDs. Aliases
resolve to those IDs; agents verify the resolved identity before mutation.

### Diagnostic leakage

Events are structured and redact credentials, tokens, keys, clipboard/session
content, private file paths where unnecessary, and raw environment dumps.
Diagnostic exports use an explicit allowlist. The bounded recent-activity list
uses the same content-free metadata boundary and is cleared by recovery restore.

### Local privilege crossing

The daemon API uses Named Pipe or Unix Domain Socket permissions. It does not
trust a caller merely because it originates from loopback.

### Global network damage

PF Remote never changes the system proxy, DNS, default route, VPN/TUN state, or
another product's configuration. Route adapters make process-scoped outbound
connections and perform read-only diagnostics.

### Legacy external-application injection

The private migration adapter never treats a legacy command string as a shell
command. A compatibility Desktop is available only when its exact canonical
target maps to one absolute regular executable already installed on the
controller, with no arguments, URI, environment override, working-directory
override, or unknown definition field. The path is checked again immediately
before direct process creation and is excluded from serialization. Any unsafe
or changed definition remains visibly setup-required while sibling actions
continue to load.

## Secret handling rules

- No real secret or private deployment identifier enters this repository.
- A local compatibility adapter may consume existing private route and account
  material to preserve working connections. It keeps that material inside the
  protected local runtime boundary and excludes it from UI, Agent context,
  events, diagnostics, reports, fixtures, and Gateway metadata.
- Device private keys are non-exportable by normal UI flows and OS-protected.
- Windows Device seeds use current-user DPAPI without machine-wide scope.
  Unix-like Device identity files require a mode-0700 directory and mode-0600
  file. Corrupt or weakly protected identity state fails closed and is never
  silently regenerated.
- Owner recovery exports are explicitly requested, encrypted, and integrity
  protected with a fresh salt and nonce; they do not centralize Device private
  keys. Restore rejects wrong-Fabric, rollback, replay, unknown-version, and
  damaged bundles before the live state transaction commits.
- Passwords for protocol executors are never stored by the Gateway.
- Device identity private keys are never reused as SSH host private keys.
- Connection invitations are signed, expiring setup artifacts, not bearer
  credentials. Their Gateway endpoint is private local setup data and is never
  emitted to UI, Agent context, status output, logs, reports, or fixtures.
- Logs must store a fingerprint or opaque ID instead of secret material.

## Security gates before public Alpha

- End-to-end encryption and target authentication are independently tested.
- Device revocation is exercised across every adapter.
- Recovery, upgrade rollback, and cached-authorization expiry are tested.
- Secret scanning, dependency/license inventory, and SBOM pass.
- Installers are signed and hashes are published.
- A private deployment can migrate beside the legacy path without weakening it.
