# Fix: Refresh labels help acceptance snapshots

**Date:** 2026-10-17

## Summary

Regenerate Terraform help snapshots for the literal-preserving, repeatable `--labels` parser.

## Context

The label regression fix replaced CSV parsing with a string-array flag so existing labels containing literal double quotes remain accepted. Cobra now displays the flag type as `stringArray` instead of `strings`. Seven acceptance snapshots still expected the previous type, causing four shards to fail on Linux, macOS, and Windows.

## Changes

Use the acceptance suite's `-regenerate-snapshots` option to refresh the seven affected Terraform, custom-command, and alias help snapshots. Keep the label parser and all snapshot assertions unchanged.

## Validation

- Reproduced all seven failing acceptance cases locally against a binary built from the current source; every mismatch was the labels flag type.
- Regenerated all seven snapshots using `-regenerate-snapshots`; reviewed the diff and confirmed exactly one flag-type change in each file.
- Existing literal-quote, repeated-label, and label-source-precedence tests pass in `cmd/list` and `cmd/terraform`.
- All 52 acceptance help cases pass without regeneration, including the seven original failures: `env -u NO_COLOR GOMAXPROCS=2 go test ./tests -run '^TestCLICommands$/.*help' -count=1 -v -timeout=10m`.
- Fix-record validation and `git diff --check` pass.

## Follow-ups

Verify the updated CI run before merging.
