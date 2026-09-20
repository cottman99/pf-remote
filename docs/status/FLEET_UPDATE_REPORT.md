# Fleet update delivery

## User-facing behavior

The Windows Settings and recovery page, in its version section, provides Check this computer, Notify other
computers, and named version reports with last-seen times. Missing reports are
explicitly shown; reports older than two minutes are marked stale. The UI uses
catalog names matched by immutable Device ID. Linux exposes the same authorized
coordination through the Agent/CLI flow, without adding a Linux desktop UI.

Each provisioned host discovers its own signed GitHub platform release, downloads
and verifies its own artifacts, and activates locally. No controller distributes
commands or packages to peers. A peer's report is not installation authority.
Older installations require a trusted bootstrap once.

## Notification and storage

Active enrolled Devices sign report/notification requests. The existing Gateway
checks identity, replay and revocation and durably coalesces notices. Offline
members compare their persisted hint cursor after reconnect. Periodic publisher
checks remain independent of the Gateway. Both Windows and Linux have sent real
notifications successfully through the deployed service.

Coordination uses a separate additive update_state table. It does not overwrite
the singleton enrollment authority or alter user configuration. A real SQLite
restart test covers preserved enrollment and retained hints. No private deployment
values or publisher secret enter the public source or packages.

## Activation and recovery

The daemon's ActionGate waits for its own running actions and temporarily prevents
new actions during installer handoff. Real fleet testing exposed a conservative
OS-process check that indefinitely blocked hosts with persistent SSH/RDP sessions.
It was removed: independent protocol executors and services are not binaries owned
by the updater, and must remain running. The Gateway entrypoint currently handles
control metadata, not content streams; future stream hosting needs its own drain.

Windows uses the existing independently running, health-checked Setup journal.
Linux supports an existing user-level pfremote-node.service installation and
updates only its managed binaries. A transient user service survives daemon
restart. An ExecStartPre recovery helper restores interrupted binary transactions;
failed health checks restore the previous binaries. Corrupt/incomplete journals
fail without removing installed binaries. Existing SSH authorization timers and
SSH/desktop services remain unchanged. Data epoch remains 1; this is not a general
database migration rollback mechanism.

## Evidence and rollout

- Full scripts/check.ps1 passed after the final client changes, including native
  build, contracts, unit tests, privacy and governance checks.
- Real signed Windows packages alpha.100 to alpha.102 passed download, installer
  handoff, profile retention and rollback verification in an isolated user profile.
- Local runtime retained 11 targets, 50 recent records and all 8 recorded private
  file hashes through bootstrap; Center retained one instance and responsive
  top-level windows. No foreground visual-click test is claimed.
- A second Windows host was bootstrapped from alpha.90, retaining 11 targets and
  50 records. Both Windows hosts are participating in independent background checks.
- The Linux host was deliberately taken offline at the management-service layer.
  A notice was saved while it was offline. After restart it independently updated
  from alpha.100 to alpha.102; its persisted installer result is complete.
- Public Windows alpha.102 metadata was authenticated against the installed trust
  store. All final package privacy scans passed; short binary substring matches
  were reviewed as framework symbols or non-text bytes, not private values.
- The fourth host's authorized SSH path is currently unreachable. No version or
  deployment success is inferred from a cached online label.

The remaining live rollout results are recorded below when observed. The only
owner-level follow-up should be fourth-host availability and final Settings UX
acceptance, once the online-host upgrade checks have completed.

## Final live result — alpha.102

Both Windows installations and the Linux managed node independently completed
alpha.100 to alpha.102 through the public update-preview channel. Each installer
persisted phase=complete/version=alpha.102. Windows and Linux can both publish a
notice; the deployed Gateway accepts and retains it. All three current reports
converged on alpha.102. The Linux upgrade followed deliberate management-service
offline/reconnect, not a controller-side installer invocation.

The local Windows profile still has 11 targets and 50 recent records; all 8 private
baseline file hashes match. Identity, connection-service and updates checks pass.
Center, daemon and Gateway each have one instance; both enumerated Center windows
respond to background health messages. The second Windows profile retains its
11 targets and 50 records, with identity/service/updates checks passing. Both remote
identity documents still match the immutable identities resolved before changes.
Temporary remote bootstrap tasks and download folders were removed.

The fourth-host result below supersedes the earlier unavailable-host observations.

## Fourth-host bootstrap and command diagnosis

The existing alternate SSH path reached the fourth Windows host and its on-disk
Device identity matched the freshly resolved canonical target. Initial state was
alpha.87, zero catalog targets and 48 history records. A compound deployment command
was rejected locally before execution; available policy rules and desktop logs did
not expose its exact rejection reason. Separately, the remote default PowerShell
execution policy rejected script files. These are distinct observations.

Separate artifact transfer and direct invocation of the released Setup executable
succeeded without changing execution policy or host networking. The remote Setup
SHA-256 matched the verified release package, and Setup returned completed with
current alpha.102 and previous alpha.87. All ten pre-existing private JSON hashes
remained unchanged. The catalog recovered to 11 targets; all 48 history records
remained. Identity, connection-service and update checks pass. Center and daemon
each have one process. All four fresh fleet reports show alpha.102/current, and
the fourth host successfully sent a notice through the deployed Gateway. Temporary
fourth-host installation media and the unused verification script were removed.

This fourth-host result proves trusted bootstrap and subsequent current-feed checks,
not an additional autonomous old-to-new upgrade. The earlier three-host upgrade and
Linux offline catch-up remain the autonomous-update evidence. The original PF Remote
Shell route still fails; the alternate SSH path works. Owner visual acceptance and
that separate connection defect remain open.

## Shell reconnect repair — alpha.103

The original Shell failure was a controller cache defect, not a remote permission
failure. A peer offline during catalog construction had no cached Tailscale node
ID. Acquisition rejected that path and selected an unreachable LAN endpoint without
probing it. The authenticated directory and SSH endpoint were healthy after the
peer returned. Refreshing only the controller daemon restored a normal PF Remote
Shell command to the exact immutable target; no private settings were changed.

Alpha.103 resolves only missing node IDs from the authenticated directory at
acquisition time, including merged private route overlays. Existing node IDs remain
pinned, and signed SSH host-key verification remains mandatory. The verifier remains
available even if no peers were online at startup. Direct Shell route acquisition
now checks reachability before selection; commands are never replayed on a new path.

Base/overlay cached-offline recovery, repeated offline rejection and changed-node
rejection tests pass. Full checks, package verification, the real alpha.102-to-103
upgrade/profile-retention fixture and binary/source privacy checks pass. The signed
Windows and Linux alpha.103 release is published; update-preview metadata was enabled
only after its artifacts uploaded. A fleet notice initiated independent discovery.

All four then independently updated to alpha.103 and reported current. Through the
updated controller, ordinary PF Remote Shell execution returned the fourth host's
expected name, and a second command read its connected fleet status with all four
reports current. The controller installer recorded phase=complete/sequence=103;
all eight private baseline hashes match, with 11 targets and 50 recent records.
Center and daemon each retain one process. This closes the Shell side task; only
owner-level UI acceptance remains. No foreground desktop interaction was used.
