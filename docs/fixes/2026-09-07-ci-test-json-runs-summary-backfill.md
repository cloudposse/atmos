# Fix: JSON-path `terraform test` summary/JUnit under-report `tests="0"` when a run event is dropped

**Date:** 2026-09-07

## Summary

`atmos terraform test --ci` parses `terraform/tofu test -json` output via `ParseTestJSON`.
Terraform's own authoritative `test_summary` event (e.g. `passed: 1`) is trusted to backfill
`data.Pass`/`Fail`/`Error`/`Skip`/`Total`, but nothing ever reconciled `data.Runs` — the slice the
JUnit report and the CI step-summary results table both iterate exclusively. If one or more
`test_run` "complete" events never made it into `data.Runs` (upstream JSON decode gap) while the
simpler `test_summary` event still parsed fine, the badges correctly showed a passing run, but the
results table was empty and the generated `<component>.junit.xml` reported
`<testsuites tests="0" ...>` despite the run genuinely passing.

## Context

This mirrors, on the JSON-parsing path, the same class of bug already fixed once on the
plain-text fallback path:

- `docs/fixes/2026-08-14-ci-summary-test-table-fallback-dropped.md` — text path, table disappears.
- `docs/fixes/2026-08-19-ci-test-summary-fallback-recovers-error-detail.md` — text path, follow-up
  detail recovery.

Both of those fixed `ParseTestOutput`'s `synthesizeFallbackRun`, which only runs when the
per-run text lines are entirely absent. They never touched `ParseTestJSON`/`finalizeTestJSON`,
which is the path always used in `--ci` mode (`cmd/terraform/test.go` force-appends `-json`
whenever CI mode is active). `finalizeTestJSON` sets `data.Total = len(data.Runs)` first, then
`applyTestJSONSummary` unconditionally overwrites `data.Pass/Fail/Error/Skip` (and backfills
`Total` only when it was still `0`) from the `test_summary` event — with no code path reconciling
`data.Runs` itself. `toJUnit` (`pkg/ci/plugins/terraform/junit.go`) and the step-summary template
(`templates/test.md`, gated on `len($test.Runs) > 0`) both only ever iterate `data.Runs`, so any
gap between the summary counts and the captured runs surfaces exactly as reported: nonzero
`TESTS-1`/`PASSED-1` badges, an empty results table, and `tests="0"` in the JUnit XML.

Verified directly: `applyTestJSONSummary`, `toJUnit`, and `templates/test.md` are not branch-gated
on pass/fail as originally hypothesized — a passing run's `<testcase>` simply has no `<failure>`/
`<error>`/`<skipped>` child. The defect is purely that `data.Runs` can be short.

## Changes

- `pkg/ci/plugins/terraform/parser.go`:
  - Added `backfillMissingTestJSONRuns(data *plugin.TerraformTestOutputData)`, called from
    `finalizeTestJSON` right after `applyTestJSONSummary` and before `populateTestFileCounts`.
    For each status (`pass`/`fail`/`error`/`skip`), it computes how many more entries the summary
    counts claim than are actually present in `data.Runs` and appends one placeholder
    `plugin.TerraformTestRun{Name: "run detail unavailable (<status>)", Status: <status>}` per
    missing unit, then resets `data.Total = len(data.Runs)`. Mirrors the existing
    `synthesizeFallbackRun` convention from the text-parsing path, but per-status rather than a
    single aggregate row, so `Total`/badges/the results table row count/JUnit's
    `tests`/`failures`/`errors`/`skipped` attributes (derived purely from `len(Cases)` by
    `junit.Report.Aggregate()`) all stay truthfully consistent. Never removes or mutates real
    captured rows — a no-op when `data.Runs` already fully accounts for the summary counts.
- `pkg/ci/plugins/terraform/test_json_test.go`:
  - `TestParseTestJSON_SummaryExceedsRuns`: a stream containing only a `test_summary` event (no
    `test_run` line at all) — the direct repro of the reported bug — now yields
    `len(data.Runs) == 1` with a synthesized `pass` row, instead of an empty slice.
  - `TestBackfillMissingTestJSONRuns`: table-driven unit coverage of the helper directly (no
    mismatch/no-op, fully missing, partial with mixed statuses, and runs-exceed-summary never
    deletes real data).
  - `TestToJUnit_BackfillsMissingRuns`: feeds the summary-only stream through `ParseTestJSON` →
    `toJUnit` and asserts `report.Tests == 1` and `report.Passed()` — the JUnit-facing assertion
    that directly encodes "must not report `tests=\"0\"` on a passing run."

No changes needed to `junit.go` or `templates/test.md` — both already key off `data.Runs`.

## Validation

- `go test ./pkg/ci/plugins/terraform/... -run 'TestParseTestJSON|TestBackfillMissingTestJSONRuns|TestToJUnit' -v`
  — new tests fail (build error: `backfillMissingTestJSONRuns` undefined) before the fix, all pass
  after.
- `go test ./pkg/ci/...` — full package, all pass, no regressions (`terraform` package coverage
  90.7%).
- `go build ./...` — clean.
- `gofumpt -l` on both touched files — clean.
- `atmos lint --changed` — 0 issues in the touched files (3 pre-existing, unrelated findings
  elsewhere on the branch: `pkg/store/providers/azure_keyvault_store.go`,
  `cmd/terraform/utils.go`, `pkg/component/helm/client.go` — not touched by this change).

## Follow-ups

This fix makes the counts and JUnit `tests` attribute truthful again, but it is explicitly a
placeholder for *missing* data, not a recovery of it: `backfillMissingTestJSONRuns` never removes
or mutates real captured rows, so when it fires, the synthesized row's name is the generic
`run detail unavailable (<status>)`, not the real `.tftest.hcl` run block name (e.g.
`applies_ecs_service_against_emulator`), and it carries no per-assertion detail. Confirmed against
the real `examples/terraform-tests` fixture with a locally built binary: `app.junit.xml` correctly
reports `tests="1"` and the results table has one row, but that row is the generic placeholder, not
the actual run name or assertion-level detail the original bug report asked for.

Closing that gap requires fixing *why* the `test_run` "complete" event is dropped from `data.Runs`
in the first place (upstream of `finalizeTestJSON`, in `completedTestRun`/`buildTestRun`/the
line-scanning loop) so the real name and detail are captured — this fix only guarantees the
reported numbers stop lying when that upstream drop happens. No root cause for the drop itself was
identified with the tools available in this pass (see the sibling investigation notes for
`ParseTestJSON`/`buildTestRun`); a future pass with access to a live, JSON-instrumented repro run
against the emulator-backed `provisions_resources_against_emulator` run block would be needed to
pin down the actual trigger.
