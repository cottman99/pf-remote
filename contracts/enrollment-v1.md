# Enrollment and version sync v1

M2 enrollment uses `pfremote.enrollment/v1` JSON over the Gateway control
plane. Remote requests require HTTPS; plaintext requests are accepted only
from a loopback address for clean-room development.

## Endpoints

| Method and path | Purpose |
| --- | --- |
| `POST /api/v1/owner/initialize` | One-time Fabric and Owner Device initialization |
| `POST /api/v1/device-authorizations` | Start a Device authorization request |
| `POST /api/v1/device-authorizations/approve` | Owner approves a displayed user code |
| `POST /api/v1/device-authorizations/poll` | Device polls at the advertised interval |
| `POST /api/v1/devices/revoke` | Owner immediately revokes one Device identity |
| `POST /api/v1/versions/sync` | Active Device reads directory and Grant versions |
| `POST /api/v1/capabilities/shell/publish` | Active Device publishes its Device-signed SSH identity binding |
| `POST /api/v1/capabilities/shell/list` | Owner reads the current confirmed Shell identities |

Requests are one JSON object, use `Content-Type: application/json`, reject
unknown fields, and are limited to 1 MiB. Responses set `Cache-Control:
no-store`. Errors use `pfremote.error/v1`.

## Signed identity proofs

Public keys and signatures use unpadded base64url. A signature covers UTF-8
lines joined by one LF with no trailing LF. The first line is a domain
separator. For example, Device authorization signs:

```text
pfremote-device-authorize/v1
<device_id>
<device_name>
<device_public_key>
<client_version>
<request_id>
```

The other domain separators are `pfremote-owner-initialize/v1`,
`pfremote-owner-approve/v1`, `pfremote-owner-revoke/v1`, and
`pfremote-version-sync/v1`. Shell publication and Owner retrieval use
`pfremote-shell-capability-publish/v1` and
`pfremote-shell-capability-list/v1`. Their exact field order is implemented by the
exported message builders in `internal/enrollment/messages.go` and protected by
tests.

The first Owner initialization is trust-on-first-use and must run locally or
over an operator-authenticated TLS channel. The included Owner public key must
derive the included immutable Device ID and verify the initialization
signature. Later Owner actions must verify against that stored key.

Device authorization requests are self-signed by the new Device identity. A
`request_id` is URL-safe, 16 to 128 characters, and single use within the
current control-plane state.

## Device authorization behavior

The flow follows the security shape of RFC 8628 without issuing a long-lived
bearer access token:

- the device code contains 256 random bits and is never shown to the Owner;
- the human user code contains 40 random bits, is displayed as `XXXX-XXXX`,
  and invalid approval attempts are rate-limited;
- both codes expire after ten minutes;
- only one activation may be pending per Device and the Gateway accepts at most
  256 pending activations overall;
- polling starts at five seconds, and an early poll increases the required
  interval by five seconds;
- the Gateway retains only SHA-256 code lookups in control-plane state;
- approval activates the registered public identity and consumes both codes;
- future authority is the Device signature, not the expired device code.

Replay history is bounded without remaining permanently allocated to abandoned
identities: up to 4,096 unregistered Device buckets are retained, and a bucket
with no registered Device or pending activation expires after 24 hours. Active
and revoked registered Devices retain the per-Device replay bound.

## Revocation and version sync

The active Owner Device may revoke any other Device. Revocation increments the
monotonic directory and Grant versions and blocks the revoked identity before
returning version state. In the local M2 clean-room deployment, the enrollment
payload and catalog revocation overlay are committed atomically; daemon list,
inspect, context, and doctor actions reload that state for each request.
Replacing the Owner Device itself requires the later recovery workflow.

Clients send strict `MAJOR.MINOR.PATCH` versions with an optional leading `v`.
M2 supports client major version 1. An unsupported schema or major version
returns a standard version error without silently downgrading.

M2.3 persists the public identity records, code hashes, polling and rate-limit
metadata, versions, revocation state, and replay data transactionally in the
local state database. Raw activation codes and private keys are never stored
there. A failed state write makes the live enrollment manager fail closed until
restart.

## Shell capability synchronization

After local public-host-key discovery, an active Device publishes the
Device-signed `pfremote.ssh-capability-binding/v1` through the Gateway. The
outer signed request adds replay protection; the inner binding proves the exact
Fabric, Device, Capability, binding version, and host-key set. The Gateway
rejects unknown or revoked Devices, invalid signatures, version rollback, and
same-version conflicts. Confirmed bindings are bounded, durable, included in
Owner recovery, and removed from the live Owner list when their Device is
revoked.

The Owner receives the Device public identity together with each binding. The
controller may merge a claim only into an already-declared Shell Capability on
that exact Device; it cannot create or rename a target from a claim. Applying a
new claim advances directory state without refreshing the cached authorization
capture time.
