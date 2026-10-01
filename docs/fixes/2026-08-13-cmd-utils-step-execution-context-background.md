# Fix: custom-command step execution now propagates Cobra cancellation instead of using `context.Background()`

**Date:** 2026-08-13

## Summary

In `cmd/cmd_utils.go`'s `executeCustomCommand`, three step-execution call
sites hardcoded `context.Background()` instead of deriving from the Cobra
command's context: the extended-step handler dispatch (`executor.Execute`),
the shell-step container-override call
(`workflowPkg.RunStepContainerOverride`), and the script-step
container-override call added earlier today (see
`docs/fixes/2026-08-13-custom-command-script-step-container-override-dropped.md`).
`context.Background()` never carries cancellation, so a Ctrl-C on the
top-level `atmos` invocation could not interrupt an extended step handler or
a container runtime operation started through one of these paths — the
operation would run to completion regardless of the user's cancellation.

## Context

Flagged by a CodeRabbit review comment on PR #2879
(`cmd/cmd_utils.go#L1308-L1310`, with sibling sites at `#L1329-L1342` and
`#L1379-L1392`) against the commit that added the script-step
container-override routing. Verified against current code: all three call
sites still existed as described. This same function already establishes the
correct pattern a few dozen lines earlier, for dependency-graph execution:

```go
// Use cmd.Context() so cancellation (e.g. Ctrl-C on the top-level Cobra invocation)
// propagates into the dependency graph; context.Background() would let it run to
// completion after the user has already cancelled. cmd.Context() is nil only when this
// command is invoked directly in tests without going through Cobra's Execute().
depCtx := cmd.Context()
if depCtx == nil {
    depCtx = context.Background()
}
```

The three flagged call sites just hadn't been updated to follow it.

## Changes

- `cmd/cmd_utils.go`: added a single `executionCtx` derivation (`cmd.Context()`,
  falling back to `context.Background()` for direct-test invocations that
  bypass Cobra's `Execute()`) right before the `runExtendedStep` closure,
  mirroring the existing `depCtx` pattern in the same function. Replaced the
  three `context.Background()` call sites the review identified —
  `executor.Execute` in `runExtendedStep`, the shell-case
  `RunStepContainerOverride` call, and the script-case
  `RunStepContainerOverride` call — with `executionCtx`.

A follow-up CodeRabbit pass on PR #2879 flagged the three remaining
`context.Background()` uses in the same function — `process.RunShellStep`,
`ExecuteCustomCommandControlStep`, and `retry.Do` (previously left unchanged
as out of scope of the narrower original comment) — since they can likewise
run shell steps, control steps, and retry loops after Cobra cancellation.
These three are now also routed through `executionCtx`, so every step-
execution call site in `executeCustomCommand` propagates cancellation.

## Validation

- `go build ./cmd/...` — clean.
- `go test ./cmd -run 'TestCustomCommandStepContainerOverride|TestCustomCommandStepContainerFalseOptOut' -v`
  — all three tests pass.
- `go test ./cmd -short` (full package, short mode) — pass.
- `./custom-gcl run --new-from-rev=origin/main` — no findings in `cmd/cmd_utils.go`.
- `gofumpt -l cmd/cmd_utils.go` — clean.

## Follow-ups

None identified at the time of the original fix. That turned out to be incomplete — see the
2026-08-17 addendum below.

## Addendum: 2026-08-17 — two more `context.Background()` gaps beneath `executionCtx`

The original fix above made `executionCtx` reach every step-execution call site in
`executeCustomCommand` (`executor.Execute`, `RunStepContainerOverride` ×2,
`process.RunShellStep`, `ExecuteCustomCommandControlStep`, `retry.Do`). It did not check whether
`executionCtx` actually reached the *subprocess* at the bottom of each of those call chains — and
for two of them, it didn't:

1. **Plain (non-TTY, non-interactive) `shell` steps.** `process.RunShellStep(executionCtx, ...)`
    only honors `ctx` on its TTY/interactive branch (`RunShellSession`); the plain branch calls the
    caller-supplied `plain()` closure directly, with no context at all. That closure called
    `e.ExecuteShellWithWriters(&e.ExecuteShellSpec{...})`, and `internal/exec.ExecuteShellSpec` had
    no `Context` field — so `pkg/utils/shell_utils.go`'s `ShellRunnerWithWriters` always fell back
    to `context.Background()` for the mvdan/sh interpreter, and the subprocess it launched was
    uncancelable.
2. **`atmos` step type.** This step type calls `e.ExecuteShellCommand` directly (not through
    `process.RunShellStep`). Its `execOpts` slice only ever included `e.WithStdoutCapture(...)`,
    `e.WithStderrCapture(...)`, and conditionally `e.WithProcessStreams(...)` — never
    `e.WithProcessContext(executionCtx)` — so `ExecuteShellCommand` also fell back to
    `context.Background()` internally.

`pkg/retry/retry.go`'s retry loop already respected `ctx.Done()` correctly (see
`TestExecutor_Execute_ContextCancelled`), so the loop itself would stop on cancellation — but a
single already-in-flight shell/atmos attempt would keep running to completion regardless, because
of the two gaps above.

### Changes

- `internal/exec/shell_utils.go`: added a `Context context.Context` field to `ExecuteShellSpec`
  (documented as defaulting to `context.Background()` when nil, matching `ShellRunnerSpec`'s
  existing doc comment). `ExecuteShellWithWriters` now passes `Context: spec.Context` into the
  `u.ShellRunnerSpec` literal it builds for `u.ShellRunnerWithWriters`, mirroring the same
  `Context: ctx` pattern already used by `pkg/runner/step/shell.go`'s `runInterpreter` and
  `pkg/workflow/control_bridge.go`'s `interpreterShellRunner`.
- `cmd/cmd_utils.go`: the plain-shell closure's `&e.ExecuteShellSpec{...}` literal now sets
  `Context: executionCtx`. The `atmos` step case's `execOpts` slice now includes
  `e.WithProcessContext(executionCtx)`.
- `cmd/custom_command_integration_test.go`: added
  `TestCustomCommandIntegration_ShellStepCancelledByContext` and
  `TestCustomCommandIntegration_AtmosStepCancelledByContext`, covering both gaps. Both use a new
  shared subprocess helper, `TestCustomCommandIntegrationSleepMarkerHelper` (writes a "started"
  marker file immediately, sleeps, then writes a "completed" marker file) plus its builders
  `customCommandSleepMarkerHelperCommand` / `customCommandAtmosSleepMarkerHelperArgs`, and a
  `cancellationOsExitStub` helper that stubs `errUtils.OsExit` with a panic/recover (the same
  pattern `TestExecuteCustomCommandUnsupportedStepTypeExits` already used) since a cancelled
  step's error reaches `errUtils.CheckErrorPrintAndExit`. Each test cancels `cmd.Context()` once
  the helper subprocess has confirmed it started, then asserts the "completed" marker was never
  written — proving the subprocess was killed mid-sleep rather than merely racing the wall clock.
  Verified both tests fail on the pre-fix code (each ran its full 2-second sleep to completion
  before failing the assertion) and pass after the fix (each completes in ~0.1s).
- `internal/exec/workflow_utils.go`'s structurally similar legacy `context.Background()` uses were
  explicitly left untouched — a separate legacy workflow-execution path, out of scope here.

### Validation

- `go build ./...` — clean.
- `go test ./cmd -run 'TestCustomCommandIntegration_ShellStepCancelledByContext|TestCustomCommandIntegration_AtmosStepCancelledByContext' -v`
  — both pass (~0.1s each); confirmed both fail pre-fix (~2s each, hitting the completion-marker
  assertion).
- `go test ./cmd -short` (full package) — pass.
- `go test ./internal/exec/... ./pkg/utils/... ./pkg/process/...` — pass.
- `gofumpt -l cmd/cmd_utils.go internal/exec/shell_utils.go cmd/custom_command_integration_test.go` — clean.
- `./custom-gcl run --new-from-rev=origin/main` — 0 issues.

### Follow-ups

None.
