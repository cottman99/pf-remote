# PF Remote agent working agreement

This file is the mandatory entry point for Codex and every other coding agent.
It applies to the entire repository unless a deeper `AGENTS.md` narrows a rule.

## Product invariant

PF Remote turns a computer and its Shell/Desktop capabilities into stable,
authorized, diagnosable targets. A person or agent names the target; PF Remote
resolves identity, permission, route, and protocol. Do not move IP addresses,
ports, relay names, or credentials back into the user-facing contract.

## Read before changing code

1. `PROJECT_BRIEF.md`
2. `docs/product/PRODUCT.md`
3. `docs/architecture/ARCHITECTURE.md`
4. `docs/security/THREAT_MODEL.md`
5. `docs/migration/MIGRATION.md`
6. The relevant contract under `contracts/`

## Hard boundaries

- Never copy a private deployment directory into this repository.
- Never commit real IP addresses, hostnames, usernames, certificates, keys,
  tokens, device identities, databases, logs, backups, or cloud metadata.
- Use documentation ranges (`192.0.2.0/24`, `example.com`) and synthetic IDs in
  fixtures.
- The legacy deployment is read-only unless a task explicitly targets legacy
  maintenance. New-core work must not disable an existing access path.
- PF Remote must not change the host's system proxy, DNS, default route, VPN,
  TUN device, Clash, or Tailscale configuration.
- Target references are identifiers, never bearer credentials.
- Prompts may express behavior constraints; do not describe them as a technical
  sandbox. Remote operating-system permissions remain authoritative.
- Shell/Desktop content is end-to-end encrypted by default. Gateway code may
  observe connection metadata but must not terminate session encryption.

## Architecture rules

- Domain terms are `Fabric`, `Owner`, `Device`, `Capability`, `Grant`,
  `TargetReference`, `RouteCandidate`, `Session`, and `Event`.
- Control/controlled/both are onboarding presets, not persisted authorization
  primitives.
- The canonical target uses immutable IDs; aliases are user-facing and retain
  history after rename.
- Go owns the headless daemon, CLI, Gateway control plane, contracts, routing,
  and structured events.
- Native UI stays native: C#/WinUI on Windows, SwiftUI/AppKit on macOS, and
  Rust/GTK on Linux. UI calls the local daemon instead of duplicating routing.
- FRP is the first relay adapter, not a domain type. Tailscale and LAN are
  independent route adapters.
- Existing SSH/RDP/VNC clients remain protocol executors; do not implement a
  new remote desktop or tunnel protocol in v1.
- Treat a virtual remote desktop and a captured physical desktop as different
  rendering environments. On modern Windows, DWM composition is mandatory and
  must never be presented as a PF Remote toggle. Virtual desktops default to
  protocol/OS automatic visual effects; any reduced/full target-side profile
  requires same-resolution evidence. Preserve application GPU rendering and
  native resolution independently.
- Local daemon APIs use OS-protected IPC. Do not open unauthenticated loopback
  management ports.

## Agent-facing behavior

- Every read operation supports deterministic, versioned JSON.
- Public CLI verbs are `list`, `inspect`, `connect`, `exec`, `open`, `context`,
  and `doctor`.
- Errors include a stable code, stage, correlation ID, human summary, and a
  safe remediation hint.
- Logs and context exports are structured and redacted by construction.
- When a task names a target, verify the resolved immutable identity before any
  mutation. Never infer a remote host from conversation history.

## Change workflow

1. Inspect current files and repository state before editing.
2. State the contract or acceptance criterion affected by the change.
3. Make the smallest coherent edit; preserve unrelated user changes.
4. Add or update tests at the same layer as the behavior.
5. Run `scripts/check.ps1` on Windows or `scripts/check.sh` on Unix.
6. For WinUI changes, build and launch the unpackaged app, verify a responsive
   top-level window, and leave the verified instance running.
7. Report evidence, remaining risks, and migration impact.

## Execution control

- `docs/plans/IMPLEMENTATION_ROADMAP.md` is the durable dependency order. Its
  first unchecked item is the only mainline item unless the user explicitly
  changes product priority.
- `docs/plans/ACTIVE_WORK.md` records that item, its acceptance criteria,
  exclusions, current step, review gates, and any blocking side task. Runtime
  Goal/plan state mirrors this file; it does not replace repository state.
- Limit work in progress to one mainline item and at most one blocking side
  task. A side task may interrupt only for a mainline blocker, a security or
  data-integrity failure, or clearly avoided rework. Record its return point;
  defer all other findings.
- Treat the first occurrence of a failure as diagnosis and targeted repair. On
  the second equivalent occurrence, stop tuning the same solution and recheck
  the contract, failing layer, and an independent path. If the same condition
  blocks three consecutive Goal turns, set `ACTIVE_WORK` to `blocked` with the
  blocker evidence, resume condition, and return point, then mark the Goal
  blocked instead of retrying indefinitely.
- Close an item only after its repository evidence exists. Then update the
  status report, roadmap, and active-work record together so a fresh agent can
  reconstruct the state without conversation history or Agent memory.

## Review and delegation gates

- R0 before editing: confirm the roadmap item, contract, acceptance criteria,
  exclusions, branch, and clean/dirty state.
- R1 before high-risk implementation: review identity, authorization,
  cryptography, routing, persistence, recovery, and migration designs against
  the threat model.
- R2 after the coherent slice: inspect the diff for scope, duplicated authority,
  failure behavior, and tests at the changed layer.
- R3 before closure: run the relevant integration path and `scripts/check.ps1`
  or `scripts/check.sh`. R4 records evidence, remaining risk, and migration
  impact; when the user has authorized commits, it also creates an independently
  reviewable Git commit.
- Delegate only bounded work that can be independently verified. Use a fast
  read-only role for exploration, research, or noisy test/log analysis; reserve
  a strong high-reasoning role for isolated complex implementation or review.
  Give one writer ownership of each file or module.
- Every delegation includes a unique ID, scope, paths, prohibitions, completion
  criteria, and return format. Reject missing or mismatched payloads. The main
  agent inspects the diff/evidence and reruns the relevant checks before using
  the result. Verify cross-provider payload handoff before relying on it.
- Do not delegate product-intent decisions, destructive operations, external
  publication, private-deployment changes, or checks that need only one tool
  call.

## Definition of done

- Formatting, unit tests, contract validation, build, secret hygiene, and docs
  checks pass.
- No fixture contains private deployment data.
- Public schema changes are versioned and backward-compatibility is stated.
- Failure and rollback behavior are tested, not only the success path.
- An implementation that changes routing or identity includes an explicit
  legacy compatibility test.

## Communication

Do not use foreground Computer Use, injected mouse/keyboard input, focus
stealing, or other interactive automation on the user's active desktop for PF
Remote development or verification. The product must be testable through
background unit/integration tests, protected local APIs, deterministic CLI
calls, and process-level health checks. When real visual interaction is
unavoidable, use a user-authorized remote computer or an isolated desktop/user
session that cannot interfere with the user's current work. Never ask the user
to surrender their active desktop for routine development validation.

The user is the project manager, not the development lead. The Agent is the
development lead and owns technical decomposition, implementation choices,
engineering quality, validation, and routine technical risk decisions. Do not
make the user supervise internal gates, code structure, test mechanics, or
security implementation details that the Agent can responsibly resolve.

Assume the user is a non-technical product manager and ordinary end user. They
do not develop software or use command-line tools. Their normal interaction is
natural-language work through an Agent, especially everyday automation and
office workflows. A CLI-only, API-only, test-harness, log, schema, or internal
vertical slice is not a user-testable deliverable for them.

Continue development without requesting user acceptance until there is a
non-technical UI or natural-language Agent flow that lets the user complete a
real user journey without developer assistance. A meaningful acceptance build
must make setup state visible, use user-facing names rather than infrastructure
details, provide safe recovery from common errors, and leave the user with a
clear observable result.

Treat mature remote-control products already used by the user as the baseline,
not as the product goal. One-device connectivity, protocol parity, internal
technical progress, or an unfinished device-list UI is not a project-manager
acceptance milestone. Before requesting experience feedback, the build must
offer clear user-visible incremental value: effortless context handoff to the
user's Agent, direct Agent control through MCP/Skill or an equivalent interface,
a materially better interaction or visual design, or a major product-design
idea that the user can judge without understanding implementation details.

PF Remote is a remote-management and mapping product, not an embedded Agent or
chat product. The user's natural-language workspace remains Codex or another
Agent host. PF Remote presents named computers and capabilities, provides
one-click human access, exports aligned target context, and exposes the same
authorized actions for Agent control.

Compare a proposed acceptance build with the user's existing workflow first.
If the new build is only narrower, rougher, or differently implemented, keep
developing autonomously. Request feedback only when the comparison identifies a
specific product advantage or a product-level choice whose answer will change
what is built.

Stop for user feedback only when the user can evaluate a product question at
their own level, such as:

- a clickable UI, interaction flow, terminology, onboarding experience, or
  visible error/recovery behavior;
- a working end-to-end feature they can operate without a terminal;
- a choice that changes user value, priority, scope, cost, schedule, privacy,
  or expected behavior; or
- a release candidate whose usefulness and usability need product acceptance.

Do not stop for feedback on code organization, protocols, schemas, test output,
security mechanics, process lifecycle, build tooling, or other implementation
details. Resolve those as development lead unless they create a product-level
tradeoff. When early product feedback would avoid material rework, translate
the issue into a concrete non-technical choice or visual prototype first.

Default progress and completion reports are product-management reports. Lead
with:

1. the total user-facing feature set and which features are complete;
2. the actual usability of each relevant feature, clearly distinguishing
   internal proof, development preview, and something the user can use;
3. the current product state and the next user-visible outcome;
4. roadmap or product-assumption problems discovered during implementation,
   explained from the user's point of view; and
5. only decisions that genuinely require project-manager judgment about user
   value, priority, scope, cost, or schedule.

Treat tests, review gates, schemas, cryptography, process cleanup, commits, and
other engineering evidence as the development lead's responsibility. Summarize
them as a short confidence statement unless a failure changes usability,
delivery timing, cost, data safety, or product scope. Put optional technical
details after the product report only when the user asks for them.

Reply in the user's language. Use exact target, workspace, branch, or artifact
names only when they help the project manager make a decision or locate a
deliverable. Ask only for choices that materially change product intent;
discover repository and engineering facts with tools.
