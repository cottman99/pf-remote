# Fleet update coordination v1

POST /api/v1/updates/sync accepts a device-signed request bound to schema, Device,
request ID, channel, platform, installed version, status and notify flag. Only
active enrolled Devices may report or notify. Reports contain no routes, keys or
commands. Gateway persists latest reports and a monotonically increasing hint in
an independent versioned control-state row, preserving old enrollment decoding and
rollback. Request IDs use existing durable replay protection. Notices coalesce
within thirty seconds; reports from revoked devices are omitted. Each device
checks the publisher feed independently; a peer report never authorizes a package.

Protected local IPC exposes update-status, check-updates and notify-updates. UI
shows named devices and last seen, explicitly distinguishing stale reports from
current observations. Missing/old Gateway support reports unavailable, not success.
Offline devices compare durable local hint cursor after reconnect; periodic GitHub
polling remains independent. First upgrade/bootstrap announcements are coalesced.

## Managed Linux installation

Linux artifacts use the same signed envelope and trust policy with platform
linux-x64. The controlled-node installer preserves the existing user service,
configuration and SSH authorization timer. It adds a recovery pre-start step to
pfremote-node.service and replaces only the managed binaries. Publisher signatures
and hashes are checked before a transient updater service executes. Protocol
servers outside PF Remote's service are not restarted.

Activation waits for PF Remote-owned actions through ActionGate. Independent OS
SSH/RDP/VNC sessions do not block a metadata/control-daemon upgrade. The concrete
updater process ownership, rather than arbitrary remote-process presence, defines
what must be drained.
