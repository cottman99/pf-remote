# Release gates

No public installable Alpha is produced merely because the repository builds.
A release candidate must provide:

- a clean golden-journey report;
- end-to-end encryption and target-authentication evidence;
- revocation, recovery, cached-expiry, upgrade, and rollback evidence;
- dependency license inventory and SBOM;
- secret-scan and structured-log redaction results;
- signed installers and published SHA-256 hashes; and
- a private side-by-side observation report with a proven legacy rollback.

Version tags and public publishing require explicit owner approval. CI builds are
development artifacts until every gate above is satisfied.

