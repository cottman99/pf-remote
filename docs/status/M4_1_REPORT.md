# M4.1 implementation report — usable Desktop launch

## Outcome

PF Remote Center can now show an authorized Desktop as either a private virtual
desktop or the computer's current physical screen, and open it without asking
the user for an address, port, protocol, credential, or route. The daemon
resolves the immutable target and current Grant before acquiring one route.

RDP uses the installed Windows client with Remote Credential Guard and public
mode. VNC supports target-pinned X.509 trust or an exact online Tailscale node;
the Tailscale mode cannot fall back to LAN. Authorization expiry, revocation,
profile changes, cancellation, route failure, missing clients, and process exit
all close safely and leave the Center usable.

## Representative user journey

A private authorized Windows laptop was used without recording its identifiers
in the repository. A signed current TightVNC server was installed on that laptop
with its HTTP listener disabled. Both server access control and Windows Firewall
restricted the VNC listener to the controlling Tailscale peer. The viewer was
kept as an uninstalled temporary client on the controller.

The running Center showed one current-screen target. Invoking its visible button
launched a responsive viewer for the expected computer. The server reported one
established connection from the expected controller. Closing the viewer
re-enabled the Center button and produced the localized closed-session status.
The daemon verified the configured immutable Tailscale node ID immediately before
exposing the route.

This run also found and fixed two real-integration failures that synthetic data
had not exposed: Desktop profiles were not preserved by SQLite, and redirected
CLI JSON was decoded with the Windows console code page instead of UTF-8. Schema
migration 4 now round-trips Desktop profiles, and Center explicitly reads CLI
output and errors as UTF-8.

## Verification

- `scripts/check.ps1` passes the private-data, governance, formatting, Go test,
  vet, build, local IPC smoke, Center test, WinUI build, and Linux cross-build gates.
- State tests cover Desktop-profile persistence, version-3 migration, legacy
  readability, and invalid protocol/authentication pairings.
- Desktop tests cover immutable resolution, expiry, live revocation, route
  ownership, RDP launch shape, X.509 VNC, Tailscale-only VNC, missing clients,
  and cancellation cleanup.
- Center tests cover Desktop-profile JSON and forced UTF-8 process streams.
- The unpackaged x64 Center launches with a responsive top-level window and
  remains running after verification.

## Migration and remaining risk

Existing databases migrate in place. Old snapshots without authentication or a
persisted Desktop profile remain readable, but `open` fails closed until a fresh
valid profile arrives. The VNC executable remains an external protocol executor;
bundling, licensing inventory, signed installation, upgrade, and uninstall belong
to the M5 installer item.

The representative current-screen path uses Tailscale. Gateway-relayed Desktop
evidence and the virtual-Desktop composition benchmark remain separate later work.
