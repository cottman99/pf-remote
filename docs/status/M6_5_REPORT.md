# M6.5 one-click Linux Desktop report

## Product result

The internal Windows candidate now turns the three currently online Linux VNC
Desktops into genuine one-click actions. On first use, Center offers a native
password dialog and explains that the existing password will be saved for the
current Windows user. After setup, the same buttons open their Desktop without
a viewer password prompt. A missing credential is shown as "set up and open"
rather than as a technical connection failure.

The installed internal candidate is `0.1.0-alpha.16`. Its previous candidate
remains available through installer rollback, and the independent old product
was not changed or stopped. The intentionally offline Windows computer was not
contacted.

## Usability and evidence

- All three online VNC Desktop targets reported that one-click setup was ready.
- A fresh open supplied no password, started a new isolated viewer, and showed
  no authentication or password window.
- Center built and launched on an isolated Windows desktop with a responsive
  top-level window. The final computer-list screenshot is retained outside the
  public repository as `PFRemote-Center-OneClick-Ready-20260830.png`.
- The full project check passed, including private-data hygiene, all Go tests,
  28 Windows UI tests, and the unpackaged WinUI build.
- The internal release manifest, archive, dependency inventory, and hashes
  passed independent verification before installation.

## User-data boundary

Saved Desktop credentials are bound to the immutable canonical Desktop target,
protected for the current Windows user, and stored outside repository and
target state. They are provided to the existing viewer only through its process
environment. They do not appear in command arguments, context exports,
structured results, logs, screenshots, fixtures, release artifacts, or this
report.

## Migration impact

No Linux password, service, route, or legacy configuration changed. Protected
saved credentials remain local across candidate updates. The next product gap
is no longer connectivity; it is the Center's unfinished information
architecture. M6.6 therefore adds clear Computers, Sessions, and Settings &
Recovery destinations before asking the project manager to evaluate the UI.
