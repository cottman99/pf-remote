# M5.2 private observation, bounded connection, and rollback

## Product result

The legacy PF Remote Center and its background agent were present, running,
and responsive on the user-authorized computer. The isolated new Center and
daemon were also running and responsive. The initial observation changed
nothing. After separate authorization, one new-path Desktop session was opened
to the exact named private Windows-screen target on an isolated Windows desktop.
The candidate viewer established a VNC connection to the uniquely resolved,
online Tailscale Device identity. It never appeared on or took focus from the
user's active desktop.

The new candidate successfully completed its own read-only journey: it listed
named computers and Shell/Desktop actions, generated a safe context envelope,
and returned the same canonical target through the MCP Agent interface. This
proves that the candidate's UI/CLI/Agent surfaces share one target model.

The read-only observation confirmed that
the authorized legacy computer currently contains three computers, eight
actions (two RDP, three SSH, and three VNC), and sixteen structurally valid LAN
or Tailscale endpoints. The real connection then proved the independent new
route to the matching physical screen. The old catalog and its third-party
screen-control entry remained present and usable as the fallback; no legacy
process, service, configuration, or connection was stopped or switched.

The real session also exposed a user-visible defect: the request timeout closed
a healthy Desktop viewer after thirty seconds and reported a local-daemon
failure. Desktop launch now returns as soon as the operating-system client has
started, while that client continues as the user's independent session. An
isolated helper-client test proves the request returns before the viewer exits.
The authorization covered one real connection, so the corrected behavior was
verified without opening a second private session.

## Planning issue discovered

The roadmap assumed that a redacted private inventory would already exist
before side-by-side observation. The installed legacy Center instead stores
user-facing computers and actions in the same source document as usernames,
route endpoints, ports, URLs, and relay secrets. Copying that source into an
outward artifact would violate the product boundary, but refusing to use it
locally would also prevent the candidate from preserving real connections.

A blocking read-only adapter now converts the known legacy Center schema to
`pfremote.legacy-inventory/v1`. The local compatibility model retains the real
connection and credential material in memory while the serialized inventory
keeps only opaque references, visible computer/action names, Shell/Desktop
kinds, and path counts. The
adapter's expected field names and types were checked against every observed
legacy entry without retaining field values or private raw evidence in this
repository.

The Windows Center now detects that bounded legacy source automatically and
shows a non-technical preview card. It tells the user how many computers and
remote actions were found, offers a readable per-computer action preview, and
states that the old setup, connections, and passwords remain unchanged. The UI
does not expose a source path, route type, endpoint, account, or credential and
does not offer an apply or cutover action under read-only authorization.

The candidate projection now assigns stable PF Remote targets to those legacy
computers and actions and privately binds their LAN and Tailscale endpoints to
the exact target identity. It does not probe or open those endpoints. Actions
whose identity or authentication is not yet ready are marked as being prepared;
the Center disables them instead of presenting a button that will fail.

For Tailscale paths, the candidate now resolves a legacy address to the
authenticated immutable Tailscale node identity before marking the action
ready, then checks that identity again immediately before route acquisition.
Offline, ambiguous, changed, or unavailable peers remain visibly in setup
rather than being opened by name alone. The immutable identity and endpoint
remain private and are excluded from UI, Agent context, JSON, and reports.
An aggregate-only check on the authorized computer found that seven of its
eight legacy Tailscale routes currently map to an online authenticated peer;
the remaining route correctly stays in setup. No endpoint value, peer identity,
connection, or service state was read into repository evidence or changed.

The legacy source has no cryptographic SSH host binding, so its three SSH
actions cannot safely become Agent-ready from route names alone. The controlled
node daemon now handles the missing confirmation automatically: when an
OpenSSH server is present, it reads only the machine's public host-key files
and signs them with that machine's protected PF Remote identity. It neither
reads private keys nor scans a network address, and it cannot sign on behalf of
another computer. This removes fingerprint copying from the intended user
journey while preserving strict wrong-target rejection. Private-deployment
confirmation remains pending because installing or changing the authorized
computer was outside this read-only observation.

The synchronization contract is now implemented behind the user interface.
An active controlled Device can publish its signed Shell confirmation to the
Gateway; the Gateway persists it across restart and recovery, rejects rollback
or conflicting identity data, and stops returning it after Device revocation.
The controller can merge that confirmation only into the already-named exact
Device and Shell action, without creating targets from network data or
refreshing the user's cached authorization period. The bounded Gateway client
and an isolated HTTP journey now prove signed publication and Owner retrieval
end to end; remote plaintext endpoints are rejected. Daemon-owned polling
is now connected at startup when a reviewed Gateway endpoint is configured:
controlled nodes publish their own bindings, while the Owner controller can
pull and merge the current claims. The Owner daemon then refreshes the signed
capability directory every thirty seconds in the background without extending
authorization or changing routes.

Product-level setup is now implemented as a single **Connect service** action.
The user selects an expiring `.pfremote-link` invitation instead of entering a
Gateway, address, port, protocol, or credential. The file is signed by the
intended Owner and is rejected if modified, expired, or mismatched to the
active Fabric or Owner. A valid profile is discovered by the running daemon
within five seconds; the existing local list remains available during retry.
Profile replacement is recoverable and status output never reveals the private
service endpoint. If synchronization cannot recover automatically, the same
status area offers **Replace invitation** instead of sending a non-technical
user to settings or a command line.

The Windows Center now presents that setup state in ordinary language as one
of three conditions: synchronized, using the local list, or restoring
synchronization. It never displays a Gateway URL, address, port, protocol, or
credential. The local-list message makes clear that existing Desktop access
still works and that confirmed Agent actions will appear automatically. The
status uses a native localized InfoBar with a complete automation name for
keyboard and screen-reader users.

The Center now offers one user-level action, **Use these computers**. It writes
only a safe activation marker, after which the running daemon loads the selected
legacy source read-only and immediately replaces the development list with the
same named computers. Restart preserves the choice. The old file, service, and
connections remain untouched. A fully isolated activation journey verified two
computers and three actions, one ready RDP action, two accurately disabled
setup-required actions, unchanged source content, and identical UI/context
target identity.

The same card now provides the visible rollback action **Keep using the old
version** after activation. It changes only the candidate's compatibility
preference and immediately restores the exact pre-activation local list in the
already-running daemon. An isolated enable/rollback journey proved identical
local targets after rollback, unchanged legacy source bytes, and repeatable
re-enablement. No legacy process, connection, configuration, or credential is
changed.

The Agent handoff evidence has also been strengthened beyond read-only target
inspection. In a fresh isolated Codex process, the PF Remote Skill required an
MCP identity check followed by an MCP action on the same canonical target. An
independent action-side record confirmed the exact target and argument vector,
so the test now covers the full handoff-to-control chain without touching a
private computer.

## Current decision

M5.2 is complete. The new path reached the exact authorized target, the old
path remained available, rollback stayed candidate-only, and the timeout defect
found by the real session is repaired. This is engineering evidence rather than
a product-manager acceptance build: signed packaging and the clean-environment
journey remain before non-technical experience review.

## Verification evidence

- The legacy catalog exporter is covered by deterministic redaction, unsafe
  schema, unsupported action, missing path, bounded file, and no-write tests.
- The Center preview is covered by exact-source invocation and secret-free
  presentation tests.
- Candidate projection and endpoint binding are covered by target-isolation,
  invalid-binding, private-serialization, and setup-state tests.
- The isolated one-click activation check proves immediate daemon reload,
  unchanged source content, exact context identity, and honest readiness state.
- The bounded private check proved one established Desktop connection to the
  uniquely resolved private Windows Device in an isolated desktop, followed by
  candidate-only cleanup with the legacy fallback still present.
- The Desktop launcher regression test proves a successfully started viewer is
  no longer tied to the thirty-second local request lifetime.
- The isolated invitation check proves that a running daemon discovers a valid
  matching profile without restart, rejects mismatched setup before replacing
  the working profile, and keeps the private endpoint out of status output.
- The unpackaged Center built without warnings and rendered the preview on an
  isolated Windows desktop without taking focus from the user's active desktop.
- Visual evidence: [migration preview](evidence/M5_2_MIGRATION_PREVIEW.png).
- Visual evidence: [activated unified legacy list](evidence/M5_2_UNIFIED_LEGACY_LIST.png).
- Visual evidence: [non-technical connection-service state](evidence/M5_2_CONNECTION_SERVICE.png).
- Visual evidence: [one-click return to the old version](evidence/M5_2_ONE_CLICK_ROLLBACK.png).
