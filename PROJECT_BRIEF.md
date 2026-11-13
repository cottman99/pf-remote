# PF Remote project brief

## Objective

Choose a named computer, open its exact Desktop, or hand the same authorized
target to an external Agent. Existing SSH/RDP/VNC clients perform the connection.

## Current release

Alpha.118 includes bounded desktop-status probes while retaining alpha.117's
end-to-end SSH Desktop forward and private Gateway routing.
Both available Windows controllers, the Linux node and cloud control run
alpha.118. Gateway paths are verified; one unavailable member still needs its
first migration. See docs/status/GATEWAY_FLEET_MIGRATION.md for current evidence.

## Current phase

Latest priority: daily VPS enrollment using a distinct peer-capable release.
Offline fleet migration remains deferred.


Current milestone: M11
Current roadmap item: M11.1

The owner replaced the old expansion roadmap with a focused closeout.
See docs/plans/ACTIVE_WORK.md and docs/status/CLOSEOUT_REPORT.md.
Historical feature and rollout records are evidence, not current fleet health.

## Retained boundaries

- Windows Center plus existing Windows/Linux controlled nodes.
- One identity and authorization core shared by UI and MCP/Skill.
- Existing independent LAN, Tailscale and Gateway paths; no host network changes.
- Exact Desktop selection is stable; only routes may fall back automatically.
- Private deployment values remain in the protected local runtime outside Git.
- Source distribution is separate from internal installers and private evidence.
- Native executors, protected IPC, target authentication and rollback remain.

## Removed from the active product plan

Automatic wake certification, other native platform frontends, browser remote
desktop and repeated cosmetic-only releases. Recovery and compatibility code
remain because current deployments depend on them; they are not expansion work.

Earlier Windows release alpha.112 added windowed, single-screen, all-screen and
selected-screen presets, remembered per controlling computer and exact target.
Both reachable Windows controllers are updated with configuration, history and
existing viewers preserved. See docs/status/DISPLAY_PRESETS.md.
It retains alpha.111's independent Linux workspace selection,
retaining connection presets, protected saved accounts and quiet repeat resource
sharing; see docs/status/CONNECTION_PRESETS.md and
docs/status/WORKSPACE_SELECTION_FIX.md. Native testing found that the shared
Windows credential cache could override the selected workspace; per-launch
DPAPI envelopes preserve the exact selection. Both reachable Windows controllers
are updated; the offline controller retains a durable notification.
Windows Home signing and battery-powered startup remain supported. Shared RDP choices retain
the corresponding TigerVNC workspace, with playback and explicit microphone
forwarding by default. Hardware acceptance remains pending. Normal multi-workspace
selection lists shared RDP workspaces for picture and sound together. All desktops
retains every original TigerVNC/RDP target. Alpha.105 restores the reviewed TigerVNC Viewer to
the Windows release and makes its binary and license mandatory release gates.
Settings includes Check this
computer, Notify other computers, and named version/last-seen reports. Offline
notification catch-up is verified. Private configuration remains outside releases.
The fourth computer was bootstrapped from alpha.87 to alpha.102 with its private
configuration and history retained, then independently updated to alpha.103.
All four report current. Its ordinary PF Remote Shell route now works; cached
offline routes recover missing node identities when connecting. Owner
UI acceptance remains open. See docs/status/FLEET_UPDATE_REPORT.md. Native-window
health is verified without foreground interaction; user visual acceptance is pending.
