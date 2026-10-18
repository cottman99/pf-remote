# Shared workspace reconnect and startup recovery

2026-10-18. Follow-through from the owner's successful keyboard and dictation
check. The acceptance path covers all three existing workspaces, reconnects and
fresh desktop/audio runtimes while preserving active user sessions.

## Behavior

| Situation | Automatic behavior |
| --- | --- |
| Desktop starts before audio is ready | A desktop startup hook prepares its private audio links and waits for its microphone and speaker devices before applications start. A failed readiness check uses the existing desktop service's retry path. |
| Application focus is stuck on XFWM's fallback | A normal left-click can restore input to that same application. No command, reconnect or application restart is needed. |
| User changes selection or opens a popup | Recovery cancels. Healthy focus and explicit no-input windows remain authoritative. |
| Display starts late or restarts | Its input helper retries independently. It does not start, restart or stop the display. |

The repeatable focus failure involves XFWM retaining a no-input decision after
an application removes its input hint. The original application's initiating
event remains unknown. The helper addresses the observed fallback-focus state;
it is not a guarantee against every application or client input defect.

The helper observes left-button releases and window/process identity properties.
It does not select keyboard events, read text, capture audio, inspect pixels or
raise windows. Recovery checks the same owner, process start times, selected
window, pointer position and native focus again, and uses the original click
timestamp so a later focus change takes precedence.

## Verification

- Nineteen Linux unit tests pass, including delayed audio, the boot-created 0755
  source child directory under a private runtime, conflicting endpoints, wrong
  owners, rollback and cancellation when user selection or process identity changes.
- A disposable XFWM session reproduces the failure without recovery. Installing
  the helper leaves focus alone until a click, then repeatedly restores typing.
  Explicit no-input windows remain unchanged.
- Six fresh disposable runtimes and nine real RDP connections pass keyboard,
  ordinary application audio discovery and generated-tone transport in both
  directions. They use the installed VNC, xrdp and audio implementations with
  temporary credentials, loopback listeners, private audio cores and policy-only
  WirePlumber. No physical audio device is opened.
- Each generated six-second tone delivers about 5.96–5.98 seconds at 997 Hz.
  Some fresh capture paths have one approximately 21-ms gap during the first
  second; steady audio has no gap over 10 ms. The test explicitly reports startup
  and steady gaps, rejecting startup gaps over 50 ms, later gaps over 10 ms,
  short delivery or the wrong frequency. This is bounded connection evidence,
  not a long-duration audio-quality guarantee.
- A second full run adds an induced focus failure to every connection. All six
  fresh runtimes and nine native RDP clicks recover keyboard input, including
  the VNC backend's event path. Live mappings are verified against the disposable
  test model. These are isolated replicas, not restarts of the active desktops.
- All three actual workspaces pass paused application playback/capture checks
  after deployment, reading and playing zero samples. All three input helpers
  are enabled, active and stable; all three startup hooks are loaded, and the
  installed readiness command passes. Original desktop, channel, audio-core and
  application processes survive, and the existing viewer stays connected.
- `scripts/check.ps1 -SkipWindowsApp` passes. Unix checks now include the Linux
  regression tests. See [sanitized evidence](evidence/workspace-reconnect-20261018.json).

## Deployment and migration

Only Linux helpers and user service configuration change. Existing X servers,
desktop sessions, channel servers, audio adapters and application roots remain
running. Helpers are installed and the three input services are enabled; only
the audio metadata router was restarted. The update does not require a Windows binary, account migration or
credential change. Private backups and diagnostics stay outside Git.

The live host is not rebooted: fresh process/runtime tests and startup dependency
inspection establish the restart path without interrupting the owner. A physical
host reboot and the physical speaker hardware are outside this run's evidence.

Rollback restores the prior helper files and removes only the exact new input
services and unchanged startup hooks. It does not stop the desktop or audio core.
The public change contains source and sanitized evidence only. No private
deployment files are part of the publication. This bounded repair is closed;
M10.3 remains the mainline return point.
