---

description: "Task list for --filter on describe affected and rejecting --query with matrix output"
---

# Tasks: `--filter` for `describe affected` and No Silent `--query` with Matrix Output

**Input**: Design documents from `/specs/004-fix-matrix-query/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/cli-contract.md, quickstart.md

**Tests**: REQUIRED. The spec (FR-013) and the constitution (Principle III: bug fixes start with a failing test) demand test-first. Write each test task, run it, and confirm it fails before the implementation task that follows it.

**Organization**: Grouped by user story. Paths are relative to the repository root. Do not use pipe redirection when running tests, and never edit existing golden snapshots.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: US1 to US5, matching the user stories in spec.md

---

## Phase 1: Setup

**Purpose**: Establish a baseline before changing anything.

- [X] T001 Run `go build ./... && atmos test` and record that the existing `internal/exec` describe-affected tests and `cmd` tests pass, so later failures are attributable to this change
- [X] T002 [P] Capture the current matrix output without `--filter`/`--query` as the byte-identical baseline: add a fixture-based test in `internal/exec/describe_affected_predicate_test.go` that asserts `convertAffectedToMatrix` plus `matrix.WriteOutput` output for a fixed affected list (this test must pass before and after, covering US4)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The shared pieces every story needs: the per-item predicate, the flag, and the arg plumbing.

- [X] T003 [P] Write failing table-driven tests for `YQPredicateFilter.Matches` in `pkg/list/filter/yq_test.go`: truthy true, non-empty string, non-empty map, non-empty slice; falsy false, nil, empty string/map/slice; invalid expression returns an error; `Apply` returns the same rows as repeated `Matches` calls
- [X] T004 Add `func (f *YQPredicateFilter) Matches(item map[string]any) (bool, error)` to `pkg/list/filter/yq.go` and make `Apply` call it (keep the existing error wrapping text, add `defer perf.Track`, end comments with periods); T003 must pass
- [X] T005 [P] Write failing tests in `cmd/describe_affected_selector_flags_test.go` (use `NewTestKit`): `--filter` is registered; `ATMOS_DESCRIBE_AFFECTED_FILTER` is picked up when the flag is not set; the flag beats the env var; a value from the environment is marked as env-sourced (like `--tags`)
- [X] T006 Register `--filter` with `flags.WithStringFlag` and `flags.WithEnvVars` (`ATMOS_DESCRIBE_AFFECTED_FILTER`) under the `describe.affected` prefix in `cmd/describe_affected_selector_flags.go`, with the help text from `contracts/cli-contract.md`, and resolve the env value in `resolveDescribeAffectedSelectorFlags` including the env-source mark; no direct `viper.Bind*` calls; T005 must pass
- [X] T007 Add `Filter` and `FilterEnvVar` fields to `DescribeAffectedCmdArgs` in `internal/exec/describe_affected.go`; map `"filter": &describe.Filter` in `SetDescribeAffectedFlagValueInCliArgs` and read `FilterEnvVar` with `FlagValueFromEnv` next to the `TagsEnvVar` lookup (small edits only, file is already over 600 lines)
- [X] T008 Create `internal/exec/describe_affected_predicate.go` with: `newAffectedPredicate(atmosConfig, expr)` returning `func(map[string]any) (bool, error)` (nil when `expr` is empty), built on `filter.NewYQPredicateFilter(...).Matches`; and a helper that converts a `schema.Affected` or `schema.Dependent` to a map through a JSON round trip. Add a package comment explaining why the shape matches `--query` input

**Checkpoint**: `Matches`, the flag, and the arg plumbing exist. Stories can start.

---

## Phase 3: User Story 1 - Filtered CI matrix honors the filter (Priority: P1) 🎯 MVP

**Goal**: `--filter` with `--format=matrix` keeps only matching components; no match gives `{"include":[]}`.

**Independent Test**: One affected component with `settings.github.actions_enabled: false`, `--include-settings --filter '.settings.github.actions_enabled // false' --format=matrix` outputs `{"include":[]}`.

### Tests for User Story 1 (write first, confirm they fail)

- [X] T009 [P] [US1] In `internal/exec/describe_affected_predicate_test.go`, add table-driven tests of the prune function for top-level items (no dependents): matches none, matches a subset (assert first and last element values, not just length), matches all (result equals the unfiltered list), empty/unset expression is identity, invalid expression returns an error and no result
- [X] T010 [P] [US1] In the same file, add the matrix end-to-end test: build a fake affected list with `actions_enabled: false` and `true` items, run the real `view()` matrix path with the `Filter` set and a temp output file, and assert the written matrix is `{"include":[]}` for the all-false case and contains exactly the enabled item otherwise; also assert the filtered matrix is written to the output file (US4 acceptance 2) and that mutating the returned slice does not change the input list, and that mutating the input before filtering does not change the result
- [X] T011 [US1] Run T009 and T010 and confirm they fail (the prune function and the filter wiring do not exist yet)

### Implementation for User Story 1

- [X] T012 [US1] Implement `pruneAffectedByPredicate(affected []schema.Affected, pred) ([]schema.Affected, error)` for top-level items in `internal/exec/describe_affected_predicate.go` (copy the slice, never mutate the input, return a non-nil empty slice when nothing matches)
- [X] T013 [US1] Wire it into the describe flow in `internal/exec/describe_affected.go` (and `describe_affected_utils_2.go` where the dependents path needs it): build the predicate from `a.Filter` once, and apply it to the computed affected list before `view()`, so matrix, json and yaml all receive the filtered list; T009 and T010 must pass
- [X] T014 [US1] Run `go build ./... && atmos test` and confirm the T002 baseline test still passes

**Checkpoint**: the reported scenario is fixed through `--filter` (MVP).

---

## Phase 4: User Story 2 - Same filter in every format, including dependents (Priority: P1)

**Goal**: json, yaml and matrix list the same components; with dependents, a failing item removes its whole subtree; pruning happens before flattening.

**Independent Test**: The same filter with `json`, `yaml` and `matrix` yields the same set of components.

### Tests for User Story 2 (write first, confirm they fail)

- [X] T015 [P] [US2] In `internal/exec/describe_affected_predicate_test.go`, add a cross-format test: for none/subset/all, assert the component set from json, yaml and matrix is identical, and json/yaml with no match is an empty list
- [X] T016 [P] [US2] Add forest tests for dependents: a failing top-level item drops its subtree even when a dependent would pass; a failing nested dependent drops only its branch while siblings stay; a passing parent keeps its passing dependents (negative path: no pruning without a failing ancestor); depth of at least 3 levels
- [X] T017 [P] [US2] Add flatten tests: with `Flatten` and a failing ancestor, its dependents do not appear in the flattened top-level list; passing branches are lifted as before (`affected: dependent`)
- [X] T018 [US2] Run T015 to T017 and confirm they fail

### Implementation for User Story 2

- [X] T019 [US2] Extend `pruneAffectedByPredicate` to recurse through `Dependents` (convert each `schema.Dependent` to a map, drop failing ones with their subtree, no promotion) in `internal/exec/describe_affected_predicate.go`
- [X] T020 [US2] Add the predicate to `AffectedDependentsOptions`/`dependentsOptions` and call the prune inside `finalizeAffectedDependents` after the selector step and before `flattenAffectedDependents` in `internal/exec/describe_affected.go`; for the no-dependents path keep the direct call from T013; T015 to T017 must pass
- [X] T021 [US2] Verify whether dependents carry `settings` when `--include-settings` is set (research R5). If they do not, populate them in the dependents resolution or document the limitation in the Phase 8 docs (T031); add a test that pins the chosen behavior

**Checkpoint**: filtering is consistent across formats and dependents.

---

## Phase 5: User Story 3 - `--query` with matrix fails loudly (Priority: P1)

**Goal**: `--query` with `--format=matrix` exits with an error that points to `--filter`; nothing is written.

**Independent Test**: `describe affected --format=matrix --query '<any>'` exits non-zero and writes no matrix.

### Tests for User Story 3 (write first, confirm they fail)

- [X] T022 [P] [US3] In `internal/exec/describe_affected_predicate_test.go`, add the regression test for the original report: arguments with `Format: "matrix"` and the original `Query` fail validation with `errors.Is(err, errUtils.ErrInvalidFlag)`, the message mentions `--filter`, and no output file is created. Run it first against the unmodified code and record that it fails (the combination currently succeeds and emits the unfiltered matrix)
- [X] T023 [P] [US3] Add tests that `--query` with json and yaml is unchanged, and that `--query` together with `--filter` runs the filter first and the query on the filtered list

### Implementation for User Story 3

- [X] T024 [US3] Add the matrix+query check to `validateDescribeAffectedBasics` in `internal/exec/describe_affected.go` using the error builder with `ErrInvalidFlag`, the hints from `contracts/cli-contract.md`, and no new sentinel; T022 and T023 must pass
- [X] T025 [US3] Ensure the evaluation order in the non-matrix path is filter, then query (`view()` receives the already-filtered list); adjust only if T023 fails

**Checkpoint**: the original silent bug can no longer happen.

---

## Phase 6: User Story 4 - Existing behavior unchanged (Priority: P1)

**Goal**: matrix output without `--filter`/`--query` is byte-identical, including file and CI output.

**Independent Test**: the T002 baseline test and the existing matrix tests pass unchanged.

- [X] T026 [P] [US4] Add tests in `internal/exec/describe_affected_predicate_test.go` that the CI auto-detected output path (`GITHUB_OUTPUT` via `ghactions.GetOutputPath()`) and the explicit `--output-file` both still receive the matrix (with and without a filter), using temp files and `t.Setenv`
- [X] T027 [US4] Run the whole existing describe-affected suite (`go test ./internal/exec -run 'DescribeAffected|Affected'` and `./cmd/...`) and confirm no snapshot or test changes were needed; if any existing test fails, fix the implementation, not the test

**Checkpoint**: no regression for pipelines that do not use the new flag.

---

## Phase 7: User Story 5 - Combines with selectors, guards `--upload` (Priority: P2)

**Goal**: `--filter` with `--tags`/`--labels` keeps items that satisfy both; `--filter` with `--upload` follows the selector rule.

**Independent Test**: filter plus tags keeps only items matching both; `--filter` flag plus `--upload` errors.

### Tests for User Story 5 (write first, confirm they fail)

- [X] T028 [P] [US5] Add tests in `internal/exec/describe_affected_predicate_test.go` for `resolveAffectedFilter`: flag + `--upload` returns an `ErrInvalidFlag` error with hints; env-sourced filter + `--upload` clears the filter and emits a warning (assert through the existing `ui` test seam used by the selector tests); no upload leaves the filter untouched; negative path: a flag filter without `--upload` is never cleared
- [X] T029 [P] [US5] Add a combination test with `--tags`/`--labels`: only items that satisfy both selectors and the filter remain, and the dependents path keeps the existing promotion behavior for selectors while the filter does not promote

### Implementation for User Story 5

- [X] T030 [US5] Implement `resolveAffectedFilter(a *DescribeAffectedCmdArgs) error` in `internal/exec/describe_affected_predicate.go` mirroring `resolveAffectedSelectors` (error for the flag form, `ui.Warningf` and clear for the env form) and call it next to `resolveAffectedSelectors` in the argument processing in `internal/exec/describe_affected.go`; T028 and T029 must pass

**Checkpoint**: all five stories work independently.

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: documentation, release obligations, and final verification.

- [X] T031 [P] Update `website/docs/cli/commands/describe/describe-affected.mdx`: add `--filter` to the flags `<dl>` (with `ATMOS_DESCRIBE_AFFECTED_FILTER`), state that `--query` is not supported with `--format=matrix`, explain the difference from `--tags`/`--labels` (dependents of a filtered-out item are dropped, not promoted), note the `--include-settings` requirement, and add the CI matrix example from `contracts/cli-contract.md`
- [X] T032 [P] Write the blog post `website/blog/2026-10-30-describe-affected-filter.mdx` following the `changelog` skill (problem-first framing, tags and authors from `tags.yml`/`authors.yml`)
- [X] T033 [P] Add the roadmap entry in `website/src/data/roadmap.js` following the `roadmap` skill (do not modify `featured[]`)
- [X] T034 [P] Write the fix record `docs/fixes/2026-10-30-describe-affected-matrix-query.md` following the `fix-log` skill
- [ ] T035 Regenerate the help recording with `atmos --chdir=demo/casts casts generate screengrabs cli` and validate with `atmos --chdir=demo/casts casts validate screengrabs cli`; never pipe the output
- [X] T036 Build the website: `cd website && npm run build`, and confirm no broken links or MDX errors
- [ ] T037 Run `atmos lint --changed`, fix findings (gofumpt, godot comment periods, cyclop and funlen limits, import grouping), then `go build ./... && atmos test` and `atmos test --full`
- [ ] T038 Run the `test-coverage` skill for the patch and confirm at least 90% patch coverage on the new and changed lines; add tests for any gap
- [ ] T039 Run the `code-hygiene` skill on the patch and resolve architectural findings
- [ ] T040 Use the `pull-request` skill to confirm the `minor` label, the blog post and the roadmap update before opening the PR, and record in the PR description the R8 finding that `list affected` and `list instances` need no change
- [X] T041 Walk through `quickstart.md` end to end against a built binary (`atmos build`) and confirm sections 1 and 2 behave as documented

---

## Dependencies & Execution Order

- **Phase 1** → **Phase 2** → stories → **Phase 8**.
- **Phase 2** blocks every story. Inside it, T003→T004, T005→T006, T007 and T008 can run in parallel with them (T007 before T008's wiring).
- **US1** (Phase 3) is the MVP and must finish first, because US2, US4 and US5 reuse its prune function and wiring.
- **US2** depends on US1 (extends the prune function and the finalize path).
- **US3** depends only on Phase 2, so it can run in parallel with US1/US2 (different code area: validation).
- **US4** is verification; run it after US1 to catch regressions early and again at the end.
- **US5** depends on Phase 2 and US1 (`resolveAffectedFilter` needs the plumbing); it can run in parallel with US2.
- Within each story: tests first, confirm failing, then implementation.

## Parallel Opportunities

- Phase 2: T003, T005 and T007 in parallel (different files).
- US1: T009 and T010 in parallel (independent test blocks), same file, so coordinate edits.
- US2: T015, T016 and T017 in parallel.
- US3 and US5 test writing in parallel with US2.
- Phase 8: T031 to T034 in parallel (different files).

## Implementation Strategy

1. **MVP**: Phases 1, 2 and 3 (US1). This fixes the reported CI case with `--filter`. Stop and validate with the quickstart section 2.
2. Add US3 next (small, closes the silent-failure hole), then US2 (dependents and flatten), then US5, with US4 checks after each step.
3. Finish with Phase 8. The PR is `minor`, so the blog post and roadmap entry are mandatory CI checks.
