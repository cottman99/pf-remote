---
name: pf-remote
description: Resolve and control authorized PF Remote computers when the user supplies a PF_REMOTE_TARGET envelope, names a PF Remote target, or asks to inspect, connect to, execute on, or open a managed remote computer.
---

# PF Remote

Use PF Remote as the mapping and control layer. The user chooses a named
computer; PF Remote owns identity, authorization, route selection, and protocol
details.

## Target alignment

- Accept `PF_REMOTE_TARGET/1` envelopes and stable `pfremote://` references.
- Reject unknown envelope major versions.
- Read the exact `target:` value. Do not infer another machine from conversation
  history, an address, a route name, or a remembered alias.
- A visible computer name is not itself a canonical target reference. When the
  user names a computer without supplying a `pfremote://` reference or target
  envelope, call `pfremote_list` first. Match the name case-insensitively against
  the returned Device display name or alias, require exactly one matching
  Device, then choose the requested capability on that Device. Prefer its
  available Shell capability for automation. Never pass the visible name
  directly to `pfremote_inspect`, `pfremote_exec`, or another action tool.
- Before any remote mutation, run `pfremote inspect <target> --json` and confirm
  that the returned canonical target exactly matches the requested target.
- A computer may expose several Desktop capabilities. When the user names a
  specific Desktop, inspect and use that exact capability. When the handoff is
  for the computer and a task needs a sibling Desktop, call `pfremote_list`,
  keep only capabilities with the same immutable Device ID, and use the
  user-facing Desktop name. If more than one still matches, ask which visible
  Desktop they mean; never pick one by address, route, or list position.
- If neither a canonical reference nor a visible name was provided, use
  `pfremote_list` and ask only when more than one plausible authorized target
  remains. If a visible name matches none or more than one Device, report that
  plainly and ask the user to choose from the visible names; do not guess.

## Actions

Prefer the PF Remote MCP tools when the Agent host exposes them. They use the
same protected action core and do not depend on shell execution policy:

- `pfremote_list`, `pfremote_inspect`, `pfremote_context`, `pfremote_doctor`
- `pfremote_connect`, `pfremote_exec`, `pfremote_open`

If MCP is not available, invoke the installed `pfremote` command directly.
Packaged PF Remote exposes this command to Agent processes. If a development
checkout has not exposed it, use `scripts/pfremote.ps1` in this Skill as a
local fallback.

- Inspect availability or identity with `pfremote inspect <target> --json`.
- Diagnose the local installation with `pfremote doctor --json`.
- Verify a Shell connection with `pfremote connect <target> --json`.
- Execute an argument vector with `pfremote exec <target> <program> [args...] --json`.
- Open a Desktop capability with `pfremote open <target> --json`.

Use the command appropriate to the resolved Capability kind. Preserve the
user's authorization boundaries and obtain confirmation immediately before any
destructive or externally consequential action when it was not already clearly
requested.

Treat structured PF Remote errors as authoritative. Report the human summary
and safe remediation without exposing addresses, ports, credentials, private
paths, relay metadata, or raw environment data. Never change the host's proxy,
DNS, default route, VPN, TUN, Clash, or Tailscale configuration.
