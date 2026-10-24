# Release builds need their own warmed Go cache

The general CI cache warms native builds without `-trimpath`. Release builds use
GoReleaser's ten-target cross-compilation matrix, `-trimpath`, and FIPS settings,
so a general cache hit does not mean those compiled packages are available.
Release hooks also compiled cosign and syft from source on every runner.

`setup-go-cache` now accepts `cache: release`. Its separate `release-go-cache-v1`
prefix includes the host OS/architecture, resolved Go version, trusted root
GoReleaser configuration hash, and source `go.sum` hash. Restore prefers matching
dependencies, then falls back within the same Go/configuration boundary. Both
the nested release source checkout and root warmup hash only that source's
`go.sum`. Existing CI cache modes and paths are unchanged.

`release-cache-warmup.yml` is the sole writer. It runs on `main` daily at 20:35
UTC, on relevant dependency/configuration merges, or by manual dispatch. It frees
runner disk space before restoring, then runs pinned GoReleaser's build-only
snapshot command using production configuration and skipping install hooks.
Only successful complete builds save, under a run/attempt-unique key. Cached
paths contain Go build/module data, not release artifacts or credentials.
Release consumers restore only, and a miss still performs the normal build.

The release action installs the existing pinned cosign and syft binaries before
GoReleaser. Hooks reuse the expected versions; missing/mismatched tools retain
the checksum-verified Go-module installation fallback and fail on install errors.
Signing, SBOM generation, provenance, and publication verification remain enabled.

## Validation and rollout

Workflow contract tests cover cache isolation, producer/consumer matching,
main-only successful writes, and tool ordering. Hermetic shell tests exercise
tool reuse, missing/mismatched versions, failed installs, and non-Actions callers.
The GoReleaser fixture rehearsal runs the warmup command against all six supported targets.

After merge, dispatch the warmup twice against the same main commit and compare
the matched keys, cache transfer time, GoReleaser duration, and total job duration.
Summaries include uncompressed cache sizes; Actions logs report transfer sizes.
Check cache storage/eviction alongside the timing results before expanding it.
Validate exact artifact identity, signatures, and provenance on the next requested
RC. Local tests cannot establish main-scoped cache reuse or a production speedup.
