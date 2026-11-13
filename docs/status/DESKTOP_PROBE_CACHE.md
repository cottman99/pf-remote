# Bounded desktop status probes

Alpha.118 formalizes the earlier local daemon repair. Repeated device-list and
inspect requests share route hints for at most 60 seconds, with a 256-entry bound
and one concurrent probe per target and directory version. Returned slices are
copied. Failed/panicking providers do not leave permanently blocked entries.

Only presentation metadata is cached. Authorization and real connection route
selection remain fresh; no credentials, session content or private deployment
configuration enters the cache or release. No public schema or data migration
is required. Existing alpha.117 clients remain protocol-compatible.

Tests cover expiry, target/directory separation, mutation isolation, concurrent
read coalescing, bounded storage and panic recovery.

## Published and deployed — 2026-11-13

Full non-UI repository checks and package validation pass. Isolated upgrade and
rollback preserve identity, target references, encrypted presets and selected
screens. The fixed GitHub release and preview feed each have twelve hash-verified
assets signed at sequence 120. Cloud control and download bundles are updated.

Both available Windows controllers match all 542 installed files; the Linux
controlled node's three binaries also match. Each Windows controller reaches all
three online members through the required Gateway. Saved identity/credential
files are unchanged; the existing RDP viewer and Linux virtual desktops/audio
services remain running. Stable MCP target inspection and remote execution pass.

Old Windows client processes and binaries were retired to protected recovery
storage. An empty locked directory is scheduled for empty-only cleanup. One
unavailable controller still needs first migration; no offline update is claimed.
No foreground automation, host networking change or peer-management activation
was performed. Physical audio and keyboard were not retested during this upgrade.
