# Public evidence log

This log records external evidence that shaped a product decision. It is not a
feature backlog and must not include private deployment facts.

| User problem | Public evidence | Product implication |
| --- | --- | --- |
| Overlay VPN and TUN/proxy combinations can conflict | [Tailscale issue 9808](https://github.com/tailscale/tailscale/issues/9808) | PF Remote must not own global routing and must diagnose routes independently. |
| SSH configuration failures are difficult for non-network specialists | [VS Code Remote SSH troubleshooting](https://github.com/microsoft/vscode-remote-release/wiki/Remote-SSH-troubleshooting) | Users and agents receive stable targets and structured stages, not raw jump-host recipes. |
| Self-hosted relay setup can fail across configuration layers | [RustDesk server discussion 643](https://github.com/rustdesk/rustdesk-server/discussions/643) | Gateway installation needs layered checks for service, port, certificate, and public reachability. |
| Local authorization updates must survive interruption without mixing versions | [SQLite atomic commit](https://www.sqlite.org/atomiccommit.html) and [WAL documentation](https://www.sqlite.org/wal.html) | Store normalized immutable snapshots transactionally, switch one active pointer last, and retain prior committed snapshots for recovery. |
| The cross-platform Go daemon should not require a host C compiler or SQLite DLL | [`modernc.org/sqlite` package documentation](https://pkg.go.dev/modernc.org/sqlite) | Use the reviewed pure-Go BSD-3-Clause driver and pin its module graph in `go.sum`. |
| Authorization expiry must be understandable without relying only on color | [Windows accessibility overview](https://learn.microsoft.com/windows/apps/design/accessibility/accessibility-overview) and [AutomationProperties](https://learn.microsoft.com/windows/windows-app-sdk/api/winrt/microsoft.ui.xaml.automation.automationproperties) | Present localized status text and absolute expiry in an Automation-readable live region and keep target actions semantic keyboard-focusable buttons. |

Additions should state the affected user, task, existing workaround, failure
reason, and the product decision supported. Prefer primary project documentation
or issue trackers and distinguish an inference from a demonstrated fact.
