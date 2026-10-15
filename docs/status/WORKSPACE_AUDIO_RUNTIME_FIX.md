# Shared workspace audio recovery

2026-10-15: the owner confirms correct workspace selection, but ordinary Linux
applications cannot use sound. The server repair is applied. The owner has now
confirmed intelligible dictation on the actual controller-to-workspace-3 connection.
Physical speaker playback and other controller/workspace combinations are not
established by this acceptance. A subsequent isolated native Windows test also
verifies following default-input changes during one RDP connection.

Follow-up on the owner's actual workspace-3 connection: existing ChatGPT/Chrome
audio utilities had retained the earlier failed audio initialization. A bounded
restart of only their verified audio utility children restores device discovery;
application roots and windows stay alive. The owner then confirms that dictation
can start, but reports no usable speech. This is not a successful speech test.

The Pulse server also reports a speaker monitor as its default input. Managed
microphones had the same zero selection priority as speaker monitors. Give real
capture nodes a higher automatic priority while preserving explicit user choices
and per-workspace stream routing. The new default-input check fails before this
change and passes on all three workspaces after it. Only idle audio adapters are
reloaded; X servers, channel servers, viewers and application windows are preserved.
The owner's subsequent test confirms normal recognition of a short spoken sentence.
The owner also confirms the product rule: use the current controlling computer's
selected microphone by default. No physical device model is pinned; the native RDP
client supplies its input and Linux routes it to the chosen workspace.

## Findings and changes

| Failure | Repair | Current evidence |
| --- | --- | --- |
| Each original VNC desktop has an isolated runtime directory, without an audio socket. | Recreate missing same-owner audio links in existing protected workspace runtimes. Keep their session buses and existing endpoints. | The same Pulse connection probe changes from refused to ready on all three desktops. |
| Chromium audio children clear their display environment. | Resolve same-owner parent processes and X11 window PID properties; ambiguous workspace ownership is not guessed. | The live browser audio process resolves to its actual workspace. |
| Audio streams stay initializing, even with an unrelated virtual sink. The running audio core still uses a replaced older executable/library set. | Recover the user audio engine and its adapters. Decouple clipboard channels from audio recovery; adapters follow engine restarts. | All three desktop applications create paused playback/capture streams and link to their own devices. |
| The old synthetic test inherits the operator shell's desktop. Automatic routing can then redirect its probes and invalidate the result. | Put every virtual test process on the isolated client display; defer when a real VNC/RDP viewer owns the tested workspace. | Fresh authenticated RDP round trips pass in both directions on every workspace. |
| A speaker monitor becomes the default microphone before application routing. | Give managed capture nodes priority over managed speaker monitors; keep explicit user defaults. | The native Pulse default is a real source and paused application streams reach their own workspace on all three desktops. |

The repair changes the Linux setup helper and its user-service definitions. It
does not require another Windows client installation or a new daemon schema.
Existing shared-desktop identities, credentials, X servers, application windows
and channel-server processes remain intact. The old helper and unit definitions
are backed up privately. Rollback removes only exact owned audio links.

## Verification

- Twelve Linux tests pass, including private-directory permissions, conflicting
  endpoint preservation, recreated-runtime recovery, rollback and browser routing.
- `verify-linux-workspace-audio.py` uses paused native Pulse streams in the real
  desktop runtime. It also rejects a monitor as the default input, so later stream
  routing cannot hide this failure. All three workspaces pass both directions; it
  reads and plays zero audio samples.
- An isolated Chromium profile enumerates actual audio devices and can open and
  immediately stop a capture track. The owner's existing Windows RDP connection
  also acknowledges a microphone open successfully. The controller's default
  microphone is enabled and unmuted. These facts do not prove intelligible speech.
- `verify-linux-shared-audio.py` sends a generated 997 Hz signal through RDP to
  disposable virtual audio devices. All three six-second bidirectional tests
  recover approximately 5.96 seconds of tone with no silence gap over 10 ms.
  Original X-server processes and window lists remain unchanged. No physical
  microphone or speaker is used by this test.
- Full repository checks, 61 native presentation tests and the Windows build pass.
  The isolated native window is responsive; the installed user client remains
  running. Later Linux-only edits pass their Linux tests and governance checks.

## Acceptance boundary

Actual dictation on workspace 3 passes owner acceptance after both repairs. The
same discovery and routing checks pass on all three workspaces. Physical speaker
playback, other controller/workspace pairs and physical USB/Bluetooth removal are
separate checks. M10.3 remains the mainline return point.

## Native input switching follow-through

The Windows native client follows the controlling computer's system default input.
Changing that input does not require closing the remote desktop. A separate
communications-default change does not replace the normal default input in the
verified native path. An application with its own explicit device selection keeps
that selection under the routing contract.

| Case during one connection | Result |
| --- | --- |
| Initial default input | Linux receives the generated 997 Hz tone. |
| Change the test selection without a native notification | Capture stays on the earlier input; the test cannot pass just by changing its tone generator. |
| Change the normal default input | Native capture reopens and Linux receives 1301 Hz. |
| Change only the separate communications default | The current 1301 Hz input remains selected. |
| Change the normal input again, then switch back | Linux receives 1709 Hz, then 997 Hz; the desktop connection remains open. |

The test uses Windows Remote Desktop Connection 10.0.26100.8875, xrdp 0.10.6.1
and PipeWire 1.4.11. A newly created Windows process runs on an isolated desktop
and connects to a disposable loopback-only Linux desktop through authenticated
SSH. Process-local instrumentation supplies synthetic endpoint-change notifications
to the native Windows audio mapper. It does not change any host default device or
attach to an existing owner process. Each newly initialized capture stream gets
its own generated tone; the instrumentation never forces a stream restart. The
Linux receiver independently measures the actual PCM delivered through RDP in
100 ms windows and retains only measurements, not samples.

All six cases pass. The three changes are observed in about 0.47–0.55 seconds in
this run; these are test observations, including the measurement path, not a
latency guarantee. Native automatic reconnection is disabled and the same client
stays connected. Original X servers and active channel connections remain intact;
all disposable test processes, listeners and credentials are removed. This verifies
native notification handling and real transport, not physical device unplug/replug
or every microphone driver. See the
[sanitized measurements](evidence/native-microphone-follow-20261015.json).

The server repair is persistent and published to GitHub main in `0c43198`.
This verification requires no new Windows binary, client reinstall, or owner
desktop reconnect. Full repository checks with `-SkipWindowsApp` pass; this
follow-through changes documentation and verification evidence only.
