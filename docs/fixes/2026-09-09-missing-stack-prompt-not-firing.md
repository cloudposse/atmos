# Fix: Missing-flag/positional-arg interactive prompts didn't actually fire for `describe component` or `aws cloudformation`

**Date:** 2026-09-09

## Summary

Three related reports about the "prompt for missing `--stack`/component" feature
(`pkg/flags/interactive.go`, `pkg/flags/standard.go`) were investigated:

1. **Reported:** the prompt is gated behind an undiscoverable `--interactive` opt-in that
  defaults to `false`. **Finding: not reproducible.** `global.NewFlags().Interactive` is
  already `true` (set in `pkg/flags/global/flags.go`, committed well before this
  investigation), and `isInteractive()` already returns `true` by default in a real TTY,
  non-CI context with no `--interactive` flag passed. No code change was needed for this
  part, but regression tests were added to lock the behavior in (see Validation).
2. **Reported:** `describe component` bypasses the prompt via
  `cmd.MarkPersistentFlagRequired("stack")`. **Confirmed and fixed.**
3. **Reported:** when the prompt does fire, it can show every stack in the org unfiltered,
  suspected to be a positional-arg-vs-flag prompt ordering bug. **Partially confirmed, and a
  second, more direct root cause was found and fixed too**, see Changes.

## Context

Live testing found that `atmos aws cloudformation apply <component>` and
`atmos describe component <component>`, run with no `--stack`, hard-failed with `stack is
required...` instead of showing the documented `? Choose a stack` interactive picker
(see `pkg/flags/interactive.go`'s `PromptForMissingRequired` doc example). Reproducing this
against the actual code (not just theory) required building the CLI and running it under
`ATMOS_FORCE_TTY=true` against `tests/fixtures/scenarios/aws-cloudformation-outputs`, plus
temporary debug instrumentation to observe `isInteractive()`'s inputs and the stack
completion function's return values at each step. That reproduction is what distinguished
the real root causes from the reported hypotheses below.

## Changes

### Bug 1 -- `--interactive` default (no code change; hypothesis corrected)

Investigated `pkg/flags/global/flags.go`'s `NewFlags()` (`Interactive: true`, line ~104),
`pkg/flags/global_builder.go`'s flag registration (`WithBoolFlag("interactive", "",
defaults.Interactive, ...)`), and `cmd/root.go`'s `init()` wiring
(`globalParser.RegisterPersistentFlags(RootCmd)` + `BindToViper`). Built the CLI and ran it
with debug prints inside `isInteractive()`: with no `--interactive` flag passed, in a forced
TTY, non-CI shell, `viper.GetBool("interactive")` was already `true`. Grepped every other
consumer of `viper.GetBool("interactive")`/`defaults.Interactive`
(`cmd/init/init.go`, `cmd/scaffold/scaffold.go`, `cmd/devcontainer/exec.go`,
`pkg/auth/manager.go`) -- each defines its own independent `interactive` flag or reads the
same global one consistently; nothing shadows or overrides the default to `false`. No fix
needed. Regression tests added in `pkg/flags/global/flags_test.go`,
`pkg/flags/global_builder_test.go`, and `pkg/flags/interactive_test.go` to catch a future
regression of this default.

### Bug 2 -- `describe component` bypassed the prompt via Cobra's native required-flag check

`cmd/describe_component.go`'s `init()` called
`describeComponentCmd.MarkPersistentFlagRequired("stack")`, which is Cobra's own
required-flag validation and runs **before** `RunE` -- so a missing `--stack` always
produced Cobra's generic `Error: required flag(s) "stack" not set` and never gave the
interactive-prompt code a chance to run, regardless of TTY/interactive state.

- Removed the `MarkPersistentFlagRequired("stack")` call (`cmd/describe_component.go`,
  `init()`).
- Added `resolveDescribeComponentStack(cmd, args)`, called at the top of
  `getRunnableDescribeComponentCmd`'s returned closure (before `parseDescribeComponentFlags`
  reads `--stack`). It calls `flags.PromptForMissingRequired("stack", "Choose a stack",
  describeComponentStackCompletion, cmd, args)` and, if a value was selected, writes it back
  onto `cmd`'s own `"stack"` flag (`stackFlag.Value.Set(selected)`), mirroring the
  write-back pattern in `pkg/flags/standard.go`'s `promptForSingleMissingFlag`.
- Introduced a package-level var `describeComponentStackCompletion =
  StackFlagCompletion` (the existing `cmd/cmd_utils.go` completion function, already used
  for `--stack` shell completion via `AddStackCompletion`) so tests can substitute a fake
  completion function.
- Moved the "is stack still missing?" check (`if f.stack == "" { return
  errUtils.ErrMissingStack }`) to run after the prompt attempt and after the existing
  flag-shape/error-mode validations, but before any real work (path resolution, auth
  manager creation, executing the describe). This replaces Cobra's generic error message
  with the same `errUtils.ErrMissingStack` used by every other stack-consuming command
  (`cmd/terraform/shell.go`, `internal/exec/terraform.go`, etc).
- Did **not** touch the `component` positional arg's `cobra.ExactArgs(1)` requirement --
  out of scope for this fix, which targeted `--stack` specifically per the reported example
  (`$ atmos describe component vpc` -> `? Choose a stack`).

### Bug 3 -- unfiltered stack list, two distinct confirmed causes

**3a. `StackFlagCompletion`'s component filter only recognized `terraform` components.**
This is the actual, directly reproduced cause of the exact scenario in the report
(`atmos aws cloudformation apply <component>` with the component given, `--stack` missing):
`cmd/terraform/shared/prompt.go`'s `stackContainsComponent` (used by
`listStacksForComponent`, used by `StackFlagCompletion`, reused as-is by
`cmd/aws/cloudformation/cloudformation.go` and `cmd/terraform/backend/*.go` /
`cmd/aws/cloudformation/backend/*.go`) hardcoded `components["terraform"]` as the only
component-type section it checked. For an `aws/cloudformation` component (or any
non-terraform component reusing this shared completion function), the filter always found
zero matches -- **not** "every stack", but an empty option list -- which silently skipped
the prompt entirely (`PromptForMissingRequired`'s `len(options) == 0` branch) and fell
through to the hard `stack is required` error. Confirmed via a debug build against
`tests/fixtures/scenarios/aws-cloudformation-outputs`: before the fix,
`aws cfn apply vpc` (no `--stack`) logged `options: []`; after the fix, it logged
`options: [test]` (the one stack that actually defines the `vpc` cloudformation
component) and proceeded to the interactive picker.

  - Fixed `stackContainsComponent` (`cmd/terraform/shared/prompt.go`) to check the
    component name across **all** component-type sections under `"components"`, not just
    `"terraform"`.
  - The identical bug existed in `pkg/list/list_stacks.go`'s `FilterAndListStacks` (used by
    `cmd/cmd_utils.go`'s `StackFlagCompletion` -- the one `describe_component.go` uses via
    `AddStackCompletion`/`describeComponentStackCompletion` -- and by
    `cmd/container/completions.go` and `cmd/ansible/completions.go`, whose components are
    never `"terraform"`). Fixed the same way, extracted into a small
    `stackContainsAnyTypeComponent` helper.

**3b. Required-flag prompts ran before positional-arg prompts.** Confirmed via a unit test
against the real `pkg/flags` pipeline: when both a required flag (e.g. `--stack`) and a
required positional arg (e.g. the component) are missing, `handleInteractivePrompts` used to
run Use Case 1 (missing required flags) before Use Case 3 (missing positional args), so the
flag's completion function always received an **empty** `args` slice -- the positional arg
hadn't been resolved yet -- causing `StackFlagCompletion` to fall back to listing every
stack in the org, unfiltered. (When the component **was** already supplied on the command
line, this ordering bug did not apply: `result.PositionalArgs` is populated during flag
parsing, step 1 of `Parse()`, well before any prompting -- confirmed by a passing unit test
with pre-supplied positional args, see Validation.)

  - Swapped the order in `pkg/flags/standard.go`'s `handleInteractivePrompts`: positional-arg
    prompts (Use Case 3) now run before required-flag prompts (Use Case 1), so a flag's
    completion function benefits from an already-resolved positional arg the majority of the
    time a command combines both.

## Validation

- `go build ./...` -- succeeds.
- `go test ./pkg/flags/... ./pkg/list/... ./cmd/terraform/... ./cmd/...` -- all pass
  (including the full `./cmd/...` suite, since this is a cross-cutting flag-handling
  change).
- `GOTOOLCHAIN=go1.26.6 ./custom-gcl run --new-from-rev=origin/main ./cmd/... ./pkg/flags/...
  ./pkg/list/...` -- 0 issues (two `godot` findings on new comments were fixed during the
  pass).
- Manual end-to-end verification with a debug build against
  `tests/fixtures/scenarios/aws-cloudformation-outputs`:
  - `ATMOS_FORCE_TTY=true atmos aws cloudformation apply vpc` (no `--stack`): before the fix,
    hard-failed immediately with `stack is required...`; after the fix, correctly reaches
    the interactive picker with a filtered one-stack option list (fails only on
    `huh: could not open a new TTY`, expected in this headless sandbox).
  - `ATMOS_FORCE_TTY=true atmos describe component vpc` (no `--stack`): before the fix,
    Cobra's own `required flag(s) "stack" not set`; after the fix, reaches the same
    filtered interactive picker.
  - Without `ATMOS_FORCE_TTY` (non-interactive): both commands still fail cleanly with
    `stack is required; specify it on the command line using the flag --stack <stack>
    (shorthand -s)` -- no regression to the non-interactive fallback.
  - `atmos describe component vpc --stack test` (explicit stack, regression check):
    succeeds and prints the expected component config.
- New/updated tests:
  - `pkg/flags/global/flags_test.go` -- `TestNewFlags` now asserts `Interactive` defaults to
    `true`.
  - `pkg/flags/global_builder_test.go` -- asserts the registered `interactive` pflag's
    `DefValue` is `"true"`.
  - `pkg/flags/interactive_test.go` --
    `TestIsInteractive_DefaultsTrueWithoutExplicitFlag` proves `isInteractive()` is `true`
    by default in a real TTY/non-CI context using only the registered flag default (no
    `--interactive` passed), and still `false` in CI or without a TTY.
  - `cmd/describe_component_test.go` --
    `TestDescribeComponentCmd_StackNotCobraRequired` asserts the `stack` flag no longer
    carries Cobra's required-flag annotation;
    `TestGetRunnableDescribeComponentCmd_MissingStackTriggersPrompt` asserts a missing
    `--stack` reaches and calls the prompt's completion function (with the component arg for
    filtering) instead of a hard Cobra error, and still fails with `errUtils.ErrMissingStack`
    when no stack is selected.
  - `cmd/terraform/shared/prompt_test.go` -- new cases in `TestStackContainsComponent` and
    `TestListStacksForComponent` proving a component under a non-`"terraform"` section
    (e.g. `"aws/cloudformation"`) is now correctly matched.
  - `pkg/list/list_stacks_test.go` --
    `TestFilterAndListStacks_NonTerraformComponentType` proving the same for
    `FilterAndListStacks`.
  - `pkg/flags/standard_test.go` --
    `TestStandardFlagParser_HandleInteractivePrompts_PositionalArgsBeforeFlagPrompts` proves
    the positional-arg prompt now runs before the required-flag prompt;
    `TestStandardFlagParser_PromptForSingleMissingFlag_ReceivesPositionalArgs` proves a
    required flag's completion function receives an already-CLI-supplied positional arg.

## Follow-ups

None. The `component` positional arg's `cobra.ExactArgs(1)` requirement on
`describe component` (a similar but distinct hard-Cobra-validation pattern) was
deliberately left untouched -- out of scope for this fix, which targeted the `--stack`
flag specifically per the reported example.
