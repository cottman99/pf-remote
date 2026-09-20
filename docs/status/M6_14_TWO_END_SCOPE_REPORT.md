# M6.14 two-end replacement scope report

## Product outcome

The project manager temporarily removed the sleeping Windows controlled
computer from the acceptance scope. The remaining Windows controller and
headless Linux workstation now form a complete internal replacement pair.
The deferred computer was not contacted or changed during this closure.

## User-visible capability status

- The installed Center is the normal PF Remote entry and opens as a responsive
  native Windows window without requiring a terminal.
- The Linux workstation remains visible by its user-facing name and supports
  Shell, VNC virtual Desktop, and RDP virtual Desktop actions.
- Smart Desktop connection selects the available Tailscale path. The adjacent
  per-connection menu can select the independent Gateway path, while RDP can
  be selected over Tailscale.
- The Center hands the exact selected Linux target to the external Agent. MCP
  inspects that immutable target before acting, a real harmless command
  completes, and the completed Agent action becomes visible in Sessions.
- Settings provides one explicit, reversible action to make PF Remote available
  in Codex. After enabling, a fresh Codex can name or receive a computer and use
  the same authorized actions without command-line setup.
- The current and previous internal candidates both include the verified VNC
  viewer. One-action candidate rollback and return complete successfully.
- The untouched old product remains installed and running as an additional
  fallback while the internal candidate becomes the default entry.

## Continuous product iteration

The current internal candidate is `0.1.0-alpha.72`; `0.1.0-alpha.71` is the
complete immediate rollback. The Settings screen now leads with Codex control,
shows not-enabled, ready, update-required, conflict, and unavailable states in
user language, and offers one explicit enable or remove action. PF Remote does
not silently replace a same-named user entry. The installed Skill and MCP entry
use a stable path that resolves the current candidate, so a fresh Codex completed
the exact Linux inspect-plus-exec journey on `alpha.55`, while rolled back to
`alpha.54`, and after returning to `alpha.55`. The Computers screen now keeps Smart connect and
Agent handoff visible without scrolling at both wide and narrow sizes, no
longer overlays refresh status on a computer card, and keeps additional
desktops collapsed until requested. Smart connect also names the route it will
try first while preserving the adjacent manual route menu. Sessions now turn
successful history into useful shortcuts: a person can reconnect a Desktop or
hand the same computer to the Agent again. When a Desktop connection fails,
the same first screen explains whether to refresh or choose another available
route and keeps the affected computer visible below the message. Settings,
old-version fallback, and device recovery now reflow to a single column in
narrow windows, and the Settings page scrolls to the final recovery actions
instead of clipping them below the window.
Progress, success, and failure feedback for synchronization, migration,
rollback, and recovery also remains visible and dismissible on the Settings
page instead of disappearing into the hidden Computers status line.
When the computer list is genuinely empty, the first screen now explains that
authorized computers arrive through synchronization without addresses, ports,
or Gateway fields and provides one action to check synchronization. A search
or filter with no matches instead offers one action to show all computers.
If the local background service exits while Center remains open, the installed
candidate now restores one installed sibling automatically and retries only an
operation that failed with the explicit local-daemon-unavailable result. A live
`alpha.45` test recovered passively in about 13.2 seconds at the existing refresh
cadence, kept Center responsive, started no duplicate daemon, and then completed
Linux Shell plus exact-target MCP actions. A user action that encounters the same
failure uses this recovery path immediately rather than waiting for refresh.
The same narrow-screen audit also found and corrected a clipped Smart connect
label: the compact card now uses the concise action name while accessibility and
the route menu retain the exact computer and Desktop context. A follow-up audit
showed that long computer names and status summaries still disappeared behind
ellipses. The compact card now gives both up to two lines, preserving the full
computer identity without pushing Smart connect or Agent handoff off the first
screen.
User-initiated refresh and Desktop opening now place a native progress ring
beside the live status instead of leaving an apparently idle screen. Refresh is
disabled until its current operation completes. Desktop opens use a per-target
single-flight guard: a repeated click on the same Desktop reports that the
connection is already starting, while a different computer remains independent.

Current-run wide, narrow, route-priority, and repeat-session screenshots were
visually inspected outside the repository so private names remain out of the
public source tree. The installed window reached a responsive state in under
one second in the latest isolated launch, and the real Linux Shell action
completed in under one second. These timings are current-machine observations,
not release guarantees.

The responsive Settings audit inspected both the top and scrolled recovery
states at the same narrow viewport. A separate current-run screenshot verified
the new visible Settings result message, and another inspected the zero-computer
guidance. The daemon-recovery audit inspected the restored Computers screen and
the corrected compact action at the same narrow viewport. The installed
`alpha.49` candidate then completed package verification, upgrade, rollback to
`alpha.48`, return to `alpha.49`, a real Linux Shell action, and an exact-target
MCP action. Background UI automation also observed the real refresh status,
disabled action during work, restored ready status, and re-enabled action after
completion without input injection. Its Center remained responsive on an
isolated desktop.

Ten consecutive installed UI refresh cycles completed with the action disabled
only while active, the ready status restored each time, and no lost catalog or
stuck progress state. A five-cycle timing follow-up measured about 0.73 seconds
from observed active status to ready status on the current machine. Direct
read-only measurements placed catalog work at about 0.69 seconds and local
connection diagnosis at about 0.02 seconds; the larger end-to-end automation
duration came from cross-desktop UI Automation invocation rather than PF Remote.
The same refreshing state was visually inspected in light and dark themes with
readable hierarchy, status, cards, and actions.

Five installed warm launches averaged about 0.49 seconds to a responsive native
window and 0.73 seconds from process start to the usable computer catalog. Three
starts after stopping the candidate Center, daemon, and Gateway averaged about
0.50 and 0.71 seconds respectively and each produced exactly one candidate
daemon. A real Smart Desktop action against the Linux computer then displayed
the bundled VNC viewer about 1.67 seconds after the UI action was invoked; Center
reported that the desktop opened and remained responsive. The first isolated
measurement found the viewer on a previous isolated desktop because the daemon
had been deliberately preserved there; restarting the candidate processes on
one shared isolated desktop proved the real user path. No active-desktop input,
L34 action, old-product change, or networking change was used.

A genuine 480-pixel audit found that the action row still carried desktop-width
minimums, which widened the card and hid the computer identity and route text.
The compact layout now removes the decorative device icon, allows the complete
name, uses a shorter route summary, and stacks Smart connect above Agent handoff.
The split-button route dropdown and both actions remain inside the card. A final
installed `alpha.48` run also opened the real Linux VNC window about 1.80 seconds
after invocation and kept Center responsive.

The follow-up 480-pixel audit covered Sessions plus the top and bottom of
Settings. Recent session identity, action, result, time, and repeat action now
form one readable column. Version, synchronization, migration, and recovery
cards collapse their unused desktop-width columns, wrap every heading and
description, and keep all actions inside the card. The installed `alpha.49`
package then repeated the real Linux VNC path: the bundled viewer became visible
about 1.81 seconds after invocation, Center stayed responsive, and no input
injection, L34 action, old-product mutation, or network change was used.

A current-flow audit at a common high-scaling laptop viewport found that the
phone action stack was still selected even though Smart connect and Agent
handoff fit side by side. The threshold now preserves both core actions on the
first screen while retaining the proven vertical stack at a genuinely narrow
480-pixel viewport. The same audit found that failure guidance pointed toward a
route menu pushed below the visible area. Failures now include a direct
keyboard-reachable retry that preserves the exact named Desktop and route; an
isolated real retry opened the Linux VNC window, and the installed `alpha.50`
package repeated upgrade, rollback to `alpha.49`, return, VNC, Shell, and
exact-target MCP verification.

The installed background set now self-heals as one product instead of treating
only the daemon as recoverable. The stable setup helper starts a configured
Gateway before the daemon, skips components already running, and Center retries
a pending local connection service after a bounded cooldown. Calling startup
twice kept exactly one daemon and one Gateway. In the final installed
`alpha.51` fault trial, a deliberately stopped candidate Gateway was detected
when synchronization became pending and returned automatically in about 31.7
seconds. Synchronization plus the same Linux Shell returned about 12.1 seconds
later; Center stayed responsive, and exact-target Shell and MCP actions both
completed. `alpha.53` upgrade, rollback to `alpha.51`, return, bundled VNC, and
the untouched old-product boundary also passed.

Three installed warm `alpha.53` launches averaged about 0.53 seconds to a
responsive native window and 2.18 seconds to the verified two-computer catalog.
A cold launch after stopping both candidate background components reached those
states in about 0.57 and 2.19 seconds and restored exactly one daemon plus one
Gateway. Startup now shows a truthful loading state until the real catalog count
arrives. The first packaged version of that change exposed an invalid localized
resource reference before its window appeared; `alpha.53` fixes the release-only
failure, and an automated bilingual resource-contract check prevents recurrence.
The installed `alpha.54` narrow-screen refinement removes desktop-only gaps from
computer cards, lets Session cards use the full content width, and makes recovery
copy shorter and denser. Its startup, rollback to `alpha.53`, return, Linux VNC,
Shell, and exact-target MCP paths passed while the old product remained untouched.
An installed daemon-only termination recovered passively in about 3.8 seconds,
kept the existing Gateway single-instance, and left Center responsive. The same
immutable Linux target then completed both Shell and MCP actions, covering both
background components independently.

Installed `alpha.55` completed five responsive launches with an average of about
0.49 seconds to the window and 2.14 seconds to the verified two-computer catalog.
Its real isolated Linux VNC action displayed the viewer about 1.80 seconds after
invocation. One-click Codex enable, remove, and re-enable all passed through the
native Settings UI without input injection; the final installed state is enabled.
An initial fresh-Codex trial using only the visible computer name stopped safely
because the Agent passed that name directly to canonical inspection. `alpha.56`
corrects the user journey in the installed Skill: it lists named computers,
requires one exact Device-name match, selects that Device's available Shell,
then inspects the canonical target before acting. A second fresh Codex resolved
the Linux workstation from its visible name alone and completed the harmless
command on the same inspected target without contacting the other computer.

`alpha.58` closes the misleading handoff state. When Codex control is not yet
ready, the Computers screen keeps the exported context but clearly explains
what is missing and provides a keyboard-reachable action to the Codex Settings
card. The warning and navigation passed isolated native UI automation, and the
final ready state was restored through the same native one-click action. A fresh
Codex again resolved only the visible Linux computer name and returned the
expected harmless Shell result. Rollback to `alpha.57` preserved that stable
Agent entry, and the final selected version returned to `alpha.58`. Its installed
window became responsive in about 0.56 seconds and the exact two-computer list
was usable in about 2.15 seconds.

`alpha.59` removes the last ambiguity from the ready state. The confirmation
now says that the context was copied and asks the person to paste it into Codex,
while also reminding them that an enabled Codex can resolve the visible computer
name directly. This avoids implying that PF Remote injected content into an
already-open conversation. The exact localized message and keyboard-reachable
handoff action passed isolated native UI automation, and `alpha.58` remains the
complete immediate rollback.

`alpha.60` fixes the responsive result presentation exposed by a new user-flow
audit. Long handoff guidance no longer competes with search and filter controls
inside the toolbar. A short copied result stays in the toolbar, a green InfoBar
explains how to continue in Codex, and the yellow not-ready InfoBar still offers
the direct Settings recovery action. Accepted captures cover wide and narrow
light layouts plus the dark ready state. The real accessible handoff click,
fresh visible-name Codex Shell action, release verification, rollback to
`alpha.59`, and return to `alpha.60` pass. Three warm installed starts average
about 0.46 seconds to the responsive window and 2.16 seconds to the exact
two-computer catalog.

A 25-cycle installed stability run completed every Linux catalog plus Shell
action without failure. Parallel independent route-health checks reduced the
same machine's average catalog refresh from about 2.65 seconds to about 0.76
seconds while preserving the visible LAN, Tailscale, and Gateway order and
availability. The combined catalog-and-Shell cycle's measured p95 fell from
about 3.07 seconds to about 1.17 seconds. Connection policy and fallback order
did not change.

`alpha.61` makes Sessions a durable person-Agent alignment surface instead of a
daemon-lifetime debug view. A real visible-name Linux Agent Shell action appeared
as completed, remained after the installed daemon self-recovered, survived
rollback to `alpha.60`, and returned unchanged with `alpha.61`. The stored record
is bounded and content-free; command text and remote output are excluded, while
recovery restore clears local history. The accepted post-recovery screenshot is
`D:\PFRemotePrivateTest\experience-audit\20260831-alpha61-sessions\01-after-daemon-recovery.png`.

`alpha.62` removes the awkward final-character wrap exposed by that screenshot
without changing the feature or privacy boundary. The accepted copy capture is
`D:\PFRemotePrivateTest\experience-audit\20260831-alpha62-sessions-copy\01-sessions-copy.png`.
The verified package installed successfully, rolled back to `alpha.61`, returned
to `alpha.62`, and retained the same real Agent result.

`alpha.63` closes the version-switch readiness gap found during that rollback.
Installation and maintenance now finish only after the protected local daemon
answers successfully, rather than when its process merely exists. The first
catalog read passed immediately after install, rollback to `alpha.62`, and
return to `alpha.63`; the rollback legs completed in about 0.92 and 0.72 seconds,
and the persisted Agent result remained visible throughout.

`alpha.64` resolves the largest ambiguity in the fresh Computers capture: when
one computer offers several Desktops, the primary button now states the exact
Desktop that Smart connect will open at ordinary window widths. UI automation
also confirms that the accessible name contains both computer and Desktop. The
accepted capture is
`D:\PFRemotePrivateTest\experience-audit\20260831-alpha64-target-clarity\01-computers.png`.

`alpha.65` turns Desktop failure recovery into an in-place action. The shorter
notice exposes a Recovery menu whose keyboard-accessible choices are retry,
Tailscale, and Gateway for the current Linux Desktop; only routes reported
available are offered. The accepted capture is
`D:\PFRemotePrivateTest\experience-audit\20260831-alpha65-error-recovery\02-recovery-menu-entry.png`.
The installed package completed a real named-Linux Agent action, rollback to
`alpha.64`, and return to `alpha.65` without readiness gaps or lost history.

`alpha.67` makes background connection recovery fast enough to feel automatic.
Center now checks connection health every five seconds without refreshing the
full computer catalog; the daemon separately probes the trusted Gateway and
retries synchronization on the first healthy cycle after a failure. In the
installed isolated fault trial, stopping the candidate Gateway produced the
expected retry state, started one replacement, and returned the connection
service to ready in about 6.9 seconds. A real exact Linux Shell action then
completed, rollback to `alpha.66` was immediately usable in about 2.8 seconds,
and return to `alpha.67` was immediately usable in about 1.6 seconds with recent
Sessions retained. The old product was not changed or stopped.

`alpha.68` fixes the main problem found in a new three-screen product audit.
Computers remains direct and Settings keeps Codex control first, but Sessions
previously rendered every repeated Agent action as a large identical card. The
new presentation groups the same computer, capability, and action around the
latest result and adds a total count without deleting the underlying bounded
history. The accepted current-run screenshot is
`D:\PFRemotePrivateTest\experience-audit\20260831-alpha68-grouped-sessions\01-sessions.png`.
The installed build then completed the eighth real Linux Agent action, rolled
back to `alpha.67` in about 1.6 seconds, and returned to `alpha.68` in about 1.6
seconds with every record retained and the responsive isolated Center left running.
The same installed Center recovered a deliberately stopped `alpha.68` daemon in
about 5.1 seconds, never exceeded one daemon or one Gateway, and remained responsive.

`alpha.69` removes the remaining product-language mismatch on that screen. The
technical Sessions label is now Recent activity, the card uses the same friendly
computer title as Computers, recent results use relative time, and the explanation
states that addresses, commands, and operation content are not displayed. The
accepted current-run screenshot is
`D:\PFRemotePrivateTest\experience-audit\20260831-alpha69-recent-activity\02-relative-time.png`.
The installed build completed another real Linux Agent action, rolled back to
`alpha.68` in about 1.9 seconds, and returned to `alpha.69` in about 1.9 seconds
with eleven bounded records retained and the responsive isolated Center left running.

`alpha.71` closes two follow-up usability gaps. The Recent activity explanation
is short enough that its repeat action remains fully visible in a narrow window;
the accepted current-source capture is
`D:\PFRemotePrivateTest\experience-audit\20260831-alpha70-narrow-activity\01-narrow.png`.
A real upgrade then exposed that a long-lived Codex process can legitimately keep
an older verified MCP executable open. Installation now activates the new version
without interrupting that Agent and retries obsolete-version cleanup later. The
installed candidate completed the exact Linux Agent action, rolled back to
`alpha.69` in about 1.65 seconds, returned to `alpha.71` in about 3.76 seconds,
retained all 12 recent records, and left the existing Codex MCP host alive. Its
isolated installed Center showed a responsive top-level window in about 1.34
seconds. A three-minute installed soak completed 66 catalog and health cycles
with no failures, an average catalog time of about 0.70 seconds, and no more
than one daemon or one Gateway. The old product was not changed or stopped.

`alpha.72` reduces first-screen waiting without weakening recovery. The Center
loads the already authorized computer catalog while the independent installed
background-set check confirms or repairs synchronization. Three warm installed
launches showed the two computers in about 1.56 to 1.60 seconds versus about
2.17 seconds before the change. With the candidate daemon and Gateway both
stopped, the installed screen recovered in about 1.55 seconds, restored exactly
one of each process, and then completed the exact Linux Agent action. Rollback
to `alpha.71` took about 1.48 seconds and return about 3.87 seconds; all 14 recent
records and three observed live Agent hosts survived. The responsive installed
`alpha.72` Center remains running on an isolated desktop, and the old product
was not changed or stopped.

## Product problems found and corrected

The first final-package attempt omitted the external VNC viewer even though
earlier engineering tests supplied one from a scratch runtime. That would have
made a user's one-click VNC action fail after installation. The internal
package now carries the previously verified signed TigerVNC viewer, its
license, hash, and software-bill-of-materials entry. Both sides of the rollback
pair contain the same complete Desktop capability.

The isolated UI journey also exposed a language- and width-dependent
automation assumption around the Sessions navigation item. The navigation now
has a stable accessibility identifier, and the verification opens the compact
navigation pane before selecting it. No foreground input is used.

The first `alpha.70` installation attempt also exposed an upgrade assumption:
obsolete candidates are not always immediately removable because a legitimate
Agent host can still be finishing work through one. Cleanup is therefore no
longer allowed to block verified activation. The pinned version remains bounded
and is removed by a later maintenance cycle after its consumer exits.

## Evidence boundary

Real Smart/Tailscale VNC, manual Gateway VNC, Tailscale RDP, Shell, exact-target
MCP, Sessions visibility, installed startup, responsive WinUI launch, upgrade,
rollback, return, release verification, and repository checks are required for
this two-end claim. Private names and connection evidence remain outside the
repository.

The full three-computer roadmap item remains open. The deferred Windows
computer still requires the longer post-wake recovery candidate and a final
cross-device journey when the project manager restores it to scope.
