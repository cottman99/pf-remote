# M6.13 restored Windows Shell and physical Desktop report

## Product outcome

The restored Windows controlled computer now supports both Agent automation
and one-click access to its current physical screen. PF Remote uses the exact
Tailscale device identity for the physical-screen route and does not ask the
user for a redundant VNC password when Tailscale is the authorization boundary.

## User-visible evidence

- Shell execution returns the declared restored-computer identity through both
  the normal PF Remote action and the MCP Agent action.
- The physical Desktop completes an RFB 3.8 server initialization and reports
  the real 2560 by 1600 screen before an isolated viewer remains running.
- The Center shows the computer as online with one available current screen,
  a dominant Smart connect action, an adjacent available Tailscale choice, and
  a secondary Agent handoff.
- The Linux controlled computer still opens VNC and RDP virtual Desktops over
  Tailscale, opens VNC over the Gateway/FRP route, and Smart connect skips an
  unavailable LAN path in favor of Tailscale.

## Compatibility correction found during testing

Desktop route data and newer Desktop presentation metadata originally shared
one strict versioned file. That made an older candidate unable to start after
the physical-screen profile was added. They are now separate versioned files:
older candidates can read the unchanged route schema, while newer candidates
overlay the physical-screen profile. Candidate rollback and return both recover
in under one second on the local controller.

## Confidence

Real Shell, VNC, RDP, Smart/manual route, exact-target context, direct MCP,
process restart, rollback, native UI, release verification, and the full project
check pass. All visual and connection evidence containing private names stays
outside the repository.

## Remaining risk

Actual host sleep has not been claimed from process-restart evidence. It remains
the next roadmap item and requires a verified wake path before suspending a
remote computer, so a failed experiment cannot strand the test machine.
