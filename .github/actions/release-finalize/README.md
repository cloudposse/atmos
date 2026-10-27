# Finalize release publication

Repeat live policy checks and call the tested release-publish action with the
sealed artifact directory at `$RUNNER_TEMP/release-upload`. The caller downloads
that directory by immutable artifact ID from the successful verification job.

Retry **failed jobs**, so signatures and manifests are reused byte-for-byte. An
identical partial upload is resumed; conflicting assets are never overwritten.
The next job completes milestone bookkeeping independently of publication.

Credentials are explicit App inputs from the protected environment. Tests:
`go test -tags=mage ./magefiles/release/...` and the release-publish Node tests.

The required `guarded-releases-enabled` input comes from the caller's
`${{ vars.ATMOS_GUARDED_RELEASES_ENABLED }}`. Composite actions cannot read
workflow `vars` directly; the authoritative checks still require `true`.
