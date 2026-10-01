# UI and desktop continuity review — 2026-09-30

The owner requested a unified UI review and clarified that protocol switching
must resume the same desktop, with audio available where supported.

## Client repairs

- Unavailable desktops retain their canonical identity and remain visible in
  expanded lists. A single unavailable desktop no longer becomes "No desktop".
- Historical Shell handoff resolves its original capability from the current
  catalog, including when another capability becomes the computer's default.
- Recent sessions sort by timestamp before grouping and limiting the list.
- Desktop/repeat handlers reject stale catalog state. Completion restores the
  initiating control according to its current model, not the previous target.
- Device handoff reads the current bound computer rather than a recycled Tag.

Reviewed catalog refresh, desktop menus, history actions, connection feedback,
settings operations and adaptive layout handlers. This is a bounded code review,
not a claim that all visual states or all possible defects are covered.

## Evidence and delivery

All 57 presentation tests pass, including regression assertions for retained
unavailable desktops, non-default Shell handoff and out-of-order history.
`scripts/check.ps1` passes (Go tests/vet, privacy/governance, compatibility,
Agent alignment, presentation tests, single-instance checks and WinUI build).
The unpackaged Debug build starts on an isolated Windows desktop with a
responsive top-level window in 4.185 seconds; its Computers page was rendered
and inspected. The verified instance remains running on that isolated desktop.
This is a local development build, not a new public release or installed-fleet
upgrade. No schema, stored private configuration or remote session was changed.

## Desktop continuity and audio

The previous assertion that VNC and RDP cannot continue the same desktop was too
broad. xrdp's VNC backend can connect to an existing VNC server, preserving that
server's desktop while clients alternate protocols. This needs explicit binding
to the existing workspace; selecting an unrelated Xorg session does not do so.

- [xrdp configuration manual](https://github.com/neutrinolabs/xrdp/blob/devel/docs/man/xrdp.ini.5.in)
- [Upstream PipeWire audio module](https://github.com/neutrinolabs/pipewire-module-xrdp)

The authorized Linux node reports xrdp 0.10.6.1 and PipeWire 1.4.9. Its package
database does not list either queried xrdp audio module package; this alone does
not exclude an unmanaged installation. Audio is not verified. Attaching xrdp to
VNC does not automatically attach that desktop's audio server to xrdp channels.
Keep existing desktops until a pilot proves the same windows survive VNC/RDP
switches and verifies speaker output and microphone input separately. A separate
VNC audio bridge remains an alternative; neither path is delivered by this patch.
