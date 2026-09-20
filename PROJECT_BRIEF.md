# PF Remote project brief

## Objective

Choose a named computer, open its exact Desktop, or hand the same authorized
target to an external Agent. Existing SSH/RDP/VNC clients perform the connection.

## Current phase

Current milestone: M10
Current roadmap item: M10.3

The owner replaced the old expansion roadmap with a focused closeout.
See docs/plans/ACTIVE_WORK.md and docs/status/CLOSEOUT_REPORT.md.
Historical feature and rollout records are evidence, not current fleet health.

## Retained boundaries

- Windows Center plus existing Windows/Linux controlled nodes.
- One identity and authorization core shared by UI and MCP/Skill.
- Existing independent LAN, Tailscale and Gateway paths; no host network changes.
- Exact Desktop selection is stable; only routes may fall back automatically.
- Private deployment values remain in the protected local runtime outside Git.
- Source distribution is separate from internal installers and private evidence.
- Native executors, protected IPC, target authentication and rollback remain.

## Removed from the active product plan

Automatic wake certification, other native platform frontends, browser remote
desktop and repeated cosmetic-only releases. Recovery and compatibility code
remain because current deployments depend on them; they are not expansion work.

Current candidate: alpha.102. Two Windows installations and one Linux node have
independently updated from the public preview feed. Settings includes Check this
computer, Notify other computers, and named version/last-seen reports. Offline
notification catch-up is verified. Private configuration remains outside releases.
The fourth computer's management connection is unavailable; its rollout and owner
UI acceptance remain open. See docs/status/FLEET_UPDATE_REPORT.md. Native-window
health is verified without foreground interaction; user visual acceptance is pending.
