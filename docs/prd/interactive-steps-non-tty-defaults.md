# Interactive Steps in Non-TTY (CI) via Defaults

## Summary

Interactive workflow and custom-command step types (`choose`, `input`,
`confirm`, `filter`, `file`, `write`) previously failed with
`ErrStepTTYRequired` whenever they ran without a TTY (for example, in CI). This
change lets an interactive step fall back to its configured `default` value in a
non-TTY environment, so the same automation is interactive locally and
unattended in CI. When no `default` is set, the historical TTY-required error is
preserved.

## Motivation

Interactive steps make workflows and custom commands friendly to run by hand,
but they made those same definitions unrunnable in CI. Teams worked around this
by maintaining a second, prompt-free copy of the automation. Because a `default`
is already an explicit, author-provided non-interactive value, honoring it when
there is no TTY is safe and removes the need for duplicate definitions.

## Behavior

For an interactive step running without a TTY (neither stdin nor stdout is a
terminal; `--force-tty` still forces the interactive path):

- **`default` set** — the step returns the resolved default without prompting.
  - `choose` / `filter` (single): the default value verbatim.
  - `filter` (multiple / `limit > 1`): the default is split on commas into
    `values`.
  - `confirm`: `yes`/`true` → `"true"`, otherwise `"false"`.
  - `input` / `write`: the default text.
  - `file`: the default path.
- **`default` not set** — the step returns `ErrStepTTYRequired` (unchanged), so
  an unattended run never proceeds with an unintended value.

With a TTY, all step types prompt exactly as before (the default is still used
as the pre-selected/placeholder value where applicable).

## Design

Three cohesive parts:

1. **Non-TTY default fallback (`pkg/runner/step`).** A shared
   `BaseHandler.resolveInteractive(step)` helper returns whether to prompt (TTY
   present), use the default (no TTY + default set), or error (no TTY + no
   default). Each interactive handler calls it in place of the old `CheckTTY`
   gate. Because workflows, custom commands, hooks, and the task runner all
   execute steps through the same handler registry, this single change covers
   every consumer.

2. **YAML functions in workflow step fields (`internal/exec`).** Workflow
   manifests are parsed with `utils.UnmarshalYAML`, which stringifies custom
   tags (`default: !env FOO bar` becomes the literal string `"!env FOO bar"`).
   `resolveWorkflowStepFunctions` evaluates the context-free functions `!env`
   and `!exec` in each step's `default`, `prompt`, `placeholder`, and `options`
   before execution. Stack-dependent functions (`!terraform.output`, `!store`,
   `!secret`, ...) are intentionally left unevaluated (a workflow step has no
   component/stack context). Custom commands defined in `atmos.yaml` already
   resolve `!env` during config load.

3. **Step-variable templating in workflow commands (`internal/exec`).** Workflow
   `shell`/`atmos`/`exec` step commands are now resolved through the shared step
   `Variables` (`resolveWorkflowStepCommand`), so `{{ .steps.<name>.value }}`,
   `{{ .env.* }}`, and `{{ .flags.* }}` resolve — parity with custom command
   steps, which already did this. This lets a value captured by an interactive
   step (or its CI default) flow into later steps.

## Non-Goals

- Changing interactive behavior when a TTY is present.
- Evaluating stack-dependent YAML functions in workflow step fields.
- Adding new configuration fields. `default` already exists on the step schema;
  this change is runtime-only.

## Testing

- `pkg/runner/step`: unit tests assert each interactive handler returns its
  default (single, multi, confirm parsing, template-resolved default) in
  non-TTY, and still returns `ErrStepTTYRequired` without a default.
- `internal/exec`: unit tests for `!env`/`!exec` resolution and passthrough of
  plain/template/stack-dependent values, nested steps, and
  `{{ .steps.* }}`/`{{ .env.* }}` command resolution.
- End-to-end validated with the built binary for both a workflow and a custom
  command (with and without environment overrides).

## Known Follow-ups

- The manifest JSON schema (`pkg/datafetcher/schema/atmos/manifest/1.0.json`)
  uses `additionalProperties: false` for steps but predates several interactive
  fields (`options`, `default`, `placeholder`, `multiple`, `limit`, ...). This
  is a pre-existing gap (the fields work at runtime today) and is not addressed
  here; completing the interactive-step schema is tracked separately.
