# Shared Linux desktop and audio candidate

The owner explicitly prioritizes alternating RDP/TigerVNC access to the same
existing desktop and bidirectional sound. This is a development candidate,
not a released or end-to-end verified feature.

The administrator-only installer appends fixed loopback VNC backend entries to
xrdp 0.10, retaining existing Xorg/Xvnc entries. It never starts or stops an X
server. Per-display user channel services supply RDP channels; separate PipeWire
module processes expose named speaker/microphone nodes. No global audio default,
network rule, password file, or PF Remote capability identity is changed.

In VNC viewing mode an RDP connection to the same workspace must remain alive
as the audio carrier. This is not audio transport over VNC. Native-client launch
integration and an explicit microphone control remain pending. Current ordinary
RDP and VNC capabilities must not be presented as interchangeable until the live
mapping and client integration are verified.

## Operator setup

The deployment bundle contains `linux-shared-desktop.py`,
`install-linux-shared-desktop.sh`, and a Git archive of upstream
`neutrinolabs/pipewire-module-xrdp` v0.2, commit
`748f562d616590c678d5b06722f3fe5ae9707465`. Generate the archive using
`git archive --format=tar --prefix=pipewire-module-xrdp/ HEAD`; its SHA256 is pinned
in the installer. Upstream source and its license stay together in that archive.
The script uses EL9 package repositories for build dependencies, installs only
the upstream module, and retains a private on-server xrdp config backup.

Run the installer as administrator with the existing desktop account. It targets
existing displays 2, 3 and 4. Setup restarts xrdp (disconnecting any RDP clients),
but does not restart TigerVNC or the applications inside its desktops.

Rollback: `sudo python3 linux-shared-desktop.py rollback --user DESKTOP_USER`.
It removes the managed entries and services while retaining unrelated later
configuration edits. The inert installed module and distribution build packages
remain; removing OS packages automatically could damage unrelated dependencies.

## Evidence and remaining gate

Live preflight: xrdp and sesman active; three existing Xvnc processes; a standalone
channel server successfully started for an existing workspace and exited cleanly.
Initial preflight found the xrdp module and development headers absent, with
noninteractive sudo denied. The owner subsequently enabled passwordless sudo;
`sudo -n true` passed and the staged installer completed successfully. Distribution
dependencies updated PipeWire to 1.4.11 and the pinned upstream module was built
and installed. All six channel/audio services are active, with three named
speaker nodes and three named microphone nodes visible in live PipeWire state.
All three original Xvnc process IDs survive; fixed xrdp mappings match their VNC
ports. The PulseAudio compatibility socket is active. Client audio controls and
end-to-end alternating-client/audio verification remain pending.

Preparation verification passed: four Python regression tests (mapping, rejected
inputs, channel/audio service lifetime and non-destructive config rollback), Linux
`systemd-analyze --user verify` for all six generated units, shell syntax and bundle
SHA256 checks, and the full Windows `scripts/check.ps1`. The bundle is staged in
the authorized desktop owner's private local-share directory. These checks do
not establish successful installation or sound transport.

Acceptance after installation: preserve window identity and a visible workspace
marker across RDP -> VNC -> RDP; test generated playback and synthetic microphone
audio separately; test disconnect/reconnect and rollback; confirm all three
workspaces and original VNC access survive. Do not record private microphone
content as automated test evidence.

## Isolated RDP audio verification

Live tests now establish TLS-authenticated FreeRDP connections to the fixed VNC
backends for all three workspaces, using an Xvfb client display and dedicated
virtual audio devices. Certificate trust is pinned from the authorized server;
credentials stay in memory and enter FreeRDP through stdin. No physical microphone,
speaker, foreground desktop or clipboard was used. Test clients and virtual nodes
are removed on exit.

Both directions carry the generated 997 Hz tone. Workspace 2 also passes repeated
connection after disconnect. All three original Xvnc processes and their window
lists remain unchanged across the tests. This proves the server-side RDP round
trip, not Windows client integration, WAN performance or physical audio quality.

A playback defect remains: internal silence measured approximately 43–86 ms in
the short tone tests. A direct PipeWire control and RDP microphone return both
measure only 0.02 ms around waveform zero crossings. Per-node latency 2048 did
not remove the playback gap and the trial was reverted. Do not claim audio quality
acceptance; investigate the RDP playback path independently before tuning more.
Windows native client audio controls and actual local speaker/microphone acceptance
remain incomplete. The earlier short-tone duration-based frequency estimate was
affected by this silence; measuring individual cycles confirms approximately
997 Hz while retaining the gap as a separate failing quality observation.

## Owner acceptance audit

The current acceptance verdict is **not passed**. The ordinary Windows client
still launches mstsc with an endpoint only: it does not explicitly enable audio
capture, select one shared workspace, or own a VNC audio-carrier lifecycle.
The installed catalog retains three VNC targets and one ordinary xrdp target;
these entries do not yet offer a user-operable shared-workspace audio workflow.

Windows Core Audio reports one active render endpoint and zero active capture
endpoints. WTS reports RDP protocol for the current Windows session; microphone
privacy is Allow and no capture-denial policy was observed. Thus the upstream
RDP session currently has no microphone available to forward to Linux. Enable
audio recording redirection in the client connecting to this Windows session
and reconnect before physical microphone acceptance. No user's microphone was
recorded during this audit.

A six-second isolated test on shared workspace 2 confirms approximately 997 Hz
in both directions. The playback gap is 85.76 ms at 0.341 seconds after the first
nonzero sample, with no further >10 ms gap in that sample; microphone capture has
no >10 ms gap. Original X server and window list survive. This narrows the defect
to playback startup in the tested FreeRDP path; it does not clear native Windows
audio quality, reconnection stability, or physical hardware acceptance.

## Local preview closeout — 2026-09-30

| Feature | Current usability |
| --- | --- |
| Shared desktop identity | Three added RDP choices use the existing TigerVNC workspaces; original canonical references remain stable. |
| Playback | Native RDP profile explicitly redirects playback; server and synthetic transport checks pass. Physical listening still requires the owner. |
| Microphone | Explicit native menu action; ordinary opening leaves capture off. Current Windows has no active microphone because upstream RDP has not forwarded it. |
| Application audio | Per-user helper maps process DISPLAY to per-stream speaker/source metadata, leaving global defaults and explicit non-PF device choices alone. Default playback streams tested on all three workspaces link correctly. |
| VNC voice | Keep the corresponding shared RDP connection alive while using VNC picture. No automatic companion process ownership is implemented. |
| Distribution | Local signed alpha.106 audio preview.3 is installed. No GitHub or fleet publication in this task. Healthy alpha.105 rollback restored and authenticated. |

The previous default FreeRDP Pulse backend gap is not fully cleared: workspace 4
still measured a 43 ms discontinuity after server idle processing was enabled.
An explicit 200 ms client buffer clears the measured gap in the acceptance script.
All three six-second samples and a ten-second sample carry approximately 997 Hz
in both directions without internal silence exceeding 10 ms. This is bounded
software transport evidence, not proof of native Windows hardware or WAN quality.

Generic verification is scripts/verify-linux-shared-audio.py. It uses only generated
samples and disposable virtual devices, authenticates the server certificate,
keeps VNC credentials in memory, and removes clients/nodes/sample files on exit.
A connected audio listener may disappear from the directory after accepting its
one stream; presence of the pathname alone is not a health criterion.

The new shared desktop sidecar originally revealed a legacy identity bug: deriving
Fabric identity from the expanded inventory changed the signed Shell context.
The loader now retains the original inventory as the Fabric anchor. Regression
checks preserve all original capability profiles and IDs through add/remove of
the sidecar. Live current catalog retains the original Fabric and Device, adds
three available shared desktops and preserves the original successful Shell path.

Validation: full Windows check, 58 presentation tests, six Python tests, unit tests
for explicit request-scoped consent, native RDP profile cleanup and injection
rejection, protected API consent forwarding and rejected non-RDP capture, sidecar
identity/rollback compatibility, authenticated package verification and responsive
isolated native UI using the actual protected catalog. Private UI evidence is
outside Git. Local profiles, routes, credentials and gateways are not packaged.

Next owner action: enable recording redirection in the upstream RDP client,
reconnect to this Windows computer, then select a shared desktop's Open with
microphone action. Until then the UI explains the missing device; do not declare
physical microphone acceptance passed. Replay state stays monotonic despite the
available program rollback. No proxy, DNS, route, VPN or existing X server changed.

## Published acceptance preview — 2026-10-01

The owner authorized release before physical hardware acceptance so the second
Windows controller can be used for testing. Alpha.106 is published with the
existing Ed25519 publisher signature (sequence 108); the Windows binaries remain
without paid Authenticode signing. Sanitized source is published independently
from all private runtime configuration. Full checks, isolated native-window and
package verification, and alpha.105-to-alpha.106 configuration retention pass.
The optional shared-workspace sidecar is installed privately on the testing
controller; its original catalog remains byte-for-byte unchanged. Hardware and
WAN audio acceptance remain pending. The Linux node release is unchanged.

Live rollout completed: the second controller runs alpha.106 with alpha.105 as
rollback, and its signed update completion records sequence 108. All six private
baseline JSON files are unchanged; 50 history records remain readable. Protected
catalog has 14 targets including three available shared workspaces. The native
Center and daemon run in the logged-in user session and respond; identity,
catalog, connection-service synchronization and latest-release checks pass.

The first background discovery returned retry, although independent HTTPS
download reached the published descriptor. The controller downloaded the fixed
release using its existing Windows network environment, matched all four
artifact hashes against the reviewed package, and ran the signed apply-update
installer. Publisher/replay verification remained enabled. The final independent
update check reports current; the transient discovery failure is not established
as resolved for every future network condition. The owner controller also
independently activated alpha.106 from the preview feed during verification.
