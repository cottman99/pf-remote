# Peer management v1

Draft additive contract. See ../docs/architecture/PEER_MANAGEMENT.md.

`pfremote.peer-journal/v1` stores a pinned Fabric ID, root Device public identity,
and signed records. Signatures use Ed25519 and strict unpadded base64url.
Canonical signed bytes are the Go JSON encoding of the typed record with an empty
signature, prefixed by `pfremote-peer-record/v1` and a newline. SHA-256 of the
complete signed record identifies it in the issuer chain. No maps occur inside
signed record bodies. Unknown fields and trailing JSON are rejected.

Record kinds are manager, member, capability, grant, revoke-device and
revoke-grant. Root alone appoints managers. Managers approve signed join requests
and exact access grants. A Device publishes only its own capabilities. Revocation
of a manager or root is root-only; ordinary member/grant revocation is manager
authority. Root self-revocation is rejected to prevent unrecoverable accidental
bootstrap invalidation. Key replacement uses a new Device ID and explicit grants.

A single root-signed `migration` record may attest the existing Owner directory
as a typed state snapshot. It retains immutable IDs, Desktop profiles and earlier
grant deadlines. Every Device key must derive its ID; migrated Shell capabilities
must retain their Device-signed host bindings. Its capture time cannot be newer
than issuance. This explicitly authorized local import is not available to other
managers or automatic peer discovery. Baseline revocations remain permanent.
Later Device publications and manager grants use the normal signed-record rules.
Legacy state is retained separately and rollback cannot drop known revocations.

The protected management action accepts `status`, `join`, `bootstrap`, `appoint`,
`approve`, `publish`, `grant`, `revoke-device`, `revoke-grant`, `migrate`, `renew`,
`export`, `import`, `exchange`, `activate` and `rollback`. MCP exposes
`pfremote_peers`; Agent maintenance may send the same typed request to
`pfremote doctor --peer-stdin --json`. A separate migration executable supports
isolated validation without replacing the active client. Endpoint configuration
is private `pfremote.peer-transports/v1` and reuses strictly verified native SSH.

Per-issuer sequence starts at one with an empty predecessor. Each later sequence
must reference the preceding record hash. Replay of the identical record is an
idempotent merge, never renewed authority. Conflicting signatures for the same
sequence, missing predecessors and invalid batches leave durable state unchanged.
Root device revocation signs `revoked_through` and `revoked_digest` for the
observed issuer head. A revoked manager's later negative records are inert;
timestamps cannot bypass this cutoff. Earlier witnessed tombstones remain.
Member requests expire after ten minutes, and approvals must be signed within
that window. Grants have a positive lifetime of at most seven days. Records more
than five minutes in the future fail validation. Historical approval replay is
checked against signed approval time, not import time, to permit offline catch-up.

Old clients ignore the separate peer journal. Absence retains existing behavior.
Explicit peer activation replaces authorization, never merges legacy grants into
peer authority. Route and credential state stays private and is not journal data.

Daily activation persists an authority floor. Supported installers reject legacy
downgrade, and missing or empty activated authority fails closed. An administrator
manually replacing protected state and programs is outside this application guard.
