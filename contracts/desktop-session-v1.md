# Desktop session contract v1

PF Remote opens an existing operating-system RDP or VNC application. During
private side-by-side migration it may also launch one exact, reviewed installed
screen-control application already used by the legacy setup. It does not
implement a remote-desktop codec, authentication protocol, or replacement
desktop client.

## Capability profile

A Desktop Capability may add this profile to the existing Capability object:

```json
{
  "desktop_profile": {
    "protocol": "rdp",
    "rendering_environment": "virtual",
    "authentication": "windows-sso",
    "visual_effects_policy": "automatic"
  }
}
```

`protocol` is `rdp` or `vnc`. `rendering_environment` is `virtual` or
`physical`; it describes the desktop the user will see, not the controlling
computer. RDP requires `windows-sso`, or `tailscale-device` for a private
migration target whose immutable online Tailscale identity has been verified.
VNC requires either
`x509-route-grant` with a daemon-owned CA file for the exact target, or
`tailscale-device` with an exact online Tailscale node identity. A side-by-side
migration may use `legacy-vnc-password` only when the protected credential is
bound to the immutable target and the selected LAN, Tailscale, or existing
Gateway route is also identity-bound. The profile is additive to the v1
catalog schema. Existing snapshots without authentication metadata remain
readable, but `open` fails closed until the target publishes a valid profile.

`visual_effects_policy` is additive and does not change target identity. Its
allowed values depend on the rendering environment:

- `virtual`: `automatic`, `reduced`, or `full`; an omitted value means
  `automatic` for backward compatibility.
- `physical`: `system`; an omitted value means `system`.

`automatic` delegates supported cosmetic and encoding choices to the operating
system and protocol. `reduced` and `full` describe target-owned, measured
profiles; they do not authorize the controlling client to mutate the remote
machine. A physical Desktop rejects virtual-session overrides. The effective
policy is pinned for the Session and a change ends the Session like any other
authenticated capability-profile change.

## Open sequence

Immediately before `open`, the daemon:

1. reloads current authorization for its protected subject Device;
2. resolves the alias or canonical reference to one immutable Device and
   Desktop Capability;
3. requires an active Grant, available target, and valid Desktop profile;
4. acquires and pins one authorized RouteCandidate;
5. launches the matching existing RDP or VNC client without an intervening
   command shell and without placing a password in arguments or files;
6. keeps the route owned until the desktop application exits, authorization
   expires, or the caller cancels, then cleans up route and temporary resources.

The private `external` compatibility case replaces steps 4 through 6 with one
exact process-local target-to-application lookup and direct process start. It
does not acquire or fabricate a network RouteCandidate because the installed
third-party application owns its own authenticated connection lifecycle.

The caller supplies the target and may optionally choose one eligible adapter
for this connection (`lan`, `tailscale`, `frp`, or the exact reviewed legacy
external executor). Omitting the adapter uses Smart connect. The caller can
never supply an address, port, protocol, credential, rendering environment, or
unregistered route. The daemon resolves the chosen adapter only after the same
immutable-target and authorization checks used by Smart connect.

## Client security boundary

An executor must use the protocol client's encrypted, server-authenticated
mode. It must not automate acceptance of an unknown certificate or host. If a
route endpoint cannot be associated with the authenticated Desktop identity,
the executor fails instead of asking a non-technical user to make a trust
decision. Credentials remain with the operating-system client or its protected
credential store and never enter a Gateway, relay, target reference, or
structured diagnostic.

RDP with `windows-sso` launches the Windows client in Remote Credential Guard
and public modes so credentials are not sent to the remote computer or cached
by PF Remote. RDP with `tailscale-device` is limited to that exact encrypted
route and launches the Windows client without credential arguments; saved
credentials, certificate prompts, and any first-use sign-in remain owned by
the Windows client and its protected credential store. It must not fall back
to LAN. A first connection may show the Windows client's own server-certificate
confirmation; PF Remote must explain this state in user language and must not
automate acceptance of an unknown RDP certificate. VNC
either uses TigerVNC X.509 server authentication with daemon-owned target trust
material, or runs across an end-to-end encrypted Tailscale route after PF Remote
matches the configured immutable node ID to the current online peer. The latter
must not fall back to LAN. Anonymous Internet/LAN VNC, unknown certificates,
and password material in process arguments are not accepted. A synthetic executor test proves launch shape only; a representative
authenticated client launch is required before M4.1 closes.

The private `legacy-vnc-password` profile preserves the existing migration
viewer credential while allowing the same exact capability to use its eligible
LAN, Tailscale, or already-running Gateway visitor. The secret remains in the
protected current-user store and route choice never weakens target binding.

The additive private-compatibility profile uses `protocol: external`,
`rendering_environment: physical`, and
`authentication: legacy-private-executor`. It is accepted only for an exact
canonical target whose process-local legacy mapping contains one absolute,
regular installed executable, an empty argument string, and no URI. PF Remote
starts that executable directly without a shell and never serializes its path
or configuration. Missing, changed, argument-bearing, URI-bearing, or unknown
definitions remain setup-required. This adapter delegates session identity and
content protection to the already-installed third-party product and is not a
general command launcher or a Gateway capability.

## Failure and evidence boundary

Stable stages are `resolve`, `authorize`, `target-auth`, `route`, `executor`,
and `desktop`. Errors may expose the protocol and rendering-environment label,
but not private endpoints, credentials, certificate contents, command lines,
or temporary paths. A started Session never changes target or route. A retry
creates a new Session and repeats authorization and immutable-target checks.
For VNC profiles that use an existing viewer password, PF Remote stores that
credential separately from the target and route. The current-user protected
store is keyed by a digest of the immutable canonical Desktop target. The
executor passes the secret only through the launched viewer's environment and
clears its temporary byte buffer after launch. Missing or unreadable state
returns `DESKTOP_CREDENTIAL_REQUIRED` so the native Center can offer one-time
setup without exposing infrastructure details.

## Native UI target binding

Primary and expanded Desktop actions have distinct automation identities. A menu
resolves the current bound control model when opened, then each menu item pins
that canonical target. Catalog refresh, recent activity and route health must not
substitute another Desktop for the action the user selected. Search filters do
not change the target of an existing recent-activity action.
