# Shell session contract v1

PF Remote uses an existing OpenSSH client as its Shell protocol executor. It
does not implement an SSH transport, cryptographic algorithm, user
authentication method, or remote operating-system authorization layer.

## Capability-scoped SSH identity binding

A Shell Capability may carry one Device-signed binding:

```json
{
  "schema_version": "pfremote.ssh-capability-binding/v1",
  "fabric_id": "fabric-example",
  "device_id": "device-opaque-public-identifier",
  "capability_id": "shell-main",
  "binding_version": 1,
  "host_keys": [
    {
      "algorithm": "ssh-ed25519",
      "public_key": "standard-base64-encoded-ssh-public-key-blob"
    }
  ],
  "signature": "base64url-without-padding"
}
```

The example values are descriptive placeholders, not valid keys or identities.
The corresponding Device catalog entry contains its Ed25519
`identity_public_key`. The Device ID must derive from that public key according
to `device-identity-v1` before the binding signature is considered.

The signing input is domain-separated binary data:

1. ASCII `PFREMOTE-SSH-CAPABILITY-BINDING-V1`;
2. each of schema version, Fabric ID, Device ID, and Capability ID as a
   big-endian uint32 byte length followed by UTF-8 bytes;
3. `binding_version` as a big-endian uint64;
4. host-key count as a big-endian uint32;
5. each host-key algorithm and public-key value with the same length-prefix
   encoding.

Host keys are sorted by algorithm and then public-key value before signing.
Duplicate keys, empty lists, more than four keys, invalid OpenSSH algorithm
names, invalid base64, key blobs larger than 16 KiB, invalid identifiers, a zero
binding version, a version above the signed 64-bit SQLite range, and an invalid
signature fail closed. A key list permits an
explicit Device-signed rotation overlap without accepting an unbound key.
The state store keeps a durable high-water mark for each exact
Fabric/Device/Capability tuple. A lower version is rejected; reusing the same
version with a different signature is rejected; only a higher version may
change the authorized key set. The high-water mark is not pruned with retained
snapshots, so recovery cannot silently re-authorize a rotated-out key.

The binding deliberately excludes route addresses, ports, usernames,
credentials, relay identifiers, and private keys. A route cannot change the
expected SSH identity.

## Verification sequence

Immediately before a new Session, the daemon:

1. obtains the subject Device ID from its protected local Device identity,
   reloads the latest valid catalog/Grant state for that subject, and resolves
   the requested canonical target or alias; the Shell request cannot supply or
   override the subject;
2. requires an available Shell Capability, active subject and target Devices,
   and unexpired authorization;
3. derives the Device ID from the catalog Device public key;
4. verifies the binding signature and exact Fabric/Device/Capability tuple;
5. pins the authorization deadline, one RouteCandidate, one executor kind, and
   the accepted host-key fingerprints into a new Session;
6. gives OpenSSH an isolated known-hosts file containing only the signed keys;
7. requires strict host-key checking before any Shell command or credential is
   sent.

The OpenSSH host-key alias is derived from the immutable canonical target, not
from the selected address. A different route to the same Capability therefore
checks the same signed key, while a route that reaches another SSH server fails
before Shell content is sent.

Private legacy route caches may lack a Tailscale node ID when the peer was offline
at startup. Only a missing ID may be resolved later through the authenticated
Tailscale directory; an existing ID is never replaced on verification failure.
Acquisition still requires online node verification and the same signed SSH keys.
Shell checks direct endpoint reachability before pinning a route; failed probes
may select another configured route before execution, never replay a command.

## Automatic local identity confirmation

A controlled node may confirm its own Shell Capability without asking the user
to copy a fingerprint. The local daemon reads only bounded, regular OpenSSH
public host-key files from the operating system's standard server location,
then signs those public keys with that node's protected Device identity. It
never reads an SSH private key, runs `ssh-keyscan`, learns a key from a selected
route, or signs for another Device. Missing or unsafe public-key sources leave
the Capability unconfirmed.

The signed binding increments directory state without refreshing the cached
authorization capture time. A controller still requires the resulting signed
binding through the authorized catalog synchronization path before it can mark
the remote Shell available.

## Session lifecycle

`Session` is one authorized, route-pinned attempt. Reconnect creates a new
Session and repeats resolution, authorization, binding, and host-key checks.
The Session ends at the earliest of:

- executor exit;
- caller cancellation or deadline;
- the pinned authorization expiry;
- observed subject/target revocation or binding invalidation.

While the executor runs, the coordinator periodically re-resolves the pinned
canonical target for its protected subject identity. A missing/revoked Grant,
unavailable or revoked target, invalid Device signature, or binding
version/signature change cancels the current executor. An event-driven watcher
may replace polling without changing the fail-closed comparison.

An already-started Session never migrates to another route. Route failure ends
the attempt; a caller may request a new Session, which selects and verifies
again.

M3.1 proves this lifecycle through an internal coordinator and a local
RouteCandidate. M3.2 and M3.3 add relay and independent route adapters. M3.4
exposes the lifecycle through protected local `connect` and `exec` actions.

## OpenSSH executor boundary

The executor launches `ssh` directly without an intervening shell. Its
session-owned configuration:

- ignores user and system SSH configuration for route and host-key policy;
- uses `StrictHostKeyChecking=yes`;
- uses only the session known-hosts file and disables global known-hosts input;
- disables host-key learning, DNS host-key lookup, connection sharing, agent
  forwarding, X11 forwarding, and unexpected port forwarding;
- uses batch mode for non-interactive execution;
- accepts credentials only from an explicit local source or the user's
  operating-system SSH agent.

The known-hosts file is created beneath protected local PF Remote state with
owner-only permissions where the platform supports POSIX modes and is removed
after the attempt. Public host keys are not secrets, but their association with
the target is integrity-sensitive.

Gateway and route adapters receive only a binary-transparent SSH byte stream
and bounded connection metadata. They do not receive host private keys, SSH
session keys, OS passwords, remote commands, stdout, or stderr.

## Error and evidence boundary

Stable stages are `resolve`, `authorize`, `target-auth`, `route`, `executor`,
and `session`. Errors may contain an opaque Session/correlation ID, route
adapter kind, and host-key fingerprint. They must not contain credentials,
private keys, full process environments, Shell content, or raw private paths.
The live caller-owned stderr stream may contain remote-program stderr and
ephemeral OpenSSH diagnostics; it is never copied into structured errors or
durable PF Remote evidence and must not be logged by default.

SSH provides the encrypted channel, integrity protection, server host
authentication, and per-connection session keys. PF Remote's responsibility is
the prior trusted association between its immutable target and the SSH host
key. This separation follows the SSH architecture and OpenSSH's strict
host-key controls:

- [RFC 4251 host-key trust models](https://www.rfc-editor.org/rfc/rfc4251.html#section-4.1)
- [RFC 4253 SSH transport security](https://www.rfc-editor.org/rfc/rfc4253.html)
- [OpenSSH `ssh_config` host-key controls](https://man.openbsd.org/ssh_config.5)
- [OpenSSH `ssh` client behavior](https://man.openbsd.org/ssh.1)
