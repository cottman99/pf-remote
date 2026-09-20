# ADR 0004: Signed Device enrollment

- Status: accepted
- Date: 2026-08-18

## Decision

Use the protected Ed25519 Device identity for enrollment and control-plane
proofs instead of issuing long-lived bearer credentials. The first local or
TLS-protected Owner initialization registers one Owner Device public key.
Subsequent approvals and revocations require a signature from that identity.

New Devices self-sign their authorization request. The Gateway issues a
high-entropy device code and a separately displayed, rate-limited user code.
After Owner approval, polling activates the public identity and consumes the
short-lived codes. Version sync is signed by the active Device identity and
returns monotonic directory and Grant versions. Revocation is enforced before
version state is returned.

Plaintext remote enrollment is rejected. Loopback plaintext remains available
for the isolated development Gateway; production Gateway deployment requires
TLS and operator-controlled bootstrap.

## Consequences

- A copied target reference or expired activation code is not an authority.
- Device-code polling follows RFC 8628 timing and error concepts without
  adopting OAuth bearer access tokens.
- User-code guessing is bounded by expiry and rate limiting.
- Request IDs prevent replay within current control-plane state; M2.3 makes
  that state durable and bounded.
- Owner Device replacement remains blocked until the recovery contract exists.

## Primary references

- [RFC 8628: OAuth 2.0 Device Authorization Grant](https://www.rfc-editor.org/rfc/rfc8628)
- [RFC 6750: OAuth 2.0 Bearer Token Usage](https://www.rfc-editor.org/rfc/rfc6750)
- [Go `crypto/subtle`](https://pkg.go.dev/crypto/subtle)
- [Go `crypto/ed25519`](https://pkg.go.dev/crypto/ed25519)
