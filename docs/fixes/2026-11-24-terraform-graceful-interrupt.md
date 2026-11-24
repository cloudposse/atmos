# Fix: Ctrl-C or SIGTERM during `atmos terraform apply` kills Terraform before it saves state

**Date:** 2026-11-24

## Summary

Issue #3751. Pressing Ctrl-C (or sending SIGTERM to Atmos) during `atmos terraform apply` made Atmos exit within
milliseconds and kill Terraform mid-apply. With a local backend this left a 0-byte `terraform.tfstate` and a stale
`.terraform.tfstate.lock.info`. Atmos now lets Terraform and OpenTofu stop the way they do when run directly: the
first signal reaches Terraform exactly once, Atmos waits for it to finish or taint the in-flight resource, write
state, and release the lock, and then exits with the usual interrupted status (130 for SIGINT, 143 for SIGTERM). A
second signal forces the exit as before.

## Context

- `main.go` handles SIGINT/SIGTERM by calling `signals.RequestExit`, `signals.WaitForCleanup` (credential cleanup
  scopes only, bounded by 90 seconds), and `signals.RunExitCleanups`, then exits. The exit cleanup registered by
  `pkg/process` cancels every running child: a grouped child gets SIGTERM and SIGKILL after 2 seconds; a child whose
  stdin is a terminal (not grouped, because it must stay in the terminal's foreground group) is killed with
  `Process.Kill` (SIGKILL) at once.
- In an interactive terminal, Terraform's stdin is the terminal, so it shares the foreground group. Ctrl-C reached
  Terraform directly, and Atmos then SIGKILLed it a few milliseconds later while it was writing state. This is the
  0-byte state and stale lock reported in the issue.
- Multi-component runs (`--all`, `--affected`, query) used `signal.NotifyContext` for the subprocess context. Its
  cancellation on the same signal also made `process.DefaultRunner` terminate the running Terraform.
- Plain Terraform waits indefinitely after the first interrupt and hard-aborts on a second one.

## Changes

- `pkg/signals/graceful.go` (new): `GracefulStops` registry. Children that stop on their own register with a
  display name and a forward function. `Begin` (the first SIGINT/SIGTERM while children are registered) retains
  the exit request through `RequestExit`, forwards the signal once to each child, and tells the caller to keep
  running. A child registered after the stop began receives the signal immediately. A second `Begin` forces:
  it returns false and releases `Wait`. `Wait` blocks until every child has exited or the exit was forced.
- Bound decision: the graceful wait is deliberately unbounded, matching Terraform's own behavior after the first
  interrupt, because an in-flight resource can legitimately take minutes. The second signal is the escape hatch.
  The 90-second credential cleanup budget (`maxCleanupShutdownWait`) no longer starts its clock during that wait:
  `restartCleanupBudget` gives it a fresh budget when the last graceful child exits.
- `pkg/signals/owned_cleanup.go`: `ResolveExitCode` waits for graceful children before returning an interrupted
  exit code, so a command that returns early (for example a multi-component scheduler) cannot reach `os.Exit` and
  its exit cleanup while Terraform is still saving state.
- `pkg/signals/signal_context.go` (new): `InterruptContext` replaces `signal.NotifyContext` for multi-component runs.
  It cancels with a cause wrapping the new static `ErrSignalReceived`, and `InterruptedBySignal` lets the runner
  tell an interrupt from a timeout.
- `pkg/process`: new opt-in `TaskSpec.Interrupt` (`InterruptKill` default, `InterruptGraceful`). In graceful mode
  (`graceful.go`), the child runs under a context detached from interrupt-caused cancellation; any other
  cancellation (timeouts, caller cancel) still terminates it. The started child joins the default registry, and a
  graceful child refuses to start once a stop has begun (`ErrProcessInterrupted`), so an interrupted pipeline starts
  no new Terraform command. `Run` was split (`buildCommand`) to stay within function-length limits.
- Signal routing (`graceful_unix.go`): a grouped child gets SIGINT on its process group, as a terminal Ctrl-C
  would deliver it. A child in the terminal's foreground group already received the terminal's SIGINT, so nothing
  is sent (a second SIGINT makes Terraform abort). SIGTERM goes to the child process only, as
  `kill -TERM <terraform pid>` would, because provider plugins ignore SIGINT but not SIGTERM and Terraform still
  needs them to finish the in-flight resource. On Windows (`graceful_windows.go`) nothing is forwarded: the console
  delivers Ctrl-C to every attached process and `os.Process.Signal` cannot send an interrupt, so Atmos only waits.
- `pkg/component/provider.go`: `ExecutionOptions.Interrupt` and `component.WithGracefulInterrupt()`.
  `internal/exec/shell_utils.go` passes it to the runner (now the injectable `shellProcessRunner`).
- `pkg/component/terraform`: `gracefulInterruptOptions` opts in every Terraform/OpenTofu invocation that goes through
  `executeStreamingOrShell` (main command, `init`, `workspace`), `terraform version`, the reviewed-plan `get`, and
  Terragrunt executable mode. User lifecycle hook commands keep the default.
- `cmd/terraform/utils.go`: multi-component runs use `signals.InterruptContext`, so the scheduler stops starting
  components on the first signal while running components get the graceful treatment.
- `main.go`: on a signal that would exit, `signals.BeginGracefulStop` is consulted first. When it defers the exit,
  Atmos prints one warning (`Interrupt received; waiting for terraform to stop gracefully. Press Ctrl-C again to
  force exit.`, or the SIGTERM variant) and keeps waiting. Interactive TTY steps (`SuspendInterruptExit`) and
  credential cleanup (`DeferExitForCleanup`) are unchanged.
- `errors/errors.go`: `ErrProcessInterrupted`, `ErrSignalReceived`.
- Docs: "Interrupting Terraform" in `website/docs/cli/commands/terraform/usage.mdx` and "Interrupting an Apply" in
  `terraform-apply.mdx`.

## Validation

- New tests failed before the fix and pass after it. With the graceful process path disabled, 8 of the new
  `pkg/process` tests failed; with the Terraform wiring disabled, the new `pkg/component/terraform` tests and the
  `main` package `TestFirstSignalWaitsForTerraformToStop` failed. Guard tests (non-signal cancellation still kills,
  canceled context refuses to start, second signal forces) pass both ways by design.
- `pkg/signals/graceful_test.go`: forward exactly once, late registration, no participants, unbounded wait until
  drain, forced wait, `ResolveExitCode` waits and returns 130, cleanup budget restart, signal-cause context.
- `pkg/process/graceful_test.go` and `graceful_unix_test.go` (test binary re-exec helpers, no shell tools): grouped
  Ctrl-C reaches the child group once (child and grandchild each record one SIGINT), SIGTERM reaches only the child,
  a terminal-foreground child gets no duplicate SIGINT, a foreground child gets a forwarded SIGTERM, a second signal
  forces termination, non-signal cancellation kills, signal-caused cancellation does not, non-opted-in runs never
  join, `interruptTarget` table.
- `main_graceful_interrupt_test.go`: runs the test binary as Atmos with a fake Terraform; SIGINT and SIGTERM make
  Atmos wait for the fake Terraform to save and exit 130/143 with the warning; a second SIGINT forces exit 130.
- `go test -race -shuffle=on ./pkg/signals/ ./pkg/process/ ./pkg/component/ ./cmd/terraform/` passed;
  `go test ./pkg/component/terraform/ ./cmd/terraform/ ./internal/exec/ .` passed.
- Patch coverage on added non-test Go lines, measured with `-coverpkg` over the changed packages against
  `git diff -U0 origin/main`: 175/175 statements (100%).
- `go build ./...` passed; `go vet` on the changed packages passed for darwin, linux, and windows; gofumpt clean.
  golangci-lint could not run in this environment; its rules were checked by hand.
- `pnpm run build` in `website/` passed. Its broken-anchor warnings are pre-existing and none involve the
  changed pages.
- Manual end-to-end check with a built binary, Terraform 1.5.7 and OpenTofu 1.13.2, a `terraform_data` resource
  with `local-exec` `sleep 8`, a local backend, and an isolated `HOME`/`XDG_*` and `env -i` environment, comparing
  origin/main with this branch:
  - Real terminal Ctrl-C (typed into a pseudo-terminal): origin/main left a 0-byte state and a lock file with both
    Terraform and OpenTofu (exit 130 after 0.02s). With the fix the state was valid (tainted resource), no lock file
    remained, exit 130, and the warning was printed.
  - SIGINT and SIGTERM to the Atmos pid with non-terminal stdin: both versions saved valid state here, because this
    resource stops within the 2-second grace that origin/main allowed; with the fix Atmos forwarded the signal and
    exited after Terraform (130/143).
  - `apply --all` with two dependent components and a terminal Ctrl-C: origin/main left a 0-byte state and a lock;
    with the fix the first component's state was saved, no lock remained, and the second component was skipped.
  - A second SIGINT/SIGTERM 5 ms after the first forced the exit; Terraform reported
    "Two interrupts received. Exiting immediately" and no lock file remained.

## Follow-ups

- Pre-existing, not changed here: the streaming TUI path (`--ui`, `pkg/terraform/ui`) runs Terraform through its own
  `exec.CommandContext` and kills it when the TUI is cancelled. It needs a separate decision on whether to route it
  through the graceful path.
- Intended behavior, documented: a SIGINT sent with `kill -INT` to the Atmos pid while Terraform shares the
  terminal's foreground group is not forwarded, because it cannot be told apart from a terminal Ctrl-C that already
  reached Terraform. Atmos then waits for Terraform to finish; a second signal forces the exit.
