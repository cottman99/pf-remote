# M4.5 encrypted recovery report

## User-visible result

Windows Center now has a `备份与恢复` entry. The Owner can create one encrypted
recovery file and later restore a replaced or reset Gateway with normal file
pickers and a recovery password; no command line or database handling is part
of the product journey.

![Windows Center recovery journey](evidence/M4_5_RECOVERY.png)

The page explains the scope before either action: device names, immutable
identities, capabilities, Grants, aliases, and revocations are recoverable.
Computer passwords, private keys, connection paths, activity history, sessions,
and desktop content are not. The primary action is creating a recovery file;
restore remains available but visually secondary.

## Loss and restore behavior

Export removes temporary activation records, replay history, and rate-limit
state. It then encrypts and authenticates the bounded, versioned payload with an
Owner-chosen password. The password is passed to the local recovery helper over
standard input, never as a process argument, and is not stored by PF Remote.

Restore decrypts and validates a separate staging database before touching the
Gateway. It rejects a wrong password, a damaged or unsupported file, a different
Fabric, a lower directory or Grant version, and an exact-version replay. A
validated state replaces all Gateway-owned control tables in one transaction,
so a failed validation or write leaves the live state unchanged. Routes and
sessions are deliberately established fresh after recovery.

## Product audit

The first isolated Windows render exposed two hierarchy problems: the recovery
description was accidentally centered within its column, and the destructive
restore action had stronger visual emphasis than the normal backup action. The
verified render above aligns the explanation with its heading and makes
`创建恢复文件` the primary action. The recovery panel fits above the same two
computer cards without hiding or restructuring the core one-click and Agent
handoff journey.

## Verification

- The isolated loss-and-restore test rebuilds the original Fabric into a new
  database, opens it through the normal state and enrollment readers, and proves
  a revoked Device remains revoked.
- Negative tests cover wrong passwords, ciphertext damage, unsupported or
  incomplete payloads, exact replay, downgrade, wrong Fabric, weak passwords,
  bounded password input, and preservation of an existing live state.
- Windows Center tests cover the exact user-selected recovery file argument;
  the native x64 app builds without warnings and its isolated Windows 11 render
  passed without stopping or modifying the installed application.
- `scripts/check.ps1` passes private-data hygiene, governance, Go tests and vet,
  16 Center tests, and the native x64 build.

## Compatibility and remaining work

The recovery schemas are new and versioned. Existing catalog, Grant, Agent,
Shell, and Desktop contracts are unchanged. Recovery repairs Gateway control
state; it does not copy Device private identities or replace a lost Owner
Device. Optional browser emergency access and the M5 migration/release gates
remain later roadmap items.
