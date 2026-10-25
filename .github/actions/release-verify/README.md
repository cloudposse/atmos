# Verify release artifacts

Runs on a fresh trusted runner after the build job. It takes the reserved tag,
source SHA, mode, and preparation artifact from this run. App credentials and
optional summary credentials are explicit inputs; the caller uses the release
(or feature-releases) environment.

The action repeats release eligibility, inspects all binaries and Linux packages,
verifies GoReleaser's existing checksum signature and Aqua installer compatibility,
seals a manifest, creates notes
for the tagged source, and verifies build provenance and SBOM attestations.
It does not re-sign artifacts or create replacement attestations. Every build
artifact, including the source SBOM, must have both standard SLSA provenance and
an authenticated Atmos release-source attestation. Both require the expected
workflow and tooling SHA; the custom predicate additionally pins the selected
repository, tag, build source SHA, and selected signing run. The required
`signing-workflow-ref`, `signing-workflow-sha`, and `signing-run-id` inputs identify
the parent build or the dispatched tagged child. For tag-signed releases, the
workflow SHA must equal the source SHA; the legacy main workflow may differ.
The new verification manifest is not itself a build artifact.

Before sealing, `release:verifyInstaller` checks staged bytes with the native Aqua
reader and verifier against a freshly resolved upstream registry revision. SHA256
and Cosign must pass using the selected version's full policy. The revision is
logged and appended to the successful step summary. A valid internal signature
with an incompatible public installer policy still fails this action. Merge the
version-scoped Aqua correction and rerun failed jobs; do not disable verification.
See [release guidance](../../../.claude/skills/releases/SKILL.md) for historical
policy, Mise propagation, and cache/override cleanup.

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
