# Fix: Windows test-precondition tool cache never matched `.exe` binaries

**Date:** 2026-07-18

## Summary

`TestCLITerraformClean` (and any other `internal/exec` test gated by `tests.RequireTerraform`/
`RequireExecutable`) failed on the `Acceptance Tests (windows)` CI job with
`process start failed: exec: "terraform": executable file not found in %PATH%`, even though the
same CI run's `tests` package had already downloaded and cached `terraform.exe` via
`testhelpers.ProvisionToolchain`. `tests/preconditions.go`'s `prependCachedTestTool` — the
fallback that lets *other* packages' test binaries (which don't call `ProvisionToolchain`
themselves) find that same shared on-disk cache — checked for the binary using its bare name
(`"terraform"`) with no `runtime.GOOS == "windows"` handling, unlike every other binary-name
lookup in this codebase (see `pkg/toolchain/install_helpers.go`, `pr_artifact.go`, etc.). On
Windows the cached file is `terraform.exe`, so `os.Stat(binDir/"terraform")` always failed, the
`sync.Once` guard consumed itself without ever prepending the cache dir to `PATH`, and no other
attempt is made for the rest of that test binary's process lifetime.

## Context

`internal/exec` and `tests` are separate `go test` invocations (separate OS processes), so
`ProvisionToolchain`'s `os.Setenv("PATH", ...)` in the `tests` package's `TestMain` never reaches
`internal/exec`'s process — `internal/exec` relies solely on `prependCachedTestTool`'s fallback to
see the same shared `os.UserCacheDir()/atmos/test-toolchain/bin/...` cache directory. This bug is
unrelated to PR #2763's own patch (cast-renderer auto-install) but was surfaced while fixing that
PR's CI failures at the user's explicit request, per the `fix-all`/`test-coverage` skills' updated
policy of fixing genuinely pre-existing failures when the root cause can be confidently traced —
not just reporting them.

## Changes

- `tests/preconditions.go`'s `prependCachedTestTool`: append `.exe` to the expected cached-binary
  filename when `runtime.GOOS == "windows"`, matching the existing convention used throughout
  `pkg/toolchain`.
- `tests/precondition_cached_tools_test.go`: added a `cachedToolBinaryName` test helper mirroring
  the same platform check, and updated the two `TestPrependCachedTestTool` subtests that create a
  fake cached binary to write it under that platform-correct name — otherwise those tests would
  have silently passed on Windows using the wrong (pre-fix) filename and never caught this.

## Validation

- `go build ./...` — passes.
- `go test ./tests/... -run 'TestCachedTestToolForBinary|TestPrependCachedTestTool' -v` — all
  subtests pass on macOS (darwin/arm64); the Windows-specific branch is exercised by the same
  subtests when run on a Windows runner (this repo's CI), since the fake binary filename is now
  computed the same way the production code computes it.
- `atmos fix lint` (patch-scoped vs `origin/main`) — 0 issues.
- Not independently reproduced on live Windows (no Windows environment available in this session);
  the diagnosis is based on static code inspection (confirmed absence of the `runtime.GOOS`
  check present in every analogous binary-name lookup elsewhere in the codebase) plus the CI
  failure log's exact symptom matching this code path precisely. Follow-up: confirm the next
  `Acceptance Tests (windows)` CI run on PR #2763 goes green for this test.

## Follow-ups

None — self-verifying on the next Windows CI run.
