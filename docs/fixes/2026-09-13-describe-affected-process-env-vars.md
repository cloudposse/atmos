# Fix: `describe affected` now honors `ATMOS_PROCESS_TEMPLATES` / `ATMOS_PROCESS_FUNCTIONS`

**Date:** 2026-09-13

## Summary

`atmos describe affected` ignored the `ATMOS_PROCESS_TEMPLATES` and `ATMOS_PROCESS_FUNCTIONS`
environment variables. The `--process-templates` and `--process-functions` flags worked, but
their env-var equivalents — honored by the `list`, `terraform`, and `terraform generate` command
families — silently had no effect on `describe affected`, so operators had to pass the CLI flags
explicitly (e.g. in CI). This wires the two env vars to the flags with correct
CLI > env > default precedence.

## Context

Every command in the `list`/`terraform` families binds these env vars through `pkg/flags`
(`flags.WithEnvVars(...)`). `describe affected` did not: its `--process-templates` /
`--process-functions` flags were registered as raw Cobra `PersistentFlags()` and read back in
`exec.SetDescribeAffectedFlagValueInCliArgs`, which only copies a flag's value into
`DescribeAffectedCmdArgs` when `cmd.Flags().Changed(name)` is true. Setting an env var never
flips a Cobra flag's `Changed()` bit, so the value was dropped on the floor.

This surfaced while speeding up an Atmos Pro "affected" CI job, where `--process-functions=false`
is the correct way to skip credential-backed YAML functions (`!store`, `!terraform.state`,
`!terraform.output`) during affected detection — a Git-config diff that never needs their
resolved values. Being unable to set that via the environment (the natural CI knob) was the gap.

The command already solved this exact class of problem for `--error-mode` via a minimal
`flags.StandardParser` plus a resolve step (`cmd/describe_error_mode_flag.go`); this fix mirrors
that pattern rather than migrating the whole (pre-unified-flag-parsing) command.

## Changes

- `cmd/describe_affected_process_flags.go` (new): `newDescribeAffectedProcessFlagsParser()` — a
  minimal `StandardParser` that registers `--process-templates` / `--process-functions` and binds
  them to `ATMOS_PROCESS_TEMPLATES` / `ATMOS_PROCESS_FUNCTIONS` (namespaced under the `describe`
  Viper prefix, like the error-mode parser, to avoid colliding with the `list` family's bare
  keys on the shared global Viper). `resolveDescribeAffectedProcessFlags()` writes the env-sourced
  value back onto the Cobra flag (via `Flags().Set`, which marks it `Changed`) so the legacy
  reader picks it up — only when the flag wasn't set on the CLI (CLI wins) and the resolved value
  differs from the flag default. It compares values rather than using `viper.IsSet`, because
  `SetDefault` makes `IsSet` report true even with no env var set.
- `cmd/describe_affected.go`: removed the two raw `PersistentFlags().Bool(...)` registrations for
  these flags (now owned by the parser), added the parser var + its `RegisterPersistentFlags` /
  `BindToViper` wiring in `init()`, and called `resolveDescribeAffectedProcessFlags` in `RunE`
  right after the error-mode resolve and before the args are parsed.
- `cmd/describe_affected_process_flags_test.go` (new): table-driven unit tests for the resolver
  (no-op default, each env var independently, CLI-wins-over-env precedence, direct Viper key, the
  unregistered-flag defensive no-op) plus an end-to-end regression test asserting
  `ATMOS_PROCESS_FUNCTIONS=false` reaches `DescribeAffectedCmdArgs.ProcessYamlFunctions` through
  `exec.SetDescribeAffectedFlagValueInCliArgs`.

## Validation

- `go build ./...` — clean.
- `go test ./cmd/ -run 'ProcessFlags|Describe'` — pass (new tests plus the existing describe /
  error-mode / skip-auth suites, which the change leaves green).
- `go vet ./cmd/` — clean.
- New-file coverage: `newDescribeAffectedProcessFlagsParser` 100%, `resolveDescribeAffectedProcessFlags`
  80% — the uncovered 20% are three defensive `return err` branches (`BindToViper`, `GetBool`,
  `Set`) that are unreachable for this flag config, documented in the test file (same rationale as
  `describe_error_mode_flag_test.go`).
- `./custom-gcl run --new-from-rev=origin/main --config=.golangci.yml ./cmd/` — 0 issues
  (forbidigo included: no direct `viper.BindEnv`/`BindPFlag`; all binding goes through `pkg/flags`).

## Follow-ups

None.
