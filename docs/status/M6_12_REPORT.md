# M6.12 restored Windows identity enrollment report

## Product outcome

The restored Windows controlled computer is enrolled from its own protected
Device identity and appears under its existing private user-facing name in the
installed Center. The repository records no private name, address, account,
node identity, credential, or deployment path.

The old Agent, old Center, and old relay remain installed and running. The new
identity binding affects only the candidate catalog, so the old route remains
an immediate fallback.

## User-visible evidence

- The restored computer appears online beside the Linux controlled computer.
- Its Shell and physical-screen capabilities resolve to the same immutable
  Device identity from the Center, CLI, context handoff, and MCP.
- Restarting the candidate service preserves the name, authorization, and
  catalog while the old product continues running.

## Confidence

Enrollment approval, signed capability synchronization, restart recovery,
candidate rollback, full repository checks, and private-data hygiene pass.
Public source and reports contain only synthetic or role-based identifiers.

## Migration impact

No legacy file, service, connection, or credential was changed. Removing the
candidate binding returns the product to the read-only legacy projection; the
old product remains independently usable.
