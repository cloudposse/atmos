# Fix: `TestRunLock_ReportsAllTargets`/`TestRunUpdate_ConcurrentAllSkippedReportsEveryTarget` deadlock on Windows CI

**Date:** 2026-08-08

## Summary

The Windows leg of the `Acceptance Tests` GitHub Actions job hung for the full 40-minute `go
test -timeout 40m` budget, then failed with a `panic: test timed out` goroutine dump. The
stack trace pinned the hang inside `TestRunLock_ReportsAllTargets`, blocked in
`syscall.WriteFile`, reached via `(*liveBatchRenderer).render` → `.tick()` →
`(*liveBatchDisplay).tick()` → `runConcurrentBatchWithLiveProgress` → `RunLock` →
`captureCleanTestOutput`. Root cause: `captureCleanTestOutput` (in `pkg/toolchain/clean_test.go`)
redirects `os.Stderr` to a real `os.Pipe()` and forces `viper.Set("force-tty", true)`, but only
drains the pipe's read end *after* the tested function returns. The live batch renderer added in
`docs/fixes/2026-08-07-toolchain-update-live-usage-bugs.md` activates whenever `isTTY()` is true
and writes to the terminal on an 80ms ticker for the duration of the batch. Once the un-drained
pipe's bounded OS buffer filled — a buffer small enough to fill within seconds on Windows, per
the observed hang — every subsequent write blocked forever, and since nothing reads the pipe
until `RunLock`/`RunUpdate` returns, the write (and the whole batch, and the test) never
completed. Linux/macOS runners either have a larger default pipe buffer or scheduled the flaky
race differently and didn't reproduce the hang, which is why this only showed up on the Windows
matrix leg.

## Context

`pkg/toolchain/lock_test.go`'s `TestRunLock_ReportsAllTargets` and
`pkg/toolchain/update_test.go`'s `TestRunUpdate_ConcurrentAllSkippedReportsEveryTarget` both run a
real concurrent `RunLock`/`RunUpdate` batch (against tools that fail fast, but still go through
the full `runConcurrentBatchWithLiveProgress` path) and use `captureCleanTestOutput` to assert on
the printed output. That helper predates the live-renderer work and was written for tests that
print a bounded, already-known number of lines and return quickly — its drain-after-return
design was never a problem until a genuinely live, ticker-driven writer was introduced on top of
it. `captureCleanTestOutput` itself is not being changed here: other callers that don't exercise
the live renderer are unaffected, and the pipe-based redirect is still the more faithful
simulation of a real TTY for tests that care about exact column-wrapped output.

## Changes

- `pkg/toolchain/clean_test.go` — added `captureUITestOutput`, a new test helper modeled on the
  existing `captureDataOutput` (`pkg/toolchain/get_test.go`): it redirects the `ui` package's
  output to an in-memory `bytes.Buffer` via `iolib.NewContext(iolib.WithStreams(&testStreams{...}))`
  instead of a real OS pipe, and defensively forces `force-tty` off regardless of prior global
  viper state. A `bytes.Buffer` never blocks on write, so it's safe for a test whose function
  under test runs a real, ticker-driven live renderer.
- `pkg/toolchain/lock_test.go` — `TestRunLock_ReportsAllTargets` now uses `captureUITestOutput`
  instead of `captureCleanTestOutput`.
- `pkg/toolchain/update_test.go` — `TestRunUpdate_ConcurrentAllSkippedReportsEveryTarget` now uses
  `captureUITestOutput` instead of `captureCleanTestOutput`.

## Validation

- `go build ./...` — clean.
- `go test ./pkg/toolchain/ -run 'TestRunLock_ReportsAllTargets|TestRunUpdate_ConcurrentAllSkippedReportsEveryTarget' -v -timeout 50s` —
  both pass in under a second (previously hung indefinitely under the old helper once the live
  renderer was active).
- `go test ./pkg/toolchain/... ./cmd/toolchain/...` — all packages pass.
- `gofmt -l` — clean on both touched files.
- `./custom-gcl run --new-from-rev=origin/main` — 0 issues.
- Not yet reproduced on an actual Windows runner (macOS/Linux dev environment only) — the fix
  removes the blocking OS pipe from the code path entirely rather than tuning buffer sizes or
  timing, so it isn't timing-sensitive, but the next CI run against this branch is the real
  confirmation.

## Follow-ups

`--force-tty` combined with a slow or non-draining consumer of the CLI's real stderr (e.g. a
pipe to another process, or a recording tool, during a long `atmos toolchain update`/`lock`
batch) could in principle hit the same class of blocking write in production, not just in this
test harness — real usage almost always has stderr connected to an actual terminal or a
freely-draining redirect, and the live renderer already has a non-TTY fallback via
`isTTY()`/`log.GetLevel()`, so this is a narrow residual risk rather than a known bug. No
production code change is proposed here since nothing concrete currently exercises it; flagging
it in case a future report of a hung `--force-tty` batch traces back to this.
