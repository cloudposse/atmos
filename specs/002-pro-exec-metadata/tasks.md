---

description: "Task list for Atmos Pro Command-Execution Metadata Upload — remaining work"
---

# Tasks: Atmos Pro Command-Execution Metadata Upload

**Input**: Design documents from `/specs/002-pro-exec-metadata/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/interactions.md, quickstart.md

**Tests**: Included — the constitution's Test-First principle (III) is NON-NEGOTIABLE and CLAUDE.md's Bug-Fixing Workflow requires a failing regression test before any fix.

**Already shipped, no tasks generated for these**: the `CaptureSync`/`CaptureAsync`
dedup fix (`pkg/proexec/classify.go`, `async.go` — verified present and correct in the
current tree) and the `terraform deploy` sync-allowlist join (`pkg/proexec/classify.go`'s
`syncAllowlist` already includes `"atmos terraform deploy"`). Do not re-open these.

**Organization**: Tasks are grouped by user story. This regeneration covers three
still-open gaps, verified against the current tree:
1. **US1 scope**: the `Command`/`Args`/`Flags` shape fix (FR-003a/FR-003b) — `Args` is
   currently always empty (`maskArgs(nil)`, `pkg/proexec/envelope.go:59`), `Command` is
   inconsistent between the async path (`cmd.CommandPath()`, includes `atmos` root) and
   the sync path (already `"terraform "+subCommand`, no root — `internal/exec/terraform.go:234`
   and `internal/exec/describe_affected.go:372`), and there is no `Flags` field at all.
2. **US2 scope**: multi-component aggregation (FR-006a) — `captureExecMetadataSync` still
   fires once per graph node from inside `ExecuteTerraform` (`internal/exec/terraform.go:199`),
   not once per CLI invocation.
3. **US3 scope**: structured infrastructure-change data (FR-006) — `captureExecMetadataSync`
   still always passes `nil, nil` for `data`/`dataItems` (`internal/exec/terraform.go:234`);
   the already-existing `WithStdoutCapture`/`ParsePlanOutput`/`ParseApplyOutput` plumbing
   (research.md Decision 12) is not yet wired to it.

## Format: `[ID] [P?] [Story] Description`

---

## Phase 1: Setup

No setup tasks — no new packages, dependencies, or project structure changes (plan.md
Scale/Scope).

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Shared DTO/plumbing changes that every user-story phase below depends on.

**⚠️ CRITICAL**: T001–T003 MUST complete before any Phase 3+ task in the same file.

- [ ] T001 Add `Flags []string` field (`json:"flags"`) to `ExecUploadRequest` in `pkg/pro/dtos/exec.go`, alongside the existing `Args []string` field
- [ ] T002 Extend `buildRecord`'s signature in `pkg/proexec/envelope.go` to accept `args []string` and `flags []string` parameters (replacing the hardcoded `maskArgs(nil)` call at line 59), masking both through the existing `maskArgs` path before assigning to `dtos.ExecUploadRequest.Args`/`.Flags`
- [ ] T003 Update `CaptureSync`'s signature in `pkg/proexec/sync.go` and `CaptureAsync`'s internals in `pkg/proexec/async.go` (`uploadExecMetadata`) to accept and thread `args []string`/`flags []string` through to `buildRecord` (T002)

**Checkpoint**: `buildRecord` can now populate `Args`/`Flags` correctly once callers pass real data — Phase 3 wires up the actual callers.

---

## Phase 3: User Story 1 - Automatic visibility into CI command execution (Priority: P1) 🎯 MVP

**Goal**: Every qualifying `atmos` command reports a correct, complete base execution
record — including this delta's `Command`/`Args`/`Flags` shape fix (FR-003b) and the
explicit non-coordination with `uploadStatus` (FR-003a).

**Independent Test**: Run `atmos terraform plan cdn -s plat-use2-dev --upload-status` in
CI with Atmos Pro configured; inspect the logged request body and confirm `command` =
`"terraform plan"`, `args` = `["cdn"]`, `flags` = `["-s", "plat-use2-dev", "--upload-status"]`
(quickstart.md step 8).

### Tests for User Story 1 ⚠️

- [ ] T004 [P] [US1] Table-driven test in `pkg/proexec/envelope_test.go` asserting `buildRecord` populates `Args` from positional arguments and `Flags` from CLI flags, never combining them, and never leaving `Args` empty when positional arguments were passed — reproduces the current always-empty-`Args` bug before fixing it
- [ ] T005 [P] [US1] Test in `pkg/proexec/async_test.go` asserting `CaptureAsync`'s `Command` strips the `atmos` root segment (e.g. `"terraform plan"`, not `"atmos terraform plan"`), matching the sync path's existing shape
- [ ] T006 [P] [US1] Test in `internal/exec/terraform_test.go` asserting `captureExecMetadataSync` passes the invocation's component (`info.ComponentFromArg`) as `Args` and `info.AdditionalArgsAndFlags` (flag-shaped entries only) as `Flags`
- [ ] T007 [P] [US1] Test in `internal/exec/describe_affected_test.go` asserting `describe affected`'s `CaptureSync` call passes `Args`/`Flags` consistent with the same shape (empty `Args` is correct here — `describe affected` has no positional component)

### Implementation for User Story 1

- [ ] T008 [US1] In `pkg/proexec/async.go`'s `CaptureAsync`, derive `Command` by stripping the leading `atmos` root segment from `cmd.CommandPath()` (e.g. via `strings.TrimPrefix`), and derive `args`/`flags` from `cmd.Flags().Args()` (positional) and changed flags (`cmd.Flags().Visit`), masked, then pass through to `uploadExecMetadata`/`buildRecord` (depends on T001–T003)
- [ ] T009 [US1] In `internal/exec/terraform.go`'s `captureExecMetadataSync`, pass `[]string{info.ComponentFromArg}` (when non-empty) as `args` and a flags-only filter of `info.AdditionalArgsAndFlags` as `flags` to `proexec.CaptureSync` (depends on T001–T003)
- [ ] T010 [US1] In `internal/exec/describe_affected.go`'s `Execute`, pass `nil`/appropriately-empty `args`/`flags` to `proexec.CaptureSync` explicitly (no positional component applies) (depends on T001–T003)
- [ ] T011 [US1] Update any existing test in `pkg/proexec/*_test.go` / `internal/exec/terraform_test.go` that asserted the old always-empty-`Args` or `atmos`-prefixed-`Command` shape — these are now incorrect and must be updated, not preserved (plan.md Testing note)

**Checkpoint**: User Story 1's execution records now carry correct, cross-path-correlatable `Command`/`Args`/`Flags` for every qualifying command, independent of US2/US3.

---

## Phase 4: User Story 2 - Reliable reporting for critical operations (Priority: P2)

**Goal**: A multi-component `--affected`/`--all` `plan`/`apply`/`deploy` invocation
produces exactly one execution record for the whole run (FR-006a), not one per graph node.

**Independent Test**: Run `atmos terraform plan --affected` against a stack with 2+
affected components; confirm exactly one `POST /v1/atmos/exec` upload attempt is logged
for the whole invocation (quickstart.md step 6).

### Tests for User Story 2 ⚠️

- [ ] T012 [P] [US2] Test in `cmd/terraform/utils_test.go` asserting a multi-component graph run (`wasMultiComponentExecution == true`) triggers exactly one `proexec.CaptureSync` call after the whole graph completes, not one per node
- [ ] T013 [P] [US2] Test in `internal/exec/terraform_test.go` asserting `captureExecMetadataSync`/`ExecuteTerraform` does NOT call `proexec.CaptureSync` directly when invoked as part of a multi-component graph run (the aggregator in `cmd/terraform/utils.go` owns the single call instead)

### Implementation for User Story 2

- [ ] T014 [US2] In `internal/exec/terraform.go`, gate the existing per-node `captureExecMetadataSync` call (line 199) behind `!wasMultiComponentExecution` (or an equivalent signal passed in), so single-component invocations are unaffected but multi-component graph runs skip the per-node call (depends on T001–T003)
- [ ] T015 [US2] In `cmd/terraform/utils.go`, add a per-node result accumulator (component, stack, exit code) alongside the existing `wasMultiComponentExecution` bookkeeping, and after the graph run completes, fire exactly one `proexec.CaptureSync` call with the aggregated results folded into `dataItems` (one entry per component, per data-model.md's aggregation shape) (depends on T001–T003, T014)
- [ ] T016 [US2] Verify/update `quickstart.md` step 6's expected log output still matches (one upload, not N) after T014/T015

**Checkpoint**: Multi-component runs now report exactly one execution record, independent of US1's Command/Args/Flags fix and US3's structured-data work.

---

## Phase 5: User Story 3 - Structured infrastructure-change data for plan/apply/deploy (Priority: P3)

**Goal**: `terraform plan`/`apply`/`deploy` execution records carry itemized
created/updated/deleted/replaced/moved/imported resources, outputs, and warnings (FR-006),
using the already-existing stdout-capture plumbing (research.md Decision 12), not a new
tee mechanism.

**Independent Test**: Run `atmos terraform plan` against a component with pending
changes; confirm the uploaded execution record's `data`/`data_items` contain the resource
counts/addresses visible in the plan's own output (spec.md SC-007).

### Tests for User Story 3 ⚠️

- [ ] T017 [P] [US3] Test in `cmd/terraform/plan_test.go` asserting the existing `WithStdoutCapture` buffer is captured whenever the exec-metadata gate (`telemetry.IsCI() && Pro-configured`) is open, decoupled from the `ciMode`/Native-CI gate
- [ ] T018 [P] [US3] Test in `internal/exec/terraform_test.go` asserting `captureExecMetadataSync` passes non-nil `data`/`dataItems` (parsed via `terraform.ParsePlanOutput`/`ParseApplyOutput`) when a captured stdout buffer is available, matching `TerraformExecData`'s shape (data-model.md)

### Implementation for User Story 3

- [ ] T019 [US3] In `cmd/terraform/plan.go`/`apply.go`/`deploy.go`, decouple the existing `WithStdoutCapture`/`WithStderrCapture` buffer construction from the `ciMode` gate so it also runs whenever the exec-metadata gate is open (research.md Decision 12)
- [ ] T020 [US3] Thread the captured, ANSI-stripped buffer into `ExecuteTerraform` (new `opts` param) and, inside `captureExecMetadataSync`, parse it via the now-public `pkg/ci/plugins/terraform.ParsePlanOutput`/`ParseApplyOutput` into `data`/`dataItems` before calling `proexec.CaptureSync` (depends on T009, T014)
- [ ] T021 [US3] For the multi-component aggregation path (US2, `cmd/terraform/utils.go`), fold each node's parsed `TerraformExecData` into the single aggregate record's `dataItems` (one entry per component per resource action, per data-model.md) instead of discarding it (depends on T015, T020)

**Checkpoint**: All three user stories are independently functional; `plan`/`apply`/`deploy` execution records now carry full structured infrastructure-change data, correctly aggregated for multi-component runs, with correct `Command`/`Args`/`Flags`.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [ ] T022 [P] Regenerate the Pact contract: `go test -tags pact ./pkg/pro/... -v -run TestPact/UploadExecMetadata`, then `git diff pacts/atmos-AtmosPro.json` to confirm the `flags` field and corrected `command`/`args` shape appear (contracts/interactions.md)
- [ ] T023 [P] Run `atmos test --coverage` for `pkg/proexec/`, `pkg/pro/`, `internal/exec/`, `cmd/terraform/` and confirm the 85% coverage floor holds after T001–T021
- [ ] T024 Manually walk `quickstart.md` steps 1–8 end-to-end against a local Pro stub or test workspace
- [ ] T025 `atmos lint --changed` and `go build ./...` across all touched files

---

## Dependencies & Execution Order

### Phase Dependencies

- **Foundational (Phase 2, T001–T003)**: No dependencies — BLOCKS all of Phase 3/4/5's implementation tasks (tests may be written first, per Test-First, but will fail to compile against the new signatures until T001–T003 land)
- **US1 (Phase 3)**: Depends on Phase 2 only — independently testable/deliverable as-is
- **US2 (Phase 4)**: Depends on Phase 2 only — independently testable; T015's aggregate `CaptureSync` call benefits from US1's `Args`/`Flags` shape but does not require it to be functionally correct
- **US3 (Phase 5)**: Depends on Phase 2, and on T009/T014 (US1/US2's call-site shape) since T020/T021 modify the same call sites — implement after US1 and US2 land
- **Polish (Phase 6)**: Depends on all three user stories

### Parallel Opportunities

- T001–T003 are sequential (same files, layered signature changes) — not parallelizable
- T004–T007 (US1 tests) are `[P]` — different files
- T012–T013 (US2 tests) are `[P]` — different files
- T017–T018 (US3 tests) are `[P]` — different files
- US1 (Phase 3) and US2 (Phase 4) implementation can proceed in parallel once Phase 2 completes, since they touch different call sites (`async.go`/`terraform.go`'s per-node call vs. `cmd/terraform/utils.go`'s aggregator) — but both eventually touch `internal/exec/terraform.go`'s `captureExecMetadataSync`, so coordinate T009 and T014 if worked by different people
- US3 (Phase 5) should start after US1/US2 land, since T020/T021 build directly on T009/T014's call-site shape

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 2 (Foundational)
2. Complete Phase 3 (US1 — Command/Args/Flags fix)
3. **STOP and VALIDATE**: run quickstart.md step 8, confirm the shape fix via `ATMOS_LOGS_LEVEL=Debug`
4. This alone resolves the reported production bug's most actionable symptom (always-empty `Args`) and the cross-feature correlation gap (FR-003a/b) — ship it independently of US2/US3 if desired

### Incremental Delivery

1. Phase 2 → Phase 3 (US1) → validate → ship
2. Phase 4 (US2) → validate (single upload for multi-component runs) → ship
3. Phase 5 (US3) → validate (structured data present) → ship
4. Phase 6 (Polish) → Pact contract regenerated, coverage confirmed, quickstart walked end-to-end
