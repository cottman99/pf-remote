# Default resource sharing — 2026-10-14

The owner wants desktop connections to carry sound and supported local devices
without choosing a separate resource-sharing mode each time.

Normal Linux workspace selection now lists the configured shared RDP workspaces.
They enter the existing TigerVNC desktops and carry picture, playback, microphone
and clipboard in one connection. Original VNC and ordinary RDP targets remain in
All desktops with their unchanged identities. An unavailable shared workspace
stays unavailable; it does not silently open a VNC sibling.

Windows RDP profiles explicitly enable playback, microphone, clipboard, supported
local drives, printers, serial ports, smart cards, WebAuthn, POS devices, location,
USB/MTP devices and cameras. They request full screen. The extra microphone menu
is removed. No resource-sharing question is added by PF Remote; native Windows
privacy, device support, authentication and certificate checks still apply.
Enabling redirection is not proof that every physical device works remotely.

Windows now also requires a trusted signed RDP file to avoid repeated resource
selection. Each controller creates a non-exportable local code-signing key and
pins its public certificate in the current user's trusted RDP publisher policy.
No global warning-disable setting or root CA is installed. Existing publisher
entries are preserved; server TLS validation remains enabled. Private keys stay
in Windows' user certificate store. Local profile signing fails closed and cleans
up the unsigned temporary profile. Initial policy preparation needs the owner's
existing administrative maintenance path; later ordinary opens reuse that trust.

The legacy microphone API flag remains accepted for older callers. RDP now
forwards microphone audio on ordinary open; VNC/external explicit microphone
requests are still rejected. Profiles contain no saved password and are removed
on client exit or launch failure.

The incomplete secondary-controller catalog is repaired through its private
optional sidecar. Its original catalog is preserved. No private configuration is
compiled into the public package. Existing desktop sessions are left alone.

Windows Home does not include rdpsign.exe. Alpha.108 adds a dependency-free
fallback using Windows/.NET certificate signing APIs. The secure-settings scope,
UTF-16 payload, SHA-256 signature and native envelope match rdpsign.exe; live
differential testing produced the same RSA signature. Tests reject modified
targets and duplicate settings. The non-exportable private key remains in the
Windows store, including on Home. No Windows edition upgrade is needed.

Laptop rollout also found that Windows' default scheduled-task settings blocked
interactive activation on battery, including rollback. Alpha.109 explicitly
allows activation on battery and keeps it running when AC power is removed.
Only PF Remote's task settings change; the host power plan remains unchanged.
The live laptop was restored and updated through an interactive, battery-compatible
maintenance task. Existing remote viewer sessions were preserved.

Delivery candidate: Windows alpha.109, publisher sequence 111. Linux node binaries
are unchanged because the existing shared-workspace channel services provide the
required transport. Actual hardware and clipboard round trips remain owner
acceptance; no active desktop or clipboard is manipulated during verification.
