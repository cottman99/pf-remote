# PF Remote

PF Remote is a self-hosted personal compute fabric for AI-augmented technical
work. It turns each computer and its Shell/Desktop capabilities into stable,
authorized targets that people and agents can use without knowing IP addresses,
ports, gateways, or transport protocols.

> Name your computers once. Reach them anywhere. Hand them safely to agents.

## Status

This repository is the clean-room next-generation core. It is intentionally
separate from every private deployment. The current milestone is a contract-first
sidecar that can describe and resolve targets without replacing a working remote
access path.

The first golden journey is:

1. Deploy a self-hosted Gateway in about 15 minutes.
2. Enroll and name two devices.
3. Confirm their Shell/Desktop capabilities.
4. Copy a stable target from the Windows Center.
5. Let Codex run `inspect`, verify `hostname`/`whoami`, then `exec` without
   receiving an IP address or protocol configuration.

## Repository map

- `cmd/` — Go executables: `pfremote`, `pfremoted`, and `pfgateway`.
- `internal/` — domain, target reference, catalog, routing, event, and store logic.
- `contracts/` — versioned JSON schemas and protocol examples.
- `apps/windows/` — native C#/WinUI Center (unpackaged for development).
- `docs/` — product, architecture, security, migration, evidence, and handoff.
- `scripts/` — deterministic developer and hygiene checks.

## Development

Read [AGENTS.md](AGENTS.md) and [PROJECT_BRIEF.md](PROJECT_BRIEF.md) before making
changes. The shortest verification loop is:

```powershell
./scripts/check.ps1
```

The current clean-room core implements protected Device identity, signed Device
enrollment and revocation, transactional catalog/Grant snapshots, bounded
seven-day cached authorization, daemon-backed `list`, `inspect`, `context`, and
`doctor` actions, protected local IPC, and a launchable Windows Center with
expiry status. Start `pfremoted` before using the CLI or Center; neither client
creates an independent authorization view. Encrypted routes, sessions,
recovery, release artifacts, and private deployment migration remain
milestone-gated.

PF Remote is licensed under Apache-2.0. It is not yet a public Alpha.
