# Gateway fleet convergence

The owner requested one current client family, Gateway-backed connections and
removal of obsolete clients while retaining personal setup and active desktops.
Alpha.115 adds private routing configuration shared by Shell and Desktop. A
required Gateway cannot silently fall back to another network. Explicit Desktop
route choices remain available for recovery. Diagnostics describe configured
adapters instead of an obsolete development placeholder.

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
capability bindings. Public HTTPS fronts protected loopback control traffic;
native Shell/Desktop content continues through its own encrypted protocol.
Offline members remain pending, with no claim of remote installation.
