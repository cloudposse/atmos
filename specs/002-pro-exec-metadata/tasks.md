---

description: "Task list for Atmos Pro Command-Execution Metadata Upload — 2026-08-18 delta"

---

# Tasks: Atmos Pro Command-Execution Metadata Upload (Delta)

**Input**: Design documents from `/specs/002-pro-exec-metadata/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, quickstart.md (all
re-planned 2026-08-18)

**Tests**: Included. The repository constitution (Principle III, NON-NEGOTIABLE) requires
test-first development — this is not optional. This delta additionally starts from a
reproduced production defect (duplicate records), so several tasks are explicitly
regression tests written to fail against today's shipped code before the fix lands.

**Organization**: US1/US2/US3 below map to spec.md's priorities exactly as before. This
task list covers ONLY the 2026-08-18 delta on top of the already-shipped, already-merged
US1/US2 base implementation and the previously-blocked US3 — it does not re-list
already-`[X]`-complete work from the original `tasks.md` (T001–T027, T029–T031, all
still valid and unaffected by this delta unless called out below).

> **Regeneration note (2026-08-18)**: Regenerated after the `/speckit-clarify` session
> that (a) confirmed a production defect — sync-allowlisted commands were producing two
> execution records per invocation — and (b) changed scope: multi-component
> `--affected`/`--all` runs must report one aggregate record instead of one per
> component, and `terraform deploy` joins the synchronous allowlist with its own
> structured data. It also corrects the User Story 3 blocker recorded in
> [#2924](https://github.com/cloudposse/atmos/issues/2924): the raw stdout capture US3
> needs already exists (`WithStdoutCapture`/`WithStderrCapture`, already wired in
> `cmd/terraform/plan.go`/`apply.go`) — the gap is plumbing, not a new tee mechanism. See
> research.md Decisions 10–12 and plan.md's Summary for full context.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: No new packages or dependencies are needed for this delta — it is entirely
targeted fixes/extensions to already-shipped files. This phase creates the one new file
needed as the shared foundation for the dedup fix.

- [ ] T001 Create `pkg/proexec/classify.go` with package doc comment and an empty
  `IsSyncCommand(commandPath string) bool` stub (implementation in T004) — the single
  new file this delta introduces, replacing `internal/exec/terraform.go`'s private
  `isExecMetadataSyncSubcommand`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The shared sync-allowlist predicate that both the async call site
(`cmd/root.go`) and the sync call site (`internal/exec/terraform.go`) must consult so
they can never independently drift out of sync again (research.md Decision 10 — this was
the root cause of the production duplicate-record defect).

**⚠️ CRITICAL**: No user-story work below can begin until T002–T005 are complete.

- [ ] T002 [P] Regression test reproducing the duplicate-record defect: given a fake
  `cobra.Command` for `terraform plan` and a mocked `AtmosProAPIClientInterface`, assert
  that calling `proexec.CaptureAsync(cmd, err)` followed by
  `internal/exec.captureExecMetadataSync(...)` for the same invocation today results in
  `UploadExecMetadata` being called **twice** — in `pkg/proexec/async_test.go`, written to
  FAIL against current code before T003–T005 land, confirming the bug before the fix
  (constitution Principle III bug-fix workflow)
- [ ] T003 [P] Unit test matrix for `proexec.IsSyncCommand` covering all four allowlisted
  command paths (`"atmos terraform plan"`, `"atmos terraform apply"`, `"atmos terraform
  deploy"`, `"atmos describe affected"`) returning `true`, and representative non-allowlisted
  paths (`"atmos version"`, `"atmos terraform validate"`, `"atmos terraform output"`)
  returning `false` — in `pkg/proexec/classify_test.go`
- [ ] T004 Implement `proexec.IsSyncCommand(commandPath string) bool` in
  `pkg/proexec/classify.go` — matches on the four allowlisted full command paths (depends
  on T001, T003)
- [ ] T005 Remove `internal/exec/terraform.go`'s private `isExecMetadataSyncSubcommand`
  and update `captureExecMetadataSync` to call `proexec.IsSyncCommand("terraform " +
  info.SubCommand)` instead — update `internal/exec/terraform_exec_metadata_test.go`'s
  `TestIsExecMetadataSyncSubcommand` accordingly (depends on T004)

**Checkpoint**: The single source of truth for the sync allowlist exists and is proven
correct in isolation. User-story work can now begin.

---

## Phase 3: User Story 2 - Reliable reporting for critical operations (Priority: P2)

**Goal**: Fix the dedup defect (exactly one record per sync-allowlisted invocation, per
FR-007's 2026-08-18 clarification and the new Acceptance Scenario 4), add `terraform
deploy` to the synchronous allowlist, and aggregate multi-component `--affected`/`--all`
runs into one record per invocation instead of one per component (FR-006a).

**Independent Test**: Run `atmos terraform plan <component> -s <stack>` in CI with Atmos
Pro configured (or a mocked client in a unit test) and confirm `UploadExecMetadata` is
called exactly once, not twice. Run `atmos terraform plan --affected` against a fixture
with 3+ affected components and confirm exactly one `UploadExecMetadata` call for the
whole run. Run `atmos terraform deploy <component> -s <stack>` and confirm it now blocks
on / reports upload outcome like `plan`/`apply` do.

### Tests for User Story 2

- [ ] T006 [P] [US2] Unit test: `cmd/root.go`'s `Execute()` does NOT call
  `proexec.CaptureAsync` when `proexec.IsSyncCommand(cmd.CommandPath())` is true, for each
  of the four allowlisted commands — in `cmd/root_exec_metadata_test.go` (new file;
  depends on T004)
- [ ] T007 [P] [US2] Unit test: for a non-allowlisted command, `cmd/root.go`'s `Execute()`
  still calls `proexec.CaptureAsync` exactly once (no regression to US1) — same file as
  T006
- [ ] T008 [P] [US2] Unit test: `internal/exec/terraform.go`'s `isExecMetadataSyncSubcommand`
  replacement returns true for `deploy` — extend
  `internal/exec/terraform_exec_metadata_test.go`'s existing subcommand-classification
  test table (depends on T005)
- [ ] T009 [P] [US2] Unit test: a mocked multi-component graph run (3 components, one
  failing) through `cmd/terraform/utils.go`'s `terraformNodeHooks` results in exactly one
  `proexec.CaptureSync` call after the graph completes, with `dataItems` containing 3
  entries (one per component, each with its own `exitCode`) — in
  `cmd/terraform/utils_exec_metadata_test.go` (new file)

### Implementation for User Story 2

- [ ] T010 [US2] In `cmd/root.go`'s `Execute()`, guard the existing
  `proexec.CaptureAsync(cmd, err)` call (line ~1913) with `if
  !proexec.IsSyncCommand(cmd.CommandPath()) { proexec.CaptureAsync(cmd, err) }` (depends
  on T004, T006, T007)
- [ ] T011 [US2] Add `terraform deploy` to `cmd/terraform/deploy.go`: call
  `captureExecMetadataSync`-equivalent behavior by ensuring `deploy` routes through the
  same `ExecuteTerraform`/`executeSingleComponent` path `plan`/`apply` already use for
  single-component runs (verify `deploy.go`'s `RunE` already delegates to
  `terraformRunWithOptions` → `ExecuteTerraform`; if so this is confirmation-only, no new
  code) (depends on T005, T008)
- [ ] T012 [US2] In `cmd/terraform/utils.go`, extend `terraformNodeHooks` with an
  accumulator field (e.g. `componentResults []proexec.ComponentResult`, a small new
  exported struct in `pkg/proexec/classify.go` or `envelope.go` holding
  `Component/Stack/ExitCode/Data`) appended to in `AfterWithWriters` for each node
  (depends on T004)
- [ ] T013 [US2] In the multi-component graph run's top-level orchestration function
  (where `wasMultiComponentExecution` is set — `cmd/terraform/utils.go`), after the graph
  scheduler returns, call `proexec.CaptureSync(atmosConfig, "terraform "+subCommand,
  aggregateExitCode, nil, accumulatedDataItems)` exactly once using the
  `terraformNodeHooks` accumulator from T012, and remove/guard the now-redundant
  per-node `captureExecMetadataSync` call inside `ExecuteTerraform` so it does not also
  fire for graph-node invocations (single-component `executeSingleComponent` calls are
  unaffected — they still call it directly since there is no separate aggregation point)
  (depends on T012, T009)
- [ ] T014 [P] [US2] `settings.pro.exec` docs and CLI allowlist mentions in
  `website/docs/cli/commands/pro/` (or wherever the sync-allowlist is documented) updated
  to list `terraform deploy` alongside `plan`/`apply`/`describe affected`; `cd website &&
  npm run build` verified

**Checkpoint**: Every sync-allowlisted invocation (including multi-component and
`deploy`) produces exactly one execution record; every other command remains
fire-and-forget async, exactly once.

---

## Phase 4: User Story 3 - Structured infrastructure-change data for plan/apply/deploy (Priority: P3)

**Goal**: Attach itemized created/updated/deleted/replaced/moved/imported resources,
output values, and warnings to `plan`/`apply`/`deploy` execution records — previously
blocked by #2924, now unblocked per research.md Decision 12 (reuse the existing
`WithStdoutCapture` tee already wired in `cmd/terraform/plan.go`/`apply.go`/`deploy.go`,
decoupled from the unrelated `ciMode` gate, instead of inventing a new pipeline-wide tee).

**Independent Test**: Run `atmos terraform plan` against a component with pending changes
(CI+Pro gate open) and confirm the uploaded `ExecUploadRequest`'s `Data` field carries
`ResourceCounts`/`Outputs`/`Warnings` and `DataItems` carries one `{action, address}`
entry per changed resource, matching the plan's own resource counts — without requiring
`--ci`/`ATMOS_CI` to be set (proving the capture no longer depends on the unrelated
Native-CI gate).

### Tests for User Story 3

- [ ] T015 [P] [US3] Unit test: `cmd/terraform/plan.go`'s stdout/stderr capture
  (`WithStdoutCapture`/`WithStderrCapture`) now fires whenever the exec-metadata gate
  (`telemetry.IsCI() && Pro-configured`) is open, independent of `ciMode` — assert the
  captured buffer is non-empty and parsed even when `--ci`/`ATMOS_CI` are unset, in
  `cmd/terraform/plan_exec_metadata_test.go` (new file)
- [ ] T016 [P] [US3] Unit test: `ExecuteTerraform` receives the captured plan/apply/deploy
  output (via a new opts-carried value or parameter), calls the now-public
  `terraform.ParsePlanOutput`/`ParseApplyOutput`, and maps the result's `.Data` into
  `CaptureSync`'s `data` (`ResourceCounts`/`Outputs`/`HasOutputChanges`/`ChangedResult`/
  `Warnings`) and `dataItems` (flattened `{action, address}` list) arguments, matching
  data-model.md's `TerraformExecData` mapping — in `internal/exec/terraform_test.go`
- [ ] T017 [P] [US3] Unit test: `buildRecord` with a populated `dataItems` produces the
  expected `DataItems []json.RawMessage` on the marshaled `ExecUploadRequest` (already
  covered for the `nil` case by the shipped `TestBuildRecord_NilDataOmittedFromJSON`;
  this is the populated-case counterpart) — in `pkg/proexec/envelope_test.go`

### Implementation for User Story 3

- [ ] T018 [US3] In `cmd/terraform/plan.go`/`apply.go`/`deploy.go`, decouple the existing
  `stdoutBuf`/`stderrBuf` capture from the `ciMode` gate: always construct the buffers and
  pass `WithStdoutCapture`/`WithStderrCapture`, independent of whether `ciMode` is true
  (Native CI job-summary hooks continue to use the same buffer unchanged; this is
  additive, not a behavior change to the existing `ciMode`-gated consumer) (depends on
  T015)
- [ ] T019 [US3] Add a new `ShellCommandOption`-adjacent mechanism (or reuse the existing
  captured buffer directly via a new `ExecuteTerraform` parameter) to thread the
  ANSI-stripped captured output from `cmd/terraform/plan.go`/`apply.go`/`deploy.go` down
  to `internal/exec/terraform.go`'s `captureExecMetadataSync` (depends on T018)
- [ ] T020 [US3] In `internal/exec/terraform.go`'s `captureExecMetadataSync`, for
  `plan`/`apply`/`deploy` subcommands, call `terraform.ParsePlanOutput`/`ParseApplyOutput`
  on the threaded captured output and pass the mapped `data`/`dataItems` into
  `proexec.CaptureSync(...)` instead of today's hardcoded `nil, nil` (depends on T016,
  T019)
- [ ] T021 [US3] Same wiring as T020 for the multi-component aggregation path (T013): each
  `terraformNodeHooks` accumulator entry (T012) additionally carries that node's own
  `Data`/parsed resource-change items, so the final aggregate `CaptureSync` call's
  `dataItems` includes structured data per component, not just identity/exit-code
  (depends on T013, T020)
- [ ] T022 [US3] `buildRecord` (`pkg/proexec/envelope.go`) — confirm no change needed
  beyond T017's test coverage; `Data`/`DataItems` marshaling already handles populated
  values correctly per the shipped US1 implementation

**Checkpoint**: `plan`/`apply`/`deploy` execution records (single- and multi-component)
carry full structured infrastructure-change data. Update/close
[#2924](https://github.com/cloudposse/atmos/issues/2924) referencing this checkpoint.

---

## Phase 5: Polish & Cross-Cutting Concerns

- [ ] T023 [P] Update `pacts/atmos-AtmosPro.json`/`contracts/interactions.md` only if the
  multi-component aggregated `DataItems` shape (component-tagged items, T012/T013)
  warrants a distinct Pact example beyond the existing single-request/chunked
  interactions — otherwise confirm the existing shape already covers it and no change is
  needed
- [ ] T024 [P] Run `atmos test --coverage` for `pkg/proexec`, `internal/exec`,
  `cmd/terraform`, and `cmd` (root); add table-driven cases for any gap below the 80%
  floor introduced by this delta
- [ ] T025 Run `atmos lint --changed` and fix all findings (gofumpt, golangci-lint,
  `godot` comment periods, cyclomatic complexity ≤15 / function length ≤60 lines) —
  `cmd/terraform/utils.go`'s aggregation changes (T012/T013) are the most likely spot to
  need extraction into named helpers per CLAUDE.md's complexity-refactoring pattern
- [ ] T026 Execute the updated manual validation steps in `quickstart.md` end-to-end
  (single-command exactly-once check, multi-component aggregate check, `deploy` allowlist
  check) and record any discrepancies as follow-up fixes
- [ ] T027 Update or close [cloudposse/atmos#2924](https://github.com/cloudposse/atmos/issues/2924)
  referencing this delta's Phase 4 checkpoint, since the originally-filed blocker
  (new `MultiWriter` tee) was superseded by reusing existing capture infrastructure
  (research.md Decision 12)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS both user-story phases below.
  Contains the shared `IsSyncCommand` predicate (T002–T005) that both the dedup fix and
  the deploy-allowlist addition depend on
- **User Story 2 (Phase 3)**: Depends on Foundational only
- **User Story 3 (Phase 4)**: Depends on Foundational AND on US2's T011 (`deploy`
  allowlist membership) and T013 (the aggregation call site T021 extends) — must follow
  US2 in this delta, unlike the original feature where US3 only depended on US2's T023
- **Polish (Phase 5)**: Depends on both user-story phases being complete

### User Story Dependencies

- **US2 (P2)**: No dependency on US3 in this delta — independently shippable (fixes the
  duplicate-record defect and adds `deploy`/aggregation even if US3's structured-data
  wiring is deferred further)
- **US3 (P3)**: Builds directly on US2's T011 (deploy allowlist) and T013 (aggregation
  call site) — must follow US2

### Parallel Opportunities

- T002, T003 (Foundational tests) run in parallel with each other, and with T001
- T006, T007, T008, T009 (US2 tests) run in parallel with each other once Foundational
  lands
- T015, T016, T017 (US3 tests) run in parallel with each other once Foundational and
  US2's T011/T013 land
- T023, T024 (Polish) can run in parallel with each other once Phase 4 completes

---

## Parallel Example: User Story 2

```bash
# Launch all US2 tests together (after Foundational lands):
Task: "Unit test: cmd/root.go skips CaptureAsync for sync-allowlisted commands in cmd/root_exec_metadata_test.go"
Task: "Unit test: cmd/root.go still calls CaptureAsync once for non-allowlisted commands in cmd/root_exec_metadata_test.go"
Task: "Unit test: isExecMetadataSyncSubcommand replacement returns true for deploy in internal/exec/terraform_exec_metadata_test.go"
Task: "Unit test: multi-component graph run produces exactly one CaptureSync call in cmd/terraform/utils_exec_metadata_test.go"
```

---

## Implementation Strategy

### Fix First (US2 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL — the shared predicate that prevents the
  dedup bug from being reintroduced)
3. Complete Phase 3: User Story 2
4. **STOP and VALIDATE**: exactly one execution record per sync-allowlisted invocation
  (single- and multi-component), `deploy` now participates — this alone resolves the
  production defect and is independently deployable
5. This satisfies the corrected FR-007/FR-006a without yet delivering US3's structured
  data

### Incremental Delivery

1. Setup + Foundational → shared predicate ready
2. Add US2 → validate independently → production defect fixed, `deploy` covered,
  multi-component aggregated
3. Add US3 → validate independently → `plan`/`apply`/`deploy` records gain structured
  summary + itemized resource-change data (closes #2924)
4. Polish (docs, coverage, lint, quickstart validation, issue closure) → ready for
  `/pull-request`

### Notes

- [P] tasks touch different files and have no unmet dependencies
- Commit after each task or logical group, per CLAUDE.md's git guidance
- Stop at each phase checkpoint to validate before continuing
- T012/T013 (aggregation) are the highest-risk tasks in this delta — they change the
  call-site lifecycle for exec-metadata capture in multi-component runs; write T009's
  regression test first and keep it passing throughout
