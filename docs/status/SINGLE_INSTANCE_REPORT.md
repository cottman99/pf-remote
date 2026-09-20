# Single Center instance

## User-visible behavior

Repeated launches reuse the existing Center. Explicit launch restores and shows
the original hidden/minimized window; background startup remains silent. This
rule applies to the manager, not independent remote-desktop viewer sessions.

## Implementation boundary

A user/session-scoped Windows mutex is acquired before creating the window,
tray icon or starting daemon recovery. Secondary instances exit after optionally
signalling a named auto-reset event. The event carries no command, target or
credential. A signal sent during startup remains pending until the owner listens.
Windows releases ownership on process exit. Synthetic debug IPC fixtures use a
separate scope so tests cannot activate the installed client.

## Verification

The production service is linked into a separate-process test harness. It tests
12 concurrent secondary launches, startup-time activation, three explicit
activation requests, silent background launches, independent session scope,
normal exit and forced-process-exit recovery without desktop input.

Full scripts/check.ps1 passed, including 49 Windows tests, the process harness,
Go checks, contracts and native build. The alpha.92 development package passed
release verification and was installed locally with alpha.91 retained for rollback.

Eight concurrent installed --background launches all exited successfully.
Exactly one original Center and one daemon remained; two top-level windows
responded to WM_NULL health probes. All five checked private configuration files
retained their hashes. No foreground input or focus automation was used.
Explicit activation delivery is process-tested; visible foreground restoration
is code-reviewed, not visually exercised on the user's active desktop.
