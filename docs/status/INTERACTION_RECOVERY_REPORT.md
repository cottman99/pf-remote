# Interaction recovery — alpha.93

## User-visible changes

- Native dialogs share one queue. A failed dialog releases the queue and shows a
  safe message rather than allowing an unhandled async event exception.
- A failed catalog load shows a persistent notice and retry on the computer page,
  including first-load failure. Cached names stay visible with unknown status;
  connection and Agent actions are disabled until a successful refresh.
- Operation messages survive automatic refresh, filtering and navigation. Catalog
  failures use an independent notice so neither failure is silently overwritten.
- Computer and recent-session collections reconcile by stable identity without
  Reset. Unchanged items retain their instances; changed rows alone are replaced.
- Credential-save errors and route-open errors retain their error classification
  and the existing exact-target recovery menu.

## Scope and migration

Windows presentation only. No new dependency, identity, authorization, routing,
protocol, private-data format or remote-deployment change. alpha.92 is the local
rollback candidate. Public source export remains separate from private runtime.

## Verification

Five regression tests cover concurrent dialog waiting and failure recovery,
action-feedback persistence through catalog failure/recovery, stale action
suppression with identity preservation, collection changes without Reset, and
credential-versus-route error messages. Existing tests remain included.

Full scripts/check.ps1 passed: 54 Windows tests, separate-process single-instance
checks, Go tests/vet, contracts, hygiene, integration paths and native Debug build.
The final Release rebuild includes a follow-up guard so an old completed Agent
operation cannot re-enable a now-stale action. Release-package verification passed.

Installed alpha.93 with alpha.92 retained for rollback. Process-level validation
found one Center, one daemon and two responsive top-level windows. Five private
identity/credential/connection files retained their hashes. No remote upgrade or
public upload was performed.

An unused intermediate package remains in ignored scratch storage because
automatic approval review blocked its deletion. The installed package was built
separately and verified; the intermediate package was never installed.
Visual focus, dialog layout and overlap are not claimed as verified: no input or
foreground automation is performed on the user's active desktop.
