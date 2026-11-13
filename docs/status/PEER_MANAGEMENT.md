# Peer management delivery

Release candidate: alpha.120. Daily migration verification is in progress.
The earlier alpha.118-peer.2 pilot and internal alpha.119 acceptance package are
not public release artifacts.

## Implemented

Pinned-root, Device-signed management records, exact expiring grants, independent
manager enrollment, permanent revocation and atomic journal merging. Protected
local API, CLI and MCP share the same identity and authorization core. Existing
native SSH and signed host-key bindings perform remote execution.

Daily activation retains original canonical targets, local credentials and native
viewer processes. Installers reject incompatible downgrade after activation;
recovery cannot silently restore legacy authority. Missing activated authority
fails closed. Signed update sequence guards also reject older published packages.
These are supported-application controls, not restrictions on an OS administrator
who deliberately replaces both programs and protected state.

## Evidence

Targeted security, migration, rollback, missing-state and compatibility tests pass.
The full repository check and isolated upgrade preserve encrypted accounts and
screen presets. Daily Windows catalog/MCP and Linux execution now reach the newly
admitted Shell with its original Device identity. Final package and publication
verification follow before this report marks delivery complete.

## Limits

No new desktop environment is installed on the headless node. Root recovery and
key replacement require explicit re-enrollment. A bounded full-journal exchange
requires checkpointing before its size limit. Offline members learn revocations
when they reconnect; no instantaneous partition-wide revocation is claimed.
Existing native sessions retain OS lifecycle behavior. Legacy cloud services are
retained for existing transport and mixed-version compatibility.
