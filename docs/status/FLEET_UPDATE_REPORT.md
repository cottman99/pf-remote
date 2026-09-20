# Fleet update delivery

## User-facing behavior

Windows Settings > Version and updates provides Check this computer, Notify other
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

The fourth host still fails its management connection. It has not been updated or
reported as current. Its availability and the owner's Settings experience are the
remaining acceptance items; no additional internal approval is required.
