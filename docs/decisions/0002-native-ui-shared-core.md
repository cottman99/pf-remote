# ADR 0002: Native UI with one shared headless action core

- Status: accepted
- Date: 2026-08-17

## Decision

Go implements the daemon, CLI, Gateway, resolver, routing, and event contracts.
Each desktop platform may use its native UI toolkit, beginning with C#/WinUI 3
on Windows. All interfaces call the same daemon actions over protected local IPC.

## Consequences

The Windows application must not duplicate authorization or routing rules. CLI
and JSON contracts are first-class interfaces. Native UX can evolve without
fragmenting the security or target model.

