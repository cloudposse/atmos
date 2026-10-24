# Verify release artifacts

Runs on a fresh trusted runner after the build job. It takes the reserved tag,
source SHA, mode, and preparation artifact from this run. App credentials and
optional summary credentials are explicit inputs; the caller uses the release
(or feature-releases) environment.

The action repeats release eligibility, inspects all binaries and Linux packages,
verifies GoReleaser's existing checksum signature, seals a manifest, creates notes
for the tagged source, and verifies build provenance and SBOM attestations.
It does not re-sign artifacts or create replacement attestations. Every build
artifact, including the source SBOM, must have authenticated build provenance.
The new verification manifest is not itself a build artifact.

The caller stores the sealed artifact set and passes its immutable artifact ID to
the separate publication job. Upload retries therefore reuse the same signature
and manifest bytes. This action does not publish. The build runner has no
publishing credentials.

Tests: `go test -tags=mage ./magefiles/release/...` and
`node --test .github/actions/release-publish/publish.test.cjs`.
Live OIDC signing and repository immutability require a GitHub Actions rehearsal.

The required `guarded-releases-enabled` input comes from the caller's
`${{ vars.ATMOS_GUARDED_RELEASES_ENABLED }}`. Composite actions cannot read
workflow `vars` directly; the authoritative checks still require `true`.
