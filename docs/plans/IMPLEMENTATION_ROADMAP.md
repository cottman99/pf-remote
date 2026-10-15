# Implementation roadmap

The owner replaced the previous feature-expansion roadmap on 2026-09-19.
Historical completion claims and the unfinished sleep work are retained in
[the archived roadmap](ARCHIVED_ROADMAP.md), not in the active delivery queue.

## M7 — focused local closeout

- [x] Consolidate the source branches, stabilize exact-target UI actions, and deliver a private local candidate with a separate sanitized source export.

No platform expansion, automatic wake, web desktop, or cosmetic release train
is scheduled. Preserve existing authorized connections and rollback. Future
work starts from an observed user failure or a new product decision.

Evidence: [Focused closeout report](../status/CLOSEOUT_REPORT.md).

## M8 — single client instance

- [x] Repeated launches reuse the existing Center without duplicating windows or background startup.

Evidence: [Single instance report](../status/SINGLE_INSTANCE_REPORT.md).

## M9 — interaction recovery

- [x] Stabilize concurrent dialogs, stale catalog recovery, action feedback and incremental lists.

Evidence: [Interaction recovery report](../status/INTERACTION_RECOVERY_REPORT.md).

## M10 — zero-cost trusted updates

- [x] Verify signed release metadata and artifacts against an independently provisioned publisher key.
- [x] Deliver autonomous per-device GitHub update discovery, verified installation, data preservation and automatic failure rollback.
- [ ] Deliver cross-device version visibility, independent offline catch-up and compatibility gates.

Paid code-signing services are not required for update authenticity. Public OS
publisher reputation is separate. Existing private deployment remains usable;
bootstrap older nodes once before they can participate in managed updates.

Evidence M10.1: [Trusted update foundation](../status/TRUSTED_UPDATE_REPORT.md).
Evidence M10.2: [Live fleet update delivery](../status/FLEET_UPDATE_REPORT.md).

The owner's connection-preset priority is tracked in
[Connection presets](../status/CONNECTION_PRESETS.md). M10.3 remains the return
point after this bounded usability repair and authorized release rollout.
The owner's display-mode follow-up is tracked in
[Display presets](../status/DISPLAY_PRESETS.md), with four locally remembered
modes and recovery when selected screens change. M10.3 remains the return point.
The follow-up exact-workspace repair is tracked in
[Independent Linux workspace selection](../status/WORKSPACE_SELECTION_FIX.md).
Alpha.111 is published and installed on both reachable Windows controllers;
the offline controller retains its notice. Original workspaces are preserved.

Alpha.102 is publicly available for Windows and managed Linux nodes. Two Windows
installations and one Linux node independently updated from alpha.100 to alpha.102.
Linux also consumed a notice retained while its management service was offline.
The fourth managed computer was bootstrapped from alpha.87 to alpha.102; all four
report current and the fourth can send update notices. All four subsequently
updated independently to alpha.103. Its cached-offline Shell route defect is fixed
and ordinary PF Remote execution is verified. M10.3 owner UX acceptance remains open.
Alpha.104 incorporates owner desktop-selection feedback: explicit TigerVNC/RDP
choices replace the ambiguous multi-desktop primary action. The second Windows
controller is running this acceptance build with all VNC workspaces preserved.
Alpha.105 repairs the Windows release payload after live use found that alpha.104
could publish the VNC choices without the TigerVNC executable. Windows release
build and verification now require the reviewed Viewer, its signature and GPL
license. The owner controller is running alpha.105; other Windows members were
notified to discover and verify the update independently.

2026-09-30 UI closeout: local development repairs retain unavailable desktop rows,
pin historical Agent handoff to the original target and guard stale actions.
Full checks and isolated native-window validation pass; no new fleet release.
See [UI and continuity review](../status/UI_CONTINUITY_REVIEW.md). Shared VNC/RDP
session continuity is technically feasible, while audio integration requires a
separate live pilot before any existing desktop is replaced.

The owner subsequently authorized implementation of shared xrdp/VNC workspaces
and bidirectional audio. Administrator setup completed after owner sudo setup;
all six channel/audio services and six audio nodes are live. No existing
workspace is replaced. Native shared-workspace/audio controls are installed in the local signed preview.
Alpha.109 is published and active on all available Windows controllers. Normal
Linux workspace selection carries picture and sound together; supported Windows
resources are enabled by default with locally signed profiles. Home signing and
battery startup are verified. See [Default sharing](../status/DEFAULT_RESOURCE_SHARING.md).
Physical microphone/speaker and WAN quality acceptance remain open; see
[Shared desktop audio](../status/SHARED_DESKTOP_AUDIO.md).

2026-10-15: the owner confirms independent workspace selection. Linux application
audio discovery, Chromium workspace routing and stale audio-engine recovery are
repaired on the server. All three workspaces pass real paused application routing
and bidirectional synthetic RDP audio. Physical owner hardware acceptance remains
open. See [Workspace audio recovery](../status/WORKSPACE_AUDIO_RUNTIME_FIX.md).
The owner's next test exposes stale application audio initialization and a
speaker monitor selected as the default microphone. Device discovery is recovered;
managed capture-node selection and its regression check are repaired. The owner's
actual controller-to-workspace-3 dictation now passes. Default capture follows
the controller's selected input; other hardware paths remain separately unverified.
The follow-through now verifies default-input changes in one isolated Windows
native RDP connection with generated audio, including switch-back and negative
controls. The input follows the normal system default; a communications-only
change does not override it. The repair is synchronized to GitHub main as
0c43198. No Windows binary or schema migration is needed. Physical USB/Bluetooth
hot-plug remains separate from this native notification/transport verification.

The owner clarified that every node must poll GitHub and update itself; a controller
must not be required to push or execute updates on other nodes. Center aggregates
status. Background checks use jitter/backoff; verified downloads and safe local
activation must also work when the management UI is closed. Gate activation on
PF Remote-owned active operations, gateway availability and supported mixed-version
compatibility. Independent SSH/RDP/VNC services are preserved, not counted as a
permanent update blocker.

Any authorized member may announce a successful update through the Gateway.
Persist/coalesce the latest hint for offline members; reconnect and periodic
checks provide catch-up without broadcast loops. Hints never carry execution
authority, trust keys or arbitrary download locations. Each recipient still
verifies its own channel/platform release and installs locally at a safe time.
