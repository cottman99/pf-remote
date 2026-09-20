# M5.2 generic side-by-side observation report

## Product result

PF Remote now has a read-only comparison harness for the moment before a legacy
remote path is retired. It compares the same named computer/action on the old
and new paths and produces one of three plain outcomes: ready, keep the old path
and roll back, or observation incomplete.

It also renders a Chinese or English product-review document that groups named
computer actions by those outcomes. Opaque mapping keys remain secondary
evidence, so a project manager can review the decision without reading JSON or
understanding migration internals.

[Synthetic Chinese review](evidence/M5_2_SYNTHETIC_REVIEW.md)

The comparison requires the old path to be rechecked after each successful new
path test. It refuses readiness when an action is missing, the computer/action
identity differs, the new path fails, revocation differs, the fallback was not
rechecked, timing was not observed, or performance moves from the fast class to
the slow class. A revoked action is ready only when both paths agree it is
revoked.

## Clean-room boundary

Observation files contain only opaque mapping keys, user-facing computer and
action names, Shell/Desktop kind, ready/revoked/unavailable result, and a coarse
fast/normal/slow/not-measured class. They contain no address, route, port,
credential, command, log, process, or service-control field. The harness cannot
connect, install, restart, disable, edit, cut over, or roll back anything; it
only evaluates explicitly supplied redacted evidence.

## Verification

- Positive synthetic evidence proves one Desktop remains ready with fallback
  rechecked and one Shell remains revoked on both paths.
- Negative tests cover candidate failure, revocation mismatch, missing fallback
  recheck, major slowdown, missing target, missing timing, unsupported rollback
  rule, unknown fields, and stale ordering of observations.
- The versioned synthetic comparison is part of both Windows and Unix repository
  checks. The full Windows check passes with all Go, migration, Center, Agent,
  Desktop, and recovery tests.

## Remaining authorization gate

This is generic evidence, not a claim about the private deployment. Closing
M5.2 requires two explicitly named, redacted observation files from the real
legacy and candidate target set, plus permission to observe those paths without
changing services. No private source was opened and no legacy access path was
changed in this work.
