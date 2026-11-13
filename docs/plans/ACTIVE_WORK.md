---
schema_version: 1
roadmap_item: M10.3
roadmap_milestone: M10
roadmap_text: Open a named headless computer in the local terminal using saved SSH access.
status: active
base_commit: 1b47eb1
---

# Active work — native SSH terminal

## Objective

Provide one-click terminal access on headless computer cards.

## Acceptance criteria

Open local native SSH with saved access and pinned host identity, without
address or credential entry. Retain the route while the terminal runs.
Revocation, daemon shutdown and exit release resources. Preserve private settings.

## Out of scope

Remote GUI installation, offline migration, foreground automation, network changes
and private deployment data in source or packages.

## Current step

Verify native launch, authorization, UI and signed deployment. The owner
explicitly prioritizes this item ahead of deferred M10.4.

## Blocking side task

None.

## Review gates

R0: clean public-main worktree and additive terminal contract.
R1: protected IPC, signed binding, saved key and process ownership.
R2: focused diff and failure tests; no routing authority in UI.
R3: full check, isolated launch and actual remote PTY verification.
R4: sanitized release evidence and private deployment verification.

## Deferred findings

Offline migration returns to M10.4 when that member becomes available.

## Evidence

[Native SSH terminal](../status/NATIVE_TERMINAL.md).
