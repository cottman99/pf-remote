# Zero-cost trusted update work

## Current user-facing state

The owner approved a zero-cost update route and creation of an empty public
repository. No source history, binary or private deployment has been uploaded.
Installed clients remain alpha.93; automatic updates are not enabled.

## Foundation implemented

The releaseauth package signs and verifies a bounded release envelope with the Go
standard library's Ed25519 implementation. Trust comes from an independently
provided publisher key, never from the downloaded envelope. Product, channel,
platform, expiry, release sequence and artifact size/hash are checked. Exact
payload bytes are domain-separated; duplicate JSON members and unsafe artifact
basenames are rejected. Tests use newly generated synthetic keys only.

The package exposes a checkpoint for the installer to persist and checks it on
subsequent verification; it does not yet persist or rotate publisher trust itself.
There is no downloader, installer execution or new network authority in this slice.

## Validation

Targeted signature and artifact tests passed, including tampering, wrong publisher,
wrong platform/channel, expiry, future dates, replay, same-sequence substitution,
truncated/oversized files, duplicate fields and reserved/traversal filenames.
Full scripts/check.ps1 passed, including Go tests/vet, contract/governance and
privacy checks, existing integration paths, all 54 Windows tests, single-instance
process tests and native Debug build (zero warnings/errors).

## Remaining delivery

Connect installer bootstrap and publisher key management to durable replay state;
add verified download/staging and data-compatible automatic rollback; expose the
flow in Center. Each host independently polls GitHub and updates itself; Center
aggregates version/status rather than pushing installations. Add jitter/backoff,
independent offline catch-up, mixed-version compatibility and active-session
protection. No claim of TUF conformance or Windows Authenticode
reputation is made. Paying for a certificate is not a prerequisite for this work.

## Configuration audit and delivery boundary

Code paths separate versioned application files from the current user's identity,
Desktop credential store, state database and connection-service settings. Gateway
startup settings are separately retained under LocalAppData/PFRemote/Gateway and
refer to external TLS files. The current local TLS references exist and are outside
the version directories. The compatibility adapter can additionally depend on the
legacy catalog/relay files under ProgramData; these are not embedded in releases.

The prior alpha.93 installation checked five identity/credential/connection hashes,
not every database row, Gateway startup setting or external configuration. A clean,
generic old-to-new package test with synthetic user data must cover all of those,
including restart and rollback, before asserting unattended update readiness.
Windows protected secrets belong to the original user context; changing the updater
service account must not silently create a different empty profile or new identity.

The installed alpha.93 processes were initially absent during this read audit.
Normal installed background startup succeeded; the daemon then loaded four devices,
eleven targets and fifty recent records. Identity and connection-service doctor
checks passed. This confirms current-profile loading after startup, not a complete
new-package upgrade/recovery test or end-to-end remote Desktop connection proof.

## M10.2 transport and durable trust slice

Implemented explicit non-overwriting trust bootstrap, publisher/channel/platform
binding, transactional release checkpoints and a persistent greatest-observed
clock. Normal open cannot recreate a missing trust database. Rejected documents
still record observed time; concurrent or failed persistence cannot return an
accepted release. This database belongs outside program/application-data rollback
and requires an OS-protected per-user directory supplied by the installer.

Added fixed GitHub channel-feed discovery and bounded HTTPS transport, with no
peer-controlled URL or key. Downloads reject HTTP redirects, excess size, changed
hashes and truncated content. Verified staging returns an open rewound handle;
failures remove their temporary file. No downloaded code executes in this slice.

Tests cover restart/replay/substitution, clock rollback, missing/corrupt state,
failed writes, repeated bootstrap, concurrent acceptance, full-width sequence
values, pinned discovery, verified download and failed-stage cleanup. Full
scripts/check.ps1 passed; final added discovery/concurrency cases also passed
go test ./internal/releaseauth and go vet ./internal/releaseauth.

M10.2 stays open: these are internal library paths, not an installed background
updater. Installer trust provisioning, polling/backoff, session draining, data
compatibility, health rollback, Center status and real package validation remain.
Gateway persistence of authorized update hints and reconnect catch-up follow that
local lifecycle work. No public release or private deployment mutation occurred.

## M10.2 installer recovery slice

Setup now uses a journaled upgrade path for existing installations and recovers
interrupted upgrades on normal sign-in startup. New-version activation failure
restores the exact old version selection and attempts old-version activation.
If that recovery also fails, the journal remains for the next retry. Maintenance
has an OS-held exclusive lock; old versions are not pruned before health succeeds.
State replacement no longer temporarily removes current.json. Corrupt existing
state and unsafe version paths fail closed. Payload hashing streams the archive.

The Windows activator waits for selected-version background processes, a working
protected local API, runtime and identity. Remote catalog expiry alone does not
trigger rollback. Process stop waits now hold the required synchronization right
and check termination completion. No private directory is copied or rewritten.

Unit tests cover healthy commit, failed activation, exact restoration, interrupted
recovery, repeated failed recovery, maintenance exclusion, corrupt state, unsafe
versions and six external synthetic data files. scripts/check.ps1 passed, followed
by targeted package tests/vet after final hardening. An alpha.94 development-only
package was built and the isolated package upgrade script verified alpha.93 to
alpha.94 and rollback with the six external synthetic files unchanged. The script
also starts each packaged daemon (old, new, restored) with isolated AppData,
LocalAppData, ProgramData and named pipe. Its real CLI reads the retained identity
and target catalog after each transition; identity hash and canonical targets are
unchanged. This proves isolated profile loading, not private Gateway credentials,
real historical records or live remote connectivity. Automatic process health
failure is tested through injected callbacks, not by crashing the user's app.

The local installed product remains alpha.93. This candidate is not publicly
published or enabled for unattended upgrades. Data compatibility/session draining,
trusted bootstrap/discovery integration, background polling, visible update status
and Gateway notification remain open. Program rollback does not reverse database
migrations, and process presence does not certify Gateway TLS/service availability.

## M10.2 background discovery and compatibility slice

pfremoted now owns an independent background discovery loop and exposes its
redacted state as the `updates` doctor check. It does not require Center to stay
open. Initial checks use a one-to-two minute delay; successful checks repeat every
six hours with positive jitter. Failures back off from one minute to six hours,
plus jitter. Hint bursts coalesce and cannot bypass failure backoff. Each discovery
has a thirty-second context deadline, and shutdown cancels pending work.

Publisher key/channel are trusted build inputs. Unprovisioned builds explicitly
report bootstrap-required and make no update requests. A provisioned build opens
existing per-user update trust state for every check; it cannot recreate missing
state or learn keys from peers. No production publisher key has been provisioned
and these changes are not installed on the user's alpha.93 client.

Metadata v2 adds a signed data epoch and supported protocol range. V1 remains
verifiable but cannot satisfy automatic-install compatibility. The monitor reports
current, verified-update-available, compatibility-review or retry state without
exposing download/transport details. Native Settings presentation is not wired yet.

This slice does not install anything: Desktop requests return after launching
long-lived external viewers, so request completion is not a reliable idle signal.
Incoming controlled sessions and Gateway activity also require authoritative idle
checks. Session draining, installer bootstrap, visible status and Gateway hint
transport remain open; do not infer automatic-install readiness from discovery.

Unit coverage includes compatibility mismatch/immutability, unprovisioned/missing
trust, polling status, bounded failure backoff, hint-flood suppression and shutdown.
The daemon smoke path checks that an unprovisioned real daemon exposes bootstrap
required through protected IPC. Full check evidence is recorded in ACTIVE_WORK.

## M10.2 installed Windows loop — alpha.96

This section supersedes the earlier slice limitations above. The local Windows
client now has a pinned publisher key, trusted first-install checkpoint, independent
polling, verified HTTPS staging, a separately executing Setup and localized update
status in Settings. The Ed25519 publisher secret is outside source and payloads.
Windows Authenticode remains NotSigned; free update authentication does not supply
Windows publisher reputation. Go module identity matches the public repository.

Both the daemon and Setup verify the signed cached package. Compatibility is data
epoch 1 / protocol 1; unsupported metadata cannot activate. Missing initialized
trust state fails closed. An in-flight local action blocks update admission;
external viewer, SSH child and active non-console Windows sessions postpone Setup.
The current Gateway executable provides enrollment/control endpoints, not content
streams; idle HTTP keepalive sockets are not treated as active remote sessions.
Its brief restart may require control-request retry. Future content hosting needs
a stream drain gate. No host networking configuration or protocol executor is changed.

Installer results persist outside program versions. Failed or rolled-back release
sequences are held instead of repeatedly reinstalled. Recovery uses the existing
health-checked upgrade journal. Data schemas are unchanged; binary rollback is not
a general database migration rollback. Automatic installation is Windows-only.

Evidence on 2026-09-20:

- Full scripts/check.ps1 passed, including 56 Windows tests and native build.
- Real packaged alpha.95 to alpha.96 fixture verified signed local HTTPS download,
  downloaded Setup verification and activation, identity/catalog/history retention,
  DPAPI credential readability, and old-version rollback with the retained profile.
  This is a local HTTPS fixture, not a public GitHub or four-machine test.
- Final generic package verification passed: 540 payload files, 43 dependencies.
  UTF-8/UTF-16 scan of payload and release assets found no configured private terms.
- Local alpha.96 installation succeeded; 11 targets and 50 recent records loaded;
  all 8 recorded private identity/config/credential/Gateway file hashes were unchanged.
  Identity and connection-service checks passed; Center, daemon and Gateway each
  had one process. Both enumerated Center windows responded to background WM_NULL.
  No foreground visual-click validation is claimed.
- No private runtime directory was copied into source or release artifacts. Private
  backup copies are outside Git; live SQLite file copies are not certified snapshots.

M10.2 remains open: the public GitHub channel is not published, so production feed
fetches cannot yet deliver a release. Next is approved sanitized source/package
publication and real feed verification. M10.3 durable Gateway update hints, offline
reconnect catch-up and multi-host convergence remain unimplemented. Users do not
need to start all four machines for the local verification already completed.

## Authorized public preview publication — 2026-09-20

The owner explicitly authorized publication and enablement. Sanitized source was
published to https://github.com/cottman99/pf-remote as a new history with a generic
release author; existing private Git history was not pushed. Seven generic release
assets are published at https://github.com/cottman99/pf-remote/releases/tag/update-preview.
The channel is a prerelease, version 0.1.0-alpha.96, signed sequence 96.

Production Store.Discover fetched and authenticated the public GitHub feed using
the existing installed trust database and independently provisioned publisher key.
Production Stage downloaded all three installation artifacts from GitHub, and
VerifyPackage authenticated the downloaded installer, manifest and payload again.
All passed. No private runtime content or publisher private key was uploaded.

The running daemon retains its scheduled retry/backoff from the previously absent
feed until its next poll; publication does not remotely trigger or force an install.
No machine was downgraded just to demonstrate an upgrade. Unattended older-client
rollout, automatic Linux installation and M10.3 Gateway hints remain follow-ups.

## Fleet completion evidence

Alpha.102 supersedes the previous slice limitations: independent Windows and Linux
updates, durable peer notices and Settings controls are implemented and validated
on three live hosts. See FLEET_UPDATE_REPORT.md for exact evidence and the remaining
fourth-host availability/owner acceptance step.
