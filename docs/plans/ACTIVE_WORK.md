---
schema_version: 1
roadmap_item: M10.3
roadmap_milestone: M10
roadmap_text: Deliver cross-device version visibility, independent offline catch-up and compatibility gates.
status: active
base_commit: 1421d96
---

# Active work — fleet rollout and owner acceptance

## Objective

Finish fleet verification and let the owner evaluate
the completed Settings update controls. The owner authorized managed-node updates,
public sanitized releases and complete fleet closure without intermediate gates.

## Acceptance criteria

Named versions and last-seen states in Settings; any active member can notify;
Gateway-retained notices survive offline recipients and restart; recipients verify
and install independently; private identities, credentials, gateways and records
survive. All intended available hosts converge and the owner can use the controls.

## Current step

2026-10-14 owner prioritizes default playback on Linux desktop connections.
R0: main is dirty with retained alpha.106/UI/audio work; preserve those edits.
Bounded side task: ordinary chooser uses the configured shared RDP workspaces,
retaining exact VNC references in All desktops. Playback is always enabled;
clipboard and full-screen settings become explicit. Complete the missing private
sidecar on available controllers, then release and update the Windows clients.
The owner also authorizes default supported hardware sharing, including microphone,
without repeated resource questions. Native OS privacy and server policy remain.
No new audio protocol, silent exact-target substitution,
host network change or manipulation of existing viewer sessions. R1: reuse the
existing authorized shared-workspace profiles and native executor. Return to
M10.3 owner acceptance after normal connections carry picture and sound together.

The Windows Home controller came online during rollout and independently applied
alpha.107. It lacks the native rdpsign utility. Alpha.108 adds signing through
existing Windows/.NET APIs; differential signatures match the native tool, and
target tampering and duplicate properties are rejected. Local publisher trust is
prepared through the owner-authorized administrative path; later user connections
reuse it. No root CA or global warning-disable policy is added.

Live laptop activation was blocked by the scheduled task's default battery rules.
The queued task was repaired, the previous client restored, and alpha.108 applied
in the interactive session. Alpha.109 makes battery-compatible activation the
default and tests the settings actually passed to task registration. No host power
plan or unrelated task changes. Return point remains M10.3 owner acceptance.

Alpha.102 is published. Both Windows installations and the Linux managed node
independently upgraded from alpha.100 to alpha.102, with installer completion
records. Linux was offline at the management-service layer when notice was saved,
then caught up automatically after restart. Both Windows and Linux successfully
sent notices. Current Windows identity, connection service and update checks pass.

The fourth host was identity-verified through its existing alternate SSH path and
bootstrapped from alpha.87 to alpha.102. All ten private JSON file hashes remained
unchanged, 48 history records were retained and the catalog recovered to 11 targets.
All four fleet reports are current; the fourth successfully sent an update notice.
All four subsequently updated independently to alpha.103. The original PF Remote
Shell path now succeeds through the updated controller, including remote status
queries. The controller retains all eight private baseline hashes and 50 records.
The Settings UI is ready for owner-level experience feedback now.

## Out of scope

Paid signing, new protocol servers, host network changes, Linux desktop UI and
unrelated feature expansion. Do not weaken publisher verification to speed rollout.

## Blocking side task

2026-09-30 owner reprioritized shared xrdp/TigerVNC workspaces and audio.
Local signed preview 0.1.0-alpha.106-audio-preview.3 is installed and active;
alpha.105 is restored from its authenticated package as the healthy rollback.
Three shared RDP entries map to the original VNC workspaces. The original Fabric,
Device and capability references survive; an optional private sidecar adds only
new entries and remains invisible to alpha.105. Identity regression covers add
and rollback. The original catalog is unchanged; no private runtime is published.

Server playback nodes retain processing while idle. FreeRDP's Pulse playback
backend also needs an explicit test buffer: its unbuffered/default path still
showed a 43 ms gap on one workspace. With 200 ms client buffering, all three
six-second synthetic round trips and a ten-second sample are continuous above
our 10 ms gap threshold. These are local Linux RDP tests, not physical Windows
acceptance. A per-user audio helper assigns ordinary application streams to the
workspace identified by their process DISPLAY, using per-stream WirePlumber
metadata; global defaults and explicit non-PF choices are preserved. Real default
playback streams on all three workspaces link to their corresponding sinks.

The native UI offers explicit microphone opening and detects missing capture
hardware. Normal open keeps microphone capture off. Protected API/CLI requests
carry transient consent through the same authorization and exact-target path;
retry preserves it. Full checks, 58 UI tests, six Python tests, package verification
and isolated responsive native-window checks pass. The native preview reads the
actual catalog through protected IPC; original Shell access also succeeds.

Required owner step: upstream RDP must forward the physical microphone and
reconnect. Current Windows still has one render endpoint and zero capture
endpoints. The owner can now choose a shared RDP desktop for playback, or its
explicit microphone action after forwarding is enabled. VNC picture with audio
requires keeping that workspace's RDP connection alive; automatic companion
launch/lifecycle is not implemented. Do not claim hardware sound/microphone or
WAN quality acceptance. On 2026-10-01 the owner explicitly authorized publishing
alpha.106 and updating the second Windows controller for physical audio testing.
This replaces the pre-publication hardware acceptance gate; hardware acceptance
remains open and is not implied by release. Publication and target update are now
complete: alpha.106 is active with alpha.105 rollback, six private baseline files
unchanged, 50 readable history records, three available shared workspaces and
responsive native/background processes. Final identity, catalog, connection and
update checks pass. An initial discovery retry required the controller's existing
Windows HTTPS path for fixed-release download; all hashes and publisher/replay
checks remained enforced. No host network setting changed. Return to
M10.3 after the owner completes this usable audio journey; see
[Shared desktop audio](../status/SHARED_DESKTOP_AUDIO.md).

2026-09-30 owner feedback reopens UI closeout under M10.3. R0: main is clean
before changes. Audit exact-target actions, unavailable desktop visibility,
history handoff and asynchronous recovery. Acceptance: unavailable desktops keep
their identity and visible row; repeat handoff uses the original capability;
stale catalog actions cannot use old availability. Preserve native protocol
executors, private runtime and existing remote sessions. Run presentation tests,
full checks and isolated native-window validation. Return to M10.3 acceptance.
Shared VNC/RDP desktop continuity and audio are feasibility work in this turn;
do not replace existing remote desktops on the basis of protocol research alone.
The bounded UI patch now passes all 57 presentation tests and full checks. An
isolated unpackaged native window is responsive; Computers rendering was reviewed.
Unavailable-row identity, exact history handoff, chronological history and stale
action/completion handling are repaired. Evidence and delivery limits are in
docs/status/UI_CONTINUITY_REVIEW.md. This is a local development candidate, not
a fleet release; audio/session migration remains unimplemented and unverified.

Live owner use found that the local alpha.104 Windows payload exposed the three
EDA server TigerVNC desktops but omitted `vncviewer.exe`. Device identity,
authorization, SSH, all three remote VNC listeners and controller-to-listener TCP
reachability were healthy. Alpha.105 restores the reviewed signed TigerVNC Viewer
and GPL license, and makes both required by build and installed-package verification.
Full checks and isolated package verification pass. The public source, fixed release
and preview feed are updated; the owner controller runs alpha.105 with alpha.104 as
rollback. A fleet notice was sent so other Windows members discover and verify the
update independently. No private profile was copied or compiled into the package,
and no foreground VNC session was opened during background verification. Return
point: owner can retry any visible `虚拟桌面 :2/:3/:4` action.

Owner UX feedback now prioritizes desktop selection on the second Windows controller:
retain all TigerVNC workspaces and stop defaulting the Linux computer action to RDP.
The bounded UI repair makes multi-desktop primary clicks choose an exact desktop,
labels TigerVNC/RDP explicitly, and keeps all workspace identities unchanged. Main
and arrow clicks must share the chooser; individual rows retain route selection.
All 57 presentation tests, full checks, signed package/profile retention and privacy
checks passed. Alpha.104 is published and enabled; the second controller updated
independently and its running Center uses the new installation. All three VNC
workspaces remain available and credential-ready. A Viewer was present on that
controller, but alpha.104 package verification did not require one; the alpha.105
side task above closes that release gap. Local alpha.104 retained one Center and two responsive native windows.
This UI side task is closed; return to owner experience feedback. No server/protocol
migration and no remote VNC session was opened automatically.

The owner explicitly requested repair of the fourth-host Shell path. Live diagnosis
found a missing Tailscale node ID cached while the peer was offline, followed by an
unprobed unreachable LAN selection. Refreshing the controller daemon restored the
exact PF Remote Shell target. The bounded fix resolves only missing IDs at route
acquisition, preserves existing pins and probes direct Shell routes before selection.
Offline/reconnect and changed-node rejection tests cover base and overlay caches.
The fix is published and enabled as alpha.103. All four update reports are current;
ordinary PF Remote hostname and remote update-status execution succeed. Full checks,
signed package upgrade/profile retention and privacy scans pass. This side task is
closed. Return point: owner Settings acceptance. No host network or private
configuration edits were required.

## Review gates

R0: main at fd18f3c was clean; scope expanded explicitly to full managed fleet.
R1: signed requests and publisher trust remain separate; notices contain no URLs,
keys or commands. Additive update_state table preserves enrollment/rollback.
R2: retained action admission protection; removed unrelated OS-session blockers
found by live rollout. Independent SSH/RDP/VNC services are not stopped.
R3: full scripts/check.ps1, real signed package upgrade/rollback, SQLite restart,
replay/revocation/failure tests, privacy scans and live three-host automatic upgrade
passed. The final native Center is responsive and remains running.
R4: evidence in docs/status/FLEET_UPDATE_REPORT.md. Fleet rollout is complete;
M10.3 remains open only for owner acceptance; the Shell defect is repaired.

## Deferred findings

Publisher key rotation/revocation and future data-epoch migrations need separate
work. If Gateway starts hosting content streams, add authoritative stream draining.

## Evidence

contracts/fleet-updates-v1.md and docs/status/FLEET_UPDATE_REPORT.md.
