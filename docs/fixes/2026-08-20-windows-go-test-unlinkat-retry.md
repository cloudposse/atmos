# Fix: retry the transient Windows `go: unlinkat ...` race in acceptance test shards

**Date:** 2026-08-20

## Summary

`Acceptance Tests (windows, shard 10/10)` (GitHub Job ID: 96582175875) failed even though every
one of its 39 packages reported `ok`. The actual failure was Go's own toolchain diagnostic:

```text
go: unlinkat C:\Users\RUNNER~1\AppData\Local\Temp\go-build860437867\b2679\list.test.exe: The process cannot access the file because it is being used by another process.
```

This fires when `go test` tries to delete its own compiled temp test binary after the test run
has already completed and reported its real result, but another process (commonly Windows
Defender's real-time scanner) still briefly holds the file open. It is a documented Go-on-Windows
race, not a test failure. `internal/ci/acceptance.commandRunner.run` now retries (up to 2 times,
2s apart) when it detects exactly this diagnostic on stderr, and gives up immediately for any
other failure so a real test failure still fails the job on the first attempt.

## Context

Reading the attached job log showed every package in the shard's `pkgs` source-test group
(`cmd/auth`, `cmd/list`, ..., `pkg/web`) printed `ok` before the shard failed. `cmd/list` in
particular had already printed `ok  github.com/cloudposse/atmos/cmd/list  19.301s`. The failure
surfaced only afterward, from `go test`'s own cleanup step, and propagated up through
`runSourceTestGroup` → `runWindowsShard` → `mage acceptance:run` as a generic `exit status 1`,
making the job (and the required `Acceptance Tests` gate) fail despite no actual test regression.

`internal/ci/acceptance/run.go`'s `runWindowsShard` already runs three test groups concurrently
on Windows (`tests.test.exe`, `internal-exec.test.exe`, and the on-the-fly `go test <pkgs>` for
everything else) specifically to save wall-clock time; that concurrency is the likely trigger for
the antivirus-scan race, since several test binaries are being written/executed/deleted around
the same time.

## Changes

- `internal/ci/acceptance/command.go`:
  - `commandRunner.run` now captures stderr (via `io.MultiWriter`, so live CI log streaming is
    unaffected) and retries the whole command up to `transientUnlinkRetries` (2) times, 2s apart,
    when `isTransientWindowsUnlinkError` matches. Any other failure (including a real test
    failure, which never emits this diagnostic) still returns immediately on the first attempt.
  - Added `isTransientWindowsUnlinkError`, a pure string-match helper.
  - Added a `retryDelay` field on `commandRunner` (defaulted to the real 2s in
    `newCommandRunner`) so tests can zero it out instead of paying real wall-clock time.
- `internal/ci/acceptance/command_test.go`: added `TestIsTransientWindowsUnlinkError` (table
  test over the pure matcher) and three end-to-end tests
  (`TestRunRetriesTransientWindowsUnlinkError`, `TestRunGivesUpAfterExhaustingRetries`,
  `TestRunDoesNotRetryUnrelatedFailures`) that drive `commandRunner.run` against a real child
  process -- this test binary re-execing itself via a `TestMain` sentinel (this repo's
  cross-platform convention for simulating subprocess behavior instead of relying on
  platform-specific binaries like `false`), controlled by a counter file so the same binary can
  simulate N transient failures followed by success, or an unrelated non-transient failure.

## Verification

- `go test ./internal/ci/acceptance/...` passes, including the new retry tests.
- `go build ./internal/ci/acceptance/...` and `go vet ./internal/ci/acceptance/...` clean.
- `gofumpt -l` clean on both changed files.

## Follow-up: scope the retry to `go test` invocations only

A PR review (CodeRabbit) correctly flagged that `commandRunner.run`'s retry, as first written,
applied to every call through the shared `run` helper -- including `go test -c` (which writes a
persistent `-o` binary Go never auto-deletes), `go tool covdata merge`/`textfmt`, and precompiled
`*.test.exe`/`cmd.test` binaries executed directly. None of those can hit the actual unlinkat
race (only a bare, on-the-fly `go test <pkgs>` compiles-runs-deletes its own temp binary), so
retrying them on a coincidental stderr match risked rerunning a command with real side effects
(e.g. writing coverage data twice) or silently masking an unrelated failure that happened to
mention both substrings.

Fixed by replacing `run`'s four positional parameters (`dir`, `env`, `retryTransient`, `name`)
with an explicit `runOptions{dir, env, retryTransient}` struct (also resolving a `revive`
`argument-limit` violation from the extra bool), passed as `true` only from the two call sites
that run bare `go test <pkgs>`: `runSourceTestGroup` (`run.go`) and `CollectCoverage`
(`coverage.go`). Every other call site (`go test -c`, `go tool covdata`, and the three precompiled
test binary executions) now explicitly passes `retryTransient: false`. Added
`TestRunDoesNotRetryWhenNotRetryTransient` to `command_test.go`, which drives the same transient
diagnostic through the helper subprocess with `retryTransient: false` and asserts `run` fails on
the first attempt without ever invoking the command a second time.
