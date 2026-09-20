# Signed release v1

The zero-cost release signature authenticates PF Remote update metadata. It does
not claim Windows Authenticode, SmartScreen reputation or TUF conformance.

## Metadata compatibility extension

The envelope remains v1. Payload schema pfremote.release-metadata/v2 adds a required
signed compatibility object: positive data_epoch, protocol_min and protocol_max
(max >= min). Exact data-epoch equality and a protocol within the inclusive range
are necessary for unattended activation; they are not sufficient without session,
Gateway and installation health checks. v1 payloads remain verifiable/downloadable
but cannot authorize unattended activation. v1-only clients reject v2 rather than
silently ignoring its compatibility constraints. v1 forbids the extension field.
This slice uses local epoch/protocol 1 as the initial compatibility baseline; any
persisted-format change must explicitly revisit that baseline and rollback tests.

The envelope contains schema_version=pfremote.signed-release/v1, payload (standard
base64 JSON bytes) and signature (standard base64 Ed25519 signature). Sign exactly
UTF-8 "PF Remote release v1\n" followed by the decoded payload bytes. There is no
key from the server to trust: the verifier receives a publisher key provisioned
independently by the trusted bootstrap installer. Publisher keys are distinct from
Owner/Device keys and must remain outside this repository.

Payload fields: schema_version=pfremote.release-metadata/v1, product="PF Remote",
version (three numeric components with optional prerelease), channel (stable or
preview), platform (windows-x64 or linux-x64), sequence (positive monotonic integer),
issued_at, expires_at (RFC3339 instants), and artifacts. Lifetime is at most 90 days.
At most 16 artifacts are allowed. Each has name (safe ASCII basename), size (1 byte
to 2 GiB) and lowercase sha256. Names are unique case-insensitively; no reserved
Windows basename, traversal, path separator, URL, hidden filename or trailing dot.
Downloaded metadata contains no execution commands, endpoints or personal values.

The verifier binds expected channel/platform from local policy and checks expiry
and monotonic sequence. Equal sequence is allowed only for the identical signed
payload digest. Lower sequences and same-sequence substitutions are rejected.
The caller must persist the accepted sequence/digest before installation; missing
or damaged previously initialized trust state must fail closed. The trust store
provides explicit one-time bootstrap and transactional replay/clock persistence;
normal open never creates missing state. The caller supplies an independently
trusted key and an OS-protected per-user directory outside program versions.
The trusted Windows installer bootstraps once and accepts its own signed package
as the initial checkpoint. Normal discovery cannot recreate missing trust state.
Root-key rotation remains pending.

Artifact verification accepts a reader rather than a pathname; consumers must use
private staging and hold verified content stable until installation. Byte count
and hash must both match. No automatic fallback to unsigned metadata is allowed.
Existing alpha.93 clients do not understand this contract and need a trusted
bootstrap upgrade before managed updates. Public binary publication remains a
separate explicit action; existing unsigned internal packages are not promoted.

The planned GitHub channel tags are update-stable and update-preview. Each holds
windows-x64.release.json and/or linux-x64.release.json plus the exact artifact
basenames bound by that signed document. Discovery never trusts a peer-provided
URL or key. HTTPS downloads have bounded size, time and redirect count; redirects
cannot downgrade to HTTP. Staging returns an open verified file, rewound for the
consumer. Failed downloads delete their temporary file. A publisher replacing a
channel asset during download can cause a retry, never unsigned installation.
