# Gateway route lease contract v1

Gateway-primary Shell routing uses a short-lived route lease. A lease is
authorization for one subject Device to ask one route adapter for a temporary
path to one immutable Capability; it is not a TargetReference and is never used
as SSH trust material.

## Request and response

The controlling Device signs a request containing:

```json
{
  "schema_version": "pfremote.route-lease/v1",
  "subject_device_id": "device-controller",
  "canonical_target": "pfremote://fabric-example/devices/device-target/capabilities/shell-main",
  "request_id": "opaque-single-use-id",
  "client_version": "1.0.0",
  "signature": "base64url-without-padding"
}
```

The signing input is the ASCII domain `pfremote-route-lease/v1`, followed by a
newline and the subject Device ID, canonical target, request ID, and client
version in that order. The Gateway verifies the active Device identity, replay
state, Grant, target availability, and authorization deadline before issuing a
lease.

The successful internal response contains the lease ID, exact canonical target,
adapter kind `frp`, expiry, and the minimum FRP visitor material needed by the
local daemon. Mutable server address, port, proxy name, and lease secret stay
inside the daemon/adapter boundary. They never enter copied targets, events, or
diagnostic exports.

## Lifecycle and failure behavior

- The lease expires no later than the underlying Grant and has a short Gateway
  maximum lifetime.
- The local FRP adapter writes a session-owned protected configuration, starts
  `frpc` directly without a command shell, waits for a loopback-only visitor
  endpoint, and returns that endpoint as a temporary `RouteCandidate`.
- Acquisition failure, expiry, cancellation, or Session completion terminates
  the owned process, removes the configuration, and releases the lease.
- A Session pins the candidate. It never migrates or falls back after start.
- Route and relay success do not authenticate the target. OpenSSH still checks
  the Device-signed Capability host-key binding with strict isolated
  `known_hosts`; a wrong target fails before Shell content is sent.
- Structured errors expose only stable route stages, adapter kind, and opaque
  correlation/lease identifiers. FRP secrets, raw configuration, private paths,
  commands, stdout, and stderr are excluded.

PF Remote uses FRP STCP visitor semantics because the access side binds a local
TCP endpoint and the service is not exposed as a public relay port. FRP remains
an replaceable route adapter; its TLS and STCP controls are defense in depth
around the independently encrypted and authenticated SSH stream.
