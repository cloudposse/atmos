# Fix: Child Process Trees Are Killed When Atmos Is Signalled

**Date:** 2026-10-07

## Summary

Children spawned through `pkg/process` now run in their own process group, and the whole group is terminated when a step context is canceled (for example a step `timeout:`) or when Atmos receives SIGINT or SIGTERM. Before this change, `kill -TERM <atmos pid>` exited Atmos with 143 but left the child, such as the `sleep 31` started by `exec.run(["sleep","31"])`, running.

## Context

A terminal Ctrl-C hides the problem because the terminal signals the whole foreground process group. CI cancellation and `kill` signal only the Atmos process. Two causes combined:

- `pkg/process/process.go` built commands with `exec.CommandContext` and no `SysProcAttr`, `Cancel`, or `WaitDelay`. Context cancellation killed only the direct child, so grandchildren survived a step timeout.
- No signal handler canceled any context or killed children. `main.go` ran `signals.RunExitCleanups()` and `cmd.Cleanup()` and then exited.

## Changes

- `pkg/process/process_group.go`: new `childGroup` helper applied to every command started by `DefaultRunner.Run` and `RunScript`. It tracks live children in a mutex-protected registry and installs a single exit cleanup with `signals.RegisterExitCleanup`. The cleanup sends SIGTERM to every live child tree, waits up to 2 seconds, then sends SIGKILL to survivors. `main.go` already runs exit cleanups before `os.Exit`, so no `main.go` change was needed.
- `pkg/process/process_unix.go`: sets `SysProcAttr.Setpgid`, installs `cmd.Cancel` (SIGTERM to the group, falling back to `Process.Kill`, then waits for the group to exit and sends SIGKILL to survivors after the grace period), and the signalling helpers.
- `pkg/process/process_windows.go`: no-op group handling so the package keeps compiling. Windows still terminates only the direct child.
- Terminal exception: a child whose stdin is a real terminal is not moved into a new process group, because a background group that reads or configures the terminal is stopped by SIGTTIN/SIGTTOU (editors, prompts, browser login flows). Such children are still tracked, and the exit cleanup signals the direct child.
- `cmd.WaitDelay` is 2 seconds for grouped children. A successful exit that only trips `exec.ErrWaitDelay` (a backgrounded grandchild holding an output pipe) is treated as success instead of a failure; previously `Wait` would have hung until that grandchild exited.
- Cancellation cleanup completes before `Wait` returns. A grandchild that ignores SIGTERM is still killed after the grace period even if its parent exits immediately and the grandchild holds no output pipes. Cleanup stops when the process group disappears and leaves no delayed signal timer behind.
- `Run` post-wait result handling moved into `recordWaitOutcome` to stay under the function-length limit; behavior is unchanged.
- Shell sessions (`RunShellSession`, PTY or attached) are unchanged: the PTY path already calls `Setsid`, and these sessions own the terminal.

- Add a portable direct-child exit-cleanup regression test. It waits for the Go
  helper to start and register, invokes `signals.RunExitCleanups()` while the
  context remains active, and requires an unsuccessful reaped process and an
  empty child registry. The test runs on Windows as well as Unix and does not
  use a platform-specific process-liveness stub.

## Validation

- `go test ./pkg/process/... ./pkg/signals/... -race -count=1` passes.
- `go test ./pkg/runner/step/ -run 'Timeout|Process' -count=1` passes.
- `go build ./...` and `GOOS=windows go vet ./pkg/process/` pass.
- New tests in `pkg/process/process_group_test.go` use the test-binary re-exec pattern (gate added to the existing `TestMain`): context cancel and deadline kill child and grandchild, including a SIGTERM-ignoring grandchild whose parent exits first; `signals.RunExitCleanups()` kills registered trees; a stubbed terminal stdin leaves the child in the original process group while a non-terminal stdin makes it lead its own group; a detached output-holding grandchild does not hang or fail an exit-0 command. Grandchild assertions skip on Windows with a stated reason.
- Manual check with a built binary and `exec.run(["sleep","31"])` in a `.star` script: `kill -TERM` (143) and `kill -INT` (130) on Atmos now leave no `sleep 31` process behind.

- The portable direct-child test passed on macOS. The complete process and
  signals package tests passed with race detection and shuffled order: process
  coverage was 94.2%, signals coverage was 100%. Windows amd64 test
  cross-compilation and `go vet` passed; Windows execution remains a CI check.
  These local coverage figures cover the touched packages' own tests, not
  full-suite coverage or Windows-only statements; CI Codecov is authoritative.

## Follow-ups

- Other `exec.Command` call sites outside `pkg/process` (for example `pkg/terminal/pty`, `pkg/auth`, `internal/exec`) do not go through this helper and keep the old behavior. Routing them through `pkg/process` would extend the guarantee.
- A grouped child now receives SIGTERM instead of the terminal's SIGINT when Atmos is interrupted with Ctrl-C while stdin is not a terminal.
