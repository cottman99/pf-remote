# M6.11 faithful Computers visual implementation

## User-visible result

The Computers page now implements the selected generated design instead of only
borrowing its interaction idea. A computer appears as one compact, readable
row with status and device iconography, recent-use context, one dominant Smart
connect action, a secondary Agent handoff action, and a clear expand/collapse
control.

Expanding a computer shows all available Desktops as a structured list. Search
and online/offline filtering work directly beside the computer count, and a
visible refresh action gives the user a clear way to update the page.

## Usability state

- The new layout works at both wide and narrow Windows sizes without clipped or
  overlapping persistent actions.
- Smart connect, route selection, per-Desktop connection, Agent handoff,
  search, filter, expansion, and refresh remain usable without a command line.
- Internal candidate `0.1.0-alpha.28` is installed. `0.1.0-alpha.27` is retained
  as the immediate rollback version.
- The local daemon is healthy and exposes the existing seven authorized targets.
- Windows Home client was not contacted, woken, probed, or configured.

## Product judgment

The previous milestone was functionally useful but visually misclassified as a
completed redesign. The user's criticism exposed a planning error: progressive
disclosure alone did not deliver the chosen visual system. This item therefore
made the generated image the source of truth and treated differences in density,
iconography, search/filter, and list structure as real product defects.

The result is now close enough to the selected design to continue the replacement
roadmap. It is still an internal candidate rather than a complete old-version
replacement because the intentionally offline Windows computer has not yet been
enrolled or verified.

## Evidence

- [Source and implementation side by side](evidence/M6_11_UI/design-qa-combined.png)
- [Wide native Windows capture](evidence/M6_11_UI/M5_4_GOLDEN.png)
- [Narrow native Windows capture](evidence/M6_11_UI_NARROW/M5_4_GOLDEN.png)
- [Design QA](../../design-qa.md)

Both isolated journeys completed Desktop and Agent actions in two user steps,
with no foreground input, private target, or legacy mutation. The full project
check passed, including 31 Windows presentation tests and the native WinUI build.

## Migration impact

No target identity, authorization, route priority, protocol, or private stored
configuration changed. The installed candidate retains a one-version rollback
path. The next roadmap item remains paused until the owner reports Windows Home client online.
