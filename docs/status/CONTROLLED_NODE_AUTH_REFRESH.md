# Controlled-node authorization renewal — alpha.113

The packaged daemon now renews self-only controlled-node authorization at startup
and hourly using its existing signed identity. Failed initial synchronization
retries correctly without an Owner-only pull. No reenrollment or new grants occur.

Scope: only snapshots containing the matching local Device and no Grants can
renew through version confirmation. Controller authority continues to use its
separate directory path. Network rejection, revoked/mismatched identities,
backwards versions and clock failures retain the previous expiry.

Regression tests cover successful expired-cache renewal, signed identity proof,
offline/revoked/mismatched/schema/version failures, clock rollback, foreign
authority exclusion, periodic/retry scheduling and persistence failures.
Linux packaging already includes pfremoted; no additional payload or service is
needed. The updater replaces only its three managed binaries and restarts only
the node daemon, preserving identity/configuration and independent desktops.

Validation: scripts/check.ps1 -SkipWindowsApp passed. The signed alpha.113 Linux package (sequence 115) upgraded an existing alpha.104 installation through the ordinary updater. With the temporary refresh timer stopped, the packaged daemon renewed its cached authorization. Installed daemon hash matches the signed artifact. Identity bytes and all existing shared desktop/audio service PIDs and startup times match the baseline. The temporary timer is now disabled. Source commit aeea4549bee1ab199d69108454969d6a91d7b2af and the signed Linux alpha.113 release are published. The preview feed uses sequence 115; Windows remains on alpha.112.
