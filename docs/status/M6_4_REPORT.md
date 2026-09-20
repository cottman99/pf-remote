# M6.4 online-scope reliability report

## Product result

The internal Windows candidate is now installed as a normal per-user
application rather than run from a development folder. It appears in the Start
menu and Windows Installed apps. Its local service and private Gateway start
from the selected installed version at user logon, while the old Center remains
available as an independent fallback.

The currently online headless Linux computer kept the same identity and all
five actions through controller and Gateway restarts: one Shell, three VNC
virtual desktops, and one RDP virtual desktop. During a controlled Gateway
outage, the named computer list and direct Linux Shell path remained usable.
Gateway synchronization recovered automatically after the service returned.

The offline Windows controlled computer remained explicitly offline with its
actions disabled. No connection, wake, deployment, or probe was sent to it.

## Usability

- The installed Center launches without a terminal and remains responsive in
  an isolated Windows desktop.
- A temporary control-plane interruption does not make an already known
  computer disappear or prevent a direct action.
- Restarting candidate services does not rename computers or create duplicate
  action entries.
- The old Center continues to run side by side, so this internal candidate has
  a preserved recovery path and is not yet the default entry.

## Reliability evidence

Twenty observations over approximately ten minutes all showed a responsive
installed Center, running background services, the same five Linux actions, the
offline Windows computer still disabled, and the old Center still available.
There was no restart loop. The full repository check passed after the installed
runtime test, including private-data hygiene, all Go tests, 26 Windows UI tests,
and the unpackaged WinUI build. The internal Windows release also passed its
manifest and payload verification.

## Migration impact

No legacy configuration, route, service, or entry point was removed or changed.
The candidate uses its own per-user installation and launch definitions. The
next rollout item is deliberately waiting for the owner to report the offline
Windows computer online; no work on that computer is authorized before then.
