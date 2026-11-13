---
schema_version: 1
roadmap_item: M10.3
roadmap_milestone: M10
roadmap_text: Deliver cross-device version visibility, independent offline catch-up and compatibility gates.
status: active
base_commit: 95e6182
---

# Active work — deferred offline migration

## Objective

Resume the remaining offline member migration only when it becomes available.
The owner explicitly deferred it; do not request that it be powered on now.

## Acceptance criteria

Retain that member's identity and private settings, install the signed current
release, reconcile exact peer grants and verify its actual authorized connections.

## Current step

No online deployment work remains. M11.1 daily peer enrollment is complete:
alpha.120 is published and installed on four online members. Existing identities,
original fourteen targets, credentials and viewers remain. Both controllers list
one new headless Shell and execute through daily CLI and MCP; the Linux manager
also executes on it. All online update checks report current. Private evidence
is stored outside Git. The offline member is not counted as upgraded.

## Out of scope

Powering on the offline member, foreground automation, global networking changes,
new VPS desktop, GitHub Support contact and physical audio retesting.

## Blocking side task

None. Offline work is deferred by the owner, not an online-service failure.

## Review gates

R0/R1: distinct public source and pinned existing identities; no private key copy.
R2: signed records, exact grants, fail-closed state loss and maintenance guards.
R3: full checks, isolated upgrade, final file hashes and daily CLI/MCP execution.
R4: anonymous source commits, signed public artifacts and sanitized delivery report.

## Deferred findings

Explicit root recovery and bounded journal checkpointing remain documented limits.
Physical reboot was not performed; startup enablement and service restart passed.

## Evidence

[Peer management delivery](../status/PEER_MANAGEMENT.md) and
[Gateway fleet convergence](../status/GATEWAY_FLEET_MIGRATION.md).
