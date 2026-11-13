---
schema_version: 1
roadmap_item: M11.1
roadmap_milestone: M11
roadmap_text: Deliver peer management without mandatory central registration.
status: active
base_commit: 08db08f
---

# Active work — daily peer enrollment

## Objective

Admit the existing VPS identity through daily installed Center, CLI and MCP.

## Acceptance criteria

Distinct signed release, all original targets retained, unique VPS Shell,
authenticated commands from both Windows controllers and Linux, persistent
background services and synchronization. Preserve proxy and desktop processes.
No extra VPS manager authority or copied private keys.

## Current step

Prepare alpha.120 from public alpha.118 with peer management and downgrade guards.
Verify packages and migration before daily cutover; keep the pilot separate.

## Out of scope

Offline member, VPS desktop, global networking, foreground automation and
administrator restoration of arbitrary old binaries.

## Blocking side task

None. Supported recovery must preserve peer authority.

## Review gates

R0: original dirty workspace preserved; separate release checkout. R1: pinned
root, immutable identity, exact grants and signed host binding. R2: fail-closed
activation and supported installer downgrade guards. R3: full checks, packages,
migration and real daily-runtime integration. R4: actual deployment evidence.

## Deferred findings

Root loss requires explicit recovery. Bounded journal checkpointing remains a
scale limit. Offline migration resumes only when that device becomes available.

## Evidence

[Peer management](../status/PEER_MANAGEMENT.md).
