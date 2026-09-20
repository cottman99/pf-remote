# PF Remote MCP server v1

`pfremote-mcp` is a local stdio MCP server for Codex and other Agent hosts. It
does not expose a TCP management port and never receives a route, credential,
or subject-identity override from the Agent.

The server supports the MCP 2026-07-28 `server/discover` path and the
2025-11-25 initialization path for compatible clients. It exposes:

- `pfremote_list`
- `pfremote_inspect`
- `pfremote_context`
- `pfremote_doctor`
- `pfremote_connect`
- `pfremote_exec`
- `pfremote_open`

Every tool calls the same protected local daemon action used by Center and the
CLI. Target arguments accept only an alias or stable `pfremote://` reference;
the daemon owns identity resolution, authorization, route selection, target
authentication, executor configuration, and cancellation.

Read operations return versioned structured content and the same serialized
JSON as text for compatibility. Remote failures return `isError: true` with
only the safe summary and remediation. They do not include session content,
credentials, endpoints, relay metadata, or raw environment data.

`pfremote_exec` accepts an argument vector rather than shell text. Tool
annotations mark list, inspect, context, and doctor read-only; connect, open,
and exec remain action tools for the MCP host to present through its normal
human-control policy. PF Remote Grants and remote operating-system permissions
remain authoritative regardless of host approval behavior.

The external-Agent acceptance check must not stop at `inspect`. It creates one
safe target envelope, requires a fresh Codex process to inspect that canonical
target and then invoke an action tool for the same target, and verifies an
independent action-side record of the target and argument vector. The fixture
never connects to a real computer; protocol-level encrypted execution remains
covered separately by the real OpenSSH integration path.
