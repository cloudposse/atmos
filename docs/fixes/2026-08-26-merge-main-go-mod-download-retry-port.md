# Fix: Port `go mod download` retry into `magefiles/build.go` during `main` merge

**Date:** 2026-08-26

## Summary

Merging `origin/main` into `osterman/fips140-godebug-support` produced a modify/delete conflict
on `scripts/build-atmos.sh`: this branch had already deleted the script (converted to the native
`magefiles/build.go` `Build.Binary` mage target), while `main` had modified it in the meantime to
add a retry loop around `go mod download` for transient `proxy.golang.org` failures
(`docs/fixes/2026-08-25-build-atmos-go-mod-download-retry.md`). Resolving the conflict by simply
keeping the deletion would have silently dropped that fix. Ported the retry loop into
`magefiles/build.go` instead.

## Context

`main`'s fix wrapped the shell script's bare `go mod download` call in a 3-attempt/15s-backoff
`until` loop, matching the convention already used for artifact downloads
(`.github/actions/download-artifact-retry`), after two CI jobs failed on a mid-stream HTTP/2
stream reset from the Go module proxy's CDN — a transient network blip unrelated to any actual
dependency problem. Since `scripts/build-atmos.sh` no longer exists on this branch (replaced
during the FIPS-140/mage-build-target work earlier in this PR), the fix had to move to
`magefiles/build.go`'s equivalent `go mod download` call site instead of being re-applied to the
now-deleted file.

## Changes

- `magefiles/build.go`: added `runGoModDownload(dir string, env []string) error`, retrying `go
  mod download` up to `goModDownloadMaxAttempts` (3) times with a `goModDownloadRetryDelay` (15s)
  pause between attempts, matching the shell script's original behavior. `Build.Binary` now calls
  this instead of a single unretried `runIn(..., "go", "mod", "download")`. The sleep call is
  routed through a package-level `goModDownloadSleep` variable (defaults to `time.Sleep`) so tests
  can override it to a no-op.
- `magefiles/mage_test_helpers_test.go`: extended the shared fake-binary test harness with a
  `fakeBinFailUntilEnv`-driven mode (`setUpFakePathBinaryFailingNTimes` /
  `readFakeBinInvocationCount`) that fails the first N invocations of the fake `go` binary and
  succeeds afterward, so retry-then-succeed and retry-exhausted paths can be exercised
  deterministically without real subprocess network calls.
- `magefiles/build_test.go`: added `TestRunGoModDownload` (first-attempt success, recovers after
  transient failures, fails after exhausting all attempts) and updated the existing "propagates go
  mod download failure" `TestBuildBinary` subtest to use the new no-sleep/fail-N-times helpers
  (it previously took ~30s per run because it triggered two real 15s sleeps once the retry loop
  was wired in).
- Resolved the `scripts/build-atmos.sh` merge conflict by accepting the deletion (this branch's
  `magefiles/build.go` conversion supersedes it) rather than restoring `main`'s modified version.

## Validation

- `go build ./...` — clean, full repo compiles after the merge.
- `go vet -tags mage ./magefiles/...` — clean.
- `go test -tags=mage ./magefiles/...` — all pass; `runGoModDownload` at 100% coverage
  (`go tool cover -func`), full package suite completes in ~26s (previously ~34s with the
  unfixed slow subtest still doing real sleeps).
- `go tool mage build:binary default test` — produces a working `build/atmos` binary.
- `./build/atmos lint --changed` — `0 issues` (fixed two `godot` comment-capitalization findings
  and one `unparam` finding — a test helper's `name` parameter that only ever received `"go"` —
  introduced while porting this fix).
- No leftover `<<<<<<<`/`=======`/`>>>>>>>` conflict markers outside of pre-existing
  documentation/test-fixture content that legitimately contains example conflict markers as
  literal text (`docs/prd/atmos-init.md`, `docs/prd/atmos-scaffold.md`,
  `docs/prd/three-way-merge/*.md`, `pkg/generator/merge/text_merger_test.go`).

## Follow-ups

None.
