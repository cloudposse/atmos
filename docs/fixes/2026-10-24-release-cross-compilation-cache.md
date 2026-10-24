# Warm the release cross-compilation cache

The native CI cache warms `go build ./...`, acceptance binaries and race builds
for each runner's host platform. A release runs on Linux and cross-compiles the
entire GoReleaser matrix with `-trimpath`, so restoring the native archive does
not precompile the release's target/flag combinations.

`Release Cache Warmup` now runs on main nightly, on dependency/build-configuration
changes, or by manual dispatch. It uses the same pinned GoReleaser version and
`.goreleaser.yml` as production, running `build --snapshot --skip=before`. That
warms all configured targets, architecture variants, CGO/FIPS settings and build
flags without publishing, signing, or installing the signing/SBOM tools.

The release build restores the dedicated `release-go-cache-v1-*` archive instead
of the native/race archive. Keys include runner OS/architecture, Go version,
configuration and dependency hashes. Prefix fallbacks reuse compatible entries
after dependency or configuration changes; Go invalidates individual package
entries when their inputs change. Warmup and release use the same cache paths,
with a single `go.sum` hash even when source is checked out under `source/`.

Only a successful main warmup saves a run/attempt-specific cache. Feature and
release builds only restore it. Snapshot binaries are never cached as release
artifacts: each release still links its tag/SHA and runs the existing packaging,
signing, SBOM, provenance and verification stages. The job summary reports the
matched cache key or explicitly reports a cold build.

The first warmup is a cold cross-platform compile. After merging, dispatch
`release-cache-warmup.yml` on main (or wait for its push/scheduled run) and confirm
the cache save succeeds before measuring a subsequent release. Compare restore
time and GoReleaser build time separately; archive restoration, linking, signing,
packaging and tool installation still cost time. This change does not distribute
release compilation across runners or change the release artifact contract.
