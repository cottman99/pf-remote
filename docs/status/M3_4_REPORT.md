# M3.4 implementation report — Agent and UI Shell actions

Date: 2026-08-29

## Product outcome

PF Remote now exposes the verified Shell journey as local `connect` and `exec`
actions for a natural-language Agent and the native Windows Center. A user can
choose a named computer and describe a task without handling an address, port,
route, credential, or immutable identifier.

The Windows Center now provides a visible “use this computer” flow and prepares
the selected computer plus the user's natural-language task for an Agent. This
is an enabling preview, not the product-manager acceptance build: a private
deployment has not been migrated and the Center does not yet open an interactive
Desktop session.

## Evidence

- The daemon-side action resolves the authorized target and owns subject
  identity, route selection, strict host identity verification, cancellation,
  and bounded output.
- Versioned local action responses and safe failures are covered at the action,
  protected-local-API, CLI, Session, and OpenSSH layers.
- Tests cover unavailable setup, wrong or changed identity, expiry, revocation,
  route failure, cleanup, cancellation, and bounded output without falling
  through to another computer.
- The Windows Center builds and launches as an unpackaged native application;
  the verified Chinese top-level window remained responsive.
- `scripts/check.ps1` passed all repository checks, 9 Windows Center tests, and
  the WinUI build with zero warnings and zero errors.

## Git evidence and next boundary

- `901cebf` — protected `connect` and `exec` actions across daemon, local API,
  CLI, contracts, and tests.
- `2f11f02` — non-technical Windows task composer for selecting a computer and
  describing work for an Agent.

M4.1 is next: open existing RDP and VNC clients through capability-accurate
Desktop executors. The product manager should not be asked to accept the
current preview; meaningful acceptance starts when a real named computer can
be opened and used without a terminal.
