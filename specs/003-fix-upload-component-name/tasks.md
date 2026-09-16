# Tasks: Full Component Name in Atmos Pro Uploads

**Input**: Design documents from `/specs/003-fix-upload-component-name/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/upload-component-identity.md, quickstart.md

**Tests**: INCLUDED — mandated by spec FR-007 and Constitution III (bug fixes MUST begin with a failing test). Test tasks precede fix tasks within each story and MUST fail before the fix lands.

**Organization**: Grouped by user story. US1 (instance-status upload) and US2 (multi-component execution record) touch disjoint files and are fully parallelizable; US3 is a regression guard.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: US1 / US2 / US3 from spec.md

## Phase 1: Setup

**Purpose**: Confirm a green baseline so failing tests are attributable to the bug, not the environment.

- [X] T001 Verify baseline: `go build ./...` and `go test ./internal/exec/ -run 'TestUploadStatus' ./cmd/terraform/ -run 'ExecMetadata'` pass on branch `1199-fix-upload-component-name` before any change (run as two separate `go test` invocations)

---

## Phase 2: Foundational

**Purpose**: No blocking prerequisites — the codebase, mocks (`pro.AtmosProAPIClientInterface`, `git.GitRepoInterface`), and test seams already exist (plan.md Constitution Check II). Phase intentionally empty.

**Checkpoint**: Baseline green → user stories can proceed, US1 and US2 in parallel.

---

## Phase 3: User Story 1 — Single-component upload reports full name (Priority: P1) 🎯 MVP

**Goal**: `--upload-status` instance-status payload carries the full logical component name (`foo/bar/baz`), fixing the issue #3102 repro and the Atmos Pro approvals-page "No previous plan found" mismatch.

**Independent Test**: `go test ./internal/exec/ -run 'TestUploadStatus' -v` — nested-name case asserts `InstanceStatusUploadRequest.Component == "foo/bar/baz"`.

- [X] T002 [P] [US1] Add failing table cases to `TestUploadStatus` in internal/exec/pro_test.go: (a) nested — `info{ComponentFromArg: "foo/bar/baz", Component: "baz", Stack: "plat-use2-dev", SubCommand: "plan"}` asserting the mock-captured `dto.Component == "foo/bar/baz"`; (b) flat control — `info{ComponentFromArg: "vpc", Component: "vpc"}` asserting `dto.Component == "vpc"`; run and confirm case (a) FAILS with got `"baz"`
- [X] T003 [US1] Fix site 1 in internal/exec/pro.go (~line 254): change `Component: info.Component,` to `Component: info.ComponentFromArg,` in the `InstanceStatusUploadRequest` literal inside `uploadStatus`; preserve surrounding comments
- [X] T004 [US1] Verify: `go test ./internal/exec/ -run 'TestUploadStatus' -v` — all cases (new and pre-existing) PASS

**Checkpoint**: The exact issue #3102 repro is fixed — MVP deliverable.

---

## Phase 4: User Story 2 — Multi-component execution records report full names (Priority: P2)

**Goal**: Each per-node entry in the aggregate execution record of `--affected`/`--all`/`--components`/`--query` runs carries the node's full logical name.

**Independent Test**: `go test ./cmd/terraform/ -run 'ExecMetadata' -v` — nested-name `recordExecResult` case asserts the accumulated entry's `component == "foo/bar/baz"`.

- [X] T005 [P] [US2] Add failing test to cmd/terraform/utils_exec_metadata_test.go: call `nodeHooks.recordExecResult` with `info{Stack: "dev", Component: "baz", ComponentFromArg: "foo/bar/baz", ComponentType: "terraform"}` and assert the accumulated results entry has `component == "foo/bar/baz"`; include a flat control assertion (existing fixtures with `Component == ComponentFromArg` already cover it — reference one explicitly); run and confirm the nested case FAILS with got `"baz"`
- [X] T006 [US2] Fix site 2 in cmd/terraform/utils.go (~line 581): change `buildTerraformExecData(n.subCommand, ansi.Strip(output), info.Component, info.Stack, exitCode)` to pass `info.ComponentFromArg`; preserve surrounding comments
- [X] T007 [US2] Verify: `go test ./cmd/terraform/ -run 'ExecMetadata' -v` — all cases PASS, pre-existing fixtures unmodified

**Checkpoint**: Both truncation sites fixed; contract table rows 1–2 now ✅.

---

## Phase 5: User Story 3 — Regression guard for already-correct channels (Priority: P3)

**Goal**: Lock in the channels that are correct today and prove nothing else moved.

**Independent Test**: guard test passes both before AND after the Phase 3/4 fixes; existing working-dir and pact suites pass unmodified.

- [X] T008 [P] [US3] Add regression-guard test in cmd/terraform/utils_exec_metadata_test.go: `terraformExecMetadataParserFunc("foo/bar/baz", "dev")` closure invoked with a covered subcommand ("plan"), asserting the wrapped entry's `component == "foo/bar/baz"` (guards spec FR-004; must pass before and after the fixes)
- [X] T009 [P] [US3] Verify no truncated-value assumptions elsewhere: `go test ./pkg/pro/ -v` (pact fixtures use flat names — expect PASS unmodified per research.md Decision 5) and confirm `internal/exec/utils.go` splitting logic (~1106–1116) has zero diff (`git diff internal/exec/utils.go` is empty)

---

## Phase 6: Polish & Merge Readiness

**Purpose**: Repo-mandated quality gates and the FR-008 follow-up-tracking requirement.

- [X] T010 Full verification: `go build ./...`, `atmos test`, `atmos lint --changed`; then `atmos test --full` before opening the PR
- [X] T011 [P] Open the FR-008 follow-up GitHub issue on cloudposse/atmos: single-component execution records report the raw filesystem path for path-style args (`execComponent = args[0]` captured in RunE before `resolveComponentPath` rewrites `info.ComponentFromArg` — cmd/terraform/plan.go:91–95, apply.go, deploy.go); reference contract exception in specs/003-fix-upload-component-name/contracts/upload-component-identity.md — opened as cloudposse/atmos#3110
- [ ] T012 Run the `/pull-request` skill and open the PR: label `patch` (no blog post / roadmap needed), body follows the what/why/references template, `Closes #3102`, links the T011 issue by number, and notes the corrected two-site root cause (research.md Decision 1) so the issue history stays accurate

---

## Dependencies

```text
T001 (baseline)
 ├─→ US1: T002 → T003 → T004 ─┐
 └─→ US2: T005 → T006 → T007 ─┤   (US1 ∥ US2 — disjoint files)
          US3: T008 ∥ T009 ───┤   (T008 anytime after T001; T009 after T003+T006)
                              └─→ T010 → T012
                                  T011 ∥ (anytime after T001, before T012)
```

- US1 and US2 are fully independent (internal/exec/pro.go+pro_test.go vs cmd/terraform/utils.go+utils_exec_metadata_test.go).
- T008 is independent of the fixes (guards an untouched path); T009 requires both fixes present.
- T011 has no code dependency; T012 requires everything.

## Parallel Execution Examples

- After T001: run T002, T005, T008, T011 concurrently (four disjoint work items).
- T003 and T006 in parallel once their respective failing tests are confirmed.

## Implementation Strategy

**MVP = Phase 3 (US1) alone**: fixes the reported repro and the user-visible approvals-page failure. US2 completes the bug class; US3 locks the perimeter. Given the diff size (2 production lines), the pragmatic path is a single PR delivering all phases, executed in task order with the test-first gates observed literally (each fix task starts only after its failing test is demonstrated).
