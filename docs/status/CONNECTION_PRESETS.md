# Connection presets and quiet repeat access

Released: Windows alpha.110, publisher sequence 112 (2026-10-15).

## User journey

Open Settings → Connection presets and accounts, or the same action in a
computer's connection menu. Select the named desktop, enter its account once,
choose full screen and optionally make it that computer's one-click desktop.
Saving does not open a session. The next normal connection uses the preset.
Sound, microphone, clipboard and supported hardware sharing remain enabled.

Passwords are protected by the signed-in Windows user's DPAPI identity outside
the installation. The UI reports whether a password is saved, never its value.
A blank password keeps the current account's password; changing the account or
computer identity requires new credentials. Clear password also removes the
corresponding PF Remote-owned native credential cache. Exact Agent/CLI targets
continue to select the requested desktop independently of UI preferences.

Alpha.110 projected the selected preset into Windows Credential Manager's
per-server cache. The alpha.111 repair replaces this with a current-user DPAPI
envelope in each temporary native profile, preserving independent workspace
selection across concurrent connections. See WORKSPACE_SELECTION_FIX.md.
Windows retains its own credential and server
authentication rules. Accounts with an OS policy that requires interactive login
can still require a sign-in challenge.

## Repeated publisher prompt

The previous signed profile still showed an unknown-publisher resource dialog:
the self-signed publisher was in TrustedPublisher and the exact RDP allow-list,
but its certificate chain ended in UntrustedRoot. Native MSTSC reproduced this
on an isolated desktop.

One-time administrator setup now anchors only the exact local non-CA,
code-signing-only leaf in the machine certificate store. The private key remains
non-exportable in the owner's user store. A code-signing chain passes; the same
certificate fails TLS-server usage. Native MSTSC then proceeds to connection
without the publisher/resource prompt. Target authentication remains enabled;
no global warning suppression, network change or shared signing key is used.
New installations have an explicit repair button for this one-time setup.

## Verification

- Full repository checks, Go tests and 61 native presentation tests pass.
- Protected IPC save/read/clear, daemon restart, identity/desktop separation,
  update-maintenance exclusion and native vault ownership are checked.
- Native preset UI renders correctly and has a responsive top-level window
  on an isolated desktop; the user's active desktop is untouched.
- The signed 542-file package and dependency/license verification pass. The
  upgrade verifier installs alpha.109, upgrades to alpha.110, saves a protected
  preset, rolls back, then re-upgrades. Identity, targets and encrypted account
  settings remain readable.

## Delivery

The fixed GitHub release and the preview feed are published. All seven uploaded
asset digests match the verified local files. The two reachable Windows
controllers independently installed alpha.110 and report current. Their Center
and daemon are responsive in the signed-in sessions; the new preset API works.
All 24 and 27 private baseline files respectively match, including protected
desktop passwords; both retain 14 targets and 50 recent records. Their exact
local publisher chains pass code-signing use and reject TLS-server use.

No foreground UI or user input was automated. The existing RDP server session
remained connected across both installation completion times; its next recorded
disconnect/reconnect occurred several minutes after installation. No viewer
stop or reconnect was issued by deployment.

The third Windows controller has a stale heartbeat and an unreachable management
path. It remains unverified on alpha.109; the Gateway retains the update notice
for independent catch-up. Its one-time publisher-chain preparation still needs
completion when it becomes reachable. This is not reported as an updated host.
The available Linux node remains on its latest platform release, alpha.104;
this Windows-only change does not replace its node or shared-desktop services.

M10.3 owner experience acceptance remains the roadmap return point. Existing
Windows Credential Manager accounts can be reused. A new account is entered once
on each controlling computer through the settings UI, never through Agent chat.

Migration is additive: existing desktop passwords, identities, Gateway settings
and records remain in the private user profile. Existing sessions keep their
original settings until the owner reconnects. Physical audio/clipboard/hardware
round trips remain separate from settings and installation verification.
