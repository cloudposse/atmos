# Fix: PTY resize goroutine no longer races with `os.Stdin` reassignment

**Date:** 2026-10-19

## Summary

`pty.setupTerminal` started a SIGWINCH goroutine that read the `os.Stdin` global on every resize and was never
waited for. Its cleanup closed the signal channel but returned before the goroutine exited, so the goroutine could
still read `os.Stdin` after the caller moved on. The goroutine now uses the stdin captured at setup, and cleanup
waits for it to exit.

## Context

The `[race] non-acceptance test suite (shard 3/4)` job failed on a docs-only PR with a `DATA RACE` in
`pkg/runner/step`. `TestInteractiveMaskingUsesActualInput` (added on `main` in #3434) reassigns `os.Stdin`,
`os.Stdout` and `os.Stderr` in its `promptTerminal` helper. An earlier test, `TestShellHandlerTtyStepSuccess`, runs a
PTY session through `ExecWithPTY` and `setupTerminal`; the resize goroutine it left behind read `os.Stdin`
(`setup_unix.go`) with no happens-before relationship to that write.

The race is timing-dependent: the last runs on `main` passed, and it did not reproduce locally when running the two
step tests together, even on unfixed `main`. The code that races is production code in `pkg/terminal/pty`, not the
test, so the fix belongs there. The `os.Stdin` reassignment is an established test pattern.

## Changes

- `pkg/terminal/pty/setup_unix.go`: capture `os.Stdin` once before starting the resize goroutine and pass that value
  to `pty.InheritSize`. Add a `done` channel closed when the goroutine exits, and a `stopResize` helper (stop signal
  delivery, close the channel, wait on `done`). Both the raw-mode error path and the returned cleanup use it.
- `pkg/terminal/pty/setup_unix_test.go`: regression test that calls `setupTerminal` and its cleanup repeatedly and
  reassigns `os.Stdin` immediately after each cleanup.

## Validation

- Before the fix, `CGO_ENABLED=1 go test -race -run TestSetupTerminalCleanupStopsResizeGoroutine ./pkg/terminal/pty/`
  failed with the same race CI reported: a write to `os.Stdin` in the test against a read at `setup_unix.go:28`.
- After the fix, that test passes with `-race -count=5`, and `CGO_ENABLED=1 go test -race ./pkg/terminal/pty/` passes.
- `go vet ./pkg/terminal/pty/` passes for the host platform and with `GOOS=windows`, and `gofumpt -l` reports nothing.
- Running `TestShellHandlerTtyStep*` and `TestInteractiveMaskingUsesActualInput` 30 times under `-race` against
  `origin/main` passed both with and without the fix, because the original race did not reproduce locally. That run
  shows no regression but is not evidence of the fix; the regression test above is.
- Full CI for the PR was not available at the time of writing.

## Follow-ups

None.
