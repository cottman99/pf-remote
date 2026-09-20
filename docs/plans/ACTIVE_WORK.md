---
schema_version: 1
roadmap_item: M10.3
roadmap_milestone: M10
roadmap_text: Deliver cross-device version visibility, independent offline catch-up and compatibility gates.
status: active
base_commit: a5245747712aa54cec0a818e6700b91260c5fd9c
---

# Active work — fleet rollout and owner acceptance

## Objective

Finish the fourth computer's trusted bootstrap/update and let the owner evaluate
the completed Settings update controls. The owner authorized managed-node updates,
public sanitized releases and complete fleet closure without intermediate gates.

## Acceptance criteria

Named versions and last-seen states in Settings; any active member can notify;
Gateway-retained notices survive offline recipients and restart; recipients verify
and install independently; private identities, credentials, gateways and records
survive. All intended available hosts converge and the owner can use the controls.

## Current step

Alpha.102 is published. Both Windows installations and the Linux managed node
independently upgraded from alpha.100 to alpha.102, with installer completion
records. Linux was offline at the management-service layer when notice was saved,
then caught up automatically after restart. Both Windows and Linux successfully
sent notices. Current Windows identity, connection service and update checks pass.

The fourth host's authorized SSH management path fails to connect. No mutation was
attempted there. Resume its bootstrap when the owner confirms it is powered on,
network-connected and signed in, then verify its report and final convergence.
The Settings UI is ready for owner-level experience feedback now.

## Out of scope

Paid signing, new protocol servers, host network changes, Linux desktop UI and
unrelated feature expansion. Do not weaken publisher verification to speed rollout.

## Blocking side task

Fourth-host availability is the only deployment dependency. Return point: resolve
and inspect its immutable target, bootstrap through the approved signed package,
then verify identity, retained configuration and the cross-device version report.
Do not infer successful deployment from its cached catalog online label.

## Review gates

R0: main at fd18f3c was clean; scope expanded explicitly to full managed fleet.
R1: signed requests and publisher trust remain separate; notices contain no URLs,
keys or commands. Additive update_state table preserves enrollment/rollback.
R2: retained action admission protection; removed unrelated OS-session blockers
found by live rollout. Independent SSH/RDP/VNC services are not stopped.
R3: full scripts/check.ps1, real signed package upgrade/rollback, SQLite restart,
replay/revocation/failure tests, privacy scans and live three-host automatic upgrade
passed. The final native Center is responsive and remains running.
R4: evidence in docs/status/FLEET_UPDATE_REPORT.md. M10.2 closed; M10.3 remains open
for the fourth-host rollout and owner acceptance, not for another internal module.

## Deferred findings

Publisher key rotation/revocation and future data-epoch migrations need separate
work. If Gateway starts hosting content streams, add authoritative stream draining.

## Evidence

contracts/fleet-updates-v1.md and docs/status/FLEET_UPDATE_REPORT.md.
