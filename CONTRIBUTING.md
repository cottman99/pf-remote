# Contributing

PF Remote is contract-first and security-sensitive. Read `AGENTS.md` before
opening a change.

1. Create an issue or ADR for a public contract, identity, authorization, route,
   or persistence change.
2. Use synthetic fixtures only.
3. Add failure-path tests with the implementation.
4. Run `scripts/check.ps1` or `scripts/check.sh`.
5. Explain user-visible behavior, compatibility, security, and rollback in the
   pull request.

Do not include screenshots, logs, or configuration captured from a real private
deployment unless they have been deliberately reconstructed with synthetic data.
