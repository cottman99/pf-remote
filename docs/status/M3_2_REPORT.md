# M3.2 implementation report — Gateway-primary FRP route

Date: 2026-08-29

M3.2 attaches a Gateway-authorized FRP route provider to the internal M3.1
Shell Session without enabling public `connect` or `exec` and without accessing
any private deployment.

## Outcome

- `pfremote.route-lease/v1` binds a single-use Device-signed request to the
  subject Device and already resolved canonical target.
- The Gateway rechecks authorization, bounds the lease by the Grant deadline,
  rejects replay, limits active leases, and owns broker cleanup on failure,
  release, and expiry.
- The FRP adapter starts `frpc` directly from a protected session-owned STCP
  visitor config, enables TLS and visitor encryption, binds only loopback, and
  deletes the config and process on every exit.
- The Session coordinator authenticates the target before route acquisition,
  pins the acquired candidate, never silently falls back, and releases it after
  success, failure, authorization cancellation, or caller cancellation.
- The route remains mutable internal state. FRP, relay endpoints, ports, lease
  secrets, and broker identifiers do not enter TargetReference or SSH trust.

## Security and failure evidence

- Gateway requests and releases require exact Ed25519 signatures; remote HTTP
  control-plane URLs are rejected while HTTPS and loopback development HTTP are
  accepted.
- Tests cover wrong target binding before acquisition, invalid/mismatched and
  expired leases, request replay, expired broker cleanup, partial broker-open
  failure, visitor start/readiness failure, cancellation, idempotent cleanup,
  and diagnostic redaction.
- `TestRealOpenSSHEndToEndAuthenticatesPinnedHostKeyAndEncryptsCommand` now
  obtains an `frp` candidate through the provider boundary, launches real
  OpenSSH through a synthetic STCP-style byte relay, confirms the command is
  absent from captured relay bytes, rejects the wrong host key before command
  execution, and releases the lease. The test passed five consecutive uncached
  runs.
- The real adapter process boundary is implemented, while tests substitute a
  deterministic local visitor process. No FRP binary, production relay, system
  network setting, or private configuration is installed or changed.

## Repository checks

- `go test ./...` and `go vet ./...` passed.
- `go test -count=5 -run TestRealOpenSSHEndToEndAuthenticatesPinnedHostKeyAndEncryptsCommand ./internal/executor/openssh`
  passed all five runs.
- `scripts/check.ps1` passed private-data hygiene, execution governance, all Go
  tests, Go vet and builds, daemon/CLI smoke, all 9 Windows Center tests, and the
  WinUI build with zero warnings and zero errors.

## Git evidence and remaining boundary

- `b63595b` — route-lease contract and manager, Gateway endpoints, signed
  client, owned FRP visitor adapter, Session integration, and negative/vertical
  tests.

M3.3 adds independent Tailscale/LAN candidates and route diagnostics. M3.4
exposes public identity-bound Shell actions. A production target-side Broker
distribution implementation and packaged `frpc` binary remain deployment work;
the interface requires target-side cleanup no later than lease expiry.

No remote target, private credential, private mapping, private production
deployment, system proxy, DNS, default route, VPN/TUN, Clash, or Tailscale state
was accessed or modified.
