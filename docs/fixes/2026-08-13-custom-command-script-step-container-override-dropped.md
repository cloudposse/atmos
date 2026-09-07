# Fix: custom-command `type: script` steps now honor a step-level `container:` override

**Date:** 2026-08-13

## Summary

In `cmd/cmd_utils.go`'s `executeCustomCommand`, only `type: shell` steps
checked for a step-level `container:` override before executing. `type:
script` steps had no explicit case in the step-type switch, so they fell into
the `default` branch and routed through the registered `pkg/runner/step`
handler (`ScriptHandler`) straight to `process.RunScript` on the host,
unconditionally — a `container:` override on a script step was silently
ignored. The workflow-file execution path
(`internal/exec/workflow_utils.go`) already special-cased `commandType ==
schema.TaskTypeScript` correctly; only the custom-command path had the gap.

## Context

This gap was explicitly identified and deliberately deferred in
`docs/fixes/2026-08-07-custom-command-container-block-dropped.md`'s "Scope
notes / follow-ups" section, which covered `type: shell` container overrides
for custom commands and flagged the identical `type: script` gap as
out-of-scope at the time. It resurfaced as a code-review finding on
`osterman/test-container-fields-ignored` and was verified as still open by
reading the current `cmd/cmd_utils.go` step-type switch and confirming no
container-override check exists on the script/default execution path.

`pkg/workflow/container.go`'s `containerStepCommand` was already
script-aware (it special-cases `step.Type == schema.TaskTypeScript` to invoke
via `process.ScriptInvocation` instead of wrapping the display command in a
generic `sh -lc`), so `workflowPkg.RunStepContainerOverride` itself needed no
changes — this was purely a wiring gap in the custom-command step dispatcher.

## Changes

- `cmd/cmd_utils.go`: added an explicit `case schema.TaskTypeScript:` to the
  step-type switch in `executeCustomCommand`, mirroring the `"shell"` case's
  container-override branch (`step.ToWorkflowStep()` →
  `workflowPkg.StepContainerOverride` check → `workflowPkg.RunStepContainerOverride`
  with `Command: process.FormatScriptDisplay(step.Interpreter, step.Script)`
  on the override path). Extracted the step-execution body previously inlined
  in the `default` case's `IsExtendedStepType` branch into a shared
  `runExtendedStep` closure, so both the new script case's non-override
  fallback and the `default` case's extended-step handling (`input`,
  `confirm`, `choose`, etc.) call the same code — no duplicated execution
  logic, and no behavior change for any other step type (`exec`, `atmos`,
  `parallel`, `matrix`, extended types).
- `cmd/custom_command_container_override_test.go`: added
  `TestCustomCommandStepContainerOverrideRunsInsideContainer_ScriptType`,
  mirroring the existing shell-step container-override test's fixture and
  assertion pattern (real `atmos.yaml` on disk, `cfg.InitCliConfig`,
  `processCustomCommands`, `RootCmd.Execute()`,
  `testhelpers.InstallFakeContainerRuntime`). Defines a `type: script` step
  with a `container:` override and asserts the fake docker's captured `exec`
  argv shows the interpreter invoked directly with `-c` and the raw script
  body (the `ScriptInvocation`-shaped form `containerStepCommand` produces
  for script steps) — and explicitly asserts `-lc` is absent, distinguishing
  the script-aware container path from the generic shell-wrap path the
  `"shell"` case uses.

## Validation

- `go build ./...` — clean.
- `go test ./cmd -run 'TestCustomCommandStepContainerOverride|TestCustomCommandStepContainerFalseOptOut' -v`
  — all three tests pass (the existing shell-step test, the new script-step
  test, and the existing `container: false` opt-out test).
- `go test ./cmd -short` (full package, short mode) — pass, confirming the
  `runExtendedStep` extraction didn't regress `input`/`confirm`/`choose` or
  other extended step types.
- `./custom-gcl run --new-from-rev=origin/main` — no findings in the changed
  files.
- `gofumpt -l` on the changed files — clean.

## Follow-ups

- Updated `docs/fixes/2026-08-07-custom-command-container-block-dropped.md`'s
  "Scope notes / follow-ups" section with a pointer to this fix.
