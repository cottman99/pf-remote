# Private local routing v1

Optional `local-routing-v1.json` resides beside the protected state database.
Its schema is `pfremote.local-routing/v1`; `catalog_path` and `frp_config_path`
are optional absolute regular-file paths. They remain local and never enter a
target, context export, package or Gateway metadata. Unknown fields, invalid
sources or unsupported schema versions fail closed at startup.

An absent file preserves existing compatibility behavior. `required_adapter`
may be absent or `frp`. The latter requires a configured managed FRP visitor
file, and restricts automatic Shell and Desktop selection to Gateway routes.
It never silently falls back to LAN or Tailscale. Explicit Desktop route choices
remain recovery actions. Established sessions keep their selected route.

Managed visitors must match the exact private service name and credential.
Route availability never replaces Device-signed SSH host-key verification or
native Desktop authentication. No host proxy, VPN, DNS or routing change occurs.
Migration preserves canonical identities, presets and independent desktop/audio
services. Keep a protected configuration backup before removing old software.
