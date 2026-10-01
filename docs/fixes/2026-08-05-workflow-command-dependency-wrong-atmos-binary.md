# Fix: Workflow→command dependencies now dispatch via the running binary, not PATH-resolved `atmos`

**Date:** 2026-08-05

## Summary

`dependencies.commands` on a workflow (resolved via a subprocess, since `internal/exec` cannot
invoke a registered `*cobra.Command` in-process without an import cycle back into `cmd`) shelled
out to the bare command name `"atmos"`. `exec.Command` resolves a bare name via a `PATH` lookup,
so this could silently run a different, unrelated `atmos` binary than the one actually invoked by
the user — for example a stable release installed globally, instead of a local dev build — with no
error or warning. The subprocess dispatch now resolves `os.Executable()` (the currently-running
binary's own absolute path) and passes that instead, matching the existing, correct pattern already
used by `type: atmos` workflow steps.

## Context

Discovered while field-testing the task-runner/dependency-graph feature
(`osterman/task-runner-first-class-support`): `atmos workflow release -f task-runner.yaml`, where
`release` depends on a command `compile`, threw a hard CEL decode error
(`undeclared reference to 'timestamp'`) for an unrelated command elsewhere in the same
`atmos.yaml`. Isolated `pkg/condition` tests (single-threaded and `-race`-concurrent repeated
compiles of the exact same expression) proved the CEL environment itself was completely stable, so
that wasn't the real cause. Prepending the local dev build's directory to `PATH` made the exact
same command succeed with no error — proving the subprocess dependency dispatch was actually
running a different, older, globally-installed `atmos` (v1.225.0, predating the new CEL
identifiers this branch adds), not the branch's own build.

`pkg/runner/step/atmos.go` already solves this exact problem correctly for `type: atmos` workflow
steps, with a doc comment explaining why: `os.Executable()` "ensures that the same binary is used
even when invoked via relative paths, symlinks, or from different working directories." The new
`commandRunnerViaSubprocess` (added by this branch) just didn't follow that established pattern.
Other pre-existing bare-`"atmos"` subprocess sites elsewhere in the codebase (e.g.
`pkg/workflow/control_executor.go`) predate this branch and were left untouched — out of scope for
this fix.

## Changes

- `internal/exec/workflow_dependency_adapter.go`: added `resolveAtmosBinary()`, mirroring
  `pkg/runner/step/atmos.go`'s `os.Executable()` pattern (returns an error rather than silently
  falling back to the bare name, which would just reintroduce the bug in the failure case).
  `commandRunnerViaSubprocess` now resolves the binary path once per dispatch and passes it to
  `ExecuteShellCommand` instead of the literal `"atmos"`. Passing an absolute path also makes
  `exec.Command` skip `PATH` lookup entirely (only bare names trigger `LookPath`).

## Validation

- New test `TestResolveAtmosBinary_UsesOwnExecutablePath`
  (`internal/exec/workflow_dependency_adapter_test.go`): confirmed it fails to compile
  (`undefined: resolveAtmosBinary`) before the fix, and passes after — asserts the resolved path
  equals `os.Executable()`'s value, is never the literal `"atmos"`, and is absolute.
- Existing dependency tests in the same file
  (`TestExecuteWorkflow_DependenciesWorkflowsSameFile`/`...CrossFile`) still pass.
- End-to-end: rebuilt `atmos`, ran `atmos workflow release -f task-runner.yaml` against the
  `examples/task-runner-dependencies/` fixture with no `PATH` manipulation — `compile` (the
  workflow's command dependency) now runs correctly before `release`'s own step, exit 0.

## Follow-ups

None.
