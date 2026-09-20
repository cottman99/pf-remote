# ADR 0007: Device-signed SSH Capability binding

- Status: accepted
- Date: 2026-08-18

## Decision

Use the platform OpenSSH client as the Shell executor. A target Device signs a
versioned list of SSH host public keys for one immutable Fabric/Device/Shell
Capability tuple with its existing protected Ed25519 Device identity. The
authenticated catalog snapshot carries the Device public identity and signed
binding.

Before launching OpenSSH, the controller re-resolves authorization, proves that
the public identity derives the expected Device ID, verifies the binding, and
creates an isolated strict known-hosts file. The OpenSSH host-key alias derives
from the canonical target rather than the route address. Routes and Gateway
transport only the resulting SSH byte stream.

The first M3 slice uses an internal Session coordinator and local
RouteCandidate. Public `connect`/`exec`, Gateway/FRP, Tailscale/LAN, credential
UX, and interactive local streaming remain later M3 items.

## Consequences

- PF Remote does not create or maintain a competing Shell or cryptographic
  protocol.
- SSH encryption alone is not considered target authentication; the Device
  signature establishes the missing PF target-to-host-key association.
- The Device identity key and SSH host keys remain separate. The protected
  Device private key signs metadata and is never installed into `sshd`.
- Host-key rotation requires a newer Device-signed binding. A bounded overlap
  list can authorize old and new keys during rollout.
- Route changes cannot silently change endpoint identity. Reconnect starts a
  new Session and repeats all checks.
- User authentication and OS permissions remain authoritative at the target.
  Gateway never stores executor passwords.
- Synthetic targets without a cryptographically derived Device identity and
  signed binding remain inspectable but cannot start an M3.1 Session.

## Rejected alternatives

- Trust-on-first-use or `StrictHostKeyChecking=accept-new`: it permits the route
  first reached by a controller to define target identity.
- Store only a fingerprint learned with `ssh-keyscan`: key scan is not an
  authenticated distribution channel and a fingerprint alone cannot populate
  strict OpenSSH known-hosts input.
- Reuse the Device private key as the SSH host key: it would export or reshape
  a protected control identity for a different protocol boundary.
- Implement SSH in Go or define a PF-specific encrypted Shell tunnel: this
  violates the v1 executor boundary and adds unnecessary production
  cryptographic code.
- Put address, port, username, or relay data into TargetReference: those are
  mutable route/executor configuration, not stable identity.

## References

- [RFC 4251, SSH host keys](https://www.rfc-editor.org/rfc/rfc4251.html#section-4.1)
- [RFC 4253, SSH transport layer](https://www.rfc-editor.org/rfc/rfc4253.html)
- [OpenSSH `HostKeyAlias` and strict host-key configuration](https://man.openbsd.org/ssh_config.5)
- [OpenSSH client manual](https://man.openbsd.org/ssh.1)
