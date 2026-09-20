# M4.6 browser-emergency necessity decision

## Product decision

PF Remote will not build a browser-emergency client in the first release. The
roadmap item is closed as a deliberate deferral, not as a partially implemented
feature. Native Center and external Agent alignment remain the daily journey.

The meaningful emergency moment is clear: the Owner has only a borrowed or
locked-down computer, cannot install PF Remote, and needs temporary access to an
already authorized machine. A browser list by itself does not complete that
journey. It would show the same names but still require an installed RDP/VNC/SSH
client, creating no usable advantage over the native product.

## Research result

Comparable products that truly work without a client do substantially more
than expose a device list:

- Chrome Remote Desktop combines a web entry point with its remote-access host
  and Google's web transport infrastructure.
- Apache Guacamole is explicitly a clientless remote-desktop Gateway. Its
  server translates RDP, VNC, or SSH into a browser protocol over HTTP/WebSocket.
- Teleport Desktop Access adds a Desktop Service and a proprietary browser
  protocol that translates to RDP while applying its own role authorization.
- RustDesk lists Web as a client platform, while its self-hosted documentation
  still exposes browser-specific WebSocket/CORS deployment requirements.

Those are valid product architectures, but each adds a browser session
executor/translator and an internet-facing authentication surface. Building a
thin imitation would not be useful; building the complete version now would
duplicate the mature protocol execution that PF Remote intentionally delegates
to existing clients and would compete with the higher-value Agent alignment,
onboarding, migration, and release work.

## Preserved future boundary

A future browser adapter may be reconsidered only as an integration with a
mature clientless Gateway. It must consume PF Remote's immutable targets,
Grants, expiry, and revocation; it may not create a second directory or parallel
permissions. Emergency sessions must be visibly temporary, online-only,
short-lived, leave no durable browser secret, and provide explicit sign-out and
borrowed-device cleanup. The Gateway must not gain access to plaintext PF Remote
session content merely to support a browser.

## User impact and evidence

No incomplete web button or settings surface was added, so the user cannot
mistake a placeholder for emergency access. M4's completed visible increment is
therefore coherent: native one-click Desktop actions, exact Agent handoff,
multiple named Desktops, and encrypted recovery. `scripts/check.ps1` remained
green when M4.5 closed; this decision changes documentation and priority only.

Research sources:

- [Chrome Remote Desktop](https://remotedesktop.google.com/)
- [Apache Guacamole introduction](https://guacamole.apache.org/doc/gug/introduction.html)
- [Teleport Desktop Access](https://goteleport.com/docs/enroll-resources/desktop-access/getting-started/)
- [RustDesk client platforms](https://rustdesk.com/docs/en/client/)
