# Independent Linux workspace selection

Released: Windows alpha.111, publisher sequence 113 (2026-10-15).

The three Linux choices must continue the original independent TigerVNC
workspaces. RDP and VNC share one numbered workspace; different numbers never
mean the same desktop. Existing applications and X servers remain intact.

## Observed failure and repair

Native Windows testing reproduced two failures. Without saved workspace login,
xrdp's interactive login chooses its default module. With a saved Windows account,
Credential Manager can replace the separate RDP domain. Concurrent connections
to one server also share this cache: a profile selecting workspace 2 reached
workspace 4 when the cache held workspace 4's account.

The executor now qualifies the account with the authenticated workspace selector
and carries a current-user DPAPI password envelope in each temporary native
profile. It no longer writes a shared native credential before launch. The same
conflicting-cache native test now reaches workspace 2. Plaintext passwords stay
out of files, process arguments, logs and public artifacts. Failure and client
exit remove the temporary profile.

Missing workspace login opens local save-and-connect setup for the exact selected
target. The interface explains that each number is independent and that the
access password belongs to the original TigerVNC desktop. Ordinary Windows RDP
and its native domain-account flow remain supported. Original VNC targets and
canonical identities are unchanged.

## Evidence and limits

Native MSTSC runs on an isolated Windows desktop, with the server certificate
verified through the authorized Shell identity. Synthetic invalid credentials
stop before opening a real desktop. Server-side connection metadata confirms
the three selected backends and reproduces the conflicting-cache failure and
repair. No keyboard/mouse input, real desktop resize, X server restart, or active
viewer interruption is issued.

Regression tests cover missing and ambiguous credentials, exact qualified
accounts, ordinary Windows compatibility, Unicode DPAPI round-trip, tamper
rejection, and encrypted-profile cleanup after signing failure. Native settings
render and respond on an isolated desktop. Physical microphone and speaker acceptance
remains separate from desktop identity and login routing.

## Delivery

Full repository checks and 61 presentation tests pass. The signed release
metadata, 542-file Windows package, 44 dependencies, upgrade, rollback and
re-upgrade checks pass. Encrypted presets, original identity and catalog survive.
All seven fixed-release asset digests match; preview payloads are verified before
the signed metadata is published last.

Both reachable Windows controllers independently updated from alpha.110 to
alpha.111 and report current. Their 10 and 11 protected baseline files remain
unchanged after the intentional preset migration; each retains 14 targets and
three saved workspace logins. The original TigerVNC credentials are unchanged.
Center and daemon are responsive, and the existing viewer on the second
controller survives the update.

The third controller remains management-unreachable with a stale alpha.109
report; a durable update notice is retained. Its preset migration and one-time
publisher trust preparation await reachability. The Linux node remains on its
current platform release; its three original Xvnc processes and ordinary Xorg
session are unchanged. Existing connections keep their old settings until the
owner reconnects; no current desktop is switched automatically.
