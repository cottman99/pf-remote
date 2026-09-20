# M6 three-end internal replacement report

## Product outcome

The internal Windows candidate now covers the complete daily journey across the
Windows controller, the restored Windows controlled computer, and the headless
Linux workstation. A person can choose a named computer, connect its Desktop
with Smart connect or a per-connection route, hand the same computer to Codex,
and let Codex inspect and act on the exact authorized target.

`0.1.0-alpha.75` is the normal installed entry and `0.1.0-alpha.74` is its
immediate complete rollback. The untouched old product remains installed and
running. Settings now separates two previously ambiguous actions: removing old
computers from the new unified list, and explicitly opening the installed old
PF Remote Center in one user action.

## User-visible verification

- Both controlled computers completed real Shell actions after exact identity
  inspection through the installed candidate.
- The Linux virtual Desktop opened through Smart connect and through a manually
  selected Gateway route. The restored Windows physical Desktop opened through
  its manually selected Tailscale route. All viewer windows were observed on an
  isolated desktop without foreground input.
- The installed MCP server discovered the modern protocol, inspected both exact
  Shell targets, and completed a real harmless action on each one.
- The route menu continues to expose Smart, LAN, Tailscale, and Gateway for the
  selected connection, including unavailable state instead of hiding a path.
- Candidate rollback to `alpha.74` and return to `alpha.75` each restored one
  usable background set and completed another real Shell action. The old product
  remained unchanged throughout.

## Product problem found and corrected

TigerVNC distinguishes `host:display` from `host::port`. The Gateway adapter
provides a high local TCP port, but the Desktop executor previously passed it in
display-number syntax. The Gateway and remote Desktop were healthy while the
viewer waited without a visible window, and the Center could misleadingly say
that the Desktop had opened. The executor now uses explicit TCP-port syntax for
every VNC route while RDP keeps its native endpoint syntax. Focused tests cover
IPv4, IPv6, direct, and relay-shaped VNC endpoints; the real Gateway viewer then
became visible in about 3.2 seconds.

## Confidence and remaining risk

The complete repository check, release verification, three-end Shell/Desktop
actions, exact-target MCP actions, and candidate rollback all pass. No private
address, credential, device identity, or deployment artifact is stored in the
repository evidence.

A follow-up installed observation completed 24 catalog/health cycles and eight
alternating real Shell actions with zero failures. It retained exactly one
current daemon and one current Gateway, averaged about 0.72 seconds per combined
catalog/health cycle on this controller, and kept the old Center running.

Automatic wake from the controlled Windows computer's S3 sleep did not pass the
bounded trial. Temporary wake tasks and wake-policy changes were removed, and
ordinary use recovered after the computer was manually powered on. The project
manager explicitly made this a non-blocking stability follow-up; it is not
claimed as complete here.
