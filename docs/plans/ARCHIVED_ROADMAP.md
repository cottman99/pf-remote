# Historical roadmap (superseded)

Frozen for evidence; unchecked work is deferred, not completed.

# Implementation roadmap

The first unchecked milestone is the default starting point for a new Codex
task. A milestone is checked only when its evidence is present in the repository.

## M0 — clean-room bootstrap

- [x] Product, architecture, Agent, threat, migration, and ADR contracts exist.
- [x] Repository-level `AGENTS.md` and durable `PROJECT_BRIEF.md` exist.
- [x] Go format, tests, builds, contract checks, and private-data hygiene pass.
- [x] The official unpackaged WinUI project builds and launches.

Evidence: [Bootstrap report](../status/BOOTSTRAP_REPORT.md).

## M1 — deterministic local vertical slice

- [x] Canonical targets and aliases parse and resolve from a synthetic catalog.
- [x] `list`, `inspect`, `context`, and `doctor` provide versioned JSON.
- [x] Windows Center lists the same synthetic targets and copies an envelope.
- [x] Local daemon exposes the shared actions over protected IPC.

Evidence: [Bootstrap report](../status/BOOTSTRAP_REPORT.md).

## M2 — identity, catalog, and grants

- [x] Device Ed25519 identity and OS-protected storage.
- [x] Owner initialization, device-code activation, revocation, and version sync.
- [x] SQLite catalog/Grant state with atomic last-valid snapshots.
- [x] Seven-day cached authorization behavior and expiry UI.

Evidence: [M2 implementation report](../status/M2_REPORT.md).

## M3 — encrypted Shell journey

- [x] End-to-end encrypted and target-authenticated Shell session.
- [x] Gateway-primary route through the FRP adapter.
- [x] Independent Tailscale and LAN candidates with route diagnostics.
- [x] `connect` and `exec` verify immutable target identity.

Evidence M3.1: [M3.1 report](../status/M3_1_REPORT.md).
Evidence M3.2: [M3.2 report](../status/M3_2_REPORT.md).
Evidence M3.3: [M3.3 report](../status/M3_3_REPORT.md).
Evidence M3.4: [M3.4 report](../status/M3_4_REPORT.md).
Evidence: [M3 completion report](../status/M3_4_REPORT.md).

## M4 — Desktop and recovery

- [x] RDP and VNC executors with capability-accurate labels.
- [x] Agent alignment: the native Center selects a named target and hands its
      safe context to Codex, while MCP/Skill or an equivalent Agent interface
      resolves and controls the exact same authorized Device and Capability.
- [x] Platform-accurate virtual Desktop visual-effects policy preserves native
      resolution and application GPU rendering, isolates physical-console
      policy, and requires same-resolution evidence only for supported target
      profiles instead of fabricating a modern-Windows DWM toggle.
- [x] Multiple named Desktop capabilities per Device.
- [x] Encrypted Owner recovery bundle and tested Gateway restore.
- [x] Optional web-emergency profile evaluated and deliberately deferred; a
      future mature clientless-Gateway adapter must consume the same Grants.

Evidence M4.1: [M4.1 report](../status/M4_1_REPORT.md).
Evidence M4.2: [M4.2 report](../status/M4_2_REPORT.md).
Evidence M4.3: [M4.3 report](../status/M4_3_REPORT.md).
Evidence M4.4: [M4.4 report](../status/M4_4_REPORT.md).
Evidence M4.5: [M4.5 report](../status/M4_5_REPORT.md).
Evidence M4.6: [M4.6 product decision](../status/M4_6_REPORT.md).
Evidence: [M4 completion decision](../status/M4_6_REPORT.md).

## M5 — migration and Alpha gate

- [x] Read-only compatibility import and private mapping procedure.
- [x] Side-by-side observation and rollback complete on the private deployment.
- [x] Internal Windows installer, SBOM, license inventory, upgrade rollback,
      and published hashes; public distribution is source-only.
- [x] Clean-environment fifteen-minute golden journey passes.

Evidence M5.1: [M5.1 report](../status/M5_1_REPORT.md).
Evidence M5.2: [private observation, bounded connection, and rollback report](../status/M5_2_READ_ONLY_OBSERVATION.md).
Evidence M5.3: [internal Windows installation report](../status/M5_3_REPORT.md).
Evidence M5.4: [clean-environment golden-journey report](../status/M5_4_REPORT.md).
Evidence: [M5 completion report](../status/M5_4_REPORT.md).

## M6 — private replacement rollout

- [x] Current private catalog imports every supported action, including the legacy external Desktop executor, without changing legacy state.
- [x] The local Windows controller and one headless Linux workstation are enrolled with their private user-facing names and survive process restart.
- [x] Every existing action on the currently online Windows controller and Linux workstation passes side-by-side identity, availability, and fallback checks.
- [x] Restart, network loss, recovery, and a sustained observation window pass for the currently online controller and Linux workstation with old access paths retained.
- [x] The currently online Linux virtual Desktops open from the Center without a separate viewer password prompt after one user-facing setup step; credentials remain private to the current Windows user and never enter target references, context exports, logs, or repository state.
- [x] The Windows Center provides clear Computers, Sessions, and Settings & Recovery destinations with visible setup and common-error recovery state.
- [x] The installed internal candidate owns its normal startup, update, and one-action rollback lifecycle without requiring command-line assistance.
- [x] A real UI-to-Agent journey passes on the currently online Windows controller and Linux workstation before the offline computer returns.
- [x] Every Desktop connect action uses a split button: the primary action makes
      an explainable smart choice with a stable fallback order, while its
      adjacent menu exposes the real availability of LAN, Tailscale, and
      Gateway paths for that computer and that connection.
- [x] The Computers page presents one dominant Smart connect action per computer,
      keeps alternate Desktops behind progressive disclosure, treats Agent
      handoff as a secondary action, and remains clear at wide and narrow widths.
- [x] The Computers page faithfully implements the selected visual direction:
      compact device rows, device/status iconography, working search and filter,
      an expanded Desktop list with recent-use context, and source-matched density.
- [x] The restored Windows controlled computer is enrolled from its own identity after the owner reports it online, with its existing user-facing name and old-version rollback path preserved.
- [x] The restored Windows controlled computer's Shell and physical Desktop pass
      the same checks without removing the untouched old-version fallback.
- [x] The internal candidate becomes the default PF Remote entry; rollback to
      the untouched old version remains one user-level action.
- [x] A real UI-to-Codex replacement journey passes across all three computers,
      closing replacement parity.
- [ ] Sleep and cross-device recovery checks pass across all restored computers.

Evidence M6.1: [current private catalog compatibility report](../status/M6_1_REPORT.md).
Evidence M6.2: [online-computer enrollment report](../status/M6_2_REPORT.md).
Evidence M6.7: [installed lifecycle report](../status/M6_7_REPORT.md).
Evidence M6.8: [online UI-to-Agent journey report](../status/M6_8_REPORT.md).
Evidence M6.9: [mature connection surface and UX audit](../status/M6_9_REPORT.md).
Evidence M6.10: [focused Computers experience](../status/M6_10_REPORT.md).
Evidence M6.11: [faithful Computers visual implementation](../status/M6_11_REPORT.md).
Evidence M6.12: [restored Windows identity enrollment report](../status/M6_12_REPORT.md).
Evidence M6.13: [restored Windows Shell and physical Desktop report](../status/M6_13_REPORT.md).
Evidence M6.14: [default entry and old-version fallback](../status/M6_15_THREE_END_REPLACEMENT_REPORT.md).
Evidence M6.15: [real routes and three-end Agent parity](../status/M6_15_THREE_END_REPLACEMENT_REPORT.md).
Evidence M6.16 interim: [three-Windows client replacement](../status/M6_16_THREE_WINDOWS_CLIENT_REPLACEMENT.md).
Evidence M6.3: [online-action parity report](../status/M6_3_REPORT.md).
Evidence M6.4: [online-scope reliability report](../status/M6_4_REPORT.md).
Evidence M6.5: [one-click Linux Desktop report](../status/M6_5_REPORT.md).
Evidence M6.6: [Center information architecture report](../status/M6_6_REPORT.md).
