# Fix: Honor color opt-outs during startup and logging

**Date:** 2026-10-09

## Summary

`--no-color` and nonempty `NO_COLOR` now override forced color and CI detection
throughout Atmos startup, logging, and UI output. A new boolean `logs.color`
setting controls log color independently of UI color. Its default, `true`, uses
color when available; existing force-color controls still apply. `false` removes
ANSI escapes from logs.

## Context

[Issue #3342](https://github.com/cloudposse/atmos/issues/3342) reported ANSI escapes
in the file produced by:

```shell
atmos --logs-level=debug --logs-file=/tmp/atmos.log --no-color version
```

Before editing code, the command was reproduced using a build of workspace commit
`8d406f13b6` on macOS, with an explicit repository fixture configuration and
telemetry/version checks disabled. With `CI=true GITHUB_ACTIONS=true`, 19 of 20
log lines contained ANSI escapes (290 escape bytes). `ATMOS_FORCE_COLOR=true`
produced 320 escape bytes despite the flag. The local non-CI case and the CI case
using `NO_COLOR=1` produced clean logs. Raw before/after artifacts are retained
locally under `.context/repro-3342/`.

Several startup paths unconditionally restored TrueColor after the opt-out.
Terminal detection also ranked forced color above `--no-color`. Early flag
parsing could consume `version` as the value of a bare `--no-color`. Separately,
Charm replaces its renderer when the log destination changes, losing an earlier
profile override. The version logo renderer checked `NO_COLOR` but bypassed the
CLI opt-out when color was forced.

## Changes

- Make the global opt-outs authoritative before configuration loading and during
  subsequent logger, terminal, error formatter, and logo initialization.
- Add `logs.color`, `--logs-color`, and `ATMOS_LOGS_COLOR` as boolean controls,
  defaulting to enabled. CLI overrides environment, which overrides configuration.
  Enabling color follows existing detection; forcing color uses `--force-color`
  or the existing force-color environment variables. There is no `auto` value.
- Keep logging color independent of UI color, retain policy across output changes
  and derived loggers (including the dedicated vendoring logger), and strip
  preformatted ANSI from disabled log output.
  Preserve terminal file capabilities, secret masking, and write errors.
- Update configuration defaults, generated JSON Schema, and CLI documentation.
  Regular log files retain existing automatic detection unless color is disabled.
  Logging controls do not alter child-process output.

## Validation

- Passed package tests for configuration, generated schema, logging, terminal,
  flags, UI, TUI utilities, and error formatting.
- Passed targeted root-command logging/color tests and compatibility flag-parser
  tests, including removal of `--logs-color` from arguments sent to Terraform.
  Vendoring tests and a derived-logger opt-out regression also passed.
- Passed `go test ./tests -run '^TestLogColorStartup$' -count=1 -timeout=5m`:
  all eight real-process scenarios passed with the final boolean controls.
- Rebuilt the binary and reran the reporter's command. Both GitHub Actions and
  forced-color cases now contain zero escape bytes in logs, stdout, and stderr.
  The CI control retains color. `--logs-color=true` on a pipe stays plain;
  adding `--force-color` produces color. `--logs-color=false` keeps logs plain
  while the forced-color UI remains colored.
- Passed logger and early color-parser tests with the race detector.
- Passed `go tool mage lint:changed` (the repository's changed-code lint target),
  reporting zero issues, and `git diff --check`.
- Passed the website production build; it reported existing unrelated broken
  anchors and webpack/MDX warnings. The final boolean-control documentation
  rebuild also passed.
- Passed the fix-log format validator for this record.

## Follow-ups

None.
