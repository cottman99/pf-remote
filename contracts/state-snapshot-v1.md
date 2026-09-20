# State snapshot v1

PF Remote stores local catalog and authorization state in a versioned SQLite
database named `pfremote-v1.db`. The logical snapshot schema is
`pfremote.state-snapshot/v1`.

Each committed snapshot contains:

- a Fabric ID, monotonic directory and Grant versions, and UTC capture time;
- Devices keyed by immutable Device ID;
- Capabilities keyed by immutable Capability ID and linked to one Device;
- Grants keyed by immutable Grant ID and linked to both a subject Device and a
  Capability, with an `active` or `revoked` state and optional UTC expiry.

A Device may include its enrolled Ed25519 public identity. A Shell Capability
may include a versioned SSH host-key binding signed by that Device. Both are
public metadata but are integrity-sensitive and are committed atomically with
the Device/Capability snapshot. A binding is invalid unless its public key
derives the Device ID and its signature covers the exact
Fabric/Device/Capability tuple.

The database separately retains the newest accepted binding version and
signature for every exact Fabric/Device/Capability tuple. That high-water mark
survives snapshot pruning and restart. Snapshot commit rejects a lower version
or a different signature at the same version, and recovery rejects retained
snapshots below the high-water mark rather than reviving an old SSH host key.

The database uses strict tables and foreign keys. Snapshot rows are inserted as
`staging`, all child rows are validated and inserted in the same transaction,
and the snapshot becomes `committed` before the singleton active pointer is
updated. A failed transaction therefore leaves the previous active snapshot
unchanged. The newest five committed snapshots are retained.

At startup PF Remote runs SQLite quick and foreign-key checks, then attempts the
active committed snapshot followed by retained committed snapshots in newest-
first order. Structurally invalid or undecodable candidates are skipped. If no
valid committed snapshot remains, startup fails closed. The clean-room first
run creates a synthetic snapshot and grants its locally protected Device
identity access to the three synthetic Capabilities.

Enrollment control state uses `pfremote.enrollment-state/v1` in the same
database. It stores public keys, code hashes, expiry/polling metadata, Device
status, monotonic versions, and request IDs. It never stores private keys or raw
device/user activation codes. Replay history is bounded to the newest 4,096
Owner request IDs and 1,024 request IDs per Device. At most 256 Device
activations may be pending, only one may be pending for a Device, and at most
4,096 recent unregistered Device replay buckets are retained. Expired
activations are pruned; inactive unregistered replay buckets expire after 24
hours.

Device active/revoked transitions replace the enrollment payload and update a
normalized control-version/revocation overlay in one SQLite transaction. Every
daemon action reloads the latest valid snapshot and applies that overlay without
changing the snapshot capture time. A Gateway revocation therefore removes the
Device both as an authorization subject and as a target before the mutation
returns. A save failure makes the live manager unhealthy until restart.

The database enables WAL journaling, `synchronous=FULL`, foreign keys, and a
five-second busy timeout. Unix-like systems require a mode-0700 directory and
mode-0600 database file. Windows places the database beneath the current user's
configuration directory; the private Device seed remains separately protected
with current-user DPAPI.

Cached authorization expires seven days after the immutable snapshot capture
time, or at an earlier Grant expiry. The database also persists a monotonic
last-observed wall-clock high-water mark so a later local clock rollback cannot
extend an already observed authorization interval.

This contract covers local durable state and recovery. A later remote catalog
transport must authenticate its versioned payload before it is accepted as a
new local snapshot.
