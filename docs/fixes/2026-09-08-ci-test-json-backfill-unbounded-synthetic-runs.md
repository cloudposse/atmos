# Fix: `backfillMissingTestJSONRuns` no longer appends unbounded synthetic runs

**Date:** 2026-09-08

## Summary

`pkg/ci/plugins/terraform/parser.go`'s `backfillMissingTestJSONRuns` synthesizes placeholder
`data.Runs` entries when the authoritative `test_summary` counts from a `terraform|tofu test
-json` stream exceed the runs the parser actually captured. The per-status loop bound came
straight from the untrusted `passed`/`failed`/`errored`/`skipped` ints in that stream with no
upper limit, so a single oversized count (e.g. `passed: 1000000000`) drove an unbounded `append`
loop that could exhaust memory or hang the `atmos terraform test --ci` command before it reported
anything. The loop is now capped per status at `maxBackfillRunsPerStatus` (10,000), and the
parser marks the result as incomplete when a count is truncated instead of silently
under-representing it.

## Context

Flagged by CodeRabbit's review of PR #3082 (thread `PRRT_kwDOEW4XoM6gUagt`, 🟠 Major) against
`backfillMissingTestJSONRuns`, which had been added and retained across two earlier commits on
this branch (`3ead9012f7`, `e57b5b8eb0`) as a last-resort guard for a schema gap in the parser.
The guard itself was never bounded, so it traded one failure mode (a schema gap silently dropping
runs) for another (an oversized summary count silently exhausting memory). This fix hardens the
guard added by `docs/fixes/2026-09-08-ci-test-json-opentofu-runs-dropped.md` without changing its
purpose.

## Changes

- `pkg/ci/plugins/terraform/parser.go`:
  - New `maxBackfillRunsPerStatus` constant (10,000) bounding synthetic rows per status.
  - `backfillMissingTestJSONRuns` clamps the per-status append count to the cap; when a count is
    truncated it escalates from `log.Warn` to `log.Error` and sets the new
    `data.BackfillTruncated` flag (never touches real captured rows).
  - `testJSONHasErrors` includes `BackfillTruncated` so a truncated backfill always marks
    `result.HasErrors`.
  - `renderTestSummaryLine` (the plain-text fallback renderer) treats `BackfillTruncated` as a
    failure condition and appends an explicit "parser output incomplete" line.
- `pkg/ci/internal/plugin/types.go`: new `TerraformTestOutputData.BackfillTruncated bool` field.
- `pkg/ci/plugins/terraform/handlers.go`: `buildTerraformTestStatusDescription` appends a
  `"parser output incomplete"` part when `BackfillTruncated` is set, alongside the existing
  `CleanupFailures` part, so the step-summary/status description surfaces the truncation.
- `pkg/ci/plugins/terraform/test_json_test.go`:
  - `TestBackfillMissingTestJSONRuns` table gained a case asserting untouched, non-truncated
    backfills leave `BackfillTruncated` false.
  - New `TestBackfillMissingTestJSONRuns_CapsOversizedCount`: a `Pass: 1_000_000_000` summary
    count is capped at `maxBackfillRunsPerStatus` synthesized runs, with `BackfillTruncated` and
    `Total` asserted.
  - New `TestParseTestJSON_OversizedSummaryCountIsBounded`: the same scenario through the public
    `ParseTestJSON` entry point, asserting `result.HasErrors` and a bounded `data.Runs`.

## Validation

- `go build ./...` -- clean.
- `go test ./pkg/ci/plugins/terraform/... -run 'TestBackfillMissingTestJSONRuns|TestParseTestJSON' -v`
  -- all pass, including the two new oversized-count tests, in under 2s (proving the cap actually
  bounds the work rather than just bounding the assertion).
- `go test ./pkg/ci/...` -- full package, all pass, no regressions in JUnit/handlers rendering.
- Confirmed compile-time (not behavioral) coverage: with the fix removed (`git stash` of the
  three non-test files), the package fails to *compile* because the tests reference
  `maxBackfillRunsPerStatus` and `BackfillTruncated`, which only exist after the fix. This proves
  the tests are wired to the new symbols, not that they'd catch a regression to the unbounded
  loop while those symbols still exist. The behavioral proof is the tests themselves: both call
  `backfillMissingTestJSONRuns`/`ParseTestJSON` with a real `Pass: 1_000_000_000` count and assert
  the actual output is capped at `maxBackfillRunsPerStatus`, completing in well under 2s -- so the
  cap is genuinely exercised, not just asserted against a hypothetical. Reverting just the loop's
  bound (to reproduce a literal billion-iteration append) was not run, since doing so would
  intentionally trigger the exact memory-exhaustion/hang this fix prevents.
- `atmos fix lint` (patch-scoped, `--new-from-rev=origin/main`) -- the only findings are 3
  pre-existing issues in unrelated files (`pkg/store/providers/azure_keyvault_store.go`,
  `cmd/terraform/utils.go`, `pkg/component/helm/client.go`); none in the files touched by this
  fix.

## Follow-ups

None.
