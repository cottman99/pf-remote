# Client core interaction audit — 2026-10-14

The released client has working desktop launch and microphone consent controls,
but desktop, audio, clipboard and full-screen operation are not yet one complete
user journey on every controller.

| Area | Current evidence | Remaining gap |
| --- | --- | --- |
| Desktop launch | Native clients and Centers respond. Existing capabilities remain available. | A secondary online controller lacks all three shared RDP entries despite running the latest Windows package. Binary updates preserve private catalogs; they do not distribute the optional sidecar. |
| Linux playback | All seven shared-workspace services are active; three speaker nodes run. | No established connection on the configured RDP listener at inspection. VNC does not automatically start its corresponding RDP audio carrier. |
| Microphone | Secondary controller has an active microphone endpoint. Native RDP microphone opening requires explicit consent. | Microphone nodes are suspended; physical transmission is not verified. The main desktop chooser does not expose microphone opening; it exists in the individual desktop route menu. |
| Clipboard | Linux cliprdr is enabled. Secondary controller's default RDP profile enables clipboard redirection; no inspected policy disables it. | PF-generated RDP profiles omit an explicit clipboard property. Actual bidirectional text, image and file transfer are not verified; PF exposes no connection-level clipboard control. |
| Full screen | Secondary controller's default RDP profile requests full screen and multiple monitors. Existing protocol clients support full-screen operation. | PF-generated profiles and VNC launch arguments do not explicitly select full screen, and PF has no unified entry. Current rendered full-screen state was not inspected. |

The inspection did not open, close or manipulate existing desktops, replace
clipboard contents, record microphone samples, or change private profiles.
Availability, enabled configuration, an active transport and physical acceptance
are separate evidence levels. The examined RDP process cannot be assumed to be
connected to the Linux workspace merely because it is running.

Priority: restore shared-workspace choices on every controller, complete the VNC
audio-carrier lifecycle, and expose visible per-connection audio, microphone,
clipboard and full-screen choices. Retain explicit microphone consent. Then
verify actual sound and bidirectional clipboard operations in an isolated session.

Desktop executor, migration and protected-API regression tests pass. The findings
above concern deployment completeness and user-visible interaction; they are not
closed by those tests. No client patch or release was produced by this audit.
