# Fix: CI test summary no longer drops the results table on the summary-line fallback

**Date:** 2026-08-14

## Summary

A Native CI `atmos terraform test` job summary whose per-run `run "name"... pass` lines weren't
captured (e.g. output buffering) fell back to parsing only the trailing `Success! N passed, 0
failed.` line. That fallback populated the pass/fail badges but left `TestResult.Runs` empty, and
the summary template's results table is gated on `len(Runs) > 0` — so the table (and the
"Detailed test results" section) silently disappeared even on an otherwise normal passing or
failing run, leaving only badges and the "Reproduce locally" block.

## Context

`pkg/ci/plugins/terraform/parser.go`'s `ParseTestOutput` parses per-run lines via `testRunRe` into
`data.Runs`. When none are captured (`data.Total == 0`), it falls back to `testSummaryRe` against
the trailing summary line, setting `Pass`/`Fail`/`Total` but never touching `Runs`. `templates/test.md`
renders the results table only under `{{- if and $test (gt (len $test.Runs) 0) }}`, so a fallback-
triggered run rendered badges and the repro command but no table — the exact symptom reported.

This fallback path is legacy/secondary: `ParseTestJSON` (used whenever `test -json` output is
captured) always populates `Runs` per event and was unaffected.

## Changes

- `pkg/ci/plugins/terraform/parser.go`: when the summary-line fallback fires and yields
  `Total > 0`, append a single synthesized `plugin.TerraformTestRun` (via new unexported
  `synthesizeFallbackRun(pass, fail int)`) standing in for the per-run detail that couldn't be
  captured, with `Status` set to `pass` or `fail`. No template changes were needed — reusing
  `Runs` means the existing table/detail sections render unchanged. `Total == 0` still leaves
  `Runs` empty (nothing ran, so no row should be synthesized).

## Validation

- `go test ./pkg/ci/plugins/terraform/... -run 'TestParseTestOutput|TestTestTemplate' -v` — all pass,
  including updated `TestParseTestOutput_SummaryFallback`/`_FailureSummaryFallback` (now assert a
  synthesized row instead of `assert.Empty`), new `TestParseTestOutput_SummaryFallback_ZeroTotal`,
  and new `TestTestTemplate_SummaryFallback_AllPass`/`_WithFailure` (render `templates/test.md`
  directly and assert the table header/row appear — this rendering path had no prior test coverage).
- `go test ./pkg/ci/...` — full package, all pass.
- `go build ./...` and `go vet ./pkg/ci/...` — clean.

## Follow-ups

None.
