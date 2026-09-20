# Cached authorization v1

PF Remote permits an already authorized Device to use a last-valid local
catalog for at most seven days after the snapshot `captured_at` timestamp. A
Grant may set an earlier `valid_until`; it can never extend the snapshot cache
window. A missing or revoked subject Device and revoked, unknown, or expired
Grants provide no authority.

The effective validity boundary for a target is:

```text
min(snapshot.captured_at + 7 days, grant.valid_until when present)
```

The interval is half-open. Authorization is active while `now < valid_until`
and expired when `now >= valid_until`. Restarting PF Remote or failing a refresh
does not move the boundary. A persisted local high-water clock prevents an
observed later time from moving backward across daemon restarts; a clock-state
write failure expires authority rather than extending it.

Expired targets are absent from `list`, cannot resolve by alias or canonical
reference, and therefore cannot produce a context envelope. Catalog JSON and
each returned target include:

```json
{
  "authorization": {
    "status": "active",
    "valid_until": "2026-08-25T12:00:00Z",
    "remaining_seconds": 604800
  }
}
```

`status` is `active` or `expired`. `remaining_seconds` is nonnegative and is
zero at and after expiry. `valid_until` is an absolute RFC 3339 timestamp.

Windows Center presents an active status, changes to a warning with 24 hours or
less remaining, and presents an error after expiry. Each visible target also
shows its own effective expiry. All status and remediation text is localized,
announced through UI Automation live regions, and does not include raw paths or
backend exception text.
Center schedules a daemon refresh at the 24-hour threshold and exact expiry
boundary; an expired or temporarily unavailable catalog is retried at a bounded
interval so a long-running window cannot retain stale target actions
indefinitely.
