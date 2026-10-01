# Fix: Telemetry test no longer deletes the shared Atmos cache root (Windows toolchain-vanishing flake)

**Date:** 2026-07-29

## Summary

Fixed the root cause of the recurring "Acceptance Tests (windows)" flake where an installed
toolchain binary (`tofu`, `terraform`) vanished from `PATH` mid-way through the `internal/exec`
test suite. `TestPrintTelemetryDisclosureOnlyOnce` in `pkg/telemetry/utils_test.go` "cleaned up"
by calling `os.RemoveAll(filepath.Dir(cfg.GetCacheFilePath()))` — the real, shared
`<cache>/atmos` root — twice per run (setup + defer). Since #2579 moved the toolchain install
root underneath that same directory (`<cache>/atmos/toolchain`, so the CI cache can archive it in
one shot), this deleted every CI-provisioned tool out from under the other concurrently-running
`go test` package binaries. All four telemetry disclosure tests now redirect
`ATMOS_XDG_CACHE_HOME`/`XDG_CACHE_HOME` to a per-test temp directory instead of deleting anything
shared.

## Context

Windows acceptance runs had been flaky on `main` since ~2026-07-24 (also failing runs on
2026-07-24 and 2026-07-29 on `main` itself), always with the same signature: a
terraform/tofu-dependent test in `internal/exec` failing with `executable file not found in
%PATH%`, with a different victim test each run. Four earlier rounds of fixes on PR #2812 removed
real-but-secondary hazards (a TOCTOU double-`LookPath` race, an empty-path error-reporting bug,
`pkg/toolchain` tests installing real binaries into the shared cache without `InstallPath`
isolation, a retry window) yet the failure kept recurring, always exhausting the full retry
window — proving sustained absence, not a timing blip.

The breakthrough came from forensic instrumentation added in `a68e195d85`
(`executableLookupForensics` in `tests/preconditions.go`, kept in-tree as a permanent
early-warning system): on the next failure it showed **every** toolchain directory (terraform,
opentofu, helm, helmfile) missing simultaneously — the whole `<cache>/atmos` tree was gone, not
one version dir. Only `internal/exec` kept failing because the `tests` package self-heals via its
`TestMain` toolchain re-provisioning.

Sibling tests (`TestPrintTelemetryDisclosure`, `...DisabledInCI`, `...DisabledByConfig`) had a
related latent bug: they "cleaned" `./.atmos` — a directory the disclosure code never used — while
actually reading/writing the real user-level `cache.yaml`, polluting developer machines.

## Changes

- `pkg/telemetry/utils_test.go`:
  - Added `isolateTelemetryCache(t)` helper that redirects `ATMOS_XDG_CACHE_HOME` and
    `XDG_CACHE_HOME` to `t.TempDir()`, with a comment documenting the shared-cache-root hazard.
  - `TestPrintTelemetryDisclosureOnlyOnce`: replaced the `os.RemoveAll(filepath.Dir(
    cfg.GetCacheFilePath()))` setup/defer pair with the isolation helper.
  - `TestPrintTelemetryDisclosure`, `TestPrintTelemetryDisclosureDisabledInCI`,
    `TestPrintTelemetryDisclosureDisabledByConfig`: replaced the ineffective `./.atmos` cleanup
    with the same isolation helper.
  - Dropped the now-unused `path/filepath` import.

Commit: `96666c3808` (originally landed on `osterman/support-ci-git-clone`, PR #2812). Supporting
earlier commits in the same investigation: `a68e195d85` (forensics), `c0181b8f07` / `fdbb83ff15`
(toolchain test `InstallPath` isolation), `3f217e6851` / `0cfc8dd925` (lookup error-reporting and
TOCTOU fixes). This self-contained investigation was cherry-picked off `osterman/support-ci-git-clone`
onto `osterman/toolchain-cache-fix-own-pr` to ship as its own PR, independent of #2812's unrelated
CI git-clone-bootstrap feature.

## Validation

- `go vet ./pkg/telemetry/...` — clean.
- `go test ./pkg/telemetry/...` — full package passes; all four disclosure tests pass
  individually.
- `./custom-gcl run --config=.golangci.yml --new-from-rev=origin/main ./pkg/telemetry/...` —
  0 issues.
- Pre-commit hooks (gofumpt, golangci-lint, etc.) passed on commit; commit signed and pushed.
- The race itself is Windows-CI-specific (concurrent package test binaries sharing one cache
  root) and cannot be reproduced locally on macOS; confirmation comes from the next Windows
  acceptance runs on this branch, with the in-tree forensics ready to identify any residual
  deleter if one exists.

## Follow-ups

None.
