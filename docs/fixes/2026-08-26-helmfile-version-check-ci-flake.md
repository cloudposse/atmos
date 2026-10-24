# Fix: `TestExecuteHelmfile_Version` flaky on Windows CI due to helmfile's own update check

**Date:** 2026-08-26

## Summary

`Acceptance Tests (windows, shard 7/10)` failed with `helmfile_test.go:41: Failed to execute
command: subcommand exited with code 3`. `TestExecuteHelmfile_Version` shells out to the real
`helmfile` binary via `ExecuteHelmfile` with `SubCommand: "version"`. Independent of anything
Atmos does, `helmfile version` itself makes an outbound HTTP call to
`https://github.com/helmfile/helmfile/releases/latest` to check for a newer release before
printing its own version info; on the Windows runner that call hit `context deadline exceeded`
and helmfile exited non-zero instead of degrading gracefully. This is a CI-environment network
flake in a third-party binary, not an Atmos bug.

## Context

`helmfile version` (any output format) always performs this check, and the check's failure mode
is inconsistent: reached-but-found-an-update degrades to a printed notice with exit 0 (confirmed
locally); a network timeout instead makes the whole subcommand exit non-zero, which
`ExecuteShellCommand` correctly surfaces as a command failure. `internal/exec/helmfile.go`'s
`SubCommand == "version"` branch runs the binary directly (via `dependencies.ForComponent` +
`ExecuteShellCommand`), inheriting `os.Environ()` as the subprocess's base environment
(`internal/exec/shell_utils.go`'s `ExecuteShellCommand`), so any env var set in the CI job's
environment reaches the `helmfile` subprocess unchanged.

Verified `HELMFILE_UPGRADE_NOTICE_DISABLED=true` (any non-empty value) skips the network call
entirely rather than just suppressing the printed message: `helmfile version` completed in ~40ms
with the var set vs. ~200ms (real GitHub round trip) without it, and `go test -count=1` on
`TestExecuteHelmfile_Version` showed no "A new release is available" output with the var set.

## Changes

- `.github/workflows/test.yml`: added `HELMFILE_UPGRADE_NOTICE_DISABLED: "true"` to the workflow's
  global `env:` block (alongside the existing `HELMFILE_VERSION` etc.), so both the acceptance
  test suite's real `helmfile` subprocess calls and the separate `helmfile version` sanity-check
  step (used elsewhere in the same workflow) skip the flaky network call in CI. No application
  code changed -- real users running `atmos helmfile version` still see the upgrade notice; this
  only affects the CI job's own environment.

## Validation

- `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/test.yml'))"` -- valid YAML.
- `go test ./internal/exec/... -run TestExecuteHelmfile_Version -v -count=1` locally, both with
  and without `HELMFILE_UPGRADE_NOTICE_DISABLED=true` set -- passes in both cases; with the var
  set, no network-bound update-check output appears at all.

## Follow-ups

None.
