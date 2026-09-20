---
schema_version: 1
roadmap_item: M10.2
roadmap_milestone: M10
roadmap_text: Deliver autonomous per-device GitHub update discovery, verified installation, data preservation and automatic failure rollback.
status: active
base_commit: 75b047e
---

# Active work — zero-cost trusted updates

## Objective

Complete the owner-approved zero-cost Windows update loop, keeping private user
state outside generic releases. Publish only the approved sanitized release.

## Acceptance criteria

Each host independently polls the public GitHub feed through its background
component with jitter/backoff; no controller must stay online. Trusted bootstrap
publisher key and durable replay state; verified bounded
release download and private staging; preflight data compatibility; safe installed
version switch with health-driven rollback; Center discovery/confirmation/result
flow. Preserve private identity, credentials, records and running access paths.
Older clients need bootstrap once. Do not call this finished from verifier tests.

## Out of scope

Paid services, public source or binary publication without the approved sanitized
release, and remote mutations. Cross-device version visibility is the next dependent item; controller-driven installation is not the default design.

## Current step

Local alpha.96 is installed. Trusted bootstrap seeds the initial signed checkpoint;
background discovery, verified staging, independent Setup activation, rollback and
localized Settings status are connected. Active action admission and conservative
Windows viewer/SSH/RDP checks postpone activation. The owner authorized publication:
sanitized source and alpha.96 are public, and the update-preview feed is enabled.
Production discovery plus all three installation artifact downloads were verified
from public GitHub using the existing trust store and production verifier.
The running daemon may still show its pre-publication retry until its next check.
M10.2 remains active for an unattended older-client rollout and Linux installation;
M10.3 durable Gateway hints and offline fleet catch-up remain next. No remote
machine was changed by this publication.

## Blocking side task

None; repository discovery does not block local trust verification.

## Review gates

R0: main at ef0d534, clean before this work; Windows automatic update loop scoped.
R1: private deployment and device identity keys are distinct from publisher keys;
trust anchors cannot be supplied by downloaded metadata. Fail closed on expiry,
replay, unsupported schema and artifact mismatches. No new network/IPC listener.
R2: reviewed pinned trust, replay persistence, cached artifact verification,
independent installer handoff, busy/retry/failed states and additive UI status.
R3: full scripts/check.ps1 passed; signed real-package alpha.95 to alpha.96
download/install/data-retention/rollback fixture passed; release verification passed.
R4: alpha.96 installed locally with unchanged private files and responsive native
windows. Public feed verification passed; evidence in TRUSTED_UPDATE_REPORT.
M10.2 is not closed from publication alone; unattended rollout remains to verify.

## Deferred findings

Publisher key rotation/revocation, new data epochs, Linux installer packaging and
multi-node convergence remain separate follow-ups. Current packages use data epoch
1 and do not migrate databases. Future Gateway content streaming must provide an
authoritative drain gate before restarting a content-hosting Gateway.

## Evidence

contracts/signed-release-v1.md and docs/status/TRUSTED_UPDATE_REPORT.md.

## Authorized fleet closeout

Owner now requests the complete multi-device update journey without intermediate
technical acceptance. M10.2 remains mainline; M10.3 notification/status integration
is an explicitly prioritized dependency of that journey. Authorized scope includes
bootstrap/update of available managed nodes and release publication, preserving
private identities/configuration and existing access paths. Current branch main
was clean at fd18f3c. Acceptance: named device versions, Check now and Notify devices
controls in Settings, durable authenticated notification across Gateway restart,
offline catch-up, independent publisher verification and safe local activation.
