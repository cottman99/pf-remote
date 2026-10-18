# Shared workspace input recovery

2026-10-18: the owner confirms that keyboard input and intelligible dictation
work again on the affected controller-to-workspace connection. All three saved
workspace credentials pass live authentication. Original desktops and application
windows remain running; the repair does not stop or reconnect remote viewers.

## Findings and recovery

| Area | Finding | Repair and evidence |
| --- | --- | --- |
| Saved accounts | One additional workspace retained a password rejected by its VNC backend. | The original protected credential passes a real challenge. Saving it through protected IPC makes the selected RDP preset pass; the other presets, default selection and original credential remain unchanged. All three workspace presets now authenticate. |
| Audio after reboot | The socket service creates a normal 0755 Pulse child directory inside the owner's private 0700 runtime. The helper incorrectly rejects it, leaving all three workspace Pulse links missing. | Accept a real, same-owner source directory under the protected runtime. Keep private destination directories, socket ownership checks and refusal of redirected or conflicting paths. All three applications discover their own playback and capture endpoints. |
| Existing application audio | An idle audio utility retained its earlier failed initialization. | Refresh only that verified same-owner audio child. The application root, windows and desktop connection survive; the owner subsequently confirms intelligible dictation. |
| Keyboard delivery | Real key events reach the X server, while the active application's actual keyboard focus remains on the window manager's fallback window. Minimizing and restoring does not recover it. | Restore the application's missing input hint, then perform one explicitly owner-authorized focus repair guarded by the current active window and original process identity. Actual focus then matches the application and the owner confirms typing works. No keyboard input is injected. |

The keyboard failure was in application/window-manager focus delivery. The
original trigger is not established. This recovery does not install a focus
watcher or change normal window-selection behavior.

## Verification

- Fourteen Linux tests pass. The boot-permission regression fails against the
  old helper and passes after the change. Tests cover wrong owners, source
  symlinks, conflicting endpoints and rollback that preserves the source socket.
- Paused native Pulse streams pass playback and capture routing checks on all
  three workspaces; these checks read and play zero audio samples.
- Idle workspaces 2 and 4 pass six-second synthetic RDP round trips in both
  directions, with about 5.96 seconds of generated tone and no silence gap over
  10 ms. An initial shorter workspace-2 run reported a playback gap and was
  repeated with the established six-second test. This is bounded transport
  evidence, not a long-duration quality guarantee.
- The recovery does not reconnect workspace 3. The owner confirms actual
  keyboard input and intelligible dictation after recovery.
  Physical speaker playback is not established by these microphone checks.
- Repository checks pass with `scripts/check.ps1 -SkipWindowsApp`; no Windows
  application source or binary changes are part of this repair.
- Authentication-only probes close before entering a desktop. Disposable
  tunnels, observers and test processes are removed. X servers, channel servers,
  audio adapters and the application root retain their original processes.
  A final controller inventory finds one earlier viewer no longer present and
  the other original viewer still responsive; this check does not establish the
  cause of that later exit. No repair action stops a native viewer.

## Deployment and rollback

The Linux audio helper is installed persistently; only its routing service is
restarted. The existing channel/audio services and X servers are preserved.
Private backups retain the earlier helper and encrypted preset. Rollback can
restore the helper and remove only its exact owned audio links without changing
workspace buses or unrelated endpoints.

No Windows reinstall, identity change, credential migration or schema change is
required. Private endpoints, identities, credentials, logs and deployment
backups stay outside the public repository. This side task is closed; M10.3
remains the mainline return point.
