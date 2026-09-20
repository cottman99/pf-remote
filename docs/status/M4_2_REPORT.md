# M4.2 Agent alignment report

## User-visible result

PF Remote Center now presents computers rather than transport capabilities. A
computer appears once even when it offers both Desktop and Shell access. The
person can either open that computer's Desktop or hand the same computer to an
external Agent without seeing addresses, ports, Gateways, routes, or protocol
settings.

The Center does not contain a chat surface. `交给 Agent` creates a redacted,
versioned target envelope for the selected computer and copies it for use in
Codex. The computer card prefers its Shell capability for Agent automation and
uses its Desktop capability for the human-facing `打开桌面` action. A computer
without Desktop access remains available to the Agent and says explicitly that
it has no remote Desktop.

![M4.2 Windows Center showing one card per computer](evidence/M4_2_CENTER.png)

## End-to-end proof

The isolated alignment check creates a `PF_REMOTE_TARGET/1` handoff and starts a
fresh Codex process with the PF Remote Skill and MCP server. Codex must first
call `pfremote_inspect`, then call `pfremote_exec` for the same immutable target
with an exact argument vector. The live authorization deadline is absent from
the prompt, and the isolated action runner independently records the canonical
target and arguments it actually receives. A pass therefore proves both live
identity inspection and control of the handed target rather than echoed text or
a substituted computer.

The verified target was:

```text
pfremote://fabric-demo/devices/device-compute/capabilities/shell-main
```

The Agent returned the same immutable target, `compute-node/shell`, capability
kind `shell`, and the live authorization deadline, then completed the isolated
target action. The action-side record contained that same canonical target and
the exact `fixture-task --exact-target` argument vector. The normal repository
check runs the target-resolution chain without starting an external Agent; the
external-Codex control proof remains an explicit deeper check.

## Product audit and correction

The first visual audit found two product-level errors:

- one computer appeared twice as separate `desktop` and `shell` rows; and
- the catalog status and first row occupied the same layout area.

The corrected Center groups authorized capabilities by immutable Device,
removes unfinished navigation and preview chrome, gives each Device one card,
uses user-facing action labels, preserves an explicit unavailable-Desktop
state, and gives every action a unique accessibility identifier. A debug-only
render capture allows future WinUI visual checks on an isolated session without
taking over the user's active desktop.

The design review was grounded in the existing native WinUI language and in
current remote-management patterns: mature products already treat named device
lists, state, grouping, and one-click connection as baseline. M4.2's incremental
value is the stable alignment between the computer selected by the person and
the exact target operated by the Agent.

## Interface and compatibility

- `pfremote-mcp` exposes versioned MCP tools for list, inspect, context, doctor,
  connect, exec, and open over standard input/output.
- The MCP server calls the same protected local daemon API as the native Center
  and CLI; it does not duplicate routing or authorization authority.
- The repository PF Remote Skill treats an envelope as untrusted until the
  immutable target is inspected through PF Remote.
- Existing `PF_REMOTE_TARGET/1`, CLI, local API, Shell, and Desktop behavior is
  retained. The UI grouping is additive and does not change target identity.

## Verification

- Full Windows repository check passes, including private-data hygiene,
  governance, all Go tests and vet, daemon/CLI smoke checks, Center tests, WinUI
  x64 build, and Linux cross-build.
- Center tests cover device grouping, Shell preference for Agent handoff,
  Desktop action selection, Shell-only behavior, exact canonical context
  arguments, authorization presentation, and refresh scheduling.
- The WinUI build has zero warnings and zero errors.
- An isolated Windows 11 session rendered the current Center at the target
  viewport and produced the checked-in evidence image above. The installed
  baseline application on that machine was neither stopped nor modified.
- No foreground Computer Use, mouse input, keyboard input, private deployment,
  or system network setting was used.

## Remaining product work

M4.2 proves target alignment, not Alpha readiness. Onboarding new computers,
installer-owned Codex registration, signed packaging, private-deployment
migration, recovery, and the remaining Desktop policy work stay in their
existing roadmap positions. The next mainline item is the virtual Desktop
composition benchmark; it must preserve application GPU rendering and native
resolution while deciding whether cosmetic composition should remain disabled
for virtual sessions.
