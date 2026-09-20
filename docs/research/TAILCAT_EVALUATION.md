# Tailcat fit for PF Remote

Status: retain as a future optional route adapter; do not make it a current
dependency or a user-facing product concept.

## What it changes for the user

Tailcat can create an encrypted, NAT-traversing connection for a short-lived
task without requiring either side to join a tailnet or complete an account
login. That makes it a promising future answer to a narrow PF Remote journey:
temporarily give one named computer to an Agent for one task, then let the
access disappear.

It does not provide the durable computer list, user/device identity, policy,
auditability, or lifecycle management that PF Remote needs. A Tailcat address
is possession-based access material and must never become a TargetReference,
appear in Agent context, logs, screenshots, or source control.

## Product decision

- Keep the current Tailscale, LAN, and Gateway work as the M5.2 mainline.
- Later evaluate Tailcat behind the existing route-provider boundary as an
  ephemeral route candidate, not as a domain type or replacement UI.
- Default to an ephemeral server key. Any durable key would require explicit
  authorization, client allowlisting, protected storage, revocation, and
  recovery design.
- Do not rely on the public Tailcat DERP service for a product availability
  promise. The project states that its API, CLI, wire format, public relay
  availability, and throughput have no stability or service-level promises.
- A prototype is worthwhile only after the current matching-target comparison
  is complete, and only if it proves a simpler no-account Agent handoff than
  the existing route adapters.

## Sources

- [Tailscale product page](https://tailscale.com/tailcat)
- [Official Tailcat repository and stability statement](https://github.com/tailscale/tailcat)
