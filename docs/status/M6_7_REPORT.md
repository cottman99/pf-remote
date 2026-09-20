# M6.7 installed lifecycle report

## Product result

PF Remote's internal Windows installation can now maintain itself without a
terminal. Settings & Recovery shows the actual installed current version and
the retained previous version. A visible one-action rollback asks for a plain
confirmation, switches versions, restarts the selected background components,
and reopens Center.

Normal Windows sign-in starts a stable PF Remote maintenance entry point that
resolves the selected version dynamically. Upgrades and rollbacks therefore do
not leave Windows pointing at an obsolete version directory.

## Actual usability and evidence

- Installed internal candidate `0.1.0-alpha.26` is selected; `alpha.25` is
  retained for rollback. The later update adds the mature per-connection route
  surface documented by M6.9.
- A real upgrade/rollback sequence switched `alpha.19` to `alpha.18` and back,
  then switched `alpha.21` to `alpha.20` through the native Center button.
- The isolated UI automation invoked “回退到上一版” and “回退并重新打开” through
  accessibility actions, with no mouse, keyboard injection, terminal, or
  foreground desktop use.
- After each switch, both background components ran from the selected version
  directory. The independent old PF Remote executable and Windows startup
  entry remained present and unchanged.
- Candidate package verification passed before each installation. The full
  project check is recorded again after final documentation and installation.
- When repeated internal candidates filled the system drive, installation
  stopped without changing the selected working version. The lifecycle now
  retains only the current and rollback versions; obsolete internal candidates
  were removed while their reproducible packages remained on the F drive.
- A first lifecycle pass revealed that a private TLS Gateway cannot be
  restarted as an unconfigured default process. Gateway startup now requires a
  protected local profile and preserves its listener and TLS files across
  version switches; the Linux Agent action returned to available after restart.

Visual evidence is retained outside the public repository as
`PFRemote-Settings-Rollback-alpha23-20260830.png`; it shows the user-facing
current/previous version state and rollback button without private connection
details.

## Failure and recovery behavior

The installer activates only a hash-verified immutable version. State changes
are atomic, and a rollback failure leaves a verified version selected. Center
shows a nontechnical retry message. Process switching is restricted by both
executable name and resolved path inside PF Remote's owned version directory,
so the legacy product and unrelated processes are outside the operation.

## Migration impact

No legacy setting, service, connection, startup entry, or system network
setting changed. The offline laptop was not contacted. The next user-visible
gap is the complete UI-to-Agent journey on the two currently online computers.
