# Fix: two config-loading DX gaps that hid useful, already-computed detail

**Date:** 2026-08-07

## Summary

Two related default-log-level / non-verbose visibility gaps found during a
hands-on field test, both in config-loading error paths:

- A YAML syntax error in a custom-commands file pulled in via atmos.yaml's
  `import:` was silently swallowed at the default log level. Atmos behaved
  as if the file had zero commands, surfacing only a generic
  `Unknown command X for atmos` — the real cause (`failed to load local
  config path=... error="While parsing config: ..."`) was visible only
  with `--logs-level=Trace`.
- Two container-step validation errors hid detail they had already
  computed: omitting `with:` on a step that requires a field (e.g.
  `run.image`) produced only `required field missing for step` by
  default — the field/step/type were already attached via `.WithContext()`
  but that's verbose-only. And the `run.pull` value-validation error
  listed the valid options but never echoed the actual invalid value the
  user typed.

## Context

### Gap A — `pkg/config/adapters/local_adapter.go`

`LocalAdapter.Resolve` reads each locally-resolved import path with a
throwaway `viper.New()` instance. When `v.ReadInConfig()` fails (e.g.
malformed YAML), the adapter logged at `log.Debug` and silently `continue`d
— the file (and anything it would have defined, such as custom commands)
is dropped with no default-visible signal. Nothing else in the import path
re-surfaces the failure; the downstream symptom is simply "the command
that file defines doesn't exist."

Fixed by adding a `ui.Warningf(...)` alongside the existing `log.Debug` —
matching the convention already used elsewhere in `pkg/config` (see
`warnVersionConstraint` in `pkg/config/utils.go`, which pairs a `log`-level
detail with a `ui.Warning` for anything a human actually needs to notice).
The warning names the actual file path and the actual parse error, and
notes that commands/configuration in the file won't be available until
it's fixed.

- Before (nothing printed by default; only `--logs-level=Trace`):
  ```text
  DEBU failed to load local config path=.../container-commands.yaml error="While parsing config: yaml: line 3: mapping values are not allowed in this context"
  ```
- After (printed by default, via `ui.Warningf`, in addition to the existing `log.Debug`):
  ```text
  ⚠ Skipping config file `.../container-commands.yaml`: While parsing config: yaml: line 3: mapping values are not allowed in this context
    Commands or configuration defined in this file will not be available until the error is fixed
  ```

### Gap B — `pkg/runner/step/handler_base.go` and `pkg/runner/step/container.go`

`BaseHandler.ValidateRequired` (shared by every step handler — container,
shell, junit, tflint, input, etc.) built its `ErrStepFieldRequired` error
with only `.WithContext("step", ...)` / `.WithContext("type", ...)` /
`.WithContext("field", ...)` — all verbose-only per `errors/builder.go`.
Every *other* step handler's own inline validation error (emulator.go,
tflint.go, junit.go) already paired its context with a
`.WithExplanation(...)`; `ValidateRequired` was the one shared call site
that computed the same detail but didn't say it out loud. Fixed by adding
a `.WithExplanationf(...)` that names the step, type, and field.

`invalidContainerField` (used for all container `run`/`build`/`push`/
`inspect` field-value validation, e.g. `run.pull`) already had a
`.WithExplanation(explanation)` with the valid options, but never included
the actual value the user passed. Fixed by folding the value into the
explanation via ``.WithExplanationf("%s (got `%s`)", explanation, value)``.
The `.WithContext("value", value)` is left in place for the verbose/Sentry
structured-context path.

- `run.image` omitted, before (default/non-verbose):
  ```text
  **Error:** required field missing for step
  ```
- `run.image` omitted, after:
  ```text
  **Error:** required field missing for step

  Step `run` (type `container`) is missing required field `run.image`
  ```
- `run.pull: sometimes`, before (default/non-verbose):
  ```text
  **Error:** required field missing for step

  Pull policy must be `missing`, `always`, `never`, or empty
  ```
- `run.pull: sometimes`, after:
  ```text
  **Error:** required field missing for step

  Pull policy must be `missing`, `always`, `never`, or empty (got `sometimes`)
  ```

## Validation

- New/updated tests, confirmed failing against the old (deficient) message
  and passing against the fixed message:
  - `TestLocalAdapter_ReadConfigError` (`pkg/config/adapters/adapters_test.go`)
    — asserts the UI-channel (stderr) warning names the file path and
    includes the parse error text.
  - `TestBaseHandler_ValidateRequired/default_(non-verbose)_message_names_the_step_and_field`
    (`pkg/runner/step/handler_base_test.go`) — asserts the default-formatted
    error contains the step name, type, and field.
  - `TestContainerHandlerValidateInvalidPullEchoesValue`
    (`pkg/runner/step/container_test.go`) — asserts the default-formatted
    error keeps the valid-options explanation and echoes the invalid value.
- `go build ./pkg/config/... ./pkg/runner/step/...` — clean.
- `gofumpt -l` on all changed files — clean (no output).
- `./custom-gcl run --new-from-rev=origin/main pkg/config/adapters/... pkg/runner/step/...` — 0 issues.
- `go test ./pkg/config/...` (full package tree, not just changed tests) — all pass.
- `go test ./pkg/runner/step/...` (full package, not just changed tests) — all pass.

## Follow-ups

None.
