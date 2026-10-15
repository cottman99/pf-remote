# Display presets

Candidate: Windows alpha.112 (publisher sequence 114), 2026-10-15.

Connection presets now offers windowed, single-screen full screen, all-screen
full screen and selected-screen full screen. A screen map and checkboxes select
adjacent local screens; the first selection is the remote primary display. The
choice belongs to this controlling computer and exact remote desktop, and is
reused on its next connection. Existing viewers are not changed.

The default is explicitly single-screen full screen. Native screen IDs come
from Windows Remote Desktop's read-only monitor listing on an isolated desktop.
Windows display-device numbers are not substituted for RDP IDs. If selected
screens disappear or their layout changes, the connection opens settings for
reselection or single-screen recovery instead of taking over different screens.
Audio, microphone, clipboard, hardware sharing and exact workspace routing keep
their existing defaults. Direct VNC entries retain their existing viewer behavior.

## Evidence

- All repository checks, 64 presentation tests and native monitor discovery pass.
- Protected IPC saves each mode, retains the account and reloads the selected
  screen mode after daemon restart. Invalid or stale selections cannot be saved
  or launch a viewer; adjacent sets and selected-primary order are covered.
- A real unpackaged WinUI instance on an isolated desktop saves and reopens all
  four modes through UI Automation. The settings dialog renders, remains
  responsive, and the verified instance is left running on that isolated desktop.
- Legacy `full_screen` files remain readable; account-only requests preserve
  advanced settings. A changed legacy boolean selects window/single. Native
  profile tests verify every display property and retain resource/authentication
  settings. No private values enter the source or generated public package.

## Compatibility and limits

The protected local v1 schema gains optional display/monitor fields. No identity,
route, Gateway or remote-desktop migration is needed. Credentials stay encrypted
locally. An older binary uses the retained full-screen flag; upgrading again
restores advanced settings unless the older binary rewrote the preset.

The live local test has one physical monitor. Multi-monitor adjacency, numbering
and profile projection have synthetic regression coverage; simultaneous rendering
on multiple physical monitors is not claimed from that test. Remote server and
protocol support determine the final multi-monitor canvas. No active remote
desktop was resized or reconnected for verification.

Native property reference: [Microsoft RDP display settings](https://learn.microsoft.com/en-us/azure/virtual-desktop/rdp-properties#display-settings).
