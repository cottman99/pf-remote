# Focused closeout

## Route assessment

Retain named target identity, shared UI/Agent action core, existing protocol
executors, protected local deployment and reversible installation. Rewriting
these working boundaries would discard tested behavior without user benefit.
Stop platform expansion, automatic-wake certification and cosmetic-only release
iterations. The old milestone ledger is archived rather than falsely closed.

## UI changes

Desktop choice no longer changes with recent activity or availability ordering.
Routes can still fall back for the same Desktop. A recycled split button resolves
its current model when opening its menu, and a click captures that exact target.
Primary and expanded actions have distinct automation IDs. Narrow views retain
the Desktop name. Recent actions resolve against the full catalog, not a search
subset. Onboarding no longer asks for route, install state or installation path.

## Distribution boundary

Source export excludes Git history, local state, internal installers and historical
screenshots and crash evidence. Private runtime stores and credentials stay outside the source tree;
local installation retains them. Public-source checks must precede distribution.

## Validation

- `scripts/check.ps1`: passed, including 49 Windows tests (zero failures/skips),
  Go tests/vet, contracts, privacy checks and native build with zero warnings.
- `scripts/verify-windows-release.ps1`: alpha.91 passed; internal unsigned package.
- Local alpha.91: one Center and one daemon in the same interactive session;
  responsive top-level windows checked without input injection or focus changes.
- Installed catalog retains four computers and eleven capabilities. All five
  existing identity, connection and Desktop-credential files are byte-identical.
- A harmless exact-target Linux command completed through the installed daemon/MCP.
- Source export succeeded with an external private-term list. An isolated synthetic
  private-name fixture was rejected before creating an archive.
- The isolated-desktop click harness could not start either the new build or the
  previously installed alpha.89. This environment limitation is recorded rather
  than treating a compiled UI as an end-to-end click pass. Target selection and
  automation-ID regressions pass model tests; full interactive click confirmation
  remains open. The local candidate passed independent background window health.

Reproduce source inspection with `scripts/export-public-source.ps1`, supplying an
output outside the source tree and a private-term file stored outside the tree.
No raw deployment evidence or crash dumps belong in the public source.

## Branch consolidation

The two older branch tips are ancestors of the developed branch. The developed
source and closeout changes are consolidated onto main without discarding history.
The pre-closeout Git bundle and working-file backup remain outside this repository.
Only main remains as the development branch; no remote upload is performed.

## Remaining limits

This closeout does not certify all remote routes, automatic wake or public binary
signing. No public upload or remote fleet upgrade is part of this local delivery.
