# Fix: Standalone script flag isolation, profile forwarding, and usage errors

**Date:** 2026-10-07

## Summary

Field testing of standalone scripts (`atmos ./tool.star`) found that a script's own flags could change Atmos behavior, that `--profile` and `--identity` did not reach nested `atmos.*` calls, and that several error and help messages were wrong or confusing. This fix isolates script flags from Atmos color setup, forwards the profile and identity selection to nested Atmos commands, turns the space-form flag mistakes into named usage errors, and polishes help and error text.

## Context

- `atmos ./capacity.star api --force-color` enabled Atmos color and set `CLICOLOR_FORCE=1` in the child environment, although the flag follows the script path and belongs to the script. Early color setup scanned all of `os.Args` before the script arguments were split off.
- `atmos --profile=dev ./tool.star` left nested `atmos.*` calls on the default profile. The profile existed only in the parent's memory; `ATMOS_PROFILE=dev` in the environment did propagate. The same gap applied to `type: atmos` workflow and custom command steps and to `--identity=`.
- `atmos --profile dev ./tool.star` and `atmos --identity dev ./tool.star` cannot be detected as script invocations, because both flags take a value only in the `--flag=value` form. The user got `Unknown command` or `profile not found`. An unknown flag before a script path (`--bogus ./tool.star`) was reported as an unknown command.
- A `validate` callback that returned `False` exited 1 with a traceback instead of a usage error.
- Help never showed `cli.arg(description=...)`, named the command `tool.star`, and labeled lists `strings`.
- Error text repeated prefixes (`Error in fail: fail:`, `Error in cli.command: cli.command:`, `invalid value for flag: invalid value ...`), environment-sourced flag errors did not name the variable, out-of-range integers were reported as "not a base-10 integer", and a timeout read `context deadline exceeded`.

## Changes

- `cmd/standalone_script.go`: `colorScanArgs` limits the early `--force-color` scan to the leading global flags of a script invocation; other invocations are still scanned in full. `standaloneSelectionEnv` adds the active profiles and an explicit `--identity` to the script's process environment. `prepareStandaloneScript` now reports leading-flag mistakes through `script.DiagnoseLeadingFlags`.
- `cmd/root.go`: `setupColorProfileFromEnvWithArgs` scans `colorScanArgs(args)` instead of all arguments.
- `pkg/script/leading_flags.go`: `DiagnoseLeadingFlags` returns a usage error (exit 2, with a hint) for `--profile dev ./x.star` style input and for an unknown flag before a script path.
- `pkg/script/selection_env.go`: `SelectionEnv` builds the `ATMOS_PROFILE` and `ATMOS_IDENTITY` entries. The interactive-selection sentinels are never forwarded.
- `pkg/runner/step/atmos.go`: `type: atmos` steps forward the same selection; the step's own `env` still wins.
- `pkg/script/command.go`, `pkg/script/starlark/stdlib/cli/module.go`, `pkg/script/starlark/engine.go`, `pkg/script/starlark/errors.go`, `cmd/standalone_command.go`: `validate` returning `False` is a usage error (exit 2, usage line, `--help` hint, no traceback) with the message `input validation failed`. `script.UsageFailure` replaces the private usage wrapper, and `CommandInput.Usage` lets the host present the error. `fail()` inside `validate` is unchanged.
- `pkg/script/standalone/usage.go`, `cmd/standalone_command.go`: help gains an `Arguments:` section (`<name>` for required, `[name]` for optional), the default command name drops the script extension, and list flags are labeled `list`.
- `pkg/script/standalone/input.go`, `cmd/standalone_command.go`: value errors carry only the reason, so the output no longer repeats `invalid value for flag:`. Environment-sourced errors read `invalid value "qa" from FT_STAGE for flag --stage: ...`, including choice checks. Out-of-range integers say `out of range for a 64-bit integer`.
- `pkg/script/starlark/errors.go`: the repeated builtin name in the final traceback line is dropped.
- `pkg/script/starlark/process.go`, `result_data.go`, `atmos.go`: a timeout reads `exec.run: command "sleep 5" timed out after 1s`; a string `argv` gets the hint `Pass a list, e.g. exec.run(["ls", "-l"]).`; `result.data` on non-JSON stdout hints `Read result.stdout, or request JSON output from the command.`, and only the `atmos.*` wrappers mention `--format=json`.
- `pkg/config/load.go`: `settings.metrics.enabled` binds to `ATMOS_SETTINGS_METRICS_ENABLED`.
- `tests/test-cases/starlark-examples.yaml`: the help expectation changes from `capacity.star <service>` to `capacity <service>` because the default name drops the extension.

## Validation

- `go build ./...` succeeds.
- `go test ./cmd/ -short -count=1`, `go test ./pkg/script/... -count=1`, and `go test ./pkg/config/ -short -count=1` pass.
- `go test ./tests -run 'TestCLICommands/(starlark|.*star)' -count=1` passes, including the new cases in `tests/test-cases/starlark-standalone.yaml`. No snapshots needed regeneration.
- New unit tests cover the color scan, selection forwarding, leading-flag diagnostics, validate usage errors, help text, environment error naming, timeout and argv messages, and the metrics environment binding.
- `go test ./pkg/runner/step` passes except six PTY-based cast session tests that time out waiting for terminal output in this headless environment; they fail while waiting for PTY output, before any `atmos` step runs, so they appear to be environment-related.
- `./custom-gcl run --new-from-rev=HEAD` reports no findings in the changed files.

## Follow-ups

- `--chdir` and `--logs-level` are not forwarded to nested `atmos.*` calls. The working directory is already changed, and forwarding the log level was out of scope for this fix.
