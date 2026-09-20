# M6.16 three-Windows client replacement

## Product outcome

All three internal Windows computers now use the same PF Remote candidate as
their normal entry. Each client presents one shared inventory of four named
computers and eleven applicable Desktop or Shell capabilities. The old product
is unchanged and remains available as rollback rather than as the daily entry.

From any Windows client, a person can now choose another computer in the new
Center, open a supported Desktop, or hand the same named computer to an Agent.
The Agent can inspect and act on the exact target through the installed MCP
server without requiring addresses, routes, or command-line setup from the
user.

## User-visible verification

- All nine cross-Windows and Windows-to-Linux Shell actions completed through
  the installed candidate.
- The installed MCP server exposed seven PF Remote actions on every Windows
  client, and one real harmless action completed from each client against a
  different computer.
- Every Windows endpoint that supports Remote Desktop completed an RDP protocol
  handshake from every applicable other Windows client.
- The additional Windows client launched a real physical-screen VNC viewer for
  the Windows Home computer and a real Linux virtual-Desktop viewer through the
  selected Tailscale route. The Windows Home client launched the Linux viewer
  through the Gateway route. Test viewers were closed after verification.
- Protected one-time Linux Desktop setup and the physical-screen route are now
  present on the additional clients. No developer terminal is needed for the
  normal connection journey.

## Product problem found and corrected

One Windows Home computer had both its real physical-screen connection and a
generated RDP connection in the shared list. Windows Home cannot host the native
RDP service, so the second action looked useful but could never work. The false
entry was removed, while the real physical-screen connection remains available
through the new Center. The product now reflects the computer's actual ability
instead of pretending every Windows edition supports the same protocol.

Each client can still see its own Desktop as unavailable. Connecting a computer
to itself is not a useful journey, so this does not block replacement, but the
Center should eventually present it as "this computer" instead of a setup
failure.

## Confidence and remaining risk

The complete repository check passes, including 44 Windows UI tests, the native
build, contract validation, private-data hygiene, and the full Go test suite.
All PF Remote-started test viewers, credential-transfer helpers, and deployment
tasks are removed after verification. Private addresses, credentials, device
identities, and deployment files remain outside the repository.

Automatic wake from host sleep remains unproved and stays as the first unchecked
M6 item. It is not required for ordinary three-client use and will not displace
higher-value usability work unless a safely bounded validation path is ready.

## Alibaba Desktop repair update

The additional Windows controller can now open a real Windows-host RDP session
through the Gateway choice. The failure was not route scoring: Alibaba rejected
the host as an unknown standalone FRP device. The repair reuses the controller's
existing authorized Alibaba SSH service as the carrier, then forwards the RDP
stream over the local network. This produced both a successful RDP client
connection event on the controller and an active RDP session on the intended
Windows host after installing `0.1.0-alpha.83`.

The carrier and tunnel start automatically after logon. The previous FRP task
action, older installed candidates, and the untouched old product remain
available for rollback. This private deployment topology is not stored in the
public repository.

## Final alpha.83 fleet audit

All three Windows clients now report `0.1.0-alpha.83` as current and retain
their preceding installed candidate as rollback. Each has exactly one current
daemon and one current Center, shows the same eleven-target inventory, and
reports all four Shell capabilities available. Harmless PowerShell actions
completed in all six directions between the three Windows clients.

The reverse Gateway journey was repaired as well. Its managed visitor had not
restarted after upgrade and its deployment identity no longer matched the
authorized Gateway identity. After restoring the reviewed identity and the
existing logon task, the local visitor stayed running and returned a valid RDP
protocol response from the intended Windows Desktop. The other Alibaba Windows
route also returned a valid RDP response through its retained carrier tunnel.

One endpoint deliberately retains Git Bash as its OpenSSH default shell for
the user's existing workflows. Windows `cmd.exe /c` is not a valid automation
smoke command through that shell because MSYS rewrites the `/c` argument;
PowerShell is the supported Windows Agent executor and passed in both
directions. No user shell or host network setting was changed to make the test
pass.

## Alpha.86 visible-session and stable-refresh repair

Two remote Windows clients exposed two daily-use failures after the later candidate
upgrade. An SSH-driven install could leave the new Center in the signed-in
desktop while the daemon remained in Windows session zero. RDP or VNC then
started successfully but its native window was invisible to the user. The
Center also rebuilt every computer card on a periodic catalog probe, causing a
visible flash and closing expanded details even when the catalog was unchanged.

`0.1.0-alpha.86` corrects both failures. Setup hands activation to the signed-in
desktop, starts the daemon before the Center, and requires both components in
that same desktop session before reporting success. The Center compares
catalog content before replacing UI objects, retains expanded cards and recent
session objects, and limits the full-catalog probe to once per minute while
keeping the lightweight connection-health check independent.

Both affected computers now report alpha.86 as current. The Windows controller
retained one daemon and one Center in its signed-in session and launched a real
alpha.86 RDP client there. The Windows Home client retained one daemon and one
Center in its signed-in session and launched a real alpha.86 TigerVNC client
there against an available Linux desktop.
The test clients were closed afterward; alpha.85 remains Windows controller client's rollback and
alpha.84 remains Windows Home client's rollback. An isolated WinUI check also preserved the
same expanded device container through three refresh cycles without keyboard
or mouse injection. The alpha.86 release verification and complete repository
check pass.

## Alpha.87 forced-Gateway recovery

Forcing the Alibaba Gateway route on Windows controller client and Windows Home client reproduced a real
failure even though the route picker labelled Gateway available. Both managed
FRP visitor tasks were configured but stopped, so configuration presence had
been mistaken for live reachability. The existing authorized tasks were
restored without changing endpoints, credentials, proxy, DNS, VPN, or route
configuration. A one-minute repeating trigger now complements the existing
logon trigger and ignores duplicate starts while the visitor is healthy.

A deliberate process-termination test then recovered Windows Home client in 36 seconds and
Windows controller client in 40 seconds. After installing `0.1.0-alpha.87`, both clients forced
the `frp` route, launched the RDP client in the signed-in desktop, and received
valid RDP protocol responses. Gateway status now probes the live provider
instead of reporting every configured visitor as available. All three Windows
clients run alpha.87, retain alpha.86 or an earlier installed candidate for
rollback, and keep responsive Center and daemon processes in their signed-in
sessions. Release verification and the complete repository check pass.

## Alpha.88 end-to-end Gateway health

A subsequent failure was traced to the controller's legacy publishing service,
not the cloud Gateway. Its local catalog had been replaced by zero-filled
content, so the service exited during JSON loading while the independent
Tailscale route remained usable. The damaged file was preserved, the live file
was atomically restored from an existing validated private backup, and the
service plus its FRP publisher returned to a stable running state. From the
affected Windows client, the Gateway visitor then returned a complete RDP
negotiation response from the intended controller Desktop.

The earlier live-listener probe was still insufficient because an orphaned FRP
visitor can accept TCP while its target publisher is absent. Alpha.88 keeps the
existing provider and route order but adds the smallest protocol-specific
check: an RDP Gateway candidate must answer an RDP negotiation before it is
shown as available or selected by Smart connect. Unit coverage includes a real
RDP response and an orphaned-listener rejection. Release verification and the
complete repository check pass. Alpha.88 is installed on the controller and
the affected Windows client with alpha.87 retained for rollback; the third
Windows client was offline and remains on its last verified candidate.

## Alpha.89 setup reliability follow-up

Follow-up alpha.90 fixes the disabled manual Gateway menu: known configured
routes can be retried after an unsuccessful health sample; missing or blocked
routes remain disabled. The menu refreshes from the latest catalog when opened.
The affected Windows client is on alpha.90 with alpha.89 retained. Full checks
and package verification passed. Direct RDP probes succeeded three times, but
catalog availability remains intermittent and full Desktop login is still
unverified. This is a retry-UX repair, not closure of the connectivity incident.

Alpha.89 keeps the same real RDP Gateway check and corrects the setup timeout
that could otherwise misreport a healthy local daemon while adding a computer.
The invitation live-reload journey passed three consecutive runs, followed by
the complete repository check and Windows release verification. Alpha.89 is
current on the controller and affected Windows client, alpha.88 is retained for
rollback, and the affected client reports the controller Gateway route
available only after the RDP negotiation succeeds.
