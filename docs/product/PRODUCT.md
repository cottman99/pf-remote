# Product definition

## One sentence

PF Remote is a self-hosted personal compute fabric that turns a device's Shell
and Desktop abilities into stable, authorized targets for both people and AI
agents.

## User problem

Technical individuals gradually accumulate Windows, Linux, and macOS computers,
cloud relays, VPNs, proxy software, and several remote-access tools. They want to
say “work on this computer” without first translating that intent into an IP,
port, jump host, tunnel, or protocol.

PF Remote owns that translation. It does not own the user's network.

## Product position

PF Remote is a remote-management product and a shared mapping layer between a
person, an Agent, and the computers they control. It does not embed a chat or
Agent workspace; natural-language work remains in Codex or another Agent host.

Behind the product may be many Gateways, routes, relays, protocols, and remote
computers. In the normal interface the person sees one stable list of named
computers and capabilities. They can connect with one click, or select a target
and send its aligned, safe context to an Agent without explaining the network.

Computers with multiple Desktops show **Choose desktop** rather than silently
opening the first internal capability ID. Both the main button and its menu list
all exact Desktops, with TigerVNC/RDP labels where those clients are used. TigerVNC
workspaces appear first; selecting one never changes to an RDP sibling. The expanded
**All desktops** list retains individual Smart connect and route actions.

Each individual desktop connect action is a split button attached to that specific computer and
capability. Its primary action is **Smart connect**: PF Remote considers current
route availability for that exact Desktop, then follows a stable, explainable
fallback order. The adjacent menu lists only the routes currently eligible for
that exact action, with ordinary labels such as **Same network**, **Private
network**, and **Gateway relay**. Choosing one starts this connection through
that route; it is not a global networking setting and does not silently change
other computers. Infrastructure names, addresses, ports, and credentials remain
hidden.

The Agent can resolve and operate the same authorized targets through MCP,
Skills, CLI, or an equivalent versioned interface. The UI and Agent therefore
refer to the same immutable Device and Capability even when aliases or routes
change. PF Remote owns this alignment and mapping; existing remote clients
remain the protocol executors.

Connection-service setup follows the same rule. The Center exposes one
**Connect service** action and accepts a short-lived `.pfremote-link` invitation
file. The running daemon applies it without restart and keeps the existing
local computer list usable while synchronization completes or retries. The
person never configures infrastructure fields.

## Golden journey

Within about fifteen minutes, a new owner can:

1. install a self-hosted Gateway;
2. join and name two devices;
3. confirm a discovered Shell or Desktop capability;
4. grant one device access to another capability;
5. connect to a named computer with one click; or select it and copy/send its
   safe context to Codex;
6. see Codex resolve and verify the exact same Device and Capability; and
7. let Codex inspect or operate it through PF Remote without the owner
   explaining any Gateway, route, address, or protocol.

The journey must not require the owner to explain addresses, ports, FRP,
Tailscale, SSH configuration, or routing to the agent.

When automatic onboarding is not yet available for a deployment, the Computers
page still exposes **Add computer**. Its optional ordinary-language form creates
a self-contained Agent task covering installation, enrollment, capabilities,
eligible route discovery, privacy boundaries, and visible acceptance. It never
asks for or exports a password, private key, token, raw route endpoint, or cloud
credential. A new Agent can continue from that task without prior conversation
history or private repository knowledge.

## Product principles

- A target is an identity, not a location.
- Capability grants are explicit, revocable, and smaller than device trust.
- Protocols and routes are adapters hidden from the normal interface.
- Automatic route choice is intelligent but predictable: health and recent
  quality can skip a poor candidate, while a stable fallback order keeps the
  result explainable. Every connect button also offers a per-action route menu
  for a deliberate one-time override; route choice is never only a global
  Settings preference.
- People and agents use the same action core and receive the same result model.
- The named computer and capability list is the primary human interface.
- One-click human control and Agent control share identity, authorization,
  routing, actions, and results.
- Context handoff aligns the user and Agent on an exact target; it is not a
  prompt that embeds an Agent inside PF Remote.
- Multi-device and multi-platform remote access is baseline completeness. The
  differentiator is hiding connection complexity while keeping people and
  Agents aligned on the same target.
- Diagnostics are local, structured, redacted, and copyable.
- Existing tools remain useful: OpenSSH, RDP, VNC, and optional web emergency
  access are executors or adapters rather than competing product centers.
- Desktop performance defaults preserve native resolution and application GPU
  rendering. Modern Windows keeps DWM composition enabled; PF Remote leaves
  virtual-session visual effects on protocol/OS automatic unless a target
  explicitly publishes a measured reduced or full profile. Visual effects
  never masquerade as transport or rendering quality.
- Global proxy, DNS, routing, VPN, and TUN state belong to the operating system
  and user, never to PF Remote.

## First-release scope

The first release supports one Owner, Windows Center, Windows and Linux Nodes,
Shell and Desktop capabilities, a self-hosted Gateway, an FRP relay adapter, and
optional Tailscale/LAN routes. macOS daemon/CLI support is preview quality.

Windows Center also lets the Owner create one password-protected recovery file
and restore a replaced or reset Gateway without using a command line. The page
states in ordinary language that device names, identities, and permission
relationships are preserved while passwords, private keys, routes, activity,
and desktop content are not.

The Center uses three stable destinations: Computers for choosing a named
target, Sessions for bounded redacted activity shared by human and Agent
actions, and Settings & Recovery for synchronization, migration, version, and
recovery state. Session rows never reveal command content or network
infrastructure.

It does not include organizations, multi-tenant approval workflows, task-level
sandboxing, a native mobile app, or a general file manager.

## Success measures

- The golden journey succeeds from a clean environment without private setup
  knowledge.
- A copied target resolves to the same immutable capability after rename.
- An unauthorized device cannot list, resolve, or connect to that capability.
- A route failure produces a safe explanation and can select an independent
  candidate without changing global network state.
- The primary half of every connect split button selects a healthy route
  automatically; its adjacent menu can start the same action through any other
  currently eligible route without affecting another computer.
- A diagnostic package contains enough context for an agent to help while
  containing no session content, credentials, private keys, or bearer tokens.

## Focused closeout scope (supersedes expansion plans above)

The active delivery scope is Windows Center with existing Windows/Linux nodes.
Other native frontends, web access and automatic wake are deferred. Desktop order
is stable by capability identity, not recent activity or transient health; Smart
connect selects a route, never a different Desktop. Onboarding asks for the name,
OS, intended use and optional notes only; the Agent discovers installation state
and routes. Public source and private local runtime are separate deliverables.
