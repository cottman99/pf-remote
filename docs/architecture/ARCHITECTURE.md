# Architecture

## System boundary

PF Remote separates stable product concepts from changeable transport details.

```text
Center / CLI / MCP
          |
          v
local protected IPC -> pfremoted -> action core
                                  |-> identity and grants
                                  |-> target resolver
                                  |-> route selector
                                  |-> structured events
                                  `-> protocol/route adapters
                                           |
                            Gateway / FRP / Tailscale / LAN
                                           |
                                  SSH / RDP / VNC executor
```

The Gateway authorizes and relays encrypted traffic. It must not terminate the
end-to-end encryption used for Shell or Desktop content.

## Domain model

- `Fabric`: the owner's recoverable trust domain.
- `Owner`: the human administrator of a v1 Fabric.
- `Device`: an independently identified and revocable computer or controller.
- `Capability`: a named Shell or Desktop instance published by a Device.
- `Grant`: permission for a subject Device to use one Capability.
- `TargetReference`: stable identity for one Capability.
- `RouteCandidate`: a temporary way to reach a Capability.
- `Session`: one authorized, route-pinned attempt or connection.
- `Event`: a structured, redacted fact about an action or state transition.

Control-only, controlled-only, and both are onboarding presets that create
Capabilities and Grants. They are not authorization primitives.

## Stable target contract

The canonical URI is:

```text
pfremote://<fabric_id>/devices/<device_id>/capabilities/<capability_id>
```

IDs are immutable. Human aliases such as `compute-node/shell` may change, but old
aliases remain resolvable until explicitly retired. References never contain a
credential, device private key, route lease, or relay secret.

## Components

### `pfremote`

The versioned CLI used by people, scripts, Codex, and other agents. Read commands
produce deterministic JSON. Mutating or remote commands verify the immutable
resolved identity before action.

### `pfremoted`

The local Node daemon. It owns device identity, catalog cache, grant cache,
route selection, events, and adapter lifecycle. It exposes an OS-protected Named
Pipe on Windows or Unix Domain Socket on Unix. It does not expose an
unauthenticated loopback administration port.

For a Shell Capability, the catalog may carry a Device-signed OpenSSH host-key
binding. The daemon verifies that the catalog public identity derives the
immutable Device ID and that the binding covers the exact
Fabric/Device/Capability tuple before it starts a Session. OpenSSH remains the
protocol executor and independently verifies the presented host key.

### Gateway

The self-hosted control plane and encrypted byte relay. It offers a versioned
HTTPS API for owner initialization, activation, catalog, grants, resolution,
heartbeat, revocation, events, route leases, recovery, and version negotiation.

M2 initialization registers one Owner Device public key over loopback or an
operator-authenticated TLS channel. Device activation uses short-lived device
and user codes, but long-lived authority remains the independently revocable
Ed25519 Device identity. Owner approvals and revocations and Device version
sync requests are signed; target references and activation codes never become
long-lived bearer credentials.

### Center

Native user interface. Windows uses C#/WinUI 3 first. It calls the same daemon
actions as the CLI and does not implement a second resolver or routing engine.
Connection-service onboarding uses the signed, expiring format in
[`connection-invitation-v1.md`](../../contracts/connection-invitation-v1.md).
The Center validates and stores that opaque setup artifact through its local
helper; the daemon discovers profile changes in the background. Fabric and
Owner identity must match before Owner synchronization can run.

### MCP

`pfremote-mcp` is a local stdio adapter for Agent hosts. It exposes the same
versioned action core as Center and CLI, so an Agent that receives a copied
target resolves and controls the identical immutable Device and Capability. It
does not open a management port or duplicate authorization, routing, or
executor logic.

## Route selection

The default Smart connect policy considers only routes configured for the exact
authorized target, verifies reachability, and tries them in the stable order
LAN, Tailscale, then self-hosted Gateway/FRP. This favors the shortest ordinary
path while keeping the result predictable; an unreachable candidate is skipped
before the Desktop application starts. The Center exposes a one-time manual
choice beside each connect action. That override pins only the named adapter
for the new Session and never changes another computer or a global network
setting. A started Session stays on one route; reconnection selects again.

The RouteCandidate contains mutable address/adapter details only inside the
daemon. A Session pins one authorized target, authorization deadline, route,
executor, and verified host-key fingerprints. A route or Gateway never defines
target identity and never receives SSH session keys or Shell plaintext.

For the Gateway-primary FRP path, the controlling Device signs a short-lived
route-lease request bound to its Device ID and the already resolved canonical
target. The Gateway rechecks route authorization, prevents request replay, and
asks a target-side broker to make an expiring STCP proxy available. The local
daemon starts a session-owned `frpc` visitor from a protected temporary config
and exposes only a loopback TCP RouteCandidate to the OpenSSH executor. Lease
expiry, cancellation, route failure, or Session completion closes both broker
and visitor resources. The Gateway route API is injectable until M3.4 exposes
public Shell actions; synthetic tests do not require a private deployment.
During side-by-side migration, PF Remote may instead reuse an already-running
legacy FRP visitor after matching both its opaque server identity and secret to
the exact target. Only its loopback endpoint becomes a process-local
RouteCandidate; the legacy configuration and process remain read-only and
untouched.

No adapter may change the system proxy, DNS, default route, VPN configuration,
Clash, Tailscale, or another TUN interface.

## Desktop rendering policy

A Desktop Capability describes the rendered session as well as its protocol.
The adapter must distinguish a virtual desktop (for example an Xvnc-owned X
server) from capture of a physical console. These environments do not share one
desktop-compositor default.

PF Remote does not own the operating-system compositor. On Windows 8 and later,
DWM composition is mandatory, including for Remote Desktop sessions, so a
`disable composition` control would be false product behavior. A virtual
Desktop defaults to `automatic`: the operating system and remote-display
protocol choose supported visual effects and encoding behavior. A target may
publish `reduced` or `full` only when it actually owns those cosmetic settings
and has same-resolution workload evidence. This policy never disables
application GPU rendering, lowers native resolution, or claims to replace the
platform compositor.

Capture of a physical console keeps the operating-system desktop policy unless
the owner changes it. PF Remote must not apply the virtual-desktop optimization
globally to unrelated local sessions.

Desktop performance diagnosis keeps resolution, color depth, and workload
constant while measuring the stages separately: application rendering, window
composition and damage, server capture/encoding, route latency/loss, and client
decoding/presentation. An A/B is required only for a supported, reversible
visual-effects profile; PF Remote does not fabricate an on/off experiment for
a mandatory platform component.

## State and availability

The authoritative Gateway catalog and grants are cached locally as one
authenticated, versioned snapshot. Remote ingestion must authenticate a payload
before it reaches the local commit boundary. The clean-room slice starts from a
local synthetic snapshot only. The default last-known authorization window is
seven days.

A read-only legacy compatibility source is a private local route provider, not
an outward catalog. It may retain the complete connection values required by
existing executors, while the catalog, UI, Agent context, events, and migration
artifacts receive only canonical identities, user-facing names, capabilities,
and safe route availability. Private legacy values never enter the Gateway or
this repository.

Private compatibility may also bind one reviewed installed external
screen-control application to an exact canonical Desktop target. The binding
keeps its executable path and legacy definition process-local, launches no
arguments or URI and uses no shell, and never becomes a general executor or
outward Capability setting. Invalid external definitions degrade only that
visible action to setup-required; they do not hide unrelated valid targets.

Local state uses normalized SQLite Device, Capability, and Grant rows. Updates
are atomic: staging rows and the active pointer are committed in one transaction,
so a failed refresh leaves the last valid snapshot active. Retained committed
snapshots permit bounded last-valid recovery. Enrollment public metadata, code
hashes, versions, and replay IDs are durable in the same protected database;
private identity material remains in its OS-protected identity store. The local
Center's bounded recent-activity list persists only the already-redacted session
ID, immutable target, coarse action/result, and start time. It never stores
command content, output, routes, endpoints, credentials, or Desktop content.

An explicitly requested recovery export serializes only durable public
identity, aliases, capabilities, Grants, revocations, and monotonic versions.
Short-lived enrollment and replay state is removed before the payload is
password-derived and authenticated-encrypted as `pfremote.recovery-bundle/v1`.
Restore fully validates a separate staging database, rejects Fabric mismatch,
rollback, and replay against initialized live state, then replaces all
Gateway-owned control tables in one transaction. Device private keys, routes,
sessions, logs, and deployment configuration are never recovered; the Owner
Device proves its existing identity and fresh routes/sessions are established
after restart.

For the local M2 clean-room Gateway, active/revoked Device transitions also
update a normalized revocation overlay atomically with enrollment state. The
daemon reloads the latest valid snapshot and applies this overlay for every
protected-IPC action. This closes local revocation without renewing the
snapshot capture time; authenticated remote snapshot transport remains a later
control-plane boundary.

Cached authorization is active only before the earlier of the seven-day
snapshot boundary and an optional Grant expiry. Expired targets are neither
listed nor resolved. A persistent time high-water mark prevents a wall-clock
rollback after observation from extending this authority. Center displays the
core's absolute expiry and status; its 24-hour warning is presentation policy,
not an independent authorization decision. Center refreshes from the daemon at
the warning and expiry boundaries and retries expired or unavailable state at a
bounded interval.

## Repository layout

- `cmd/`: executable entry points.
- `internal/`: non-public domain and application implementation.
- `pkg/`: public Go contracts suitable for other PF Remote components.
- `apps/windows/`: native Windows Center.
- `contracts/`: versioned wire and file contracts.
- `docs/`: product, architecture, security, migration, evidence, and decisions.
- `scripts/`: reproducible developer and CI entry points.
- `examples/`: synthetic, documentation-safe fixtures only.
