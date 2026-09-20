# Migration inventory and plan v1

`pfremote.legacy-inventory/v1` is the only accepted outward-facing migration
inventory. It is one explicitly selected, bounded, regular JSON file. The planner rejects
links, directories, files above 1 MiB, unknown fields, duplicate or dangling
references, unsupported capability kinds, and excessive collections.

The inventory contains only:

- opaque legacy references used to build a private mapping outside this repo;
- proposed user-facing names and display names;
- Shell or Desktop capability declarations;
- a count of existing connection paths, without their types or values; and
- active/revoked Grant relationships between declared references.

It has no fields for addresses, hostnames, ports, routes, relay names,
credentials, passwords, tokens, certificates, keys, databases, logs, commands,
or private deployment metadata. The local compatibility adapter may read the
complete private legacy source, but it must reduce that source to this contract
before data reaches UI, Agent context, planning, logs, reports, or source
control.

The dry-run output is `pfremote.migration-plan/v1`. It contains a canonical
source digest, deterministic opaque mapping keys, proposed aliases/display
names, capability kinds, path counts, Grant relationships, and visible naming
warnings. Multiple legacy paths remain one capability with a path count; route
selection stays behind the PF Remote target. Alias collisions are resolved
deterministically and reported for human review.

Planning writes only versioned JSON to standard output. It does not create
identities, import a live catalog, update a Gateway, change the legacy source,
or disable an access path. Applying a reviewed mapping is a separate,
explicitly authorized migration phase.

`pfremote-migrate export-legacy-center --input <catalog.json>` is the bounded
read-only adapter for the legacy PF Remote Center catalog. It accepts the known
legacy schema only, groups services by computer, maps SSH to Shell and RDP,
VNC, or reviewed external screen-control entries to Desktop, and reduces
populated route representations to `path_count`. External launch material stays
inside the local compatibility boundary; unsafe or unrecognized external
definitions remain visible as setup-required instead of hiding valid sibling
actions. It
retains private connection values only within the local compatibility boundary
and excludes usernames, addresses, ports, proxy names and secrets, Tailscale
hosts, LAN hosts, URLs, default routes, and generation metadata when serializing
the inventory. Unknown catalog fields, unsupported kinds, duplicate services, conflicting
computer names, missing paths, links, directories, and oversized files fail
closed. Output is standard output only; the source and its directory are never
written.

`pfremote-migrate enable-legacy-center` is the Center's local one-click
activation boundary. It writes only a versioned source-kind marker beside the
protected PF Remote state; it does not copy a path, endpoint, account, or
credential. The running daemon notices the marker, reads the standard legacy
source directly, and keeps private connection material in memory. Invalid or
linked activation state fails closed. `status-legacy-center` reports only the
enabled boolean for restart-safe UI state.
