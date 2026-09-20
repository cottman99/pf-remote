# PF_REMOTE_TARGET/1

The minimal copyable target envelope is UTF-8 text:

```text
PF_REMOTE_TARGET/1
target: pfremote://fabric-demo/devices/device-demo/capabilities/shell-main
alias: demo-device/shell
kind: shell
verify: pfremote inspect pfremote://fabric-demo/devices/device-demo/capabilities/shell-main --json
```

Required fields are `target`, `alias`, `kind`, and `verify`. Field order is
stable. Values must not contain credentials, private keys, tokens, route leases,
relay secrets, or passwords. Unknown additional fields may be ignored by a v1
reader. Unknown major versions must be rejected.

