# M6.3 online-action parity report

## Product result

Every existing action on the enrolled headless Linux computer now reaches the
same immutable Device from the new Windows controller. One Shell action, three
VNC virtual desktops, and one RDP virtual desktop completed real connections.
The VNC viewer is included in the internal candidate, so a user does not need
to install a separate client or handle an address.

The same Shell target was inspected, exported as an Agent context, and used by
the local MCP server for a real benign command. A separate existing relay path
also reached the controlled computer. UI, CLI, MCP, direct route, and relay
evidence therefore align on one named target instead of exposing
infrastructure choices to the user.

The additional Windows controlled computer was taken offline by its owner. The
new Center now displays it as temporarily offline and disables both Agent and
Desktop actions. This is a private candidate-side availability override: the
old catalog is unchanged, and no connection, wake, deployment, or probe was
sent to the offline computer.

## Usability

- Shell and all three VNC desktops are usable from the new path now.
- RDP reaches the exact computer and opens the Windows client. On first use,
  Windows shows its native server-certificate confirmation; the Center now
  explains that state in ordinary language instead of falsely reporting that
  the newly opened window was already closed.
- The device list distinguishes independent virtual desktops from a physical
  screen and presents user-facing names only.
- The old Center remains running as the independent fallback.

## Confidence

The full repository check passes, including private-data hygiene, target and
authorization tests, real action boundaries, MCP alignment, Windows UI tests,
and an unpackaged WinUI build. The internal Windows candidate includes a
signature-checked separate VNC executor with its license and SBOM entry; no
third-party binary or private deployment value is added to public source.

## Migration impact

No old process, configuration, route, or credential was changed. The new
candidate remains side by side and is not yet the default. Reliability under
control-plane interruption and user-logon persistence is the next online-scope
item; the offline Windows computer resumes only after its owner reports it
online.
