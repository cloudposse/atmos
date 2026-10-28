# Finalize release publication

Repeat live policy checks and call the tested release-publish action with the
sealed artifact directory at `$RUNNER_TEMP/release-upload`. The caller downloads
that directory by immutable artifact ID from the successful verification job.

First recheck staged bytes against freshly resolved upstream Aqua metadata using
`release:verifyInstaller`, recording its revision. Registry fetch errors or a
signer-policy mismatch block publication, even if the earlier verify job passed.
Only then mint the fresh publication token and repeat authoritative release checks
immediately before upload. Fix incompatible version-scoped Aqua metadata and rerun;
do not publish manually to bypass the installer gate. Mise's bundled snapshot is a
separate propagation check covered by the
[release guidance](../../../.claude/skills/releases/SKILL.md).

Retry **failed jobs**, so signatures and manifests are reused byte-for-byte. An
identical partial upload is resumed; conflicting assets are never overwritten.
The next job completes milestone bookkeeping independently of publication.

Credentials are explicit App inputs from the protected environment. Tests:
`go test -tags=mage ./magefiles/release/...` and the release-publish Node tests.

The required `guarded-releases-enabled` input comes from the caller's
`${{ vars.ATMOS_GUARDED_RELEASES_ENABLED }}`. Composite actions cannot read
workflow `vars` directly; the authoritative checks still require `true`.
