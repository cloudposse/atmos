# Build tagged artifacts

Checks out the reserved tag's exact commit into `source`, verifies the clean
checkout and tag resolution, and runs pinned GoReleaser with `--skip=publish`.
Existing GoReleaser hooks install cosign v3.1.3 and syft v1.51.1; GoReleaser creates
the checksum signature and binary/package SBOMs. Go caching is retained.
The caller checks out trusted workflow tooling/config first.

The job has read-only repository contents access plus OIDC signing and attestation
permissions. It has no release environment or publishing App credentials. After
creating the source SBOM, it attests every staged build artifact with the exact tag
and source SHA. Outputs stay in `source/dist` and move by immutable Actions
artifact ID to a fresh verification runner. Build failure does not create or
publish a release.

Tests: the GoReleaser fixture rehearsal and workflow contracts in
`magefiles/release`, including preserved signing/tool hooks and binary identity
regressions. Live OIDC signing requires a GitHub Actions rehearsal.
