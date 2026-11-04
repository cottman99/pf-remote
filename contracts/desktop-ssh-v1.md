# Private Desktop SSH mapping v1

An optional protected `desktop-ssh-v1.json` beside the local state database maps
an existing VNC Desktop to the Shell Capability on the same immutable Device.
Schema `pfremote.desktop-ssh/v1` contains `targets`, each with `desktop_target`,
`shell_target`, and `loopback_port`. No account, address, key or secret is exported.
Malformed, duplicate, cross-Fabric and cross-Device mappings fail at startup.

Both capabilities must be authorized. The Shell coordinator verifies its
Device-signed host binding before the installed OpenSSH client authenticates the
remote host. The required FRP route carries SSH; the Gateway cannot terminate the
Desktop encryption. Only remote loopback may be forwarded. Local VNC credentials
remain required, and the listener binds only to local loopback.

The initial check reads only the VNC protocol banner, without entering a desktop.
The native viewer owns forwarding lifetime; completing a local API request does
not close it. Viewer exit, startup failure, authorization expiry or an observed
revocation closes the forward. There is no direct-route or plaintext fallback.

Absence preserves existing installations. Older clients do not understand the
new mapping; deploy the supporting client before activating the private profile.
The private route must point to the same Device's SSH service, so an older client
cannot accidentally establish an unencrypted VNC session through that endpoint.
