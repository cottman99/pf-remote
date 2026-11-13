# Peer management v1

Status: daily migration candidate alpha.120; see the delivery report for live evidence.

## Product decision

An existing authorized manager can approve a new computer while the original
controller and Gateway are offline. Devices retain independent keys, names and
exact targets. The Gateway becomes an optional transport and legacy compatibility
service. Existing SSH, Desktop, LAN, Tailscale and FRP executors remain unchanged.

```mermaid
flowchart LR
    N[New computer / own key] -->|signed request| I[Trusted introducer]
    I --> M[Authorized manager]
    M -->|signed approval and grants| P[Peer records]
    P <--> A[Computer A]
    P <--> B[Computer B]
    A <-->|existing encrypted transport| B
    classDef confirmed fill:#dbeafe,stroke:#2563eb
    classDef planned fill:#fef3c7,stroke:#d97706
    class A,B confirmed
    class N,I,M,P planned
```

## Authority and bootstrap

The locally pinned Fabric root public key anchors management authority. Migration
must explicitly verify that this is the existing Owner identity. A root-signed
manager appointment permits enrollment, access grants and member revocation.
Only the root appoints or revokes managers. Introduction conveys no authority.
Root absence does not stop appointed managers admitting ordinary devices. Root
loss does prevent changing the manager set until root recovery; this deliberate
v1 limitation avoids turning every manager into unrestricted root authority.

Join requests prove possession of the new Device key, include a random nonce,
Fabric ID and ten-minute deadline. Approval binds the complete request digest.
An invitation is transportable public setup metadata, never a shared identity
or bearer credential. Bootstrap requires a separately verified root fingerprint
or an already authenticated existing management path; received data cannot choose
its own root. Private keys and protocol passwords never synchronize.

## Replication and conflicts

Each issuer has a signed, monotonically numbered hash chain. Records include a
domain-separated schema, Fabric, issuer, sequence, previous digest, issued time,
kind and typed body. Peers exchange bounded batches of signed records over
existing authenticated SSH or protected local IPC. No new public listener is
required. Durable state retains chains and revocation tombstones across restart.
Duplicate records are idempotent and never extend deadlines. Gaps, changed roots,
future records and unknown schemas are rejected. Same-sequence conflicting signed
records are equivocation, never last-writer-wins. Conflicts stop synchronization
pending explicit repair; a valid state is not replaced by an invalid batch.

Revocations are permanent and dominate positive records independently of arrival
order. A revoked manager's approvals/grants cease to confer authority. Capability
publication is signed by its owning Device. Grants require a manager and bind
exact subject and capability IDs. There is no implicit all-device access.
Authority is derived from the complete verified record set, not receipt order.

Root revocation freezes the manager's witnessed chain sequence and digest.
Negative records beyond that point are inert even if the compromised manager
backdates their signed timestamps. Previously witnessed tombstones remain.
The root may explicitly attest one existing Owner directory for migration;
Device IDs, signed Shell bindings and earlier authorization deadlines survive.

## Offline operation and revocation limits

Access grants expire at a signed absolute deadline, at most seven days after
issuance. A reachable manager can renew them without the root or Gateway. Reading,
replaying, restarting or copying records cannot refresh that deadline. The
existing durable clock high-water prevents rollback across observed expiry.
Known revocations take effect on the next admission check. An isolated peer
cannot know an unseen revocation. An isolated compromised manager may keep issuing
grants to peers that have not learned its revocation; no universal seven-day
revocation guarantee is claimed in that partition. Active native/OS sessions
retain their existing session-lifecycle constraints; OS access is independently
authoritative. Reconnecting peers synchronize before admitting new actions.

## Rotation, recovery and compromise

SSH host-key rotation retains the existing Device-signed binding version checks.
Device signing-key replacement creates a new Device ID, requires fresh enrollment
and explicit grants, and revokes the old ID. No copied seed or silent ID reuse.
Loss of all appointed managers and the root requires explicit Fabric recovery;
ordinary peer data cannot manufacture replacement root authority. Root recovery
material remains separate from device keys and ordinary synchronization.
An ordinary compromised device can publish its own capabilities and use its
grants. A compromised manager can enroll adversaries and issue access grants
until its revocation arrives or grants expire. Root compromise compromises Fabric
administration. Remote OS accounts/SSH keys remain separate authorities.
Shell access as a manager's operating-system user also permits use of that user's
protected signing API. Such access is owner-equivalent, not a sandboxed ordinary
membership. Containment requires separate OS accounts; a prompt or Device role
cannot restrict arbitrary code already running as the signing user.

## Migration and delivery gates

Use a separate peer state database, preserve the legacy database and route config,
and activate only after verifying root, all member identities, grants and target
bindings. Peer mode must never fall back to legacy authorization on verification
failure. Rollback is explicit and retains the peer journal and revocations; do
not use rollback to restore a revoked device's access. Pilot on an isolated state
directory before replacing any running daemon. No central shutdown or broad
fleet restart. TLS certificate repair is independent operational maintenance.

Required evidence: second-manager enrollment with root absent, forged requests,
replay/fork/gap rejection, opposite-order convergence, durable tombstones, expired
grant/clock rollback, corrupt-write rollback, old-client behavior, and actual
SSH execution without the central API. Desktop authentication and interactive
audio/keyboard acceptance are reported separately. A port probe is insufficient.
