# Fix: `!literal` step fields and reliable custom-command inputs

**Date:** 2026-10-04

## Summary

A field test of Atmos Automation Language in custom commands and workflows found
step definitions that silently changed user data. Ambient environment variables
were rendered as templates, so a value containing braces broke every script step
or reached child processes rewritten. Arguments containing commas were split.
`type: int` flags vanished from `ctx.flags`. Optional arguments without a default
failed. `timeout:` and unknown `output:` modes were silently ignored. Starlark
source containing `{{` could not be written at all, because `atmos.yaml` rejected
`!literal` and the runner rendered script bodies even when workflows accepted it.
All of these now behave as declared.

## Context

- Atmos renders step fields as Go templates at run time. Custom commands rendered
  the entire process environment, not only declared `env:` values, and escaping
  `{{` fails because custom commands render up to three passes.
- `cmd/cmd_utils.go` passed resolved arguments through a comma-joined command
  annotation, and the custom-command flag registration only handled string and
  bool types.
- Parallel and matrix children rendered with plain `text/template`, so template
  functions that worked in a sequential step failed in a child.
- Template errors named an internal template (`step-pass-1`) instead of the step,
  field, or included file.

## Changes

- `!literal` is accepted in `atmos.yaml` and listed in the supported-tags error. A
  step field tagged `!literal` (`script`, `command`, `interpreter`,
  `working_directory`, or an individual `env` value) is recorded by the loader in
  an internal `literal_fields` marker and never template-rendered: in custom
  commands, workflows, hooks, and parallel and matrix children. Markers are only
  recorded on step mappings, so `vars`, `settings`, and `env` maps are unchanged.
- Only declared environment values are rendered; the ambient process environment
  passes through verbatim (`pkg/runner/step/ambient_env.go`), including for
  workflow `test` steps.
- Custom-command arguments round-trip losslessly as JSON
  (`pkg/customcommand/arguments.go`). Optional arguments without a default resolve
  to an empty string.
- Custom-command flags support `type: int` with typed values in `ctx.flags` and
  templates; unknown types and unparsable int defaults fail at registration. The
  generated atmos.yaml schema lists the supported types.
- `timeout:` is enforced for script, shell, atmos, and `tflint` steps, including
  workflow container paths, with `ErrStepTimeout`; `exec` steps reject it at
  validation. Unknown `output:` modes
  fail validation with the list of valid modes.
- Parallel and matrix children render with the parent's renderer and pass count.
- Template failures name the step, field, and included source file, with a hint
  pointing to `!literal` when the body contains `{{`.
- Container steps share the command-step output defaults: raw output and no
  labels unless `show.labels` is set.
- Docs: the script step field table, environment and template behavior, exact
  `output` error text, freezing, the single-task prefix rule, `!literal` usage,
  integer flags, optional arguments, and a consistent `replicas.star` example.
  Agent skills were updated to match.

## Validation

- `go build ./...` passed.
- `go test` passed for `./cmd/`, `./pkg/customcommand/...`, `./pkg/config/...`,
  `./pkg/utils/...`, `./pkg/schema/...`, `./pkg/workflow/...`, `./pkg/hooks/...`,
  `./pkg/runner/step/` (excluding six cast session-mode tests that time out in
  this environment and fail identically on a clean checkout of `HEAD`), and
  `go test ./tests -run 'TestCLICommands/(starlark|custom|workflow)'`.
- `./custom-gcl run --new-from-rev=HEAD` on the changed packages reported 0 issues.
- Manual before/after runs against the field-test fixture: `FOO='{{bad'` reaches
  the child verbatim; `dump "api,v2" us-east-1` yields
  `{"region":"us-east-1","service":"api,v2"}`; `--count int` appears in help and
  `ctx.flags`; `timeout: 1s` stops a 4 second step; `output: capture` fails
  validation; `{{ "x" | upper }}` renders in a parallel child; `!literal` scripts
  print `{{ x }}` in sequential, parallel, and workflow steps.
- `cd website && npm run build` passed.
- The field-test report claimed dict outputs reached later steps as `{n:3}`.
  That was wrong: `.steps.<name>.value` already renders JSON text, and the shell
  stripped the quotes in the test. A test now pins the JSON-text contract and the
  script reference documents quoting.

## Follow-ups

None.
