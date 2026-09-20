# M4.4 multiple named Desktop capabilities report

## User-visible result

One computer can now offer several authorized Desktops without appearing
several times in the main computer list. The person still chooses the computer
first. When there is one Desktop, the card keeps the single `打开桌面` action.
When there are several, the card exposes compact, meaningful actions such as
`打开当前屏幕` and `打开独立桌面`.

![One computer with current-screen and independent-Desktop actions](evidence/M4_4_MULTIPLE_DESKTOPS.png)

A computer with no Desktop remains available for Agent work and keeps an
explicit disabled `没有远程桌面` state. Technical capability aliases and
protocol names stay out of the primary UI.

## Exact-target behavior

Each visible Desktop action carries the immutable canonical reference of that
specific capability. The Center passes that reference as one argument to
`pfremote open`; route and protocol selection remain daemon-owned. Duplicate
rendering-environment names fall back to the authenticated capability display
name, so two virtual Desktops remain distinguishable without relying on list
position.

Agent handoff still prefers the computer's Shell capability for automation.
The PF Remote Skill now instructs an Agent that needs a sibling Desktop to list
authorized targets, retain the same immutable Device ID, and use the visible
Desktop name. If more than one sibling still matches, the Agent asks the user a
product-level choice rather than guessing from an address or route.

## Product audit

The one-Desktop and multiple-Desktop states were rendered at the same isolated
Windows viewport and reviewed together. The multiple state adds two named
buttons in the existing card without changing the page hierarchy, duplicating
the computer, or introducing a dialog for the common single-Desktop path. The
buttons fit the current card at the verified viewport; the computer name,
status, capability summary, authorization, Agent action, and Desktop choices
retain a clear reading order.

## Verification

- Center tests cover grouping, one-Desktop behavior, Shell-only behavior,
  physical/virtual names, multiple-Desktop disambiguation, and exact canonical
  `open` arguments.
- The synthetic catalog and normalized state round-trip four capabilities,
  including two Desktops on the same Device.
- The isolated Windows 11 render uses a fresh protected state root and a fresh
  named-pipe endpoint; the existing installed application is not stopped or
  modified.
- The full repository check remains the closure gate after roadmap state is
  advanced.

## Compatibility and remaining work

Catalog and state schemas remain backward compatible because multiple
capabilities were already structurally supported. This item changes grouping
and selection, not identity or authorization. Recovery, emergency access,
packaging, onboarding, and private-deployment migration remain later roadmap
items.
