# M6.9 mature connection surface and UX audit

## Product outcome

The Windows Center now treats connection paths as a per-computer action rather
than a global setting. Every eligible Desktop has one native split button:

- the primary action is **Smart connect**;
- the adjacent menu offers **LAN direct**, **Tailscale private network**, and
  **Alibaba Cloud Gateway relay** for that exact Desktop;
- the fixed Smart connect order is LAN, Tailscale, then Gateway, with an
  unreachable endpoint skipped before the remote application starts; and
- a manual choice affects only that one connection and uses the same
  identity-bound Session core used by Smart connect and Agent actions.

The new candidate reuses the already-running legacy Alibaba Cloud FRP visitors
only after an exact private identity match. It does not edit the legacy
configuration, start or stop its service, or remove its fallback.

## UX audit

The three-destination information architecture is appropriate for the product:
**Computers** owns target choice and connection, **Sessions** owns recent human
and Agent activity, and **Settings & Recovery** owns setup and lifecycle. Route
choice correctly remains beside each connect button, not in Settings.

The main usability gap found in the baseline was structural rather than visual:
several equally weighted Desktop buttons gave no indication that connection
paths could be automatic or deliberately selected. The native split action now
makes that distinction visible, preserves keyboard accessibility, and wraps
without clipping at the audited window size. The offline computer explicitly
states that it is waiting for that computer and does not block other computers.

The current visual system is coherent and usable, but not a final brand-design
pass. Computer aliases inherited from the old setup can still look technical,
and Sessions is intentionally sparse until more real activity exists. Those do
not block the route-selection journey or require Windows Home client.

## Evidence

- [final Computers screen](evidence/m6-ui-audit-20260830/06-computers-final.png)
- [Sessions screen](evidence/m6-ui-audit-20260830/02-sessions.png)
- [Settings & Recovery screen](evidence/m6-ui-audit-20260830/03-settings.png)
- [route-menu keyboard automation](evidence/m6-ui-audit-20260830/route-menu-ui.json)

Installed internal candidate `0.1.0-alpha.26` reports all four currently online
Linux Desktop actions as selectable through LAN, Tailscale, and Gateway. The
real existing Gateway visitor for an online Linux VNC target passed a bounded
reachability check. The full repository check, native tests, responsive
isolated launch, release verification, and private-data check pass.

## Remaining boundary

Windows Home client is still offline, so its own identity enrollment and an actual
cross-device Desktop window remain unverified. This does not reduce the client
surface completed here; it only gates the next device-specific roadmap item.
