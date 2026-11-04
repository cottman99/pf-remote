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
