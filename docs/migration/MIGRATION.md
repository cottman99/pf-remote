# Private deployment migration

## Non-negotiable rule

The existing private PF Remote/FRP/Tailscale/Guacamole deployment stays running
and recoverable while this repository is developed. This repository never
imports production secrets, databases, backups, logs, hostnames, addresses, or
generated configuration.

## Migration phases

1. Let a local read-only compatibility adapter load the selected private legacy
   source, including the connection material needed to preserve working paths.
2. Export a redacted logical inventory for UI, Agent context, planning, and
   review while private connection values remain inside the local boundary.
3. Record a private old-ID to new-ID mapping outside this repository.
4. Resolve new `pfremote://` references through existing services without
   changing legacy generation.
   A reviewed legacy external screen-control entry remains a Desktop action and
   launches only its exact installed application with no arguments or URI;
   unsafe external definitions remain visible but disabled.
5. Install the new daemon beside each legacy agent and assign a new identity.
6. Validate restart, sleep, network loss, Gateway loss, alternate-route loss,
   revocation, and recovery.
7. Disable one legacy generator only after the matching new path has passed its
   acceptance test and rollback has been rehearsed.
8. Keep the documented observation window and recoverable backup before deletion.

## Authorization and clean-room procedure

Before a private inventory is read, the Owner must explicitly name the exact
legacy source and the isolated destination for the redacted export. General
permission to test a computer or continue development is not permission to
inspect a private deployment. The source remains read-only throughout
inventory and planning.

The local compatibility adapter may read the complete selected legacy source
and retain its addresses, accounts, routes, and credentials in memory or an
OS-protected local runtime store. These values are required to preserve real
connections. They must not enter source control, logs, screenshots, reports,
Agent context, Gateway metadata, or other outward-facing artifacts.

For planning and review, the adapter produces one
`pfremote.legacy-inventory/v1` JSON document. It may write to standard output
for transient review or be redirected to an explicitly selected location
outside this repository. It contains opaque references, proposed user-facing
names, Shell/Desktop declarations, path counts, and Grant relationships only.
It must not contain addresses, hostnames, ports, routes, relay names,
credentials, certificates, keys, tokens, databases, logs, commands, backups,
or cloud metadata.

`pfremote-migrate plan --input <explicit-file>` is dry-run only. It rejects
links, directories, oversized files, unknown fields, duplicate/dangling
references, and unsupported capability kinds. It emits a deterministic
`pfremote.migration-plan/v1` document to standard output and writes nothing
beside the source. Multiple legacy paths become one user-facing Capability
with a path count. Alias conflicts are resolved deterministically and reported
for review.

The Owner reviews computer names, Desktop/Shell choices, and collision
warnings. The private old-reference to new-identity mapping remains outside
this repository. Applying that reviewed map, installing a new daemon, or
changing any service is a distinct, explicitly authorized phase.

## Rollback

Every migration action must identify the old service/task/configuration that
remains available, the condition that triggers rollback, and the command or
operator action that restores it. Migration code must treat partial imports as
failed transactions and retain the last valid directory.

The native Center makes this reversible at the user level. After **Use these
computers**, it offers **Keep using the old version**. That action changes only
PF Remote's safe compatibility preference, immediately restores the prior local
list in the running daemon, and does not edit, stop, reconnect, or remove the
legacy application, catalog, service, credentials, or connections. The same
computers can be enabled again later.

## Evidence required per device

- Immutable identity mapping.
- Published capabilities and grants.
- Gateway-primary connection result.
- Independent optional-route result.
- Restart and network-late behavior.
- Revocation behavior.
- Legacy restoration proof.

## Local candidate versus public source

For a user-authorized local upgrade, reuse the existing protected identity,
connection configuration and per-target credentials. Verify their hashes across
the upgrade and verify the actual named inventory and a harmless Agent action.
Do not replace the local inventory with synthetic demo computers. The application
package stays generic; private values are supplied by local runtime state, never
compiled into an executable or placed in Git. Export public source separately
with `scripts/export-public-source.ps1`; do not upload the working directory,
its ignored artifacts, Git history, backups or internal installers as a source zip.
