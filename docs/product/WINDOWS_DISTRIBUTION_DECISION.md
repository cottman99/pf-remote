# Windows distribution and publisher trust

Status: on 2026-09-20 the owner chose zero-cost project-signed updates. No paid
signing service is purchased or activated. Public binary distribution remains
pending trusted bootstrap and update integration; an empty public repository has
been created, without uploading private development history.

Project signatures authenticate updates against an independently provisioned
publisher key; they do not confer Windows Authenticode/SmartScreen reputation.
See contracts/signed-release-v1.md and docs/status/TRUSTED_UPDATE_REPORT.md.
The paid-provider discussion below is historical optional context, not a dependency
or current recommendation. Its prices and eligibility must be rechecked if used.

## Recommendation

For internal testing, keep PF Remote's directly installed unpackaged WinUI
application. Do not publish its installer or payload with the open-source
release. If public Windows binaries are approved later, use
Microsoft Artifact Signing Basic when the final publisher is eligible for its
Public Trust identity validation; otherwise use a CA-issued publicly trusted
code-signing certificate through the same release builder.

This preserves the side-by-side installer, immutable version rollback, direct
sibling-daemon launch, protected local IPC, and recovery behavior already
verified in M5.3. Artifact Signing supplies a public-trust certificate without
placing a long-lived private signing key on a development computer. Microsoft
currently lists Basic at USD 9.99 per month for 5,000 signatures; that is ample
for the expected Alpha release volume.

Microsoft currently limits Public Trust certificates to organizations in the
United States, Canada, the European Union, the United Kingdom, Australia, New
Zealand, Japan, South Korea, Singapore, Switzerland, Norway, and Israel;
individual developers must be in the United States or Canada. Therefore a
mainland-China publisher cannot be assumed eligible. Publisher jurisdiction is
the single nontechnical input needed before choosing the provider.

## Alternatives considered

- Microsoft Store MSIX can have the Store sign the package, but it changes the
  packaging and update model and would force new lifecycle validation. It is a
  later distribution channel, not a shortcut around M5.3.
- A Microsoft Store MSI/EXE submission still requires the publisher to sign the
  installer and executable files, so it does not remove the current gate.
- A traditional CA-issued certificate remains supported by the builder, but it
  creates private-key custody and renewal work without a current product
  advantage.
- A self-signed certificate is suitable only for isolated development proof;
  it is not acceptable for ordinary users or a public Alpha.

## Ready integration boundary

`scripts/build-windows-release.ps1` accepts either one local CA certificate or an
external Artifact Signing dlib plus metadata file. Both routes require the
exact approved publisher subject, an RFC 3161 timestamp, signature validation,
and an independently verified installed Center. Artifact Signing account,
identity validation, role assignment, and metadata remain outside the public
repository and are not created until the project manager confirms the public
publisher identity and approves the corresponding service or certificate cost.
