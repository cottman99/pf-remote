# Shared workspace audio recovery

2026-10-15: the owner confirms correct workspace selection, but ordinary Linux
applications cannot use sound. The server repair is applied. Physical Windows
speaker/microphone acceptance remains open.

## Findings and changes

| Failure | Repair | Current evidence |
| --- | --- | --- |
| Each original VNC desktop has an isolated runtime directory, without an audio socket. | Recreate missing same-owner audio links in existing protected workspace runtimes. Keep their session buses and existing endpoints. | The same Pulse connection probe changes from refused to ready on all three desktops. |
| Chromium audio children clear their display environment. | Resolve same-owner parent processes and X11 window PID properties; ambiguous workspace ownership is not guessed. | The live browser audio process resolves to its actual workspace. |
| Audio streams stay initializing, even with an unrelated virtual sink. The running audio core still uses a replaced older executable/library set. | Recover the user audio engine and its adapters. Decouple clipboard channels from audio recovery; adapters follow engine restarts. | All three desktop applications create paused playback/capture streams and link to their own devices. |
| The old synthetic test inherits the operator shell's desktop. Automatic routing can then redirect its probes and invalidate the result. | Put every virtual test process on the isolated client display; defer when a real VNC/RDP viewer owns the tested workspace. | Fresh authenticated RDP round trips pass in both directions on every workspace. |

The repair changes the Linux setup helper and its user-service definitions. It
does not require another Windows client installation or a new daemon schema.
Existing shared-desktop identities, credentials, X servers, application windows
and channel-server processes remain intact. The old helper and unit definitions
are backed up privately. Rollback removes only exact owned audio links.

## Verification

- Eleven Linux tests pass, including private-directory permissions, conflicting
  endpoint preservation, recreated-runtime recovery, rollback and browser routing.
- `verify-linux-workspace-audio.py` uses paused native Pulse streams in the real
  desktop runtime. All three workspaces pass playback and capture routing; it
  reads and plays zero audio samples.
- `verify-linux-shared-audio.py` sends a generated 997 Hz signal through RDP to
  disposable virtual audio devices. All three six-second bidirectional tests
  recover approximately 5.96 seconds of tone with no silence gap over 10 ms.
  Original X-server processes and window lists remain unchanged. No physical
  microphone or speaker is used by this test.
- Full repository checks, 61 native presentation tests and the Windows build pass.
  The isolated native window is responsive; the installed user client remains
  running. Later Linux-only edits pass their Linux tests and governance checks.

## Remaining owner step

Use the normal shared RDP workspace, play sound, and try one spoken sentence.
Software transport and application routing are verified; physical hardware,
browser permission and the owner's particular Windows connection still require
this real interaction. M10.3 remains the mainline return point.
