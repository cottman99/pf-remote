# M5.1 read-only migration planning report

## User-visible result

PF Remote now has a deterministic migration planner for an explicitly supplied,
redacted legacy inventory. It turns infrastructure-shaped input into the
product concepts the Owner can review: named computers, their Shell/Desktop
abilities, existing permission relationships, and visible naming conflicts.

Several legacy paths to the same ability are represented as one capability
with a path count. Addresses, ports, relay names, VPN details, and credentials
do not appear in the accepted input or migration plan. Duplicate computer names
are disambiguated predictably and surfaced as review warnings rather than
silently overwriting one another.

## Safety and coexistence

Planning accepts one explicitly selected regular JSON file, never scans a
directory, rejects links and files above 1 MiB, rejects unknown or secret-shaped
fields by schema, and writes only its versioned result to standard output. It
does not create identities, alter a Gateway, write beside the source, or disable
an old access path.

The repository now documents the exact private authorization gate: the Owner
must name the source and isolated redacted-export destination before any private
inventory is read. The private mapping remains outside this repository, and
apply/cutover is a later explicit action.

## Verification

- Synthetic fixtures cover two same-named computers, three capabilities,
  multiple connection paths collapsed behind one capability, and an active
  cross-device Grant.
- Tests cover deterministic output across input ordering, alias collision,
  dangling Grant rejection, unknown password/address/route field rejection,
  bounded input, regular-file enforcement, and proof that planning writes
  nothing beside the source.
- Windows and Unix repository checks build the planner and compare two fresh
  plans byte-for-byte. The full Windows check passes alongside Agent, Desktop,
  recovery, and Center coverage.

## Remaining private gate

No private deployment was read or modified. The generic planner and procedure
are complete; M5.2 will add generic side-by-side observation and rollback
evidence. Running either workflow against the real legacy deployment still
requires explicit source-specific authorization.
