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

Other `context.Background()` uses in the same function (e.g.,
`process.RunShellStep`, `ExecuteCustomCommandControlStep`, `retry.Do`) were
not in scope of the review comment and were left unchanged to keep this fix
minimal and targeted at the reported issue.

## Validation

- `go build ./cmd/...` — clean.
- `go test ./cmd -run 'TestCustomCommandStepContainerOverride|TestCustomCommandStepContainerFalseOptOut' -v`
  — all three tests pass.
- `go test ./cmd -short` (full package, short mode) — pass.
- `./custom-gcl run --new-from-rev=origin/main` — no findings in `cmd/cmd_utils.go`.
- `gofumpt -l cmd/cmd_utils.go` — clean.

## Follow-ups

None.
