# Connection invitation v1

`pfremote.connection-invitation/v1` is a short-lived setup file used by the
native PF Remote UI. A person chooses one `.pfremote-link` file; they never
enter a Gateway URL, address, port, protocol, or credential.

## User contract

- The Center shows **Connect service** only while it is using the local list.
- Choosing a valid invitation saves the connection profile and the running
  daemon discovers it in the background within five seconds.
- Existing computers and legacy connections remain available while the daemon
  connects or retries.
- Invalid, modified, expired, wrong-Fabric, or wrong-Owner invitations fail
  without changing the active profile.

## File shape

The JSON object contains `schema_version`, `gateway_url`, `fabric_id`,
`owner_device_id`, `owner_public_key`, `preset`, `expires_at`, `nonce`, and
`signature`. `preset` is `owner` or `device`. The signature is Ed25519 over a
domain-separated canonical message containing every preceding field.

The invitation identifies the intended service and Owner; it is not a bearer
credential and does not authorize a Device. Normal Device enrollment and
operating-system permissions remain authoritative. Remote plaintext Gateway
URLs, unknown fields, links, oversized files, expired files, identity
mismatches, and invalid signatures are rejected.

## Local profile

After validation, the UI helper writes
`pfremote.connection-service/v1` beneath the current user's protected PF Remote
configuration directory. Replacement uses a same-directory temporary file and
rollback copy. The Gateway URL is private setup data: it is excluded from UI,
status output, Agent context, logs, reports, fixtures, and source control.

An Owner profile may pull the signed capability directory only when the local
Device identity and Fabric match the signed invitation. A Device profile may
publish only its own signed capability identity.
