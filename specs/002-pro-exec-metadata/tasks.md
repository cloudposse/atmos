---

description: "Task list for Atmos Pro Command-Execution Metadata Upload"
---

# Tasks: Atmos Pro Command-Execution Metadata Upload

**Input**: Design documents from `/specs/002-pro-exec-metadata/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/interactions.md, quickstart.md

**Tests**: Included. The repository constitution (Principle III, NON-NEGOTIABLE) requires
test-first development and an 80%+ coverage floor enforced by CodeCov — this is not
optional for this feature.

**Organization**: Tasks are grouped by user story (US1/US2/US3, matching spec.md
priorities P1/P2/P3) so each story is independently implementable and testable. The Pact
contract task and doc/lint/coverage polish are cross-cutting and sit in the final phase.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: US1, US2, or US3 — maps to spec.md's prioritized user stories
- File paths are exact per plan.md's Project Structure section

> **Regeneration note (2026-08-12)**: This task list was regenerated after a spec
> clarification reversed the payload-size decision: command-specific structured data
> (`DataItems`) MUST now be split across correlated requests (chunking, reusing
> `pkg/pro/chunked_upload.go`'s `sendChunked`/`BatchInfo`, the same mechanism as
> `describe affected`'s `UploadAffectedStacks`) instead of being truncated/trimmed
> client-side. Tasks below already marked `[X]` in the prior list but invalidated by this
> change are marked **REWORK** with the specific delta required; their `[X]` is downgraded
> to `[ ]`. Tasks unaffected by the change keep their `[X]` and prior wording. See
> `research.md` Decision 6, `data-model.md`'s `ExecutionRecord`, and `plan.md`'s
> "Superseded design note" for full context.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Create the new package skeletons and error sentinels this feature needs.

- [X] T001 Create `pkg/proexec/` package with `doc.go` (package comment) and
  `gate.go`, `envelope.go`, `async.go`, `sync.go` files (one concern per file, per
  plan.md's Project Structure and the constitution's 600-line-file limit)
  — **Note**: the originally-planned `truncate.go` file is superseded; per the batching
  redesign, chunking logic lives in `pkg/pro/api_client_exec.go` (mirroring
  `UploadAffectedStacks`, which likewise has no `pkg/proexec`-side chunking file), so no
  replacement file is needed here — see T009 (REMOVE) below.
- [X] T002 [P] Create `pkg/metrics/process/` package with `doc.go`, `metrics.go`,
  `metrics_unix.go` (`//go:build unix`), `metrics_windows.go` (`//go:build windows`)
- [X] T003 [P] Add sentinel errors to `errors/errors.go`: `ErrFailedToUploadExecMetadata`,
  `ErrExecPayloadTooLarge`, `ErrExecSyncTimeout` (per the mandatory static-error pattern
  — no dynamic `errors.New`/`fmt.Errorf` without `%w` at call sites). `ErrExecPayloadTooLarge`
  is retained for the still-possible case of a single non-chunkable field (envelope/`Data`)
  itself exceeding the limit, which chunking cannot fix since only `DataItems` is chunked.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: DTOs, client method, metrics primitives, gate logic, and config plumbing
that every user story below depends on.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T004 [P] **REWORK** Add `DataItems []json.RawMessage` (`omitempty`), `BatchID
  string` (`omitempty`), `BatchIndex *int` (`omitempty`), `BatchTotal *int` (`omitempty`)
  fields to `ExecUploadRequest` in `pkg/pro/dtos/exec.go`, per data-model.md's updated
  `ExecutionRecord` field table (the existing `Data`/`Metrics`/envelope fields are
  unchanged and stay as previously implemented)
- [X] T005 **REWORK** Update `UploadExecMetadata` in `pkg/pro/api_client_exec.go`: when
  `dto.DataItems` is empty or the whole marshaled record fits under `c.MaxPayloadBytes`,
  send exactly one request (existing single-request path is unchanged); otherwise call
  `pro.sendChunked(dto.DataItems, c.MaxPayloadBytes, overhead, sendFn)` — same pattern as
  `UploadAffectedStacks` (`pkg/pro/api_client.go:304`) and `UploadInstances`
  (`pkg/pro/api_client_instances.go:44`) — where `sendFn` sends a copy of the full
  envelope/`Metrics`/`Data` plus that chunk's `DataItems` slice and `BatchID`/
  `BatchIndex`/`BatchTotal` set from the `*pro.BatchInfo`. Update the function's doc
  comment (currently states records are "never chunked") to describe the new behavior.
  Remove the `//nolint` justification tied to the old "always one request" assumption if
  no longer applicable (depends on T004)
- [X] T006 [P] `ProcessMetrics` struct, `Snapshot` type, `Baseline()`, and
  `(Snapshot) Since() ProcessMetrics` in `pkg/metrics/process/metrics.go` (shared,
  platform-agnostic surface), with the Unix body (`syscall.Getrusage(RUSAGE_SELF, ...)`)
  in `pkg/metrics/process/metrics_unix.go` and the reduced Windows body
  (`GetProcessTimes`/`GetProcessMemoryInfo` via `golang.org/x/sys/windows`, wall time +
  CPU time only) in `pkg/metrics/process/metrics_windows.go`, per research.md Decision 3
  — unaffected by the batching redesign
- [X] T007 `gateOpen(atmosConfig *schema.AtmosConfiguration) bool` in
  `pkg/proexec/gate.go` — `telemetry.IsCI() && proConfigured(atmosConfig)`, where
  `proConfigured` checks `Settings.Pro.Token != ""` OR full `GithubOIDC` +
  `WorkspaceID` settings present, WITHOUT constructing an `AtmosProAPIClient` (no network
  call), per research.md Decision 4 (depends on T004) — unaffected by the batching redesign
- [X] T008 **REWORK** Update `buildRecord` in `pkg/proexec/envelope.go` to accept a second
  parameter, `dataItems []any` (alongside the existing `data any`), mask+marshal each
  independently into `Data json.RawMessage` / `DataItems []json.RawMessage` on the
  returned `*dtos.ExecUploadRequest`, and **remove the call to `truncateIfNeeded`
  entirely** — no size-based trimming happens during envelope assembly anymore; chunking
  (T005) happens later, inside `UploadExecMetadata`, over the already-built `DataItems`
  (depends on T004, T006)
- [X] T009 **REMOVE** `pkg/proexec/truncate.go` (and `pkg/proexec/truncate_test.go` if it
  exists) in full — `truncationMarker`, `maxWarningsKept`, `truncateIfNeeded`,
  `marshaledSize`, `trimDataFields` and their tests are all superseded by chunking; no
  code path should construct a `"... truncated"` marker or silently drop `Warnings`
  entries anymore (FR-011 clarification) (depends on T008)
- [X] T010 [P] `ExecSettings{ SyncTimeoutSeconds int }` nested under
  `schema.ProSettings.Exec` in `pkg/schema/pro.go`; `ATMOS_PRO_EXEC_SYNC_TIMEOUT_SECONDS`
  → `settings.pro.exec.sync_timeout_seconds` bound in `pkg/config/load.go` following the
  existing `settings.pro.token`/`workspace_id` viper pattern; env var name constant in
  `pkg/config/const.go` — unaffected by the batching redesign; the same setting now also
  bounds the total wait across all chunks of a batched delivery, not just a single request
  (documentation-only implication, see T022)
- [X] T011 Package-level `var processBaseline = process.Baseline()` in
  `pkg/proexec/async.go`, initialized at package load (process startup), shared by both
  `CaptureAsync` and `CaptureSync` — unaffected by the batching redesign

**Checkpoint**: DTOs, client method, metrics primitives, gate check, envelope assembly,
and config are all in place (with `Data`/`DataItems` split and chunked delivery). User
story implementation can now begin.

---

## Phase 3: User Story 1 - Automatic visibility into CI command execution (Priority: P1) 🎯 MVP

**Goal**: Every `atmos` command run in a CI environment with Atmos Pro configured
automatically and asynchronously reports an execution record, with zero impact on commands
outside that gate.

**Independent Test**: Run `atmos version` (or any non-critical command) with `CI=true` and
`ATMOS_PRO_TOKEN` set, and confirm (via debug logs / a test double `AtmosProAPIClient`)
that `UploadExecMetadata` is called with a correct envelope; confirm it is NOT called
without `CI=true`, and NOT called without Pro configured.

### Tests for User Story 1

- [X] T012 [P] [US1] Unit tests for `gateOpen` (CI×Pro-configured 2×2 matrix) in
  `pkg/proexec/gate_test.go` — unaffected by the batching redesign
- [X] T013 [P] [US1] **REWORK** Unit tests for `buildRecord` in
  `pkg/proexec/envelope_test.go`: keep the field-population and secret-masking
  (`Args`/`Data`/`DataItems`) coverage; **replace** the "truncation invoked when oversized"
  case (now invalid — `buildRecord` no longer truncates) with a case asserting
  `buildRecord` performs no size-based trimming itself and simply passes `DataItems`
  through unmodified for `UploadExecMetadata` (T005/T015) to chunk (depends on T008, T009)
- [X] T014 [P] [US1] Unit tests for `CaptureAsync` — dispatches on gate-open, no-ops on
  gate-closed, does not alter caller's error/exit path on upload failure, respects the 2s
  flush ceiling (use a fake clock or a slow mock client) — in `pkg/proexec/async_test.go`
  — unaffected by the batching redesign
- [X] T015 [P] [US1] **REWORK** Unit tests for `UploadExecMetadata` in
  `pkg/pro/api_client_exec_test.go`: keep the existing success/401-refresh-retry/5xx-retry/
  400/403/404-non-retry cases (single-request path); **add** a case where `DataItems`
  exceeds `MaxPayloadBytes` and assert multiple sequential requests are sent, each with
  the correct `BatchID`/`BatchIndex`/`BatchTotal` and the full envelope/`Metrics`/`Data`
  repeated on every request, mirroring `UploadAffectedStacks`'s existing chunking test
  coverage (depends on T005)

### Implementation for User Story 1

- [X] T016 [US1] `CaptureAsync(cmd *cobra.Command, err error)` in `pkg/proexec/async.go`
  — checks `gateOpen`, launches `UploadExecMetadata` in a goroutine, blocks the caller via
  `sync.WaitGroup` + timer for a fixed 2s ceiling, then returns regardless of goroutine
  completion (depends on T007, T008, T011, T005) — signature/behavior unaffected by the
  batching redesign; internally now may issue multiple chunked requests within the same
  2s-bounded goroutine
- [X] T017 [US1] `proexec.CaptureAsync(cmd, err)` wired into `cmd/root.go` immediately
  after the existing `telemetry.CaptureCmd(cmd, err)` call (depends on T016) — unaffected
- [X] T018 [US1] Async upload failure logs at debug level only (`log.Debug(...)`, never
  `ui.*`/stderr-visible) per FR-009a, and never mutates `err` returned to the caller
  (verified by T014) — unaffected

**Checkpoint**: User Story 1 is fully functional and independently testable — every
command reports asynchronously when the gate is open, with no effect otherwise.

---

## Phase 4: User Story 2 - Reliable reporting for critical operations (Priority: P2)

**Goal**: `terraform plan`, `terraform apply`, and `describe affected` block on execution
record delivery (within a configurable, defaulted-to-10s **total** ceiling covering all
chunks) and apply a warn-and-continue failure default, so CI never reports success for a
change Atmos Pro failed to record without at least a surfaced warning.

**Independent Test**: Run `atmos terraform plan` in CI with Atmos Pro configured but
pointed at an unreachable endpoint, and confirm the command still completes (does not
hang past the timeout) and emits a warning; run the same against a working mock and
confirm the command does not exit until the upload call (including all chunks, if any)
returns.

### Tests for User Story 2

- [X] T019 [P] [US2] Unit tests for `CaptureSync` timeout clamping — a configured value
  below 10s is clamped up to 10s; a configured value above 10s is honored — in
  `pkg/proexec/sync_test.go` — unaffected: the clamp wraps one call to
  `buildRecord`+`UploadExecMetadata` regardless of whether that call issues one or several
  chunk requests internally, so the timeout is already "total, not per-chunk" by
  construction
- [X] T020 [P] [US2] Unit tests for `CaptureSync` warn-and-continue behavior on upload
  failure/timeout (returns without error to the caller, logs a warning) in
  `pkg/proexec/sync_test.go` — unaffected
- [X] T021 [P] [US2] Unit test confirming `ExecuteTerraform` invokes `proexec.CaptureSync`
  for `plan`/`apply` subcommands only (not `validate`, `output`, etc.), implemented in
  `internal/exec/terraform_exec_metadata_test.go` (`TestIsExecMetadataSyncSubcommand`,
  `TestCaptureExecMetadataSync_NoOpOutsideCI`) — unaffected by the batching redesign

### Implementation for User Story 2

- [X] T022 [US2] **REWORK** Update `CaptureSync`'s signature in `pkg/proexec/sync.go` to
  `CaptureSync(atmosConfig *schema.AtmosConfiguration, cmdName string, exitCode int, data
  any, dataItems []any) error`, threading `dataItems` through to `buildRecord` (T008).
  Update the function's doc comment to explicitly state the configured timeout bounds the
  **complete** record delivery, including every chunk if `dataItems` required batching,
  not a per-chunk wait (FR-008a clarification) — the existing `select`/`time.After`
  structure already enforces this correctly (single goroutine, single channel receive);
  only the signature and doc comment change (depends on T008)
- [X] T023 [US2] **REWORK** Update the `internal/exec/terraform.go` call site to pass
  `dataItems: nil` as the new fifth argument to `proexec.CaptureSync(...)` (placeholder
  until US3/T028 unblocks and supplies real resource-change items), matching T022's new
  signature (depends on T022)
- [X] T024 [US2] **REWORK** Update the `describe affected`
  (`internal/exec/describe_affected_helpers.go`) call site the same way — `data: nil,
  dataItems: nil` — matching T022's new signature (depends on T022)
- [X] T025 [US2] `settings.pro.exec.sync_timeout_seconds` documented in the Atmos Pro
  configuration docs under `website/docs/cli/commands/pro/` (or the settings reference
  page); `cd website && npm run build` verified — unaffected; optionally add one line
  noting the timeout covers chunked delivery as a whole, not per chunk

**Checkpoint**: User Stories 1 AND 2 both work independently — critical commands block
appropriately (for the complete, possibly-chunked delivery); all other commands remain
fire-and-forget from Phase 3.

---

## Phase 5: User Story 3 - Structured infrastructure-change data for plan/apply (Priority: P3)

**Goal**: `terraform plan`/`apply` execution records include itemized created/updated/
deleted/replaced resources, output values, and warnings — not just pass/fail status — with
the (potentially large) itemized resource list delivered via chunking rather than ever
being truncated.

**Independent Test**: Run `atmos terraform plan` against a component with pending
changes and confirm (via a test double `AtmosProAPIClient`) that the uploaded
`ExecUploadRequest`'s `Data` field carries `ResourceCounts`/`Outputs`/`Warnings` and its
`DataItems` field carries one `{action, address}` entry per changed resource, matching the
plan's own resource counts (chunked across multiple calls if the fixture is large enough);
run against a command with no structured-data extension (e.g. `atmos list components`) and
confirm both fields are absent/nil while the base envelope still sends normally.

### Tests for User Story 3

- [ ] T026 [P] [US3] Unit test confirming `ExecuteTerraform` passes the already-computed
  `*plugin.TerraformOutputData`, mapped into `CaptureSync`'s `data` (summary fields) and
  `dataItems` (flattened `{action, address}` resource-change list) arguments for
  `plan`/`apply`, and that the resulting counts/outputs/warnings match the parsed plan
  output, in `internal/exec/terraform_test.go`
- [X] T027 [P] [US3] Unit test confirming `buildRecord` correctly omits/nils both `Data`
  and `DataItems` when called with `data = nil, dataItems = nil` (e.g. from `describe
  affected` or the async path for commands with no structured data) in
  `pkg/proexec/envelope_test.go` — implemented as `TestBuildRecord_NilDataOmittedFromJSON`;
  extend to also assert `DataItems` omission once T008's rework lands

### Implementation for User Story 3

**⚠️ BLOCKED — not implemented. Deferred to [cloudposse/atmos#2924](https://github.com/cloudposse/atmos/issues/2924).**
Two layered findings: (1) `pkg/ci/internal/plugin.TerraformOutputData` is Go-internal, but
this turned out NOT to be the real blocker — `pkg/ci/plugins/terraform` (which produces
it) is a *public* package exporting `ParsePlanOutput`/`ParseApplyOutput`, and since
`OutputResult.Data` is typed `any`, callers can hold/pass the value through `any` without
naming the internal type. (2) The actual blocker: `ExecuteTerraform` (where the
`CaptureSync` hook lives) never captures raw plan/apply stdout text at all — that only
happens in a separate, per-component graph pipeline in `cmd/terraform/utils.go` used for
the Native-CI-gated job-summary feature. Fixing this needs a `MultiWriter`-based stdout
tee added to `ExecuteTerraform`'s shared pipeline (there is `WithStdoutOverride` prior art
in `terraform_plan_diff.go`) without breaking streaming/TTY/masking behavior for every
terraform subcommand that shares it — a real, separately-scoped change. See the issue for
full detail.

- [ ] T028 [US3] **UPDATED TARGET** In `internal/exec/terraform.go`'s `ExecuteTerraform`,
  once unblocked: thread the `*plugin.TerraformOutputData` already produced for Native CI
  job summaries (see `pkg/ci/internal/plugin`/`pkg/ci/plugins/terraform/parser.go`)
  through as **two** arguments to the T023 `proexec.CaptureSync(...)` call — its bounded
  summary fields (`ResourceCounts`, `Outputs`, `HasOutputChanges`, `ChangedResult`,
  `Warnings`) as `data`, and its six resource-address slices (`CreatedResources`/
  `UpdatedResources`/`ReplacedResources`/`DeletedResources`/`MovedResources`/
  `ImportedResources`) flattened into one `[]any` of `{action, address}` items as
  `dataItems`, per data-model.md's updated `TerraformExecData` mapping (depends on T023)
  — **BLOCKED, see note above**
- [X] T029 [US3] `buildRecord` (`pkg/proexec/envelope.go`) confirmed to produce a request
  with `Data` entirely absent from the marshaled JSON when `data = nil`
  (`json.RawMessage`+`omitempty`, verified) — extend the same assertion to `DataItems`
  once T008's rework lands (depends on T008)

**Checkpoint**: All three user stories are independently functional. `terraform plan`/
`apply` records now carry rich structured data (summary + chunked itemized list);
everything else is unaffected.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: The Pact contract deliverable (FR-013/SC-006), documentation, and final
quality gates — cuts across all three user stories rather than belonging to one.

- [X] T030 [P] **REWORK** Update the existing "9th interaction" test cases in
  `pkg/pro/consumer_pact_test.go` (`//go:build pact`) — `TestPact_UploadExecMetadata` and
  `TestPact_UploadExecMetadata_NoData` — to include `data_items` alongside `data` in the
  populated case (still fully absent in the no-data case); **add** a new 10th interaction,
  `TestPact_UploadExecMetadata_Chunked`, covering a `DataItems` payload large enough to
  trigger 2 chunk requests, asserting `batch_id`/`batch_index`/`batch_total` are present
  and the envelope/`data` fields are repeated identically on both requests, per
  `contracts/interactions.md`'s "Batch correlation fields" section; extend
  `pkg/pro/pact_helpers_test.go` only if new shared setup is needed (depends on T005)
- [X] T031 **REWORK** Regenerate `pacts/atmos-AtmosPro.json` (now 10 interactions) via
  `go test -tags pact ./pkg/pro/... -run TestPact/UploadExecMetadata`, review the diff,
  and commit it alongside the code (depends on T030)
- [ ] T032 [P] Run `atmos test --coverage` for `pkg/proexec`, `pkg/metrics/process`,
  `pkg/pro`, and `internal/exec`; add table-driven cases for any gap below the 80% floor
  (including the new chunking paths from T005/T015/T030)
- [ ] T033 Run `atmos lint --changed` and fix all findings (gofumpt, golangci-lint,
  `godot` comment periods, cyclomatic complexity ≤15 / function length ≤60 lines)
- [ ] T034 Execute the manual validation steps in `quickstart.md` end-to-end (CI-simulated
  async run, CI-simulated sync run, negative CI-off/Pro-unconfigured runs) and record any
  discrepancies as follow-up fixes

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS all user stories. Contains the
  core batching rework (T004, T005, T008, T009) — do this before touching any story phase
- **User Story 1 (Phase 3)**: Depends on Foundational only
- **User Story 2 (Phase 4)**: Depends on Foundational only (independent of US1's `cmd/root.go`
  wiring, but shares `pkg/proexec` files — see file-level notes below)
- **User Story 3 (Phase 5)**: Depends on Foundational and on US2's T023 (extends the same
  `CaptureSync` call site with real data instead of `nil`)
- **Polish (Phase 6)**: Depends on Foundational (T005) for the Pact task; T032–T034 depend
  on all prior phases being complete

### User Story Dependencies

- **US1 (P1)**: No dependency on US2/US3 — independently shippable MVP
- **US2 (P2)**: Independent of US1 at the story level, but both touch `pkg/proexec/`
  files, so sequencing (not true parallelism) is recommended in a single-developer flow
- **US3 (P3)**: Builds directly on US2's T023 call site (adds the `data`/`dataItems`
  arguments that US2 leaves `nil`) — must follow US2

### Within Each User Story

- Tests written first, confirmed to fail, then implementation (constitution Principle III)
- `pkg/proexec` primitives (gate/envelope) before the async/sync wrappers that use them
- Wrapper implementation before call-site wiring in `cmd/root.go`/`internal/exec`

### Parallel Opportunities

- T002, T003 (Setup) run in parallel with T001
- T006, T010 (Foundational) run in parallel with the T004→T005→T008→T009 rework chain
- T013–T015 (US1 tests) run in parallel with each other once T004/T005/T008/T009 land
- T019–T021 (US2 tests) run in parallel with each other (already valid, no rework needed)
- T026–T027 (US3 tests) run in parallel with each other
- T030 and T032/T033 can run in parallel with each other once their dependencies (T005 / all prior phases) are met

---

## Parallel Example: User Story 1

```bash
# Launch all US1 tests together (after Foundational rework lands):
Task: "Unit tests for gateOpen in pkg/proexec/gate_test.go"
Task: "Unit tests for buildRecord (Data/DataItems split) in pkg/proexec/envelope_test.go"
Task: "Unit tests for CaptureAsync in pkg/proexec/async_test.go"
Task: "Unit tests for UploadExecMetadata (single-request + chunked) in pkg/pro/api_client_exec_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL — blocks all stories; includes the batching
   rework T004/T005/T008/T009)
3. Complete Phase 3: User Story 1
4. **STOP and VALIDATE**: every command in CI+Pro reports async, with zero effect
  elsewhere
5. This alone satisfies SC-001/SC-002/SC-005 (async half) and is a demoable MVP

### Incremental Delivery

1. Setup + Foundational (with batching rework) → foundation ready
2. Add US1 → validate independently → MVP demoable (async visibility for all commands)
3. Add US2 → validate independently → critical commands now block reliably on the
   complete (possibly chunked) delivery
4. Add US3 → validate independently → plan/apply records gain structured summary + chunked
   itemized data
5. Polish (Pact contract handoff including the chunked interaction, docs, coverage, lint)
   → ready for `/pull-request`

### Notes

- [P] tasks touch different files and have no unmet dependencies
- Commit after each task or logical group, per CLAUDE.md's git guidance
- Stop at each phase checkpoint to validate that story independently before continuing
- Avoid: vague tasks, same-file conflicts within a "parallel" batch, story ordering that
  breaks US1's independent shippability
- The Foundational-phase rework (T004/T005/T008/T009) must land before any US1/US2/US3
  test task that references `Data`/`DataItems` — do not parallelize rework tasks against
  story test tasks that depend on them
