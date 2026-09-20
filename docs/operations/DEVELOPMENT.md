# Development entry points

## Required tools

- Go 1.26 or the version declared by `go.mod`.
- .NET 10 SDK and the Windows App SDK workload for Windows Center work.
- PowerShell 7 on Windows, or a POSIX shell on Linux/macOS.

## One-command checks

```powershell
.\scripts\check.ps1
```

```sh
./scripts/check.sh
```

Checks must be deterministic and avoid the private deployment. Generated
temporary output belongs under `.codex_tmp/` or `artifacts/`, never a user temp
folder on the system drive.

## Codex handoff

Open this repository root as a separate Codex project. Start with the prompt in
`PROJECT_BRIEF.md`. Codex should inspect the branch and worktree, read the
required contracts, run the check script, and continue the first incomplete
roadmap item.

Do not use the legacy troubleshooting conversation as the project memory. A
decision that future work depends on belongs in an ADR, a contract, or the
roadmap. Logs and transient explorations do not.

