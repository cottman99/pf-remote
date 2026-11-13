---
schema_version: 1
roadmap_item: M10.4
roadmap_milestone: M10
roadmap_text: Deliver cross-device version visibility, independent offline catch-up and compatibility gates.
status: active
base_commit: 6f8a6b4
---

# Active work — deferred fleet catch-up

## Objective

Resume outstanding controller updates when the computers become reachable.

## Acceptance criteria

Preserve identity and private configuration, install the signed current release
and verify daily authorized connections. Do not count unavailable nodes as updated.

## Current step

Native terminal delivery M10.3 is complete on the primary Windows controller.
The user selects a headless computer and clicks Connect terminal. The second
controller's relay and direct paths were unavailable; its update is pending.
The previously offline member remains explicitly deferred by the owner.

## Out of scope

Asking to power on deferred computers, foreground automation, graphical desktop
installation, global network changes and publishing private configuration.

## Blocking side task

None. Reachability limits deferred fleet catch-up, not the delivered terminal.

## Review gates

R0/R1: exact identity, saved key, signed host binding and protected IPC.
R2: lifecycle and failed-auth tests; no launch replay after ambiguous failure.
R3: native isolated SSH PTY, UI responsiveness, upgrade retention and package hashes.
R4: anonymous public source, signed artifacts and sanitized delivery evidence.

## Deferred findings

Native selected-screen integration could not run in the disconnected Windows
session. Existing display behavior is unchanged. Physical reboot is not tested.

## Evidence

[Native SSH terminal](../status/NATIVE_TERMINAL.md) and
[Peer management delivery](../status/PEER_MANAGEMENT.md).
