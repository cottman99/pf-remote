# M6.10 focused Computers experience

## User-visible result

The Computers page now behaves like a focused remote manager instead of a grid
of equally weighted actions. Each computer has one dominant Smart connect
action. The action names the Desktop it will open and keeps the existing
per-connection route menu on its edge. Agent handoff remains visible as a quiet
secondary action.

When a computer offers several Desktops, the most recently successful Desktop
becomes primary. The others stay behind an `Other desktops` disclosure with
ordinary user-facing names. A computer without a Desktop no longer displays a
large disabled blue connection action.

## Usability state

- Wide and narrow native Windows layouts complete the same two-step Desktop and
  Agent journey without clipped or overlapping actions.
- The primary action and Agent handoff remain keyboard accessible.
- The installed internal candidate is `0.1.0-alpha.27`; `0.1.0-alpha.26` is the
  immediate rollback version.
- The background daemon is healthy after installation and exposes the existing
  seven authorized targets.
- Windows Home client was not contacted, woken, or configured.

## Product judgment

The initial implementation exposed two product problems during visual review:
the primary action did not say which Desktop it would open, and an Agent-only
computer looked as if it had a broken Desktop action. Both were corrected
before installation. A first narrow render also exposed an overlapping action
row; the final layout stacks actions below computer details at narrow widths.

Search and filtering are intentionally deferred. With two visible computers,
they add interface weight without improving selection. They become useful when
the real device count makes direct scanning slower.

## Evidence

- [Design QA](../../design-qa.md)
- [Wide isolated window](evidence/M6_10_UI/M5_4_GOLDEN.png)
- [Narrow isolated window](evidence/M6_10_UI_NARROW/M5_4_GOLDEN.png)
- [Design-to-implementation comparison](evidence/M6_10_UI/design-qa-combined.png)

Both isolated journeys passed with two user steps, a responsive top-level
window, no active-desktop input, no private target, and no legacy mutation. The
full repository check passed, including 31 Windows presentation tests, native
WinUI build, private-data hygiene, and the existing Go/integration suite.

## Migration impact

No target identity, authorization, route priority, protocol, or stored private
configuration changed. The old-version fallback remains one user-level action.
The next roadmap item resumes only when the owner reports Windows Home client online.
