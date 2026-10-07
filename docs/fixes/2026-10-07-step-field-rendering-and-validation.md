# Fix: Step fields render and validate consistently across custom commands and workflows

**Date:** 2026-10-07

## Summary

A templated `timeout:` on a custom-command step, an unknown flag `type:`, or an unknown `retry:` key each broke or silently misbehaved far beyond the step that set it. Templated `timeout:` and `output:` values now render when the step runs in every step type and in both custom commands and workflows, unknown `retry:` keys and misspelled step fields are reported, and one bad command no longer takes down the others.

## Context

Field testing the Starlark step work found these defects:

- `Task.Timeout` was a `time.Duration` decoded at config load, so `timeout: '{{ .Flags.t }}'` failed the whole config load and every command (`list stacks`, `--help`) stopped working. `WorkflowStep.Timeout` was already a string.
- An unsupported custom-command flag type returned an error from command registration and aborted startup.
- Workflow shell, atmos, and container steps passed no variables to the timeout deadline, so a templated `timeout:` failed with `invalid duration` while script steps rendered it.
- A templated `output: '{{ .Flags.mode }}'` passed validation (rendering was deferred) but the runtime read the raw string and fell back to the default mode.
- `retry:` keys that do not exist (for example `delay`) were dropped silently in YAML, while `steps.task(retry=...)` leaked a Go type name.
- `steps.sleep(duration=...)` validated against the union of all step fields, so the field that means something else on another step type was accepted and ignored.
- Error wording named steps by index, listed an incomplete set of step types, and described scheduler-policy rejections generically.
- Custom-command argument descriptions never appeared in help, the usage line omitted the arguments, and a string default `"2"` looked like an integer.

## Changes

- `Task.Timeout` is a string, parsed and rendered by `StartStepDeadline` when the step starts. `ToWorkflowStep` and `TaskFromWorkflowStep` keep the value verbatim. The Atmos config JSON schema was regenerated.
- Decode problems in a custom-command step (an unknown `retry:` key, a misspelled step field, or a step that cannot be decoded at all) no longer fail config loading. They are recorded in the new `Task.LoadError` / `WorkflowStep.LoadError`; custom-command registration warns once and registers a stub that returns the error when the command is invoked, and the step executor reports it for hook and workflow steps (together with any missing-field error from the handler).
- A flag with an unsupported type or an unusable default makes only its command a stub, with a warning that names the command. Flag-name conflicts with built-in flags stay fatal, as before.
- Workflow steps pass the shared step variables (`stepVars`) to every `RunWithStepRetry`/`RunWithStepDeadline` call, so shell, atmos, container, and script steps render a templated `timeout:` the same way.
- Added `ResolveStepOutputMode` and `ApplyStepOutputMode`; the shell, atmos, container, and script handlers, workflow command steps, and custom-command steps render a templated `output:` and fail with the valid-modes list when the result is not a mode. The workflow-level `output:` is validated as a literal mode.
- Git hook preflight and synchronous step sequences reject recorded decode errors before any step runs, including unknown retry keys and misspelled fields.
- Strict `retry:` keys: `DecodeRetryConfig` (stack manifests) and workflow/task YAML reject unknown keys with `unknown retry field "x"` and the valid keys. `steps.task(retry=...)` reports the same message with the valid keys instead of a Go type, and its timeout error names the task and value. YAML anchors and merge keys continue to work; strict validation checks the effective retry mapping, including inherited unknown keys.
- Added the optional `KnownFields()` handler interface; `sleep` implements it, so `steps.sleep(duration=...)` is rejected with the fields it accepts. Other handlers keep the union of fields.
- Unsupported step types in custom commands and workflows name the step, list the registered types, and add `Use type: script with interpreter: starlark.` for `type: starlark`.
- Scheduler-policy rejections in direct step calls name the offending fields and say they are supported in workflows and lifecycle hooks, not in direct step calls or Git hooks.
- Custom-command help renders an `ARGUMENTS` section, the usage line shows `<required> [optional]`, and string defaults are quoted.
- Regenerated 14 `starlark_*` stderr snapshots that use the `starlark-steps` fixture, which deliberately contains the invalid command `ft-retry-renamed`; they now include the one-line skip warning. The `starlark workflow step retry re-runs the script` snapshot also picks up the pending `fail()` message wording from another work package.
- The `deploy-component` example in `examples/custom-commands/.atmos.d/advanced.yaml` had an unquoted step `echo "Auto-approve: $TF_AUTO_APPROVE"`; YAML parsed the `: ` as a mapping, so the step never ran. The new unknown-field check surfaced it as a skipped command, and the step is now quoted. The `invalid workflow step type` test expectation in `tests/test-cases/workflows.yaml` follows the new wording.

## Validation

- `go test ./pkg/schema ./pkg/runner/... ./pkg/script/... ./pkg/workflow/... ./internal/exec ./cmd ./pkg/hooks/... ./pkg/git/...` pass, except the PTY-based cast session tests in `pkg/runner/step` (`TestCastHandler*Session*`), which time out waiting for cast output in this headless environment and do not touch the changed code.
- New regression tests: `pkg/schema/task_test.go`, `pkg/schema/retry_decode_test.go`, `pkg/runner/step/step_field_rendering_test.go`, `pkg/script/starlark/engine_test.go`, `cmd/custom_command_step_load_error_test.go`, `cmd/custom_command_help_test.go`, and the flag-type case in `cmd/custom_command_inputs_resolution_test.go`.
- `go test ./tests -run 'TestCLICommands/starlark' -count=1` passes, including the new `tests/test-cases/starlark-step-fields.yaml` cases (templated timeout command, `list stacks` with an invalid command present, invocation error, templated output modes, workflow timeout and output, `type: starlark` wording, argument help).

- CI correction: the unsupported-step diagnostic test now removes ANSI styling before
  checking the unchanged visible wording. CI enables color, which inserts escape
  sequences around command names and inline code; the raw substring assertions
  failed on all three operating systems and in the race suite. Reproduced with
  CI color enabled and verified the same assertions after normalization.

## Follow-ups

- Unknown `retry:` keys inside the children of a `parallel` or `matrix` custom-command step are still ignored, because those children are decoded through the workflow-step hook that has no error channel.
- Flag-name conflicts with built-in flags still abort registration of the whole command set; they could use the same stub treatment if that is wanted.
