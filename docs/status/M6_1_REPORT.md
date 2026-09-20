# M6.1 current private catalog compatibility

## Product result

The internal candidate now shows the complete current legacy inventory instead
of losing the whole list when it encounters the third-party screen-control
entry. The user sees two named remote computers and all seven existing actions:
five Desktop actions and two Shell actions. The third-party physical-screen
entry remains a one-click Desktop action, while actions that still need a
target-side service are honestly shown as being prepared.

This is compatibility completion, not replacement completion. It removes the
catalog gap and gives the next enrollment stage a faithful starting point, but
the two Shell actions still need real target-side confirmation and every
Desktop/Shell action still needs its own side-by-side operation check.

## User-point-of-view correction

The earlier clean-room Alpha proved the architecture but did not mean the
installed product was replaceable. A replacement milestone must begin with the
user's complete current computer/action list and preserve every working escape
route. M6 therefore measures parity against the actual private workflow rather
than against synthetic demonstrations.

The legacy Center can independently refresh its catalog. The correct safety
promise is that the candidate opens it read-only and performs zero writes, not
that no other legacy process can ever update the file during a long observation.

## Compatibility behavior

- SSH projects to Shell; RDP and VNC project to Desktop.
- The reviewed external screen-control action is bound to one exact immutable
  target and started directly without a command shell.
- Executable paths, arguments, endpoints, accounts, credentials, and device
  identities stay inside the private process boundary and are omitted from UI,
  Agent context, JSON evidence, and public source.
- An invalid external action or stale route degrades only that action. It no
  longer hides unrelated computers or valid actions.
- The old Center, old agent, catalog, configuration, and connections are not
  stopped or modified and remain the fallback.

## Verification evidence

- The live read-only projection returned two computers and seven actions.
- The native Center rendered the complete real list on an isolated desktop and
  remained responsive without foreground input.
- A stable live observation window retained identical source bytes and
  timestamp while both old and candidate Centers remained running.
- Exact-target, private-serialization, unsafe-definition, partial-degradation,
  and routed-fallback tests pass at the changed layers.
- The complete Windows and Go project check passes, including private-data and
  single-roadmap-item governance checks.

## Next user-visible outcome

M6.2 gives the Windows laptop and headless Linux workstation durable identities
and publishes their real capabilities. M6.3 then turns every currently listed
Shell and Desktop action from compatibility inventory into a verified working
action before any default cutover.
