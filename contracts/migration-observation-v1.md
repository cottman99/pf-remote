# Migration observation v1

Side-by-side evidence uses two `pfremote.migration-observation/v1` documents:
one with `source: legacy` and one with `source: candidate`. Each contains an
observation time and a bounded list of the same opaque migration mapping keys,
user-facing computer/action names, Shell/Desktop kind, result, and timing class.

Results are `ready`, `revoked`, or `unavailable`. Timing is deliberately
coarse: `fast`, `normal`, `slow`, or `not-measured`. Raw durations are excluded
because uncontrolled network timing is not a stable migration contract. A
candidate ready target also records whether the old path was rechecked after
the new-path test.

The candidate document must declare the rollback rule
`new-path-failure-revocation-or-major-slowdown`. The comparator requires the
same mapping key, computer name, action name, and capability kind on both sides.
It reports rollback when the candidate fails while legacy works, revocation
differs, the fallback was not rechecked, identity differs, or timing regresses
from fast to slow. A missing side or unusable legacy baseline is incomplete and
cannot authorize cutover.

The output is `pfremote.migration-observation-report/v1` with `ready`,
`rollback-required`, or `incomplete` overall status plus ordinary-language
per-action summaries and stable reason codes. A Chinese or English Markdown
review groups actions into ready, keep-old-path, and more-observation sections;
computer and action names are primary, while opaque mapping keys appear only as
supporting evidence. It is evidence only. The comparator never connects,
installs, restarts, disables, edits, or deletes a service.

Observation documents reject unknown fields and have no fields for addresses,
routes, ports, credentials, commands, logs, process details, private deployment
metadata, or automatic rollback operations. Real private observations and any
service action require separate, exact authorization.
