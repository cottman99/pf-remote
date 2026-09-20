# ADR 0006: Seven-day cached authorization with fail-closed expiry

- Status: accepted
- Date: 2026-08-18

## Decision

Bound offline use of a committed catalog snapshot to seven days from its fixed
capture timestamp. For each Capability, the effective expiry is the earlier of
that cache boundary and the authorizing Grant's optional expiry. At the exact
boundary, the target is no longer enumerable or resolvable.

Persist a last-observed local time high-water mark in SQLite. Authorization uses
the later of the current wall clock and this high-water mark so setting the
clock backward after it has advanced cannot restore expired authority. A
failure to update this state returns the snapshot expiry boundary and therefore
fails closed.

Expose effective authorization status and absolute expiry in the shared JSON
contract. Windows Center remains a view: it uses the core result, shows a
warning for the final 24 hours, an error after expiry, and the effective expiry
on each returned target. It schedules core refreshes at the warning and expiry
boundaries and performs bounded retries after expiry or failure. It does not
implement a second authorization policy.

## Consequences

- Restart and refresh failure never renew cached authority.
- A forward clock jump can expire authority early; a later rollback cannot
  reauthorize it. Repair requires a valid refresh, not a local grace bypass.
- Expired targets cannot be copied into new Agent envelopes.
- The UI gives both visible and UI Automation-readable notice before and after
  expiry without exposing backend exception details.
- Secure time from an authenticated remote catalog remains a later transport
  concern; this milestone hardens the local cache boundary.

## Implementation and test references

- [.NET time-dependent testing guidance](https://learn.microsoft.com/dotnet/core/extensions/timeprovider-testing)
- [WinUI AutomationProperties](https://learn.microsoft.com/windows/windows-app-sdk/api/winrt/microsoft.ui.xaml.automation.automationproperties)
- [Windows app accessibility overview](https://learn.microsoft.com/windows/apps/design/accessibility/accessibility-overview)
- [WinUI localization](https://learn.microsoft.com/windows/apps/winui/winui3/localize-winui3-app)
- [MSTest SDK guidance](https://learn.microsoft.com/dotnet/core/testing/unit-testing-mstest-sdk)
- [Windows UI Automation elements](https://learn.microsoft.com/windows/win32/winauto/uiauto-obtainingelements)
