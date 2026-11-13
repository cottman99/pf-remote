# Native SSH terminal

Implementation in progress. Headless computer cards offer Connect terminal.
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
Deployment evidence remains pending.
