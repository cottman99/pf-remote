# Display presets

Released: Windows alpha.112 (publisher sequence 114), 2026-10-15.

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
- Packaged upgrade, rollback to alpha.111 and re-upgrade retain the selected
  monitor preset and encrypted account. The older client reads its compatible
  full-screen setting; the new client recovers the complete selection afterward.

## Delivery

Sanitized source and seven fixed-release assets are published. All remote asset
digests match the reviewed package; preview payloads are checked before signed
metadata is published last. Source and UTF-8/UTF-16 package privacy scans pass;
reviewed SDK enum names and a fixed binary table are not deployment strings.

Both reachable Windows controllers independently update from alpha.111 to
alpha.112. Their six and seven protected identity/account/connection files are
unchanged, with 14 targets and readable history preserved on each. The existing
one and three native viewers remain running with their original start times;
both installed Centers are responsive. Monitor discovery succeeds on one- and
two-screen controllers. The third controller is management-unreachable with a
last-observed alpha.109 report; a durable notice is retained. The Linux node
keeps its current platform release because this change is in the Windows client.

## Compatibility and limits

The protected local v1 schema gains optional display/monitor fields. No identity,
route, Gateway or remote-desktop migration is needed. Credentials stay encrypted
locally. An older binary uses the retained full-screen flag; upgrading again
restores advanced settings unless the older binary rewrote the preset.

The isolated local UI test has one physical monitor. Native discovery also
recognizes the second controller's two monitors. Multi-monitor adjacency, numbering
and profile projection have synthetic regression coverage; simultaneous rendering
on multiple physical monitors is not claimed from that test. Remote server and
protocol support determine the final multi-monitor canvas. No active remote
desktop was resized or reconnected for verification.

Native property reference: [Microsoft RDP display settings](https://learn.microsoft.com/en-us/azure/virtual-desktop/rdp-properties#display-settings).
