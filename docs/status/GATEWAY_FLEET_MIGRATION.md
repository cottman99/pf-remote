# Gateway fleet convergence

The owner requested one current client family, Gateway-backed connections and
removal of obsolete clients while retaining personal setup and active desktops.
Alpha.115 adds private routing configuration shared by Shell and Desktop. A
required Gateway cannot silently fall back to another network. Explicit Desktop
route choices remain available for recovery. Diagnostics describe configured
adapters instead of an obsolete development placeholder.

Alpha.117 completes a native VNC path for a physical desktop that previously
opened an independent third-party application. The existing OpenSSH client
authenticates the same Device and carries VNC through the Gateway end to end.
Both Shell and Desktop authorization are required. The viewer owns forwarding
lifetime, so finishing the API request does not disconnect the desktop.
Regression tests cover invalid bindings, identity mismatch, authorization,
non-Gateway routes, backend failure, request completion and viewer cleanup.
The live pilot verifies password authentication and framebuffer dimensions
without requesting pixels, sending input or changing focus.

Migration retains canonical targets, Device identities, saved accounts, display
presets, shared-workspace definitions and independent input/audio services.
The old catalog and its workspace sidecar move together into protected runtime
storage. A mixed TCP/UDP FRP configuration now selects only TCP visitors for
Shell/Desktop; an unrelated UDP credential cannot disable the TCP routes.

Validation covers legacy defaults, invalid private configuration, required-route
failure without direct fallback, mixed visitor transports, exact visitor identity,
existing Shell host-key authentication, package hashes/signatures, and protected
preset retention across upgrade. Repository checks pass. Public artifacts contain
no deployment configuration. Live rollout and removal continue until every
available controller and controlled node has passed its actual Gateway path.

The cloud control service retains the existing enrollment authority and signed
capability bindings. Public HTTPS fronts an independently verified private TLS
backend. Certificate renewal covers both services and their current paths;
native Shell/Desktop content continues through its own encrypted protocol.
Offline members remain pending, with no claim of remote installation.

## Live result — 2026-11-04

| Component | Installed state | Background evidence |
| --- | --- | --- |
| Two available Windows controllers | Alpha.117 | All 542 installed files match the release; authenticated Shell reaches every online member through the required Gateway |
| Linux controlled node | Alpha.117 | All three client hashes match; startup and independent desktop, input and audio services remain active |
| Three Linux workspaces | Existing sessions retained | Both controllers' saved VNC and RDP credentials pass authentication without entering a desktop |
| Physical Windows Home desktop | Native encrypted VNC | Same-Device OpenSSH route, password and framebuffer metadata verified without pixels or input |
| Cloud control and downloads | Alpha.117 | Public HTTPS, all twelve artifact hashes and complete installation bundles verified |
| One unavailable Windows controller | Pending | No live Gateway route; no installation or removal is claimed |

Obsolete available-host clients and startup definitions are retired. Windows has
one active client version; inactive recovery copies remain in protected private
storage. One locked empty directory has an empty-only deferred cleanup task.
Personal identities, accounts, display presets and catalog references survive.
The active native RDP viewer is unchanged. The stable MCP launcher resolves the
current installation and passes discovery, exact-target inspection and execution.
Already-open Agent hosts may need to reconnect a retired stdio transport; CLI
and Skill access use the same current daemon without restarting the host.

The fixed GitHub release and preview feed each have twelve independently
hash-verified assets. Both platform feeds carry alpha.117, sequence 119.
Every available member independently verifies the feed and reports `current`;
Gateway notification and retained reports are live. The unavailable member's
older `current` report is historical, not evidence that it is online or upgraded.
It needs a one-time client/Gateway migration before normal notice catch-up.
M10.3 remains open until that member has migrated and passed its path.

Full repository checks, protected upgrade/rollback and routing/forward lifecycle
tests pass. No Windows UI source changed in this routing slice. Live verification
uses background protected APIs and authentication-only probes. It does not reboot
an in-use computer, log into its physical Windows desktop, record microphone
audio or retest subjective audio quality. Native VNC carries no audio by itself;
existing RDP/shared-workspace audio remains independent. Cloud FRP authentication
is retained until every member can use a compatible path. Certificate renewal
now targets the deployed paths and both TLS endpoints; its timer is enabled.
