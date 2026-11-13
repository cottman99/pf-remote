# Peer management delivery

Released: alpha.120. Daily installed-client migration verified on 2026-11-13.
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
admitted Shell with its original Device identity. Both Windows payloads match all 542 release files. Both Linux installations match
the signed binaries. Fixed and preview releases each verify all twelve required
assets, and all online clients report current. The two Windows MCP instances list
the unique new Shell and execute real commands. Peer synchronization remains
current after update-driven service restarts; services are enabled at boot.
Existing native viewers and proxy services retain their processes. Original
Shell paths still succeed. The isolated pilot services are disabled.

Private runtime state, host addresses, keys and acceptance evidence remain outside
the repository. Package scanning found no deployment markers in 553 payloads.
The offline member remains explicitly deferred; physical reboot and new desktop
interaction were not performed.

## Limits

No new desktop environment is installed on the headless node. Root recovery and
key replacement require explicit re-enrollment. A bounded full-journal exchange
requires checkpointing before its size limit. Offline members learn revocations
when they reconnect; no instantaneous partition-wide revocation is claimed.
Existing native sessions retain OS lifecycle behavior. Legacy cloud services are
retained for existing transport and mixed-version compatibility.
