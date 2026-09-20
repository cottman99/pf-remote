# M5.3 internal Windows installation report

Status: complete for the source-only public release and internal binary test
scope selected by the project manager on 2026-08-30.

## User-visible result

The Windows candidate is now an ordinary per-user application rather than a
developer launch. Its installer places PF Remote beside the legacy product,
adds a Start menu entry, and the Center starts its own local service. Upgrades
leave the running version alone; rollback selects the previous verified
candidate; Windows Settings lists PF Remote as an installed app, and uninstall
removes only the new installation and its entry points while retaining PF
Remote user data and the legacy fallback.

This remains an internal development candidate. It is suitable for the next
isolated golden-journey validation but is not included in the public source
release and is not described as a publicly downloadable release candidate.

## Completed package evidence

- `contracts/windows-release-v1.md` fixes the unpackaged, per-user,
  side-by-side and fail-closed promotion contract.
- `scripts/build-windows-release.ps1` produces an architecture-specific
  payload, transactional Setup application, manifest, SPDX 2.3 SBOM, reviewed
  license inventory, third-party notices, and published SHA-256 hashes.
- Unknown dependency licenses stop the build. The current package records 43
  reviewed dependencies and contains no `NOASSERTION` license result.
- `scripts/verify-windows-release.ps1` independently hashes all 537 archive
  files, checks required WinUI and Agent assets, checks SBOM/license closure,
  installs into a fresh owned root, and verifies safe uninstall. It also repeats
  the ordinary-user path through Start menu creation, Windows Installed apps
  registration, and the registered uninstall helper in isolated locations.
- The installed Center was launched on an isolated Windows desktop, remained
  responsive, started the exact sibling daemon, and loaded its target catalog.
- Clean install, running-version upgrade, rollback, and uninstall were tested
  without stopping or modifying the legacy PF Remote path. User configuration
  remained outside the owned installation root.
- `scripts/check.ps1` passes, including 24 Windows presentation tests, the
  Windows application build with zero warnings/errors, Go tests, private-data
  hygiene, Agent alignment, and legacy compatibility.

The latest independently verified development artifact is held only under
repository scratch as `0.1.0-alpha.9`; it is explicitly unsigned and is not a
published release candidate.

## Public binary distribution deferred

No usable code-signing certificate exists in the current-user or local-machine
certificate stores. The signing and signature-verification path passed earlier
with a temporary development identity and still fails closed when no identity
is supplied. The project manager selected source-only public distribution and
internal binary testing, so trusted public binary signing is no longer an Alpha
closure gate. It becomes mandatory again before any future public Windows
binary distribution.

The builder now supports both a traditional CA-issued certificate and
Microsoft Artifact Signing without committing account metadata. Artifact
Signing Basic is the lower-operations-cost preference only when the publisher
is eligible for Microsoft's current Public Trust regions; publisher
jurisdiction must be confirmed before selecting it. No service is purchased or
activated for the source-only release.

## Migration impact

None on the legacy installation. The candidate uses a separate per-user root,
does not change system networking, and keeps identities and recovery material
outside the versioned application payload. No additional private connection was
made after the single authorized M5.2 check.
