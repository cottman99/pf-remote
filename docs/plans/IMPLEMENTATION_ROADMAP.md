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

Alpha.102 is publicly available for Windows and managed Linux nodes. Two Windows
installations and one Linux node independently updated from alpha.100 to alpha.102.
Linux also consumed a notice retained while its management service was offline.
The fourth managed computer was bootstrapped from alpha.87 to alpha.102; all four
report current and the fourth can send update notices. All four subsequently
updated independently to alpha.103. Its cached-offline Shell route defect is fixed
and ordinary PF Remote execution is verified. M10.3 owner UX acceptance remains open.

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
