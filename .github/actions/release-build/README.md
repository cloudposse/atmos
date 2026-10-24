# Build tagged artifacts

Checks out the reserved tag's exact commit into `source`, verifies the clean
checkout and tag resolution, and runs pinned GoReleaser with `--skip=publish`.
Pinned installer actions provide cosign v3.1.3 and syft v1.51.1 before GoReleaser;
its hooks reuse matching versions and retain source-install fallbacks for other
callers. GoReleaser creates the checksum signature and binary/package SBOMs.
The release-only Go cache is restored from the full-matrix warmup on `main`.
This action never saves caches, including when building feature releases.
Its summary reports the matched cache key, build duration, and cache disk usage.
The caller checks out trusted workflow tooling/config first.

The job has read-only repository contents access plus OIDC signing and attestation
permissions. It has no release environment or publishing App credentials. After
creating the source SBOM, it generates GitHub's standard SLSA provenance for every
staged build artifact and a separate `https://atmos.tools/release/v1` custom
attestation binding the exact selected tag and source SHA. The standard action
must not receive a custom SLSA predicate: GitHub rejects our custom build type.
Outputs stay in `source/dist` and move by immutable Actions
artifact ID to a fresh verification runner. Build failure does not create or
publish a release.

Tests: the GoReleaser fixture rehearsal and workflow contracts in
`magefiles/release`, including preserved signing/tool hooks and binary identity
regressions. Live OIDC signing requires a GitHub Actions rehearsal.
