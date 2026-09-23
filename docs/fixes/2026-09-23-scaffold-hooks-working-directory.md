# Fix: scaffold hooks now default to the scaffold's target directory

**Date:** 2026-09-23

## Summary

`before.scaffold.generate`/`after.scaffold.generate` step-backed hooks (`kind: step`/`kind: steps`) never
anchored a step's `working_directory`, so they silently ran in the process's own cwd instead of the
scaffold's target/output directory whenever the two differed. Hooks now default to the scaffold's target
path, matching how component/stack lifecycle hooks already default to the component's directory.

This is not a behavior change for the common case. Before this fix, an unset (or bare-relative)
`working_directory` resolved against the process's cwd; after, it resolves against the scaffold's
target path instead. Whenever `target` itself resolves to the same absolute directory as the
invoking shell's cwd -- e.g. `atmos scaffold generate <template> .`, or any equivalent path -- both
resolve to that identical directory, so existing hooks that already worked continue to behave
exactly as before. The fix only changes behavior for the previously-broken case: a `target` whose
absolute path differs from cwd.

## Context

`atmos scaffold generate <template> <target>` lets `target` differ from the invoking shell's cwd. The
documented hook example (`terraform fmt -recursive` on `after.scaffold.generate`) relied on the hook
running inside the generated output, but nothing in the scaffold hook engine ever set that anchor:

- `pkg/hooks/step_engine.go`'s `stepEngine.Run`/`stepsEngine.Run` call `setDefaultStepWorkingDirectory`
  before executing each step, defaulting an empty `working_directory` to `ComponentPath(ctx)` (and
  anchoring a bare-relative value there too).
- `pkg/generator/scaffoldhooks/scaffoldhooks.go`'s `runStep`/`runSteps` had no equivalent call. An unset
  `step.WorkingDirectory` fell through `ShellHandler` straight into `exec.Cmd.Dir = ""`, which Go
  defaults to the process's own cwd.
- The scaffold's target directory (`targetPath`) was already in scope at every hook call site in
  `pkg/generator/ui/ui.go`'s `executeWithSetup`, but `scaffoldhooks.Run`'s signature had no parameter to
  receive it.

Filed as [cloudposse/atmos#3205](https://github.com/cloudposse/atmos/issues/3205).

## Changes

- `pkg/hooks/step_engine.go`: extracted the anchor-agnostic empty/bare/dot/absolute working-directory
  defaulting convention out of `setDefaultStepWorkingDirectory` into a new exported,
  `ExecContext`-independent `ApplyDefaultWorkingDirectory(step, anchorDir)`. Exported `AtmosStepType` and
  `IsBareRelativePath` so `scaffoldhooks` (which already imports `pkg/hooks`) can reuse them instead of
  duplicating the classification logic.
- `pkg/generator/scaffoldhooks/scaffoldhooks.go`: replaced `Run`'s five positional parameters plus the
  new `targetPath` with a `RunInput` struct (to stay within revive's `argument-limit`). `runStep`/
  `runSteps` now call `hooks.ApplyDefaultWorkingDirectory(step, targetPath)` before executing, and
  `stepVariables` exposes `targetPath` as `{{ .TargetPath }}` template data (alongside the existing
  `{{ .Answers }}`) and as the step executor's `componentWorkingDir` anchor for any other relative step
  field.
- `pkg/generator/ui/ui.go`: all three `scaffoldhooks.Run(...)` call sites (before-hook, after-hook
  failure path, after-hook success path) now pass `targetPath`, which was already in scope.
- Hook authors can still opt back into the old cwd-relative behavior with an explicit
  `working_directory: "."` in a hook's `with:` block — dot-prefixed and absolute values are left
  untouched by `ApplyDefaultWorkingDirectory`, same convention lifecycle hooks already use.

## Validation

- `go build ./...` — clean.
- `go test ./pkg/hooks ./pkg/generator/scaffoldhooks/... ./pkg/generator/ui/...` — all pass, including a
  new regression test (`TestExecuteWithSetup_HooksDefaultWorkingDirectoryToTargetPath` in
  `pkg/generator/ui/ui_test.go`) written first and confirmed failing against the pre-fix code (captured
  working directory was empty/the process cwd instead of the scaffold's target directory).
- New unit tests: `TestApplyDefaultWorkingDirectory` (`pkg/hooks/step_engine_test.go`) and
  `TestRun_TargetPathReachesStepTemplateData` (`pkg/generator/scaffoldhooks/scaffoldhooks_test.go`).
- `./custom-gcl run --config=.golangci.yml --allow-serial-runners --new-from-rev=upstream/main` — 0
  issues (this fork's `origin/main` is stale relative to `upstream/main`, so the lint diff base was
  pointed at `upstream/main` to scope correctly).
- Full `atmos test` (short suite) was run for additional coverage; the only failure was an unrelated,
  pre-existing timeout in `tests.TestCLICommands` (`tests/cli_test.go`), stuck in `cleanDirectory`'s
  go-git `Worktree.Status()` scan — a file this change never touched, and reproducible independent of
  this fix (likely caused by nested `.claude/worktrees/agent-*` directories in this environment slowing
  down git status scans).

## Follow-ups

None.
