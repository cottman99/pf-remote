# M4.3 virtual Desktop visual-effects policy report

## Product decision

The original roadmap asked PF Remote to default virtual Windows Desktops to
`composition off` and prove the choice with an on/off A/B benchmark. That
experiment is not valid on supported modern Windows: DWM is a core operating
system component and cannot be disabled by users or applications. Microsoft
also documents that Remote Desktop sessions keep composition enabled. A call
that appears to disable DWM returns success for compatibility but does not
disable it.

PF Remote therefore does not expose or simulate a composition switch. The
product now distinguishes the rendering environment from a target-owned visual
effects profile:

- virtual Desktop, omitted policy: `automatic`;
- virtual Desktop, measured target profiles: `reduced` or `full`;
- physical Desktop, omitted or explicit policy: `system`.

The default preserves native resolution, application GPU rendering, DWM, and
the existing protocol executor. It delegates supported visual and encoding
choices to the OS/protocol instead of pretending PF Remote can disable a
mandatory platform component.

## Evidence for the correction

- Microsoft states that DWM is always on from Windows 8, apps cannot disable
  it, and Remote Desktop sessions keep composition enabled:
  <https://learn.microsoft.com/en-us/windows/win32/w8cookbook/desktop-window-manager-is-always-on>
- The `DwmEnableComposition` API is deprecated; disabling composition has no
  effect on Windows 8 and later even though the call can report success:
  <https://learn.microsoft.com/en-us/windows/win32/api/dwmapi/nf-dwmapi-dwmenablecomposition>
- Microsoft's modern Remote Desktop guidance recommends automatic connection
  quality selection and treats animations, window dragging, background, and
  similar options as experience settings rather than a DWM lifecycle switch:
  <https://learn.microsoft.com/en-us/windows-server/administration/performance-tuning/role/remote-desktop/session-hosts>
- Microsoft separates GPU-accelerated application rendering from remote frame
  encoding and documents both as explicit remote-session concerns:
  <https://learn.microsoft.com/en-us/azure/virtual-desktop/graphics-enable-gpu-acceleration>

## Contract and runtime behavior

`DesktopProfile.visual_effects_policy` is an additive optional field. Existing
catalog snapshots remain valid. The Desktop coordinator normalizes an omitted
virtual policy to `automatic` and an omitted physical policy to `system`, pins
the effective value in the Session, and ends the Session if the authenticated
profile changes. Unsupported values and applying a virtual reduction profile
to a physical console fail before route acquisition.

`reduced` and `full` are declarations by a target that actually owns those
settings. They do not give the controlling client authority to rewrite a remote
machine. Before a target publishes either profile, its implementation still
needs a same-resolution representative-workload comparison for the specific
supported settings. No fake DWM on/off benchmark was run or claimed.

## Verification

- Targeted Desktop coordinator tests pass.
- Tests cover backward-compatible defaults, physical/virtual separation,
  supported target-declared profiles, rejection of `disable-dwm`, and Session
  pinning of the effective policy.
- The full repository check remains the closure gate after roadmap state is
  advanced.

## User impact

There is no new technical switch for the user to supervise. PF Remote keeps the
safe platform default and preserves quality. If a future target has a measured
profile that materially changes responsiveness or appearance, the product can
surface that as a meaningful user-level choice; otherwise it stays automatic.
