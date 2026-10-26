# Fix: CLI test harness accepts the documented `{not: pattern}` expectation form

**Date:** 2026-10-22

## Summary

`tests/test-cases/schema.json` documents a `{not: pattern}` object form for negated `stdout`,
`stderr`, `tty`, and `file_contains` expectations, but the harness only decoded the `!not` YAML
tag and failed to load any suite that used the documented form with `unsupported tag "!!map"`.
The decoder now accepts both.

## Context

While wiring CLI test cases for the CloudFormation defaults fixture, a negative assertion that a
stack-root `region` does not leak into a Terraform component was written in the schema's `not:`
form and the whole suite failed to load. Existing cases only use `!not` for `skip.os`, so the
mismatch between the schema and `MatchPattern.UnmarshalYAML` in `tests/cli_test.go` had never
been exercised.

## Changes

- `tests/cli_test.go`: `MatchPattern.UnmarshalYAML` decodes a mapping with a single `not` key as
  a negated pattern; any other mapping or scalar type remains an error that names the line.
- `tests/match_pattern_test.go` (new): covers the plain, `!not`, and `{not: pattern}` forms and
  rejects other mappings and non-string scalars.
- `tests/test-cases/cloudformation-defaults.yaml`: uses `not:` to assert that `region` and
  `timeout` are absent from a Terraform component's `describe component` output.

## Validation

- `go test ./tests -run TestMatchPattern` passes.
- `go test ./tests -run 'TestCLICommands/(cloudformation_defaults|fogg_migration)'` passes,
  including the negated assertions.
- `atmos lint --changed` reports no issues.

## Follow-ups

None.
