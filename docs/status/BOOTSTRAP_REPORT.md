# Bootstrap report — 2026-08-18

## Outcome

The clean-room PF Remote repository is independently buildable and contains a
durable Codex handoff, public contracts, a tested Go vertical slice, protected
local IPC, a development Gateway API, and an unpackaged/self-contained Windows
Center that lists the shared synthetic catalog and copies contract-compliant
Agent envelopes. No private deployment data was imported and no legacy service
was changed.

## Verified evidence

- `scripts/check.ps1`: passed.
- Go unit and integration tests: passed, including a real protected IPC
  round-trip on the Windows Named Pipe.
- `go vet ./...`: passed.
- CLI smoke test: versioned JSON returned from `pfremote list --json`.
- Windows Center: Debug x64 build passed with zero warnings and zero errors.
- Windows Center launch: process remained alive and responsive after startup;
  a nonzero top-level window handle and title `PF Remote 控制中心` were observed.
- Windows Center catalog: the live accessibility tree exposed the same three
  targets, aliases, canonical URIs, and capability IDs as the Go action core.
- Windows Center copy interaction: all three `Copy for Agent` buttons produced
  five-line `PF_REMOTE_TARGET/1` envelopes. The clipboard results for
  `compute-node/desktop`, `compute-node/shell`, and `workstation/shell` matched
  `pfremote context <canonical-target> --json` exactly.
- Private-data hygiene check: passed.

## Toolchain notes

- Go, .NET 10, Git, WinGet, Developer Mode, and a WinUI template are present.
- The bundled WinUI prerequisite bootstrap found Visual Studio but its optional
  component-modification step returned installer error `-2147024784`. The
  actual WinUI build and launch succeeded, so this does not block the current
  development slice.
- The installed official `winui` template only exposed single-project MSIX
  scaffolding. The generated official project was therefore converted
  explicitly to `WindowsPackageType=None` with a self-contained Windows App SDK
  runtime. A signed per-machine installer remains a release milestone.

## M1 Center evidence

The live Center window displayed three synthetic targets and exposed each copy
button through Windows UI Automation with its capability ID. A screenshot and
accessibility-tree inspection confirmed the visible catalog. The three actual
clipboard results were then compared with the Go action core and the
`contracts/target-envelope-v1.md` field order. This closes the final M1 roadmap
item without connecting to a private deployment.

## Next action

The first remaining roadmap item is M2 Device Ed25519 identity and OS-protected
storage. Replacing the bootstrap CLI process adapter with the protected local
daemon client remains a separate follow-up that must preserve the verified
public UI behavior.
