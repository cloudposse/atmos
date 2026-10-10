# Fix: Honor `CLICOLOR` in the version logo

**Date:** 2026-10-10

## Summary

The version logo now honors `CLICOLOR=0` during automatic color detection.
The environment-variable reference documents `CLICOLOR` and explains that
nonempty `NO_COLOR` wins when the variables contradict each other.

## Context

Before changing implementation, a build of the current PR was exercised with
both redirected output and a real pseudo-terminal. `CLICOLOR=0` already kept
logs plain, including in GitHub Actions, but the version logo emitted 540 ANSI
escape bytes in a terminal. Its separate supportscolor detector does not read
`CLICOLOR`. `CLICOLOR=1` preserved terminal/CI color and did not force a pipe;
nonempty `NO_COLOR` correctly suppressed it.

Raw reproduction artifacts are retained locally in `.context/repro-clicolor/`.

## Changes

- Check the existing environment color policy before automatic logo rendering,
  for both the default output and specified-output paths. Explicit forcing
  continues to override `CLICOLOR=0`; global color opt-outs still win.
- Add a pseudo-terminal regression test with a positive color prerequisite and
  cases for zero, one, forcing, and conflicting `NO_COLOR`.
- Extend real-process startup tests to cover `CLICOLOR` in CI and pipes, empty
  `NO_COLOR`, and nonempty `NO_COLOR=0` overriding color requests.
- Document the environment variables, their precedence, and concrete conflict
  examples in the environment reference, global flags, terminal settings, and
  logging documentation.

## Validation

- Confirmed the new pseudo-terminal test failed on `CLICOLOR=0` before the fix.
- Passed tests for TUI utilities, terminal packages, and logging after the fix.
- Passed all 19 `TestLogColorStartup` subprocess scenarios, including the new
  environment-variable conflict cases.
- Rebuilt and reran the reproduction: `CLICOLOR=0` in a terminal now produces
  zero ANSI escape bytes in logs and UI (previously 540 in the UI). `CLICOLOR=1`
  still produces color in a terminal and GitHub Actions, stays plain on a pipe,
  and yields to nonempty `NO_COLOR`. Forcing still overrides `CLICOLOR=0`.
- Passed `go build ./...`, changed-code lint (zero issues), and `git diff --check`.
- Passed the website production build (with existing unrelated component/anchor
  warnings) and the fix-log format validator.

## Follow-ups

None.
