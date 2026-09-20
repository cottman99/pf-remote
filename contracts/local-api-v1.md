# Local API v1

The daemon action core is exposed to Center and CLI through a protected
Named Pipe on Windows or a mode-0600 Unix Domain Socket on Unix. It is never an
unauthenticated loopback TCP service.

One connection carries one JSON request and one JSON response, each limited to
1 MiB. Example request:

```json
{
  "schema_version": "pfremote.local-api/v1",
  "action": "inspect",
  "target": "compute-node/shell"
}
```

Supported actions are `list`, `inspect`, `context`, `doctor`, `connect`, `exec`,
and `open`. `connect` validates that one visible Shell target can establish a fully
authenticated Session. `exec` additionally carries a non-empty bounded command
argument array and returns `pfremote.shell-action/v1` with the resolved target,
status, opaque Session ID, exit code, and bounded live output.
`open` carries one Desktop target and returns `pfremote.desktop-action/v1`
after the existing RDP, VNC, or reviewed private-compatibility application has
successfully started. The result includes the
resolved target, opaque Session ID, protocol, and rendering-environment label;
it never returns an endpoint or credential.

`open` and `save-desktop-credential-and-open` may add the optional
`route_adapter` field with one registered adapter name. Omitting it invokes
Smart connect; including it pins only this action to that adapter after normal
target resolution and authorization. The response may add the selected
`route_adapter`. These optional fields are additive within v1: older clients
continue to use Smart connect and older daemons reject an unsupported override
without changing network state. Addresses, ports, relay names, and credentials
are never accepted.

The caller supplies a target name, requested task, and at most the optional
one-action adapter choice described above. The daemon supplies
the protected subject identity, authorization state, route selection, remote
account configuration, credentials, and strict target verification. A caller
cannot override those values through this contract. Disconnecting the caller cancels
the action and owned route/executor resources.

Responses contain either `result` or the standard error contract. Shell output
is returned only to the live protected caller, is capped at 1 MiB per stream,
and is not copied into structured errors or durable diagnostics.

CLI and Center do not fall back to an in-process synthetic catalog. Every read
loads the daemon's current valid snapshot, so repeated client processes share
one Device identity, Grant view, revocation state, fixed cache boundary, and
persistent clock high-water mark. An unavailable daemon returns a structured
local failure instead of silently granting development authority.
The Center may use the protected-IPC-only action
`save-desktop-credential-and-open` for one-time VNC setup. Its request carries
the immutable Desktop target and one credential received over standard input;
the daemon binds and protects the credential for the current operating-system
user before opening. The credential is never returned, logged, exported in
context, accepted as a target reference, or placed in process arguments. This
optional action is backward-compatible within `pfremote.local-api/v1`; older
clients do not send it and older daemons return `UNKNOWN_ACTION`.

`pfremote.catalog/v1` may also include a bounded `recent_sessions` array. Each
entry contains only a session ID, immutable target reference, coarse
action/result, and start time. It excludes command content, output, routes,
addresses, ports, credentials, and Desktop content. The field is optional for
backward compatibility. This redacted local history is bounded and survives
ordinary daemon restarts and upgrades. It is excluded from recovery exports and
cleared when a recovery bundle replaces local state.
