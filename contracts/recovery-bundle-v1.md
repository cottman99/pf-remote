# Recovery bundle v1

The portable Owner-held recovery file uses the outer schema
`pfremote.recovery-bundle/v1`. It contains an authenticated ciphertext and only
the parameters required to derive its encryption key:

- UTC creation time;
- `PBKDF2-HMAC-SHA256`, a random 32-byte salt, and a bounded iteration count;
- `AES-256-GCM`, a fresh 12-byte nonce, and the authenticated ciphertext.

The schema, creation time, KDF name and count, and cipher name are authenticated
as associated data. The production exporter uses 600,000 PBKDF2 iterations.
Readers reject unknown fields, unsupported versions or algorithms, parameter
values outside their bounds, malformed encodings, files above 8 MiB, an
incorrect password, and any authentication failure before parsing plaintext.

The encrypted payload uses `pfremote.recovery-payload/v1` and contains one
Fabric ID, Owner Device ID, monotonic directory and Grant versions, export time,
a sanitized `pfremote.enrollment-state/v1` document, and one validated
`pfremote.state-snapshot/v1` document. Export removes activation records, code
hashes, replay IDs, failure counters, and rate-limit windows. The payload may
retain public Device identity and signed SSH host-key bindings because they are
identity metadata, not routes or private keys.

The bundle never contains Device private keys, OS or protocol credentials,
raw activation codes, route candidates or leases, addresses, ports, session
traffic, logs, or private deployment configuration. Restored routes and
sessions must be established again.

Restore decrypts and validates the entire bundle in a separate SQLite staging
database. Fabric identity, Owner identity, directory and Grant versions,
Device bindings, Grants, aliases, and revocations must agree across both inner
documents. A live initialized Gateway rejects a different Fabric, a lower
directory or Grant version, and an exact-version replay. The validated staging
tables replace Gateway-owned control tables in one serializable transaction;
any validation or write failure rolls the transaction back and leaves the live
state unchanged. An uninitialized Gateway may accept the recovered Fabric.
