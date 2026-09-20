# ADR 0003: Device identity storage

- Status: accepted
- Date: 2026-08-18

## Decision

Generate Device identities with Go's standard `crypto/ed25519` implementation.
Derive the immutable public Device ID from SHA-256 of the public key. Persist
only the 32-byte private seed, protected for the current operating-system user.

Windows uses current-user DPAPI with `CRYPTPROTECT_UI_FORBIDDEN`; it does not
set `CRYPTPROTECT_LOCAL_MACHINE`, because that flag permits other local users to
decrypt the blob. Unix-like systems use operating-system file ownership with a
mode-0700 directory and mode-0600 identity file.

Publish a complete staging file with a same-directory hard link so concurrent
first starts converge on one identity without replacing it. Treat corrupt,
unreadable, mismatched, or weakly permissioned storage as a blocking error.

## Consequences

- Copying a Windows identity file to another user or machine does not make the
  private identity usable.
- Unix protection depends on the security of the local account and filesystem.
- Administrator password reset or loss of the operating-system protection keys
  can make the identity unrecoverable; PF Remote reports failure rather than
  inventing a replacement identity.
- Device key rotation is a later, explicitly versioned lifecycle operation.

## Primary references

- [Go `crypto/ed25519`](https://pkg.go.dev/crypto/ed25519)
- [Microsoft `CryptProtectData`](https://learn.microsoft.com/en-us/windows/win32/api/dpapi/nf-dpapi-cryptprotectdata)
- [Microsoft DPAPI example and scope notes](https://learn.microsoft.com/en-us/windows/win32/seccrypto/example-c-program-using-cryptprotectdata)
- [`golang.org/x/sys/windows`](https://pkg.go.dev/golang.org/x/sys/windows)
