# Independent Linux workspace selection

Candidate: Windows alpha.111 (2026-10-15).

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
render and respond on an isolated desktop. Package and rollout verification are
recorded after release completion. Physical microphone and speaker acceptance
remains separate from desktop identity and login routing.
