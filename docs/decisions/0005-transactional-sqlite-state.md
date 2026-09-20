# ADR 0005: Transactional SQLite state and last-valid recovery

- Status: accepted
- Date: 2026-08-18

## Decision

Use `database/sql` with `modernc.org/sqlite` for the local catalog, Grant, and
enrollment control state. The driver is pure Go, supports the repository's
Windows, Linux, and macOS targets, and avoids adding a host C toolchain or
runtime DLL requirement.

Catalog updates are immutable, normalized snapshots. One transaction inserts a
staging snapshot and all Device, Capability, and Grant rows, marks it committed,
then switches the active pointer. Five committed snapshots are retained so
startup can skip a structurally invalid active candidate and recover the newest
valid prior candidate.

Enrollment mutations use an atomically replaced, versioned control-state row in
the same database. The serialized state contains public metadata, activation
code hashes, and replay identifiers, never raw codes or private key material.
Device active/revoked transitions atomically update a normalized revocation and
control-version overlay in the same transaction. The daemon applies this overlay
when loading a snapshot, without changing its capture timestamp. Persistence
failure makes the live enrollment manager fail closed.

SQLite runs with WAL, full synchronous durability, foreign keys, a bounded busy
timeout, strict tables, and startup integrity checks. Database and sidecar files
are forbidden repository content.

## Consequences

- A refresh failure cannot partially replace the active catalog.
- Restart preserves activation approvals, revocation, versions, rate state, and
  signed request replay detection.
- A successful local Gateway revocation is visible to the next daemon action;
  subject and target Device checks consume the same state.
- Readers observe one committed catalog version; stale rows are not combined
  across versions.
- A corrupt active logical snapshot can recover from retained valid history;
  complete database corruption still fails closed and requires explicit repair.
- Remote snapshot authentication remains a transport/control-plane obligation
  before a payload reaches this local commit boundary.

## Dependency review

- `modernc.org/sqlite` v1.56.0 is the direct dependency reviewed for this
  decision. Its module metadata declares BSD-3-Clause, it is maintained, and it
  supports the required operating systems without CGO.
- Transitive Go modules are pinned in `go.sum`. Complete distributable license
  inventory and SBOM generation remain the M5 release gate.

## Primary references

- [SQLite atomic commit](https://www.sqlite.org/atomiccommit.html)
- [SQLite write-ahead logging](https://www.sqlite.org/wal.html)
- [SQLite PRAGMA reference](https://www.sqlite.org/pragma.html)
- [`modernc.org/sqlite` package](https://pkg.go.dev/modernc.org/sqlite)
