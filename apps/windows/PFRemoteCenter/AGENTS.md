# Windows Center agent notes

The repository-root `AGENTS.md` remains authoritative. These rules narrow it
for the Windows application.

- This is an unpackaged, self-contained C#/WinUI 3 application. A signed MSI is
  a later release artifact; do not silently switch packaging models.
- Center is a view and interaction layer. Target resolution, authorization,
  routing, diagnostics, and context generation belong to the Go action core.
- The bootstrap `PfRemoteCliClient` is a replaceable development adapter. The
  production client will use the protected local daemon API.
- Use semantic WinUI controls, keyboard navigation, accessible names, theme
  resources, and localized `.resw` strings. Do not hard-code colors or UI text.
- Keep UI startup useful when the daemon/CLI is absent: show a bounded,
  actionable status instead of crashing or blocking the UI thread.
- Build with the current architecture and launch the resulting executable for
  every UI change. Do not claim visual validation from a successful build alone.
- The generated files under `.github/instructions/` are optional WinUI-specific
  references. Read the relevant one before changing accessibility,
  globalization, security, performance, or an unfamiliar Windows API.

