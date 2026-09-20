# M5.4 clean-environment golden-journey report

Status: complete on 2026-08-30. This closes the current roadmap and the
source-only Alpha gate; Windows binaries remain internal test candidates.

## User-visible result

An ordinary Windows user can open PF Remote and see named computers instead of
addresses, gateways, or protocols. On one computer card they can open a named
Desktop with one click or hand that same computer to their external Agent.
PF Remote does not embed a chat interface: Codex remains the natural-language
workspace and receives a safe, exact target reference.

The final isolated journey performed two user actions:

1. `打开独立桌面` opened the selected computer's Desktop capability.
2. `交给 Agent` exported the same computer's Agent context; real Codex inspected
   it and executed through PF Remote MCP on the matching immutable Device.

The real-Codex journey completed in 42.153 seconds, comfortably inside the
fifteen-minute product bar. Both buttons were keyboard focusable, the native
window remained responsive, and the visible result used user-facing computer
and Desktop names. The final UI is captured in
[M5_4_GOLDEN.png](evidence/M5_4_GOLDEN.png); the machine-readable result is
[M5_4_GOLDEN.json](evidence/M5_4_GOLDEN.json).

## Product issue found and corrected

The first automated user run exposed a real UI fault: a Desktop button could be
visibly enabled yet fail to carry the selected computer identity into its click
handler. The native UI now attaches the exact canonical target to each Desktop
and Agent button, then resolves the visible card from that target. The repeated
journey proves that the visible button, Desktop action, context handoff, and
Agent action stay aligned to one Device.

## Usability and recovery state

- A clean user reaches a useful two-computer list without a terminal or manual
  daemon startup.
- Available Desktops are obvious actions; computers without a ready Desktop
  remain visible with a disabled, explanatory action rather than disappearing.
- Authorization, connection-service recovery, backup/restore, migration
  preview, and one-click legacy rollback remain user-facing flows already
  evidenced by M4.5 and M5.2.
- Infrastructure details stay out of the main computer list and are available
  only through diagnostics when recovery needs them.

## Confidence and boundaries

The full project check passes: private-data hygiene, governance, Go behavior,
25 Windows presentation tests, deterministic invitation reload, the isolated
UI helper, and the WinUI build with zero warnings or errors. The golden journey
used no mouse or keyboard injection on the active desktop, no private target,
no credential, no system-network change, and no legacy mutation.

No Windows binary was published. Public distribution contains source code only;
the internal unsigned installer remains a reversible side-by-side test artifact.
