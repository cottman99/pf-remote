# Native SSH terminal

Alpha.121 implements the Windows entry point. Headless computer cards offer Connect terminal.
The action reuses the authorized Shell target, private route and saved key.
Windows OpenSSH receives a console and requests a PTY; input/output never
traverses management IPC. Existing connect remains an authentication probe.

The opt-in connect --terminal flag and protected API terminal action use the
existing Shell response schema, adding action terminal and status launched.
Launched confirms native process creation, not completed interactive login.
Existing clients are unchanged. Startup failures carry stable error codes.

A process job owns the console; route and maintenance leases persist until exit.
Daemon shutdown or authorization loss cancels it. The UI does not replay an
ambiguous launch failure. Windows Session 0 is rejected.

No remote desktop is installed. Private keys stay on their controllers.
## Verification and delivery

- Windows Center: 65 presentation tests and native build pass. The unpackaged
  app has a responsive top-level window on an isolated desktop, left running.
- Native console startup, exit and cancellation pass. A real saved-key SSH
  connection verifies remote PTY input/output in that isolated desktop.
- Authorization revocation, daemon shutdown, maintenance ownership and failed
  authentication tests pass. Existing Shell/legacy compatibility checks pass.
- Upgrade from alpha.120 preserves synthetic identity, targets and encrypted
  presets. The primary controller runs alpha.121; all 542 package files match,
  eight private configuration files are unchanged and daily SSH authentication
  succeeds against the intended headless target.
- Source and public packages pass private-data checks. Twelve signed release
  artifacts use sequence 123; no controller addresses or keys enter Git.

The full Windows check ran but stopped at the existing native display selection
integration: the host Windows session is disconnected and system mstsc /l exits
without listing screens. Remaining UI/build/signing/single-instance checks were
run independently and passed. Selected-screen integration is not claimed here;
this terminal change does not alter desktop display selection.

The other controller was unreachable through its current relay and direct
routes during this delivery. It is not counted as updated; the signed preview
feed permits its ordinary update when available. The previously deferred offline
member remains deferred. No active viewer, remote desktop or proxy was restarted.

