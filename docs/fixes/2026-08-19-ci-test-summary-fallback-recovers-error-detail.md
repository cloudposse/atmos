# Fix: CI test summary fallback recovers per-run assertion detail when available

**Date:** 2026-08-19

## Summary

When `terraform test`'s per-run `run "name"... pass/fail` lines aren't captured and
`ParseTestOutput` falls back to the trailing summary line, the synthesized aggregate row now
recovers the failing assertion's file, line, and message from any terraform `Error:` diagnostic
block that survived in the captured output, instead of always rendering a bare "per-run detail
unavailable" placeholder with only pass/fail counts.

## Context

`docs/fixes/2026-08-14-ci-summary-test-table-fallback-dropped.md` fixed the results table
disappearing entirely on this fallback path by synthesizing a single aggregate
`plugin.TerraformTestRun` row. That row carried no `File`/`Line`/`Error` detail — even in cases
where terraform's `Error:` diagnostic block (file, line, and assertion message) was still present
in the captured text, only the per-run `run "..."... pass/fail` status lines were missing. The gap
was reported as: CI test summaries provide only aggregate pass counts and a reproduction command,
with no individual test-run/assertion detail.

A reproduction test (`TestTestTemplate_SummaryFallback_LosesRunDetail` in
`pkg/ci/plugins/terraform/test_template_test.go`) was added first, confirming the rendered CI
summary genuinely dropped a failing run's name, file, line, and assertion message when the
captured text contained only the trailing `Failure! N passed, M failed.` line.

## Changes

- `pkg/ci/plugins/terraform/parser.go`:
  - Added `errorLocationRe`, matching the `on <file> line <N>:` locator inside a terraform
    `Error:` diagnostic block.
  - `synthesizeFallbackRun` now takes the raw `output` and, when `fail > 0`, calls
    `ExtractErrorBlocks` to recover any `Error:` blocks present; it joins them into the
    synthesized row's `Error` field and parses the first block's file/line into `File`/`Line` via
    `errorLocationRe`. When no `Error:` block survived (the residual, irreducible case), the row
    is unchanged from before — there is nothing left in the captured text to recover.
  - `ParseTestOutput` passes `output` through to `synthesizeFallbackRun`.
- No template changes were needed: `templates/test.md`'s results table already renders
  `File`/`Line`/`Error` for `fail`/`error` rows, so the recovered detail surfaces automatically.

## Validation

- `go build ./...` — clean.
- `go test ./pkg/ci/plugins/terraform/... -run 'TestParseTestOutput|TestTestTemplate' -v` — all
  pass, including:
  - New `TestParseTestOutput_SummaryFallback_RecoversErrorDetail` and
    `TestTestTemplate_SummaryFallback_RecoversErrorDetail`, confirming file/line/message are now
    recovered end-to-end (`ParseTestOutput` → `test.md` render) when an `Error:` block survives.
  - `TestTestTemplate_SummaryFallback_LosesRunDetail` (the reproduction test, retained and
    re-commented to document the residual case where no `Error:` block survives at all — genuinely
    unrecoverable from the captured text).
  - Existing `TestParseTestOutput_SummaryFallback`, `_FailureSummaryFallback`, `_ZeroTotal`, and
    `TestTestTemplate_SummaryFallback_AllPass`/`_WithFailure` — unaffected (no `Error:` block in
    their fixtures).
- `go test ./pkg/ci/...` — full package, all pass.
- `atmos lint --changed` — 0 issues.

## Follow-ups

None. The residual case (no `Error:` diagnostic block at all survives in the captured output) has
no recoverable detail in the text and remains covered by
`TestTestTemplate_SummaryFallback_LosesRunDetail` as a known, documented limit of this legacy
text-parsing fallback path.
