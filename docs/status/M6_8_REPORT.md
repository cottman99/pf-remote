# M6.8 online UI-to-Agent journey report

## Product result

The central PF Remote experience now works against the currently online Linux
workstation from the installed Windows controller. The user can choose the
named computer in Center, use “交给 Agent”, and let the external Agent act on
that exact computer. The completed action then appears in Center's Sessions
view.

This is the intended product shape: PF Remote remains the computer map and
control layer, while Codex remains the natural-language workspace. No embedded
chat surface was added.

## Actual usability and evidence

- Center generated a `PF_REMOTE_TARGET/1` envelope from its real Agent button.
- The Agent inspected the handed immutable target before acting; the resolved
  target exactly matched the UI handoff.
- A harmless real action completed on `eda-server Linux` with exit code zero.
- Center refreshed Sessions when opened and showed `eda-server Linux`,
  `Agent 自动化操作`, and `已完成`.
- Isolated UI automation verified the Agent button, target-context export, and
  visible Sessions result without foreground input or a terminal in the user
  journey.
- The session record contained no command, output, address, port, Gateway,
  route, credential, or Desktop content.

Visual evidence is retained outside the public repository as
`PFRemote-UI-Agent-Journey-Linux-alpha24-20260830.png`.

## Product issue discovered and corrected

The first installed lifecycle pass restarted a private TLS Gateway without its
deployment arguments, temporarily returning the Linux Agent action to
“正在接入”. The lifecycle now uses a protected local Gateway startup profile,
preserves listener and TLS configuration across version switches, and refuses
to invent an unconfigured private Gateway. After a real `alpha.24` to
`alpha.25` upgrade, connection synchronization returned to ready and the Linux
Shell action returned to available.

## Current product state

Internal candidate `0.1.0-alpha.25` is installed with `alpha.24` retained for
one-action rollback. The old product remains installed and its startup entry is
unchanged. The intentionally offline Windows Home client entry remains visible but was not
contacted.

The next roadmap item requires the owner to report Windows Home client online. No meaningful
offline substitute can prove its own protected Device identity, Shell, or
physical Desktop.
