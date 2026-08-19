---

description: "Task list for Atmos Pro Command-Execution Metadata Upload — remaining work"
---

# Tasks: Atmos Pro Command-Execution Metadata Upload

**Input**: Design documents from `/specs/002-pro-exec-metadata/`

**Prerequisites**: plan.md (third re-plan, 2026-08-19), spec.md, research.md, data-model.md,
contracts/interactions.md, quickstart.md

**Tests**: Included — the constitution's Test-First principle (III) is NON-NEGOTIABLE and
CLAUDE.md's Bug-Fixing Workflow requires a failing regression test before any fix.

**Already shipped, no tasks generated for these** (verified present and correct in the
current tree):
- The `CaptureSync`/`CaptureAsync` dedup fix (`pkg/proexec/classify.go`, `async.go` —
  `IsSyncCommand` shared predicate).
- The `terraform deploy` sync-allowlist join (`syncAllowlist` includes
  `"atmos terraform deploy"`).
- The `Command`/`Args` shape fix from the second re-plan: `Command` strips the `atmos` root
  (`internal/exec/terraform.go:224`, `pkg/proexec/async.go:114-117`); `Args` is populated
  from `info.ComponentFromArg`/`cmd.Flags().Args()` instead of always empty.
- The `Flags []string` field itself exists on `dtos.ExecUploadRequest`
  (`pkg/pro/dtos/exec.go`) and flows through `buildRecord`/`CaptureSync`/`CaptureAsync`.

**Organization**: Tasks are grouped by user story. This regeneration (third re-plan,
2026-08-19) covers four still-open gaps, verified against the current tree:
1. **US1 scope (this delta)**: `Flags` source-of-truth bug — `captureExecMetadataSync`
   (`internal/exec/terraform.go:239`) still sources `Flags` from
   `info.AdditionalArgsAndFlags`, which structurally cannot contain atmos-recognized flags
   like `-s`/`--stack` and has `--upload-status` stripped out of it before capture. A
   regression test already reproduces this and is RED on current HEAD
   (`internal/exec/terraform_exec_metadata_flags_test.go`).
2. **US1 scope (adjacent)**: the async path's `commandArgsAndFlags`
   (`pkg/proexec/async.go:121-123`) serializes every flag as a `--name value` pair, which
   now conflicts with the 2026-08-19 clarification locking in bare-token serialization
   (no synthesized value for bool flags).
3. **US2 scope**: multi-component aggregation (FR-006a) — `captureExecMetadataSync` still
   fires once per graph node from inside `ExecuteTerraform` (`internal/exec/terraform.go:199`),
   with no gate on `wasMultiComponentExecution` (`cmd/terraform/utils.go:66`); not once per
   CLI invocation as FR-006a requires.
4. **US3 scope**: structured infrastructure-change data (FR-006) — both sync-path call
   sites (`internal/exec/terraform.go:239`, `internal/exec/describe_affected.go:372`) still
   always pass `nil, nil` for `data`/`dataItems`; the already-existing
   `WithStdoutCapture`/`ParsePlanOutput`/`ParseApplyOutput` plumbing (research.md
   Decision 12) is not yet wired to it.

**Also tracked, not yet root-caused**: a second production symptom (two `atexec_*` DB rows
for one invocation, different `workflow_job` ids) was investigated this session but not
conclusively resolved from static inspection — see Polish phase T026. It is **not** part
of any user-story scope above until an end-to-end test confirms whether it is a live bug.

## Format: `[ID] [P?] [Story] Description`

---

## Phase 1: Setup

No setup tasks — no new packages, dependencies, or project structure changes (plan.md
Scale/Scope).

---

## Phase 2: Foundational (Blocking Prerequisites)

No foundational tasks remain — the `Flags []string` DTO field, `buildRecord` signature, and
`CaptureSync`/`CaptureAsync` signature threading from the second re-plan's Phase 2 are
already shipped (see "Already shipped" above). Phase 3+ tasks below build directly on that
existing plumbing.

---

## Phase 3: User Story 1 - Automatic visibility into CI command execution (Priority: P1) 🎯 MVP

**Goal**: Every qualifying `atmos` command reports a correct, complete base execution
record. This delta closes the last open gap: `Flags` must reflect every flag the user
actually passed (FR-003b, 2026-08-19 clarification), not an empty or partial array.

**Independent Test**: Run `atmos terraform plan cdn -s plat-use2-dev --upload-status` in CI
with Atmos Pro configured; inspect the logged request body and confirm `flags` =
`["-s", "plat-use2-dev", "--upload-status"]` (bare tokens, `--upload-status` included, no
synthesized value) — quickstart.md step 8.

### Tests for User Story 1 ⚠️

- [x] T001 [US1] Regression test in `internal/exec/terraform_exec_metadata_flags_test.go`
  (`TestCaptureExecMetadataSync_FlagsReflectRealInvocation`) asserting the uploaded
  record's `Flags` is non-empty for an invocation shaped like the production report
  (`AdditionalArgsAndFlags` empty, mirroring what `buildPlanSubcommandArgs` leaves behind
  after stripping `--upload-status`). **Already added and confirmed RED this session.**
- [ ] T002 [P] [US1] Table-driven test in `pkg/proexec/async_test.go` asserting
  `commandArgsAndFlags` serializes a bool-typed flag (e.g. `--upload-status`,
  `Value.Type() == "bool"`) as a bare token with no synthesized value, while a
  value-bearing flag (e.g. `-s`/`--stack`) still contributes both its token and value as
  separate array entries — reproduces the current `["--upload-status", "true"]` defect
  before fixing it
- [ ] T003 [P] [US1] Extend `TestCaptureExecMetadataSync_ComponentAndFlags`
  (`internal/exec/terraform_exec_metadata_test.go`) — currently only asserts
  `NotPanics` — to also assert the `Flags` passed to `proexec.CaptureSync` reflects the
  real invocation's flags, not `info.AdditionalArgsAndFlags`

### Implementation for User Story 1

- [ ] T004 [US1] In `internal/exec/terraform.go`, thread the invoking `*cobra.Command`
  down to `captureExecMetadataSync` (new parameter, or an equivalent accessor already
  available at the `ExecuteTerraform` call site) and source `Flags` from
  `cmd.Flags().Visit` (`Changed == true`) instead of `info.AdditionalArgsAndFlags`, as bare
  tokens exactly as typed — matching `pkg/proexec/async.go`'s `commandArgsAndFlags`
  pattern. Makes T001 pass. (depends on: none — all prerequisite plumbing already shipped)
- [ ] T005 [US1] In `pkg/proexec/async.go`'s `commandArgsAndFlags`, change the flag
  serialization (`flags = append(flags, "--"+f.Name, f.Value.String())`) to skip appending
  a value when `f.Value.Type() == "bool"`, appending only the flag token in that case.
  Makes T002 pass.
- [ ] T006 [US1] Extract the corrected flag-serialization logic from T004/T005 into one
  shared helper (e.g. `pkg/proexec.FlagsAsTyped(cmd *cobra.Command) []string`) if the two
  call sites' logic would otherwise diverge — both `internal/exec/terraform.go` and
  `pkg/proexec/async.go` must produce identical output for the same `*cobra.Command`
  (research.md Decision 14). Skip this task only if T004's implementation already calls
  into `pkg/proexec` directly without duplicating T005's logic.

**Checkpoint**: User Story 1's execution records now carry a correct, complete `Flags`
field for every qualifying command, independent of US2/US3.

---

## Phase 4: User Story 2 - Reliable reporting for critical operations (Priority: P2)

**Goal**: A multi-component `--affected`/`--all` `plan`/`apply`/`deploy` invocation
produces exactly one execution record for the whole run (FR-006a), not one per graph node.

**Independent Test**: Run `atmos terraform plan --affected` against a stack with 2+
affected components; confirm exactly one `POST /v1/atmos/exec` upload attempt is logged
for the whole invocation (quickstart.md step 6).

### Tests for User Story 2 ⚠️

- [ ] T007 [P] [US2] Test in `cmd/terraform/utils_test.go` asserting a multi-component
  graph run (`wasMultiComponentExecution == true`) triggers exactly one
  `proexec.CaptureSync` call after the whole graph completes, not one per node
- [ ] T008 [P] [US2] Test in `internal/exec/terraform_test.go` asserting
  `captureExecMetadataSync`/`ExecuteTerraform` does NOT call `proexec.CaptureSync`
  directly when invoked as part of a multi-component graph run (the aggregator in
  `cmd/terraform/utils.go` owns the single call instead)

### Implementation for User Story 2

- [ ] T009 [US2] In `internal/exec/terraform.go`, gate the existing per-node
  `captureExecMetadataSync` call (line 199) behind `!wasMultiComponentExecution` (or an
  equivalent signal passed in), so single-component invocations are unaffected but
  multi-component graph runs skip the per-node call
- [ ] T010 [US2] In `cmd/terraform/utils.go`, add a per-node result accumulator
  (component, stack, exit code) alongside the existing `wasMultiComponentExecution`
  bookkeeping, and after the graph run completes, fire exactly one `proexec.CaptureSync`
  call with the aggregated results folded into `dataItems` (one entry per component, per
  data-model.md's aggregation shape) (depends on T009)
- [ ] T011 [US2] Verify/update `quickstart.md` step 6's expected log output still matches
  (one upload, not N) after T009/T010

**Checkpoint**: Multi-component runs now report exactly one execution record, independent
of US1's `Flags` fix and US3's structured-data work.

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

- [ ] T012 [P] [US3] Test in `cmd/terraform/plan_test.go` asserting the existing
  `WithStdoutCapture` buffer is captured whenever the exec-metadata gate
  (`telemetry.IsCI() && Pro-configured`) is open, decoupled from the `ciMode`/Native-CI gate
- [ ] T013 [P] [US3] Test in `internal/exec/terraform_test.go` asserting
  `captureExecMetadataSync` passes non-nil `data`/`dataItems` (parsed via
  `terraform.ParsePlanOutput`/`ParseApplyOutput`) when a captured stdout buffer is
  available, matching `TerraformExecData`'s shape (data-model.md)

### Implementation for User Story 3

- [ ] T014 [US3] In `cmd/terraform/plan.go`/`apply.go`/`deploy.go`, decouple the existing
  `WithStdoutCapture`/`WithStderrCapture` buffer construction from the `ciMode` gate so it
  also runs whenever the exec-metadata gate is open (research.md Decision 12)
- [ ] T015 [US3] Thread the captured, ANSI-stripped buffer into `ExecuteTerraform` (new
  `opts` param) and, inside `captureExecMetadataSync`, parse it via the now-public
  `pkg/ci/plugins/terraform.ParsePlanOutput`/`ParseApplyOutput` into `data`/`dataItems`
  before calling `proexec.CaptureSync` (depends on T004, T009)
- [ ] T016 [US3] For the multi-component aggregation path (US2, `cmd/terraform/utils.go`),
  fold each node's parsed `TerraformExecData` into the single aggregate record's
  `dataItems` (one entry per component per resource action, per data-model.md) instead of
  discarding it (depends on T010, T015)

**Checkpoint**: All three user stories are independently functional; `plan`/`apply`/
`deploy` execution records now carry full structured infrastructure-change data, correctly
aggregated for multi-component runs, with a correct and complete `Flags` field.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [ ] T017 [P] Regenerate the Pact contract:
  `go test -tags pact ./pkg/pro/... -v -run TestPact/UploadExecMetadata`, then
  `git diff pacts/atmos-AtmosPro.json` to confirm the `flags` example reflects bare-token
  serialization (contracts/interactions.md)
- [ ] T018 [P] Run `atmos test --coverage` for `pkg/proexec/`, `pkg/pro/`,
  `internal/exec/`, `cmd/terraform/` and confirm the 85% coverage floor holds after
  T001–T016
- [ ] T019 Manually walk `quickstart.md` steps 1–8 end-to-end against a local Pro stub or
  test workspace
- [ ] T020 `atmos lint --changed` and `go build ./...` across all touched files
- [ ] T021 **End-to-end regression test for the still-open duplicate-row question
  (2026-08-19 production report, research.md Decision 14's "Open" note).** Two `atexec_*`
  DB rows were observed for what should be a single
  `atmos terraform plan cdn -s plat-use2-dev --upload-status` invocation — one row
  (sync-path shape) with populated `atmos_version`/OS/arch/metrics but empty `flags`
  (root-caused and fixed by T001/T004 above), the other (thin shape) with `flags`
  populated but `atmos_version`/OS/arch/metrics all null and a *different* `workflow_job`
  id. Static inspection of `pkg/proexec/classify.go`, `async.go`, and
  `internal/exec/terraform.go:199-242` did not turn up an obvious live double-fire bug —
  both the sync path's `"atmos terraform "+subCommand` and the async path's
  `cmd.CommandPath()` should produce the same allowlist match for a plain
  `terraform plan` — but this was not proven with a real end-to-end run, and the thin
  row's shape (all envelope fields null) is inconsistent with what current
  `buildRecord`/`ExecUploadRequest` can produce (no `omitempty` on those fields), so
  either a stale client build or a still-undiscovered live gap remains open. Write an
  integration test that:
  - Drives the real `atmos terraform plan {component} -s {stack} --upload-status`
    invocation through the actual `cmd.Execute()` (`cmd/root.go:1817`) — not just
    `RootCmd.Execute()` — since the async `proexec.CaptureAsync` hook only fires from
    that top-level wrapper. Note `Execute()` reads `os.Args[1:]` directly
    (`cfg.EarlyConfigAndStacksInfoFromArgs(os.Args[1:])`), so the test must mutate
    `os.Args` (save/restore), not just `RootCmd.SetArgs`.
  - Points `Settings.Pro.BaseURL`/`ATMOS_PRO_BASE_URL` at an `httptest.NewServer` fake,
    with `CI=true` and `ATMOS_PRO_TOKEN` set so `gateOpen` passes.
  - Runs against the existing `tests/fixtures/scenarios/terraform-generate-planfile`
    `mock`/`component-1` fixture (`tests.RequireTerraform(t)`-gated) so no real cloud
    credentials are needed.
  - Asserts the fake server received **exactly one** `POST .../atmos/exec` request for
    the invocation (would catch a live double-fire regression) with correctly populated
    `flags`/`atmos_version`/`atmos_os`/`atmos_arch`/`metrics` (including
    `major_page_faults`), AND **exactly one**
    `POST .../repos/{owner}/{repo}/instances` request (the independent
    `--upload-status` mechanism, `internal/exec/pro.go:221`) with the correct
    `stack`/`component`/`exit_code`.
  - If this test passes cleanly on current HEAD, the production duplicate is confirmed to
    be a stale-build artifact (predates commit `2fe4fabe0`) rather than a live bug, and
    this task can close as verification-only.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)** / **Foundational (Phase 2)**: Empty — all prerequisite plumbing
  already shipped.
- **US1 (Phase 3)**: No dependencies — independently testable/deliverable as-is; T001 is
  already written and RED.
- **US2 (Phase 4)**: No dependencies on US1 — independently testable; T010's aggregate
  `CaptureSync` call benefits from US1's `Flags` fix (T004) but does not require it to be
  functionally correct.
- **US3 (Phase 5)**: Depends on T004 (US1) and T009 (US2), since T015/T016 modify the same
  call sites — implement after US1 and US2 land.
- **Polish (Phase 6)**: T017–T020 depend on all three user stories. T021 (duplicate-row
  end-to-end test) has no code dependency on US1/US2/US3 and can run in parallel with them,
  though running it after T004 lands is recommended so the exec-metadata side of its
  assertions reflects the fixed `Flags` shape.

### Parallel Opportunities

- T002–T003 (US1 tests) are `[P]` — different files
- T007–T008 (US2 tests) are `[P]` — different files
- T012–T013 (US3 tests) are `[P]` — different files
- US1 (Phase 3) and US2 (Phase 4) implementation can proceed in parallel once T004/T009 are
  coordinated (both touch `internal/exec/terraform.go`'s `captureExecMetadataSync` — same
  function, different concerns: flags source vs. multi-component gating)
- US3 (Phase 5) should start after US1/US2 land, since T015/T016 build directly on T004's/
  T009's call-site shape
- T021 can be worked independently of T001–T020 by a different session/agent

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 3 (US1 — `Flags` source-of-truth fix): T001 (done) → T002–T003 (tests)
   → T004–T006 (implementation)
2. **STOP and VALIDATE**: run T001, confirm GREEN; run quickstart.md step 8
3. This alone resolves the newest production symptom (empty `flags` column) — ship it
   independently of US2/US3/T021 if desired

### Incremental Delivery

1. Phase 3 (US1 — this delta) → validate → ship
2. Phase 4 (US2) → validate (single upload for multi-component runs) → ship
3. Phase 5 (US3) → validate (structured data present) → ship
4. Phase 6 (Polish) → Pact contract regenerated, coverage confirmed, quickstart walked
   end-to-end, and T021 resolves the duplicate-row open question
