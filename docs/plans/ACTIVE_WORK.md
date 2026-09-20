---
schema_version: 1
roadmap_item: M10.3
roadmap_milestone: M10
roadmap_text: Deliver cross-device version visibility, independent offline catch-up and compatibility gates.
status: active
base_commit: 59b774b2771b6a8f259acdf34d2df8c630db3846
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

Alpha.102 is published. Both Windows installations and the Linux managed node
independently upgraded from alpha.100 to alpha.102, with installer completion
records. Linux was offline at the management-service layer when notice was saved,
then caught up automatically after restart. Both Windows and Linux successfully
sent notices. Current Windows identity, connection service and update checks pass.

The fourth host was identity-verified through its existing alternate SSH path and
bootstrapped from alpha.87 to alpha.102. All ten private JSON file hashes remained
unchanged, 48 history records were retained and the catalog recovered to 11 targets.
All four fleet reports are current; the fourth successfully sent an update notice.
Its original PF Remote Shell path still fails independently of update delivery.
The Settings UI is ready for owner-level experience feedback now.

## Out of scope

Paid signing, new protocol servers, host network changes, Linux desktop UI and
unrelated feature expansion. Do not weaken publisher verification to speed rollout.

## Blocking side task

The owner explicitly requested repair of the fourth-host Shell path. Live diagnosis
found a missing Tailscale node ID cached while the peer was offline, followed by an
unprobed unreachable LAN selection. Refreshing the controller daemon restored the
exact PF Remote Shell target. The bounded fix resolves only missing IDs at route
acquisition, preserves existing pins and probes direct Shell routes before selection.
Offline/reconnect and changed-node rejection tests cover base and overlay caches.
Return point: publish the repair and verify Shell through the updated local daemon,
then owner Settings acceptance. No host network or private configuration edits.

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
M10.3 remains open for owner acceptance and the separately recorded Shell defect.

## Deferred findings

Publisher key rotation/revocation and future data-epoch migrations need separate
work. If Gateway starts hosting content streams, add authoritative stream draining.

## Evidence

contracts/fleet-updates-v1.md and docs/status/FLEET_UPDATE_REPORT.md.
