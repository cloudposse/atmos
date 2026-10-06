# Fix: Standalone script command lines behave like real CLIs

**Date:** 2026-10-04

## Summary

A field test of interpreter mode (`atmos ./tool.star`, `#!/usr/bin/env atmos`)
found that version and profile re-execution dropped the script, global flags
before the script failed, and parsing quirks silently produced wrong values:
env-bound lists split on whitespace, case-colliding flag names lost values,
integers accepted base prefixes, and `--flag false` set the flag to true. User
input mistakes looked like Starlark crashes, help omitted required flags,
choices, and env bindings, and `output = cli.command(...)` printed `null`. All of
these now behave like a well-formed CLI.

## Context

- `prepareStandaloneScript` replaced `os.Args` with `[atmos]` before
  `version.use`/`ATMOS_USE_VERSION` and `--profile` re-execution read it, so the
  child process ran without the script.
- `DetectFile` only considered the first argument, so any leading global flag
  disabled script detection.
- Viper split env-bound lists on whitespace and Go's base-0 integer parsing
  accepted `010` and `0x10`.
- Host parsing errors were returned through the Starlark engine, which wrapped
  them with `starlark execution failed` and a traceback.

## Changes

- `pkg/reexec` records the original command line. Version and profile
  re-execution forward the script and its arguments, and no longer strip a
  script's own `--chdir` or `--use-version` arguments.
- Leading Atmos global flags (`--chdir=dir`, `-C dir`, `--logs-level Debug`,
  `--`) are applied normally and the first remaining argument selects the script.
  Only `.star` names produce script errors; other paths fall through to command
  handling.
- A top-level `output` of `None` produces no output, so help and callbacks that
  return nothing print nothing.
- Env-bound `string_list` values parse like command-line values (comma-separated
  with CSV quoting) and are validated per element against `choices`.
- Flag and argument names must be unique ignoring case.
- Integer flags parse as base 10 from the command line and the environment.
- A `true`/`false` word after a boolean flag fails with a hint to use
  `--flag=false`.
- Unknown flags, invalid values, missing or extra arguments, missing required
  flags, and failed choices return `ErrScriptUsage` with the usage line, a
  `Run <script> --help` hint, exit code 2, and no traceback. A missing argument is
  named.
- Help marks required flags and lists choices and env bindings.
- Tracebacks show paths relative to the working directory, and log fields and
  hints name the script file instead of its absolute path.
- Docs: the Custom CLI Apps page documents these behaviors and file naming:
  the `.star` extension is optional when a script runs by path, with verified
  Vim (`bzl`), Emacs, GitHub Linguist, and VS Code file-type hints. The `cli.*`,
  `ctx.args`, and `output` reference pages and the standalone-scripts skill
  reference were updated.

## Validation

- `go build ./...` passed.
- `go test` passed for `./cmd/`, `./pkg/script/...`, `./pkg/reexec/...`,
  `./pkg/version/...`, `./pkg/flags/...`, `./pkg/auth/...`, and
  `go test ./tests -run 'TestCLICommands/starlark'`. The
  `starlark_example_declared_script_invalid_type` case now expects exit code 2.
- Changed unit expectations: tests that asserted `"null"` output now assert no
  output, and env list tests use comma-separated values.
- `./custom-gcl run --new-from-rev=HEAD` on the changed packages reported 0 issues.
- Manual before/after runs: `ATMOS_USE_VERSION=1.222.0 atmos ./v.star hello`
  forwards `./v.star hello` to the selected version; `atmos --chdir=interp
  hello.star` runs the script; `FT_TAGS=a,b` yields `["a","b"]`; `--count 010`
  yields 10; `--verbose false` fails with the `--verbose=false` hint; a bare
  `./flags.star` prints a usage error naming the missing argument with exit 2;
  `--help` shows `(required)`, `(one of: dev, prod)`, and `[env: FT_STAGE]`; a
  traceback shows `boom.star:3:6`.

## Follow-ups

None.
