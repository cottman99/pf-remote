# M6.6 Center information architecture report

## Product result

PF Remote Center is now a native management shell rather than one long preview
page. It has three stable user destinations:

- Computers: named computers, one-click Desktops, and the same target handed
  to the user's Agent.
- Sessions: recent successful Desktop and Agent operations with computer,
  action, result, and time only.
- Settings & Recovery: synchronization, existing-setup migration, installed
  version readiness, and recovery files.

The installed internal candidate is `0.1.0-alpha.18`; `alpha.17` remains the
installer rollback candidate. The independent old product remains untouched.

## Usability and evidence

- Wide Computers and Settings views use a standard WinUI `NavigationView`.
- At a 700-pixel audit width, navigation becomes compact, computer information
  remains visible, and multiple Desktop actions wrap instead of leaving the
  window.
- Sessions has a clear empty state and a populated state verified from a real
  read-only Agent operation against the online Linux workstation.
- A final installed-candidate audit recorded one Agent session and one Desktop
  session. The Desktop opened without an authentication prompt.
- The final installed Center launched with a responsive top-level window on an
  isolated Windows desktop and was left running there.
- The full project check passed, including private-data hygiene, all Go tests,
  28 Windows UI tests, and the unpackaged WinUI build. The internal release
  archive and manifest also passed verification before installation.

Visual evidence is retained outside the public repository as:

- `PFRemote-Center-Navigation-Computers-20260830.png`
- `PFRemote-Center-Navigation-Computers-Narrow-Fixed2-20260830.png`
- `PFRemote-Center-Navigation-Settings-20260830.png`
- `PFRemote-Center-Navigation-Sessions-Agent-20260830.png`

## Privacy boundary

Recent sessions are bounded and reset with the local daemon. They include no
command content, output, address, port, route, credential, or Desktop content.
Human and Agent actions use the same redacted activity source, avoiding a
misleading UI-only history.

## Migration impact

No legacy setting, service, connection, or entry point changed. The
intentionally offline Windows computer was not contacted. The next user-facing
gap is installed lifecycle ownership: ordinary startup, upgrade, and rollback
still need to be completed in the Center without a command line.
