# ADR 0001: Stable target references are the product baseline

- Status: accepted
- Date: 2026-08-17

## Decision

PF Remote is modeled around Device identities and Shell/Desktop Capabilities.
Users and agents exchange stable `pfremote://` references. Addresses, ports,
gateways, protocols, and route credentials stay behind adapters.

## Consequences

The catalog, grants, alias history, resolver, CLI, UI, migration, and diagnostics
must all use immutable capability identity. Protocol-first UI or configuration
is considered an advanced diagnostic surface, not the primary product model.

