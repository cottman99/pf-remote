# Windows release v1

The owner-selected M10 release route uses the independent publisher signature in
signed-release-v1.md rather than requiring purchased Authenticode. The historical
promotion tooling described below still implements the Authenticode path; it must
be extended and verified before public zero-cost binary publication. No internal
unsigned package becomes a public trusted release merely by enabling recovery.

`pfremote.windows-release/v1` describes one architecture-specific, unpackaged
PF Remote Windows payload. The unpackaged model is intentional: the native
Center and its sibling daemon keep direct executable launch and protected local
IPC behavior, while a transactional per-user installer owns version selection,
upgrade, rollback, and uninstall.

The release manifest contains `schema_version`, `product`, `version`,
`architecture`, `created_at`, and an ordinal case-insensitive path-sorted `files` array. Every file entry
contains a forward-slash relative `path`, byte `size`, and lowercase SHA-256
`sha256`. Paths are unique, contain no traversal, and never identify a private
deployment. The archive contains exactly the files declared by the manifest.

The release set contains:

- one deterministic `PFRemote-Windows-<architecture>-<version>.zip` payload;
- `release-manifest.json` for payload verification;
- `sbom.spdx.json` with source dependency and payload-file evidence;
- `licenses.json` with dependency license review state;
- `SHA256SUMS.txt` covering every published artifact; and
- in a promoted build, a signed transactional installer whose signature is
  verified after signing and before hash publication. Promotion requires the
  exact approved publisher subject and an RFC 3161 timestamp; the
independent verifier checks the same publisher on Setup and the installed
Center.

The stable per-user installer helper is also the installed lifecycle entry
point. Windows starts that helper at sign-in; it resolves `current.json` and
starts only the selected version's daemon and, when the private deployment has
an explicit protected startup profile, its configured TLS Gateway. No startup
entry contains an immutable version directory or silently drops Gateway
arguments. Center reads the same state through
the helper and can request rollback after an explicit user confirmation.
Rollback switches the atomic state record, restarts only PF Remote background
executables whose resolved paths are inside the owned `versions` directory,
and reopens the selected Center. It does not match or stop a process by name
alone and therefore cannot replace or terminate the independent legacy
product.

The supported production providers are a local CA-issued code-signing
certificate or Microsoft Artifact Signing through SignTool's dlib integration.
Artifact Signing account/profile metadata remains external to the repository;
it is neither a credential nor part of the public release payload. The official
Microsoft Artifact Signing timestamp endpoint is permitted even though its
documented URL is HTTP because the signed timestamp response, rather than the
transport URL, is the trust evidence.

Development builds may be explicitly unsigned but may not be described as a
release candidate. Promotion fails closed when a signing identity is absent,
the signer does not match the intended publisher, signature verification fails,
or any published hash changes after signing.

The current open-source release scope publishes source code only. Internal
development packages and their binaries stay outside the public repository and
release attachments. Source-only publication does not promote an unsigned
package and therefore does not weaken the signing gate: that gate becomes
active again before any public Windows binary distribution.

The installer uses a product-specific per-user root, keeps immutable version
directories, and switches one small current-version record only after every
payload hash is verified. Upgrade never overwrites a running version. Rollback
selects the previously verified version. Uninstall removes only an installation
with the exact PF Remote marker and retains the user's PF Remote identity,
recovery data, and legacy PF Remote installation. A default user installation
creates a Start menu entry and a current-user Windows Installed apps record;
the registered uninstall helper removes those entry points and then removes
itself without requiring administrator access.

## Interrupted upgrades and startup health

Existing-installation upgrades write a pfremote.pending-upgrade/v1 record before
switching current.json. It contains the exact prior installation state, never
private configuration. Retain old current and previous versions until activation
succeeds. A failed activation restores that exact state and restarts it; failed
recovery keeps the record for retry. Sign-in startup recovers a pending operation
before starting normal service. An OS file lock serializes installer maintenance
and releases on process death. Current-state replacement has no rename-away gap.

Setup checks protected local API availability, runtime and identity readiness;
an offline remote catalog alone is not a bad installation. Version identifiers
are validated before constructing filesystem paths. User data, Gateway settings,
TLS material and compatibility files remain outside the replaced version tree.
This is program rollback, not reverse database migration. Unattended activation
still requires compatibility checks and session draining before using this path.

## Single Center instance

Each Windows user/session owns one Center instance across ordinary relaunches.
Ownership is acquired atomically before daemon startup or window creation.
A secondary explicit launch signals the existing window and exits; a secondary
background launch exits silently. The signal carries no target, path or command.
Windows user identity and session scope isolate local named synchronization
objects. Process exit releases ownership, including abnormal termination. A
startup-time signal remains pending until the owning window is ready. Restore
also handles a hidden or minimized primary window. Remote Desktop viewer windows
are not Center instances and are outside this rule.

## Center interaction recovery

Content dialogs are serialized on the UI thread; a failed dialog releases the next
request. Catalog errors retain named computers only as visibly stale, disabled
entries and expose retry on the computer page, including first-load failure.
Successful refresh re-enables current actions. Background catalog updates and
navigation must not overwrite operation feedback. List updates preserve unaffected
item instances without a collection reset. Credential setup failures use the same
classified error and exact-target retry surface as normal Desktop failures.

## Automatic update status and activation

The doctor Check object has an optional additive `code` field; older readers may
ignore it. Native Settings maps known update codes to localized resources and does
not display raw remote error text. Unknown/absent codes use bootstrap guidance.
A trusted Windows package pins the Ed25519 publisher key and initializes durable
trust outside version directories. Settings shows scheduled, current, downloading,
installing, busy, retry, incompatible or held state. An unprovisioned build performs
no discovery. Setup verifies the signed package before version activation.

The zero-cost channel uses Ed25519 release authentication independently of Windows
Authenticode. Such packages remain Authenticode NotSigned and must not claim OS
publisher reputation. This route supersedes paid signing as an update-authentication
prerequisite; public source and generic binary publication remain explicit actions.
