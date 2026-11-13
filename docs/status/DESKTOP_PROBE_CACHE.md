# Bounded desktop status probes

Alpha.118 formalizes the earlier local daemon repair. Repeated device-list and
inspect requests share route hints for at most 60 seconds, with a 256-entry bound
and one concurrent probe per target and directory version. Returned slices are
copied. Failed/panicking providers do not leave permanently blocked entries.

Only presentation metadata is cached. Authorization and real connection route
selection remain fresh; no credentials, session content or private deployment
configuration enters the cache or release. No public schema or data migration
is required. Existing alpha.117 clients remain protocol-compatible.

Tests cover expiry, target/directory separation, mutation isolation, concurrent
read coalescing, bounded storage and panic recovery. Publication and live upgrade
evidence must be added after they complete; the historical same-version repair
is not proof that alpha.118 is installed.
