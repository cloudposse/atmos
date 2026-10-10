# Fix: Log color field-test regressions and bool flag space values

**Date:** 2026-10-09

## Summary

A field test of the color opt-out work in PR #3345 found two regressions and two
usability gaps. `CLICOLOR=1` no longer disables color. `--no-color=false` now
overrides `ATMOS_NO_COLOR` for logs as well as UI output. An invalid
`logs.color` value now names its source and fails consistently. Boolean flags
now accept a space-separated `true` or `false` value, such as
`--logs-color false`.

## Context

The field test compared a build of this branch with a build of `origin/main`,
using fixtures outside the repository and a scrubbed environment.

- `CLICOLOR` was bound to the `no-color` flag in `pkg/flags/global_builder.go`,
  but not in `global_registry.go`. `CLICOLOR=1` means "color is supported", and
  macOS users commonly export it. After `--no-color` became authoritative,
  `CLICOLOR=1` also overrode `--force-color` and `CLICOLOR_FORCE=1`, and dropped
  the colored `version` logo in a real terminal. On `origin/main`, forced color
  still won.
- With `ATMOS_FORCE_COLOR=true ATMOS_NO_COLOR=true --no-color=false`, UI output
  stayed colored but every log line was plain. `Execute()` configures the logger
  before Cobra parses flags, so `viper.GetBool("no-color")` reflected only the
  environment. The same override through `settings.terminal.no_color` worked.
  The logger also could not restore automatic color detection after an opt-out
  without an explicit profile.
- `ATMOS_LOGS_COLOR=off` failed every command with "logs.color must be true or
  false" without naming the variable. `atmos version` printed a raw warning and
  ignored atmos.yaml. A non-boolean `logs.color` in atmos.yaml was silently
  coerced, because Viper casts values to the type of their default.
- The `--flag value` rewrite in `pkg/flags/preprocess` only covers string flags
  with an optional value. `atmos --logs-color false version` failed with
  "Unknown command false", and `atmos terraform plan ... --logs-color false`
  forwarded `false` to Terraform.
- CI acceptance tests on PR #3345 exposed three more problems. In CI, UI
  initialization pushes a detected terminal profile into the logger, and the
  logger reapplied it when tests redirected logs to a buffer. That broke the
  `TestPosthogLogger_*` and `TestLogDiagnostic_*` assertions. The new
  `--logs-color` flag and `logs.color` key changed 14 help and
  `describe config` snapshots. The new real-process tests also built the
  shared test binary before `TestTerraformPlanCI*` ran. Those tests built
  their own runner into the same per-process directory and their cleanup
  deleted it, so later toolchain tests failed with `no such file or directory`.

## Changes

- Remove `CLICOLOR` from the `no-color` environment binding. `CLICOLOR=0` still
  disables color unless forced, through the existing terminal detection.
- Add `env.ResolveNoColor`, which lets an explicit `--no-color` flag beat
  `ATMOS_NO_COLOR`, keeps nonempty `NO_COLOR` absolute, and uses the bound value
  only when no flag is present. Use it for the startup opt-out, the error
  formatter, the TUI print helpers, and the terminal configuration.
- Let the logger restore automatic color detection after an opt-out is lifted.
- Add `ErrInvalidLogsColor`. The error names the winning source
  (`--logs-color`, `ATMOS_LOGS_COLOR`, or `logs.color`) and gives a hint. The
  raw `logs.color` value is validated before Viper coerces it. An invalid value
  is fatal for every command, including `version` and help, matching an invalid
  log level.
- Add `env.NormalizeBoolFlagValues` and a `BoolValuePreprocessor`. They rewrite
  `--flag true|false` to `--flag=true|false` only for registered boolean flags
  and only for exact `true` or `false` literals. A bare flag before a command or
  component, such as `--logs-color version`, is unchanged. The early color
  parser, root flag parser, version-command detection, and component argument
  parser use the same rule, so Terraform no longer receives the value.
- Drop any explicit log color profile when the log destination changes. The
  new renderer uses automatic detection, and the opt-out policy still applies.
  `SetupLogger` applies forced color after it sets the log output.
- Make `TestTerraformPlanCI*` reuse the shared test runner instead of deleting
  the per-process binary directory, and regenerate the affected snapshots with
  `-regenerate-snapshots`.
- Update the terminal color precedence documentation and the `atmos-settings`
  agent skill, which still listed `CLICOLOR` as an opt-out.

## Validation

- Reran every field-test reproduction against a rebuilt binary. `CLICOLOR=1`
  with forced color and the `ATMOS_NO_COLOR` override now produce colored logs
  and UI output. `NO_COLOR` still wins over `--no-color=false`.
  `--logs-color false` works for `version` and `terraform plan`. Invalid
  `ATMOS_LOGS_COLOR` values and an invalid atmos.yaml `logs.color` now fail
  with a source-specific error and hint.
- Passed `go test -short` for `./cmd/...`, `./errors/...`, `./internal/tui/...`,
  `./internal/exec/...`, `./pkg/config/...`, `./pkg/flags/...`,
  `./pkg/logger/...`, `./pkg/terminal/...`, `./pkg/schema/...`,
  `./pkg/datafetcher/...`, and `./pkg/telemetry/...`: 83 packages.
- Passed `go test ./tests -run '^TestLogColor' -count=1`, which runs the real
  binary for `TestLogColorStartup`, including new CLICOLOR and override cases,
  and the new `TestLogColorSpace`. After the final lint edits, also reran the
  targeted `cmd`, `internal/exec`, and `pkg/config` tests, which passed.
- Reproduced the logger test failures locally with `CI=true
  GITHUB_ACTIONS=true` and confirmed `./pkg/logger/...`, `./pkg/telemetry/...`,
  `./pkg/terraform/ui/...`, and `./pkg/ui/...` pass with and without those
  variables. Passed `go test ./tests -run '^(TestLogColor|TestTerraformPlanCI)'`
  and a combined run of the log color, plan CI, and toolchain tests in one
  process. Regenerated 14 `TestCLICommands` snapshots; the diff adds only the
  `--logs-color` help entry and the `logs.color` value.
- Passed `go tool mage lint:changed` with zero issues, `gofumpt -l` on changed
  Go files, and `git diff --check`.
- Passed the website production build (`npm run build`) after the documentation
  changes.

## Follow-ups

None.
