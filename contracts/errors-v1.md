# Error contract v1

Machine-readable failures use this shape:

```json
{
  "schema_version": "pfremote.error/v1",
  "code": "TARGET_NOT_FOUND",
  "stage": "resolve",
  "correlation_id": "opaque-id",
  "summary": "The requested target is not in the current catalog.",
  "remediation": "Run pfremote list or refresh the catalog."
}
```

`summary` and `remediation` are safe to copy. They must not contain secrets or
raw environment dumps. Codes and stages are stable within major version 1.

