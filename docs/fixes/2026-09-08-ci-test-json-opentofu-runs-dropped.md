# Fix: `terraform test --ci` dropped every OpenTofu run and late assertion diagnostics from the summary/JUnit

**Date:** 2026-09-08

## Summary

In CI mode, `atmos terraform test` always runs `terraform|tofu test -json` and builds the step
summary, the JUnit report, and inline annotations from that event stream. The parser only accepted
a `test_run`/`test_file` event as final when it carried `progress: "complete"`. OpenTofu never
emits a `progress` field at all -- it emits exactly one event per run/file carrying only the final
`status` -- so under OpenTofu every run and file event was discarded. The `test_summary` event has
the same shape in both tools, so the badge counts still came out right while the results table was
empty and `<component>.junit.xml` reported `tests="0"` on a passing run. Separately, both tools
emit an assertion-failure `diagnostic` *after* the run's final event, but the parser only attached
diagnostics that arrived *before* it, so failing runs lost their message and `file:line` (and
therefore the `::error` annotation and the Details column) under Terraform as well.

## Context

Reported from an application repository whose toolchain pins `tofu`: a passing run produced
`TESTS-1`/`PASSED-1` badges, no results table, `app.junit.xml` with `tests="0"`, and a run log
ending `Success! 1 passed, 0 failed, 0 skipped.` That three-field summary line is Atmos's own
`RenderTestText` format (Terraform's native message is `Success! 1 passed, 0 failed.`), which
placed the failure squarely on the JSON path.

The bug could not be reproduced with this repository's own `examples/terraform-tests` fixture:
eleven consecutive `atmos terraform test app -s fixtures --ci` runs against the Floci emulator all
produced `tests="4"` with real run names. That fixture is Terraform-only (its `.tftest.hcl` files
use `variable` blocks, which OpenTofu rejects in favour of `variables`), so it never exercised the
OpenTofu event shape. Capturing raw `-json` streams from both tools on a minimal provider-free
module made the difference obvious:

- Terraform: `{"path":…,"run":"plan_case","progress":"complete","status":"pass"}` (preceded by a
  `progress: "starting"` event for the same run).
- OpenTofu: `{"path":…,"run":"plan_case","status":"pass"}` -- no `progress`, one event per run.

`completedTestRun`/`completedTestFile` gated on `Progress != "complete"`, so the OpenTofu stream
yielded an empty `data.Runs`/`data.Files`; `applyTestJSONSummary` then backfilled `Total`/`Pass`
from the summary event, which is exactly the reported badge/table/JUnit disagreement. The same
captures showed the assertion diagnostic arriving after the run event in both tools (Terraform
1.15.8, OpenTofu 1.12.5); the existing `sampleTestJSON` fixture had been hand-written with the
diagnostic first, which is why the diagnostic-attachment tests passed.

This is a sibling of the two earlier fixes on the plain-text fallback path
(`docs/fixes/2026-08-14-ci-summary-test-table-fallback-dropped.md`,
`docs/fixes/2026-08-19-ci-test-summary-fallback-recovers-error-detail.md`); neither touched the
JSON path.

## Changes

- `pkg/ci/plugins/terraform/parser.go`:
  - New `testEventComplete(progress, status)`: an event is final when `progress == "complete"`
    (Terraform) or when `progress` is absent and a `status` is present (OpenTofu). Used by both
    `completedTestRun` and `completedTestFile`. Terraform's intermediate `starting`/`running`/
    `teardown` events still carry a `progress` value and remain excluded.
  - Diagnostic attachment factored into `attachPendingDiag`; new `attachLateDiagnostics` runs
    first in `finalizeTestJSON` (which now receives `diagByRun`) and attaches any diagnostic keyed
    by the run's file+name to a recorded run that has no error yet -- covering the
    diagnostic-after-run ordering without changing the diagnostic-before-run path.
  - `backfillMissingTestJSONRuns` (added earlier on this branch as a stop-gap) is retained purely
    as a last-resort guard against a future unrecognised event shape, and now emits a
    `log.Warn` whenever it fires so a schema gap can never again be silently absorbed into a
    placeholder row.
- `pkg/ci/plugins/terraform/test_json_test.go`:
  - `sampleOpenTofuPassJSON` / `sampleOpenTofuFailJSON`: verbatim `tofu test -json` streams
    (OpenTofu 1.12.5).
  - `TestParseTestJSON_OpenTofu_AllPass`, `TestParseTestJSON_OpenTofu_Failure`,
    `TestToJUnit_OpenTofu`, `TestRenderTestText_OpenTofu`: real run names, files, counts,
    message, and `file:line` all captured from the OpenTofu shape.
  - `TestParseTestJSON_DiagnosticAfterCompleteEvent`: Terraform-shaped stream with the diagnostic
    after the `complete` event.
  - The earlier `TestParseTestJSON_SummaryExceedsRuns`, `TestBackfillMissingTestJSONRuns`, and
    `TestToJUnit_BackfillsMissingRuns` remain, covering the guard.

No changes were needed in `junit.go`, `templates/test.md`, or the annotation emitter -- all of
them already key off `data.Runs`/`data.Files`.

## Validation

- Before the parser change, the new tests failed with the exact reported symptom: every run came
  back as `run detail unavailable (pass)` with empty `File`, `Line`, and `Error`, and
  `data.Files` was empty.
- `go test ./pkg/ci/plugins/terraform/...` and `go test ./pkg/ci/...` -- all pass, including the
  pre-existing diagnostic-before-run fixture.
- `go build ./...`, `gofumpt -l`, `atmos lint --changed` -- clean for the touched files (three
  pre-existing findings elsewhere on the branch, untouched).
- End-to-end under OpenTofu: a throwaway Atmos project (`components.terraform.command: tofu`)
  around a provider-free module with one passing and one failing run, executed with the rebuilt
  binary as `GITHUB_ACTIONS=true … atmos terraform test min -s fx --ci`. Result:
  `min.junit.xml` reports `tests="2" failures="1"` with `<testcase name="passing_case">` and
  `<testcase name="failing_case" … line="12"><failure message="Test assertion failed: name should
  be b">`; the step summary lists both runs by name with `tests/min.tftest.hcl:12` in the Details
  column plus the per-file breakdown; and the log carries
  `::error file=tests/min.tftest.hcl,line=12,title=terraform test: failing_case::…`.
- Terraform path unchanged: the repository's own `examples/terraform-tests` fixture still yields
  `tests="4"` with all four real run names.

## Follow-ups

- `examples/terraform-tests` cannot run under OpenTofu (`variable` vs `variables` in
  `.tftest.hcl`), so there is no OpenTofu end-to-end fixture in this repository; the verbatim
  OpenTofu streams in `test_json_test.go` are the regression coverage for that shape. Adding a
  tool-agnostic fixture would let `atmos test --full` exercise both tools.
