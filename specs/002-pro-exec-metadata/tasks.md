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

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Create the new package skeletons and error sentinels this feature needs.

- [X] T001 Create `pkg/proexec/` package with `doc.go` (package comment) and empty
  `gate.go`, `envelope.go`, `async.go`, `sync.go`, `truncate.go` files (one concern per
  file, per plan.md's Project Structure and the constitution's 600-line-file limit)
- [X] T002 [P] Create `pkg/metrics/process/` package with `doc.go`, empty `metrics.go`,
  `metrics_unix.go` (`//go:build unix`), `metrics_windows.go` (`//go:build windows`)
- [X] T003 [P] Add sentinel errors to `errors/errors.go`: `ErrFailedToUploadExecMetadata`,
  `ErrExecPayloadTooLarge`, `ErrExecSyncTimeout` (per the mandatory static-error pattern
  — no dynamic `errors.New`/`fmt.Errorf` without `%w` at call sites)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: DTOs, client method, metrics primitives, gate logic, and config plumbing
that every user story below depends on.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T004 [P] Define `ExecUploadRequest` and `ExecUploadResponse` (embedding
  `dtos.AtmosApiResponse`) in `pkg/pro/dtos/exec.go`, per data-model.md's
  `ExecutionRecord`/`ResourceUsageMetrics` field tables
- [X] T005 Add `UploadExecMetadata(dto *dtos.ExecUploadRequest) error` to
  `AtmosProAPIClientInterface` in `pkg/pro/api_client.go`, and implement it in new file
  `pkg/pro/api_client_exec.go` — `POST {BaseURL}/{BaseAPIEndpoint}/atmos/exec`, following
  `UploadInstanceStatus`'s exact shape (`json.Marshal` → `doWithRetry("UploadExecMetadata",
  ..., c, defaultRetryConfig())` → `getAuthenticatedRequest` → `c.HTTPClient.Do` →
  `handleAPIResponse`) (depends on T004)
- [X] T006 [P] Implement `ProcessMetrics` struct, `Snapshot` type, `Baseline()`, and
  `(Snapshot) Since() ProcessMetrics` in `pkg/metrics/process/metrics.go` (shared,
  platform-agnostic surface), with the Unix body (`syscall.Getrusage(RUSAGE_SELF, ...)`)
  in `pkg/metrics/process/metrics_unix.go` and the reduced Windows body
  (`GetProcessTimes`/`GetProcessMemoryInfo` via `golang.org/x/sys/windows`, wall time +
  CPU time only) in `pkg/metrics/process/metrics_windows.go`, per research.md Decision 3
- [X] T007 Implement `gateOpen(atmosConfig *schema.AtmosConfiguration) bool` in
  `pkg/proexec/gate.go` — `telemetry.IsCI() && proConfigured(atmosConfig)`, where
  `proConfigured` checks `Settings.Pro.Token != ""` OR full `GithubOIDC` +
  `WorkspaceID` settings present, WITHOUT constructing an `AtmosProAPIClient` (no network
  call), per research.md Decision 4 (depends on T004)
- [X] T008 Implement `buildRecord(cmd *cobra.Command, exitCode int, metrics
  process.ProcessMetrics, data any, gitRepo git.GitRepoInterface) (*dtos.ExecUploadRequest,
  error)` in `pkg/proexec/envelope.go` — assembles the base envelope (version/OS/arch/
  command path/`ATMOS_PRO_RUN_ID`/git info, matching `internal/exec/pro.go:uploadStatus`'s
  field sourcing), runs `Args` and `data` through the existing secret-masking path before
  marshaling (FR-010), and calls the truncation step (T009) (depends on T004, T006)
- [X] T009 Implement payload-size truncation in `pkg/proexec/truncate.go` — compares the
  marshaled `ExecUploadRequest` size against `Settings.Pro.MaxPayloadBytes` (falling back
  to the existing Pro default) and, when exceeded, trims large text fields inside `Data`
  (e.g. `Warnings`, raw log text) with a `"... truncated"` marker; never chunks (FR-011,
  research.md Decision 6) (depends on T004)
- [X] T010 [P] Add `ExecSettings{ SyncTimeoutSeconds int }` nested under
  `schema.ProSettings.Exec` in `pkg/schema/pro.go`; bind
  `ATMOS_PRO_EXEC_SYNC_TIMEOUT_SECONDS` → `settings.pro.exec.sync_timeout_seconds` in
  `pkg/config/load.go` following the existing `settings.pro.token`/`workspace_id` viper
  pattern; add the env var name constant to `pkg/config/const.go`
- [X] T011 In `cmd/root.go`'s `Execute()`, capture `process.Baseline()` immediately before
  the existing `internal.Execute(RootCmd)` call (same statement grouping as the existing
  `ci.Group(...)` wrapper around it), so both the async and sync capture paths can diff
  against one shared baseline (depends on T006)
  — **Deviation**: implemented as a package-level `var processBaseline = process.Baseline()`
  in `pkg/proexec/async.go` instead, initialized at package load (process startup), which
  is at least as early as the planned call site and is shared by both `CaptureAsync` and
  `CaptureSync` without needing to thread it through `cmd/root.go`.

**Checkpoint**: DTOs, client method, metrics primitives, gate check, envelope assembly,
and config are all in place. User story implementation can now begin.

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
  `pkg/proexec/gate_test.go`
- [X] T013 [P] [US1] Unit tests for `buildRecord` (field population, secret masking
  applied to `Args`/`Data`, truncation invoked when oversized) in
  `pkg/proexec/envelope_test.go`
- [X] T014 [P] [US1] Unit tests for `CaptureAsync` — dispatches on gate-open, no-ops on
  gate-closed, does not alter caller's error/exit path on upload failure, respects the 2s
  flush ceiling (use a fake clock or a slow mock client) — in `pkg/proexec/async_test.go`
- [X] T015 [P] [US1] Unit tests for `UploadExecMetadata` (success, 401-refresh-retry, 5xx
  retry, 400/403/404 non-retry) in `pkg/pro/api_client_exec_test.go`, mirroring
  `UploadInstanceStatus`'s existing test coverage

### Implementation for User Story 1

- [X] T016 [US1] Implement `CaptureAsync(cmd *cobra.Command, err error)` in
  `pkg/proexec/async.go` — checks `gateOpen`, launches `UploadExecMetadata` in a
  goroutine, blocks the caller via `sync.WaitGroup` + timer for a fixed 2s ceiling, then
  returns regardless of goroutine completion (depends on T007, T008, T011, T005)
- [X] T017 [US1] Wire `proexec.CaptureAsync(cmd, err)` into `cmd/root.go` immediately
  after the existing `telemetry.CaptureCmd(cmd, err)` call (~line 1905) (depends on T016)
- [X] T018 [US1] Ensure an async upload failure logs at debug level only
  (`log.Debug(...)`, never `ui.*`/stderr-visible) per FR-009a, and never mutates `err`
  returned to the caller (verified by T014)

**Checkpoint**: User Story 1 is fully functional and independently testable — every
command reports asynchronously when the gate is open, with no effect otherwise.

---

## Phase 4: User Story 2 - Reliable reporting for critical operations (Priority: P2)

**Goal**: `terraform plan`, `terraform apply`, and `describe affected` block on execution
record delivery (within a configurable, defaulted-to-10s ceiling) and apply a
warn-and-continue failure default, so CI never reports success for a change Atmos Pro
failed to record without at least a surfaced warning.

**Independent Test**: Run `atmos terraform plan` in CI with Atmos Pro configured but
pointed at an unreachable endpoint, and confirm the command still completes (does not
hang past the timeout) and emits a warning; run the same against a working mock and
confirm the command does not exit until the upload call returns.

### Tests for User Story 2

- [X] T019 [P] [US2] Unit tests for `CaptureSync` timeout clamping — a configured value
  below 10s is clamped up to 10s; a configured value above 10s is honored — in
  `pkg/proexec/sync_test.go`
- [X] T020 [P] [US2] Unit tests for `CaptureSync` warn-and-continue behavior on upload
  failure/timeout (returns without error to the caller, logs a warning) in
  `pkg/proexec/sync_test.go`
- [X] T021 [P] [US2] Unit test confirming `ExecuteTerraform` invokes `proexec.CaptureSync`
  for `plan`/`apply` subcommands only (not `validate`, `output`, etc.) in
  `internal/exec/terraform_test.go`
  — **Deviation**: implemented in new file `internal/exec/terraform_exec_metadata_test.go`
  (`TestIsExecMetadataSyncSubcommand`, `TestCaptureExecMetadataSync_NoOpOutsideCI`) rather
  than the existing `terraform_test.go`, to keep the feature's tests co-located and avoid
  bloating an already-large existing test file (consistent with the 600-line file guidance).

### Implementation for User Story 2

- [X] T022 [US2] Implement `CaptureSync(atmosConfig *schema.AtmosConfiguration, cmd
  string, exitCode int, data any) error` in `pkg/proexec/sync.go` — checks `gateOpen`,
  calls `UploadExecMetadata` with a `context`/timer bounded by
  `max(Settings.Pro.Exec.SyncTimeoutSeconds, 10)` seconds, and on failure/timeout logs a
  warning via `ui.Warning`/`log.Warn` and returns `nil` (warn-and-continue default per
  data-model.md's Delivery Classification table) (depends on T007, T008, T010, T005)
- [X] T023 [US2] Call `proexec.CaptureSync(...)` from `internal/exec/terraform.go`'s
  `ExecuteTerraform`, immediately after the existing plan/apply output-parsing step,
  gated to `info.SubCommand == "plan" || info.SubCommand == "apply"` (depends on T022)
- [X] T024 [US2] Call `proexec.CaptureSync(...)` with `data = nil` from
  `internal/exec/describe_affected_helpers.go`'s `ExecuteDescribeAffected`, after affected
  stacks are computed and before the function returns (depends on T022)
- [X] T025 [US2] Document `settings.pro.exec.sync_timeout_seconds` in the Atmos Pro
  configuration docs under `website/docs/cli/commands/pro/` or the settings reference
  page (exact file per existing `settings.pro.*` doc location), then run
  `cd website && npm run build` to verify (CLAUDE.md Documentation mandate)

**Checkpoint**: User Stories 1 AND 2 both work independently — critical commands block
appropriately; all other commands remain fire-and-forget from Phase 3.

---

## Phase 5: User Story 3 - Structured infrastructure-change data for plan/apply (Priority: P3)

**Goal**: `terraform plan`/`apply` execution records include itemized created/updated/
deleted/replaced resources, output values, and warnings — not just pass/fail status.

**Independent Test**: Run `atmos terraform plan` against a component with pending
changes and confirm (via a test double `AtmosProAPIClient`) that the `Data` field of the
uploaded `ExecUploadRequest` is a populated `plugin.TerraformOutputData` matching the
plan's own resource counts; run against a command with no structured-data extension (e.g.
`atmos list components`) and confirm `Data` is absent/nil while the base envelope still
sends normally.

### Tests for User Story 3

- [ ] T026 [P] [US3] Unit test confirming `ExecuteTerraform` passes the already-computed
  `*plugin.TerraformOutputData` as `CaptureSync`'s `data` argument for `plan`/`apply`, and
  that the resource counts/outputs/warnings match the parsed plan output, in
  `internal/exec/terraform_test.go`
- [X] T027 [P] [US3] Unit test confirming `buildRecord` correctly omits/nils the `Data`
  field when called with `data = nil` (e.g. from `describe affected` or the async path
  for commands with no structured data) in `pkg/proexec/envelope_test.go`
  — implemented as `TestBuildRecord_NilDataOmittedFromJSON`.

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

- [ ] T028 [US3] In `internal/exec/terraform.go`'s `ExecuteTerraform`, thread the
  `*plugin.TerraformOutputData` already produced for Native CI job summaries (see
  `pkg/ci/internal/plugin`/`pkg/ci/plugins/terraform/parser.go`) through as the `data`
  argument to the T023 `proexec.CaptureSync(...)` call (depends on T023) — **BLOCKED, see
  note above**
- [X] T029 [US3] Confirm/adjust `buildRecord` (`pkg/proexec/envelope.go`) so a `nil` `data`
  argument produces a request with `Data` entirely absent from the marshaled JSON
  (`omitempty`/pointer semantics), not `null` where avoidable, matching
  contracts/interactions.md's "absent `data`" case (depends on T008) — done independent of
  T028; verified `Data json.RawMessage` with `omitempty` correctly omits the field for nil.

**Checkpoint**: All three user stories are independently functional. `terraform plan`/
`apply` records now carry rich structured data; everything else is unaffected.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: The Pact contract deliverable (FR-013/SC-006), documentation, and final
quality gates — cuts across all three user stories rather than belonging to one.

- [X] T030 [P] Add the 9th Pact interaction (`UploadExecMetadata`, `POST
  /api/v1/atmos/exec`) to `pkg/pro/consumer_pact_test.go` (`//go:build pact`), covering
  both a populated-`data` case (terraform plan shape) and an absent-`data` case, per
  `contracts/interactions.md`; extend `pkg/pro/pact_helpers_test.go` only if new shared
  setup is needed (depends on T005)
- [X] T031 Regenerate `pacts/atmos-AtmosPro.json` via
  `go test -tags pact ./pkg/pro/... -run TestPact/UploadExecMetadata`, review the diff,
  and commit it alongside the code (depends on T030)
- [ ] T032 [P] Run `atmos test --coverage` for `pkg/proexec`, `pkg/metrics/process`,
  `pkg/pro`, and `internal/exec`; add table-driven cases for any gap below the 80% floor
- [ ] T033 Run `atmos lint --changed` and fix all findings (gofumpt, golangci-lint,
  `godot` comment periods, cyclomatic complexity ≤15 / function length ≤60 lines)
- [ ] T034 Execute the manual validation steps in `quickstart.md` end-to-end (CI-simulated
  async run, CI-simulated sync run, negative CI-off/Pro-unconfigured runs) and record any
  discrepancies as follow-up fixes

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS all user stories
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
- **US3 (P3)**: Builds directly on US2's T023 call site (adds the `data` argument that
  US2 leaves `nil`/basic) — must follow US2

### Within Each User Story

- Tests written first, confirmed to fail, then implementation (constitution Principle III)
- `pkg/proexec` primitives (gate/envelope) before the async/sync wrappers that use them
- Wrapper implementation before call-site wiring in `cmd/root.go`/`internal/exec`

### Parallel Opportunities

- T002, T003 (Setup) run in parallel with T001
- T004, T006, T010 (Foundational) run in parallel; T005/T007/T008/T009/T011 have direct dependencies noted above
- T012–T015 (US1 tests) run in parallel with each other
- T019–T021 (US2 tests) run in parallel with each other
- T026–T027 (US3 tests) run in parallel with each other
- T030 and T032/T033 can run in parallel with each other once their dependencies (T005 / all prior phases) are met

---

## Parallel Example: User Story 1

```bash
# Launch all US1 tests together:
Task: "Unit tests for gateOpen in pkg/proexec/gate_test.go"
Task: "Unit tests for buildRecord in pkg/proexec/envelope_test.go"
Task: "Unit tests for CaptureAsync in pkg/proexec/async_test.go"
Task: "Unit tests for UploadExecMetadata in pkg/pro/api_client_exec_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL — blocks all stories)
3. Complete Phase 3: User Story 1
4. **STOP and VALIDATE**: every command in CI+Pro reports async, with zero effect
   elsewhere
5. This alone satisfies SC-001/SC-002/SC-005 (async half) and is a demoable MVP

### Incremental Delivery

1. Setup + Foundational → foundation ready
2. Add US1 → validate independently → MVP demoable (async visibility for all commands)
3. Add US2 → validate independently → critical commands now block reliably
4. Add US3 → validate independently → plan/apply records gain structured data
5. Polish (Pact contract handoff, docs, coverage, lint) → ready for `/pull-request`

### Notes

- [P] tasks touch different files and have no unmet dependencies
- Commit after each task or logical group, per CLAUDE.md's git guidance
- Stop at each phase checkpoint to validate that story independently before continuing
- Avoid: vague tasks, same-file conflicts within a "parallel" batch, story ordering that
  breaks US1's independent shippability
