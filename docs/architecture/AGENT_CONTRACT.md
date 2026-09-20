# Agent contract

## Goal

An agent receives one explicit target and can understand, verify, and operate it
without learning the underlying network topology.

## Minimal envelope

“Quick copy” emits a `PF_REMOTE_TARGET/1` envelope containing a canonical target,
a human alias, capability kind, and safe verification command. It contains no
credential and does not itself grant access.

## Required agent sequence

1. Parse the envelope or target supplied by the user.
2. Run `pfremote inspect <target> --json`.
3. Confirm the returned immutable Device and Capability identities.
4. For a Shell action, connect and verify `hostname` and the effective account.
5. Apply the user's task using the target operating-system account permissions.
6. Return the correlation ID when reporting a PF Remote failure.

An agent must not replace a missing target with one inferred from history. It
must not enumerate all devices unless the task asks it to or discovery is needed
to resolve a user-provided alias.

## Stable CLI surface

- `pfremote list`
- `pfremote inspect`
- `pfremote connect`
- `pfremote exec`
- `pfremote open`
- `pfremote context`
- `pfremote doctor`

All read commands support `--json`. JSON responses include `schema_version`.
Errors include `code`, `stage`, `correlation_id`, `summary`, and a redacted
`remediation` string.

## Behavior constraints

The rich context dialog may include a task, expected behavior, and operator
notes. These are instructions for the agent, not a security sandbox. Device and
operating-system authorization remain the technical enforcement boundary.

## Versioning

Consumers must reject unknown major envelope or JSON schema versions with a
clear upgrade message. Additive fields within a major version are allowed.

