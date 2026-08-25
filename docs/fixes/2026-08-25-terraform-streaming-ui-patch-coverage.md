# Fix: Raise `terraform` streaming-UI patch coverage from 68.78% to 85%+

**Date:** 2026-08-25

## Summary

Codecov's patch check on the `osterman/terraform-streaming-ui` PR was failing: patch coverage was
68.78% (678 lines missing) against the repo's 85% target, driven by the new streaming-UI feature
(`pkg/terraform/ui/` + `internal/exec/terraform_streaming_ui.go`) landing with little or no dedicated
test coverage. Added real behavioral tests across the gap files and two small dependency-injection
seams in `executor.go` to make previously untestable subprocess/TUI orchestration coverable. Aggregate
statement coverage across every touched file is now ~92.55%.

## Context

Commit `cae6e2b` ("fix: streaming UI Ctrl-C hang, duplicate-plan bug, destroy/refresh --ui no-ops, dead
render config") added ~3,600 new lines implementing a Bubble Tea TUI for streaming `terraform`
plan/apply/destroy output, plus its executor/orchestration layer. Several files landed with 0-30% patch
coverage: `executor.go` (11.96%, 263 missing lines — the executor/orchestration core), `confirm.go`
(0%, no test file), `executor_outputs.go` (28.57%, no test file), and `internal/exec/terraform_streaming_ui.go`
(31.25%, no test file), among others with smaller gaps (`tree_utils.go`, `model_render.go`,
`tree_render.go`, `tree_builder.go`, `model_diagnostics.go`, `init_model.go`).

An exploration pass confirmed most of the gap was pure-logic and Bubble-Tea-model code that fit this
package's existing test pattern (constructing `Model`/`ResourceTracker` state and calling
`Update`/`View`/render helpers directly — no `teatest` used anywhere in this repo). The two genuinely
hard spots were `executor.go`'s subprocess/`tea.Program` orchestration (needed a DI seam to unit test
without a real terraform binary or terminal) and `confirm.go`'s `huh.NewConfirm().Run()` call (needs a
real TTY, no seam added — out of scope for one line).

## Changes

Five batches of test-only work, run in parallel against disjoint files:

- **Pure-logic files** (`tree_utils.go`, `executor_outputs.go`, `model_diagnostics.go`, `init_model.go`):
  new/expanded `_test.go` files covering color-parsing/collapse branches, output table-building
  (new `executor_outputs_test.go`), `LogDiagnostics`/`logDiagnostic` side effects, and `readNextLine`
  outcomes. No production changes.
- **Bubble Tea render files** (`model_render.go`, `tree_render.go`, `tree_builder.go`): new tests for
  `finalView`, `renderErrorSummary`, `formatActivityVerb`, `progressHeaderLine`, complex attribute-diff
  rendering, and `BuildDependencyTree` (via the test binary standing in for `terraform`). No production
  changes. Found and left undisturbed one dead-code branch in `buildRelationships` (unreachable given
  current logic) rather than fabricate a test for it.
- **`confirm.go`**: new `confirm_test.go` covering the non-TTY early-return path (the default under
  `go test`) for both `ConfirmApply`/`ConfirmDestroy`. No production changes.
- **`internal/exec/terraform_streaming_ui.go`**: new `terraform_streaming_ui_test.go` covering the
  retry-gated shell fallback, both `ShouldUseStreamingUI` outcomes, and every `dispatchStreamingExecutor`
  switch branch, reusing existing shell-command mocks. No production changes.
- **`executor.go`** (the largest gap): added two minimal package-level DI seams —
  `var execCommandContext = exec.CommandContext` (used by `newStreamingCommand`/`newInitCommand`) and
  `var runTeaProgram = func(p *tea.Program) (tea.Model, error) { return p.Run() }` (used by
  `runTUIProgram`) — following the same self-re-exec-test-binary pattern already used by
  `TestKillIfCancelled_KillsRealProcess`. New tests cover subprocess start/pipe-error paths,
  `streamStderrToLog`, all three `runTUIProgram` branches, `finalizeExecuteResult`/
  `finalizeExecuteInitResult` branch tables, two-phase plan/apply/destroy orchestration, and the
  `-auto-approve` branches of `ExecuteApply`/`ExecuteDestroy`.

Collateral lint fixes surfaced by `atmos lint --changed` after the above (all required to pass the
shared `--new-from-rev=origin/main` gate):

- `pkg/terraform/ui/model_test.go`: reworded a comment that read as two sentences to `gofumpt`/`godot`
  (the abbreviation "vs." confused the sentence-start check).
- `pkg/terraform/ui/executor.go`: removed two `//nolint:gosec` directives that became unused once the
  DI seam indirection meant `gosec` no longer flagged those call sites.
- `cmd/terraform/utils.go`: extracted the repeated `"ci"` string literal (10 occurrences) to a new
  `ciFlagName` constant. Pre-existing from the original feature commit (`100dc060db`), unrelated to the
  coverage work, but blocking the same lint gate.

## Validation

- `go build ./...` — clean.
- `go vet ./pkg/terraform/ui/... ./internal/exec/... ./cmd/terraform/...` — clean.
- `atmos lint --changed` (`--new-from-rev=origin/main`) — 0 issues (was 4 before the collateral fixes).
- `go test ./pkg/terraform/ui/... ./internal/exec/... ./cmd/terraform/...` — all pass, no regressions;
  confirmed all pre-existing tests (including `TestKillIfCancelled_KillsRealProcess`) still pass after
  the DI seam change.
- Coverage measured via `go test -coverprofile` + `go tool cover -func` on both `pkg/terraform/ui` and
  `internal/exec`, then aggregated per-file statement counts across every file touched by this PR's diff
  vs `origin/main`: **68.78% → ~92.55%** aggregate, comfortably above the 85% target. Per-file:
  `executor.go` 11.96%→72.4%, `confirm.go` 0%→27.3%, `terraform_streaming_ui.go` 31.25%→69.6%,
  `executor_outputs.go` 28.57%→96.2%, `tree_utils.go` 65.21%→99.1%, `model_render.go` 79.73%→100%,
  `tree_render.go` 80.54%→100%, `tree_builder.go` 83.50%→98.8%, `model_diagnostics.go` 65.21%→100%,
  `init_model.go` 89.04%→100%.
- Not run: `atmos test --full` (long-running acceptance suite) and a live Codecov re-run — the local
  statement-coverage aggregation above is a close proxy for Codecov's line-based patch report but not
  identical; the real number will be confirmed by CI on push.

## Follow-ups

Two accepted, documented coverage ceilings remain by design, not oversight — neither needs a follow-up
issue, as no further work is planned or expected:

- `confirm.go`'s `huh.NewConfirm().Run()` call and the branches only reachable after it (needs a real
  TTY; no DI seam was added for this single interactive-prompt library call).
- `executor.go`'s `Execute`/`ExecuteInit` code paths gated behind `checkStreamingUIPreconditions()`
  (needs `term.IsTTYSupportForStdout()` to be true; not forceable from an isolated package test in this
  environment). The orchestration logic behind that gate is independently covered via the standalone
  `newStreamingCommand`/`runTUIProgram`/`finalizeExecuteResult` tests.
