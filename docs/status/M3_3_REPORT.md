# M3.3 implementation report — independent routes and diagnostics

Date: 2026-08-29

## Product outcome

PF Remote can now evaluate Gateway/FRP, Tailscale, and LAN as independent ways
to reach the same stable computer. Gateway remains preferred. If it is not
available before a Session starts, a permitted alternative can be selected
without changing the target identity. A new Session evaluates routes again.

This is enabling infrastructure, not a user-testable feature. No command-line
or technical acceptance is requested from the product manager. M3.4 must now
turn the completed Shell path into an action consumable by an Agent and UI.

## Evidence

- Deterministic preference, pre-Session fallback, cancellation, no-route
  failure, invalid candidates, Tailscale/LAN candidates, diagnostic redaction,
  and fresh-Session reselection are covered by tests.
- Diagnostics expose adapter status, stable reason, and coarse latency only;
  they do not copy private endpoints or raw failures.
- Providers are read-only and cannot alter proxy, DNS, routes, VPN/TUN, Clash,
  or Tailscale state.
- `scripts/check.ps1` passed all repository checks, 9 Windows Center tests, and
  the WinUI build with zero warnings and zero errors.

## Git evidence and next boundary

- `4d6a3cd` — deterministic selector, independent endpoint providers, safe
  diagnostics, Session pinning, and negative tests.

M3.4 is the next user-value step: expose verified `connect` and `exec` actions
for natural-language Agent and UI consumption. No private deployment or system
network configuration was accessed or modified.
