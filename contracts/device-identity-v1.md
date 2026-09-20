# Device identity v1

Each PF Remote Device owns one Ed25519 signing identity. The local versioned
record has this shape:

```json
{
  "schema_version": "pfremote.device-identity/v1",
  "device_id": "device-opaque-public-identifier",
  "algorithm": "Ed25519",
  "public_key": "base64url-without-padding",
  "key_protection": "operating-system-specific-protection",
  "private_key_blob": "protected-and-base64url-encoded-seed"
}
```

The example values are descriptive placeholders, not valid key material.
`device_id` is the lowercase, unpadded base32 encoding of the first 160 bits of
SHA-256 over the 32-byte public key, prefixed by `device-`.

On Windows, `private_key_blob` is protected for the current user and current
machine with DPAPI and `CRYPTPROTECT_UI_FORBIDDEN`. PF Remote does not use
machine-wide DPAPI scope. On Unix-like systems, the record is protected by a
mode-0700 parent directory and a mode-0600 regular file.

Creation publishes a fully written same-directory staging file atomically. A
concurrent process loads the identity that won publication. Corrupt records,
unknown fields, relaxed Unix permissions, public/private mismatch, and an
unexpected protection provider fail closed. PF Remote never silently replaces
an unreadable identity because that would change the immutable Device identity
and invalidate its Grants.

The private seed is never returned by public APIs, logged, included in events,
copied into diagnostics, or exported by normal UI flows.
