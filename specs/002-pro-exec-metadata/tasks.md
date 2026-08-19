---

description: "Task list for Atmos Pro Command-Execution Metadata Upload — remaining work"
---

# Tasks: Atmos Pro Command-Execution Metadata Upload

**Input**: Design documents from `/specs/002-pro-exec-metadata/`

**Prerequisites**: plan.md (fourth re-plan, 2026-08-19), spec.md, research.md, data-model.md,
contracts/interactions.md, quickstart.md

**Tests**: Included — the constitution's Test-First principle (III) is NON-NEGOTIABLE and
CLAUDE.md's Bug-Fixing Workflow requires a failing regression test before any behavior
change.

**Already shipped, no tasks generated for these** (verified present and correct in the
current tree):
- The `CaptureSync`/`CaptureAsync` dedup fix (`pkg/proexec/classify.go`, `async.go` —
  `IsSyncCommand` shared predicate).
- The `terraform deploy` sync-allowlist join (`syncAllowlist` includes
  `"atmos terraform deploy"`).
- The `Command`/`Args`/`Flags` shape fix (third re-plan): `Command` strips the `atmos` root;
  `Args` holds only positional arguments; `Flags` is sourced from
  `proexec.FlagsFromCommand(cmd)` (`cmd.Flags().Visit`, canonical long-form, bare-token
  serialization) in both `internal/exec/terraform.go`'s `captureExecMetadataSync` and
  `pkg/proexec/async.go`'s `commandArgsAndFlags`.

**Still open before this delta** (verified against the current tree — unchanged by this
regeneration, carried forward from the third re-plan's Phase 4/5):
- US2 (multi-component aggregation, FR-006a): `captureExecMetadataSync`
  (`internal/exec/terraform.go:201`) still fires once per graph node, ungated by
  `wasMultiComponentExecution` (`cmd/terraform/utils.go:66`).
- US3 (structured infrastructure-change data, FR-006): both sync-path call sites
  (`internal/exec/terraform.go:249`, `internal/exec/describe_affected.go:372`) still always
  pass `nil, nil` for `data`/`dataItems` to `proexec.CaptureSync`.

**This regeneration's scope (fourth re-plan, 2026-08-19 "redo batch uploading" +
`ExecutionID` clarifications)**: Two DTO/client-level changes that touch every existing and
still-open story:
1. **`ExecutionID`** (FR-003c) — a new UUID v4 field on every execution record, generated in
   `buildRecord`.
2. **`Data` delivery redesign** (FR-011/FR-011a, research.md Decision 16) — retires the
   shipped multi-chunk `DataItems`/`BatchID`/`BatchIndex`/`BatchTotal` model in favor of a
   binary choice on the single `Data` field: inline JSON under 4 MB, or a blob URL (via new
   `POST /v1/atmos/exec/data`, `execution_id`-keyed, never chunked) at/over 4 MB.

Since both changes touch `buildRecord`/`CaptureSync`/`CaptureAsync`'s shared signature (the
`dataItems []any` parameter is removed) and `pkg/pro`'s DTOs/client (which the still-open
US2/US3 tasks depend on), they are placed in **Phase 2: Foundational** — US2/US3
implementation tasks are revised to target the new single-`data`-field shape rather than
`dataItems`.

## Format: `[ID] [P?] [Story] Description`

---

## Phase 1: Setup

No setup tasks — no new packages or dependencies (`github.com/google/uuid` is already a
direct dependency; the 4 MB threshold reuses the existing `DefaultMaxPayloadBytes` constant,
plan.md Scale/Scope).

---

## Phase 2: Foundational (Blocking Prerequisites)

**Goal**: Redesign the DTO/client layer so every execution record carries `ExecutionID` and
delivers `Data` via the inline-or-blob-URL model, before any user-story task builds
structured data on top of it.

### Tests for Foundational Phase ⚠️

- [x] T001 [P] Test in `pkg/pro/dtos/exec_test.go` (new file, or add to an existing DTO test
  file if one exists) asserting `ExecUploadRequest` marshals `execution_id` and that
  `DataItems`/`BatchID`/`BatchIndex`/`BatchTotal` no longer exist as fields (compile-time
  removal — a struct-literal compile guard, e.g. `var _ = ExecUploadRequest{ExecutionID:
  "x"}`, is sufficient alongside a marshal-shape assertion). **Done** —
  `TestExecUploadRequest_MarshalsExecutionID` + compile guard.
- [x] T002 [P] Test in `pkg/pro/dtos/exec_test.go` asserting `ExecDataUploadRequest`
  marshals `{execution_id, data}` and `ExecDataUploadResponse` unmarshals `{success, url}`
  (data-model.md's new entities). **Done** —
  `TestExecDataUploadRequest_MarshalsExecutionIDAndData`,
  `TestExecDataUploadResponse_UnmarshalsSuccessAndURL`.
- [x] T003 [P] Test in `pkg/proexec/envelope_test.go` asserting `buildRecord` sets a
  non-empty `ExecutionID` that parses as a valid UUID (`github.com/google/uuid.Parse`), and
  that two separate `buildRecord` calls produce two different `ExecutionID` values. **Done**
  — `TestBuildRecord_ExecutionIDIsFreshUUIDPerCall`.
- [x] T004 [P] Table-driven test in `pkg/pro/api_client_exec_test.go` asserting
  `UploadExecMetadata`:
  - sends exactly one `POST .../atmos/exec` request with `data` inline when the marshaled
    record is under `MaxPayloadBytes`
  - sends exactly one `POST .../atmos/exec/data` request (asserting its body has
    `execution_id` matching the record's `ExecutionID` and `data` equal to the original
    structured data) followed by exactly one `POST .../atmos/exec` request whose `data`
    field is the JSON string returned as `url`, when the marshaled record is at/over
    `MaxPayloadBytes`
  - never sends `batch_id`/`batch_index`/`batch_total` fields in either case (regression
    guard against the retired chunking model reappearing)
  **Done** — `TestUploadExecMetadata_InlineUnderThreshold`,
  `TestUploadExecMetadata_BlobURLOverThreshold` (plus direct `TestUploadExecData_Success`/
  `_Error` coverage for the new method itself).

### Implementation for Foundational Phase

- [x] T005 In `pkg/pro/dtos/exec.go`: add `ExecutionID string \`json:"execution_id"\`` to
  `ExecUploadRequest` (placed first, before `AtmosProRunID`, matching data-model.md's field
  order); remove `DataItems`, `BatchID`, `BatchIndex`, `BatchTotal`. Add new
  `ExecDataUploadRequest{ExecutionID string \`json:"execution_id"\`; Data json.RawMessage
  \`json:"data"\`}` and `ExecDataUploadResponse{AtmosApiResponse; URL string
  \`json:"url"\`}`. Makes T001/T002 pass. **Done.**
- [x] T006 In `pkg/proexec/envelope.go`'s `buildRecord`: generate `executionID :=
  uuid.New().String()` (import `github.com/google/uuid`) once per call and set it on the
  returned `ExecUploadRequest.ExecutionID`; remove the `dataItems []any` parameter and the
  now-dead `maskedDataItemsJSON` helper (masking of `data` alone is unchanged). Update the
  function signature's callers (T009). Makes T003 pass. **Done.**
- [x] T007 In `pkg/pro/api_client_exec.go`: add `UploadExecData(dto
  *dtos.ExecDataUploadRequest) (*dtos.ExecDataUploadResponse, error)` — `POST {BaseURL}/
  {BaseAPIEndpoint}/atmos/exec/data`, same `doWithRetry`/`getAuthenticatedRequest` shape as
  `sendExecMetadataRequest`, decoding the response body itself (rather than reusing
  `handleAPIResponse`, which discards the body) so the `URL` field survives. Added to
  `AtmosProAPIClientInterface` (`pkg/pro/api_client.go`). **Done.**
- [x] T008 In `pkg/pro/api_client_exec.go`'s `UploadExecMetadata`: replaced the `sendChunked`
  call with: marshal the full record (envelope + `Metrics` + `Data` inline) once via
  `json.Marshal`; if `len(marshaled) < c.MaxPayloadBytes` (falling back to
  `DefaultMaxPayloadBytes`), send it as-is via `sendExecMetadataRequest`; otherwise call
  `UploadExecData(&dtos.ExecDataUploadRequest{ExecutionID: dto.ExecutionID, Data:
  dto.Data})`, set a copy of the record's `Data` to the marshaled JSON string of the
  returned `URL`, then send that copy via `sendExecMetadataRequest`. Makes T004 pass.
  **Done.**
- [x] T009 Updated `pkg/proexec/sync.go`'s `CaptureSync` and `pkg/proexec/async.go`'s
  `CaptureAsync`/`uploadExecMetadata` to drop the `dataItems []any` parameter (folded into
  the existing `data any` parameter) and updated their calls to `buildRecord` to match T006's
  new signature. **Done.**
- [x] T010 ~~Regenerate `pkg/pro/mock_interface.go`~~ **Not needed** — verified
  `mock_interface.go` is mockgen output for the narrower `APIClient` interface
  (`pkg/pro/interface.go`, only `UploadInstances`), not `AtmosProAPIClientInterface`. No
  generated mock exists for `AtmosProAPIClientInterface`; the hand-written test fakes
  (`internal/exec/pro_test.go`'s `MockProAPIClient`, `pkg/proexec/async_test.go`'s
  `fakeUploadClient`) were updated directly with a new `UploadExecData` method instead.
- [x] T011 Updated the two existing sync call sites (`internal/exec/terraform.go:249`,
  `internal/exec/describe_affected.go:372`) to match T009's new signature (dropped the
  trailing `nil` `dataItems` argument). No other `CaptureSync`/`CaptureAsync` call sites
  exist in production code; `cmd/root.go`'s `CaptureAsync(cmd, err)` call is unaffected
  (its signature didn't change). **Done.**

**Checkpoint**: Every execution record now carries `ExecutionID`; `Data` delivery is
inline-or-blob-URL with no chunking; the codebase compiles and all existing tests pass with
the new signatures. US1/US2/US3 phases below build on this foundation.

---

## Phase 3: User Story 1 - Automatic visibility into CI command execution (Priority: P1) 🎯 MVP

**Goal**: Every qualifying `atmos` command reports a correct, complete base execution
record, now including a fresh `ExecutionID` per invocation (FR-003c). The `Flags`
source-of-truth fix from the third re-plan is already shipped (see "Already shipped" above)
— this phase only adds the `ExecutionID` verification.

**Independent Test**: Run any `atmos` command in CI with Atmos Pro configured; inspect the
logged request body and confirm `execution_id` is present, UUID-shaped, and differs between
two separate invocations (quickstart.md step 9).

### Tests for User Story 1 ⚠️

- [x] T012 [US1] Test asserting the record built for a real invocation has a non-empty,
  UUID-shaped `ExecutionID`, distinct from `AtmosProRunID`. **Done, fulfilled via the
  Foundational-phase tests rather than a new `internal/exec` file**: T003's
  `TestBuildRecord_ExecutionIDIsFreshUUIDPerCall` covers this directly at the `buildRecord`
  level (both sync and async paths converge there — see T006), and
  `TestUploadExecMetadata_DispatchesOnGateOpen` (`pkg/proexec/async_test.go`) was extended
  with a `client.lastRequest.ExecutionID` assertion to also cover the async
  `*cobra.Command`-driven path end-to-end.

### Implementation for User Story 1

- [x] T013 [US1] No production code change needed — T006 (Foundational) already makes
  `ExecutionID` flow through every `CaptureSync`/`CaptureAsync` call. T012 is GREEN without
  further changes.

**Checkpoint**: Every execution record — regardless of command — carries a correct,
complete `Flags` field (already shipped) and a fresh `ExecutionID` (this phase).

---

## Phase 4: User Story 2 - Reliable reporting for critical operations (Priority: P2)

**Goal**: A multi-component `--affected`/`--all` `plan`/`apply`/`deploy` invocation
produces exactly one execution record for the whole run (FR-006a), not one per graph node,
with per-component results folded into the single `Data` field (not `DataItems`, which no
longer exists — research.md Decision 17).

**Independent Test**: Run `atmos terraform plan --affected` against a stack with 2+
affected components; confirm exactly one `POST /v1/atmos/exec` (or
`/exec/data`-then-`/exec` pair) upload attempt is logged for the whole invocation
(quickstart.md step 6).

### Tests for User Story 2 ⚠️

- [x] T014 [P] [US2] Test asserting a multi-component graph run triggers exactly one
  `proexec.CaptureSync` call after the whole graph completes, not one per node, with `data`
  containing one entry per component. **Done, in a new file rather than `utils_test.go`** —
  `cmd/terraform/utils_exec_metadata_test.go`:
  `TestCaptureMultiComponentExecMetadata_ExactlyOneRequestForWholeRun` (end-to-end via a real
  `httptest` server, asserting exactly one `POST .../atmos/exec` with both nodes' results in
  `Data`), plus `TestTerraformNodeHooks_RecordExecResultAccumulates` and the two no-op guard
  tests (`..._NoOpWithoutNodeHooks`, `..._NoOpForNonSyncSubcommand`). Placed in a new file
  instead of growing the already very large (1492-line) `utils_hooks_test.go` further.
- [x] T015 [P] [US2] Test asserting `captureExecMetadataSync` does NOT call
  `proexec.CaptureSync` directly when invoked as part of a multi-component graph run. **Done**
  — `internal/exec/terraform_exec_metadata_flags_test.go`:
  `TestCaptureExecMetadataSync_SkipsPerNodeWhenNodeHooksWired`.

### Implementation for User Story 2

- [x] T016 [US2] In `internal/exec/terraform.go`'s `captureExecMetadataSync`, skip the
  per-node call when `info.NodeHooks != nil` (the signal `cmd/terraform/utils.go`'s
  `wirePerComponentHook` already sets for every multi-component `--affected`/`--all`/query
  run) rather than the cmd/terraform-package-local `wasMultiComponentExecution` var, which
  `internal/exec` cannot reach (`cmd/terraform` imports `internal/exec`, not the reverse).
  Verified this signal reaches `ExecuteTerraform`'s per-node call for all three
  multi-component paths (`--affected`/`--all`/query) by tracing
  `pkg/scheduler/adapters.TerraformDispatcher.Dispatch` → `executeTerraformQueryComponent`
  → `ExecuteTerraform(execution.Info, ...)`, which passes `nodeInfo` (carrying `NodeHooks`)
  through by value. **Done.**
- [x] T017 [US2] In `cmd/terraform/utils.go`: added `execNodeResult{Component, Stack,
  ExitCode}` and a mutex-guarded `results []execNodeResult` field on `terraformNodeHooks`,
  appended to by a new `recordExecResult` call at the end of `AfterWithWriters` (thread-safe
  — the scheduler may dispatch nodes concurrently). Added `captureMultiComponentExecMetadata`
  (checks `info.NodeHooks` is wired and `proexec.IsSyncCommand`, then fires exactly one
  `proexec.CaptureSync` with the accumulated `results` as `data`), called after each of the
  three multi-component branches (`info.Affected`/`info.All`/`isMultiComponentExecution`) in
  `terraformRunWithOptions` once their execution call returns. **Done.**
- [x] T018 [US2] Verified `quickstart.md` step 6's expected log output ("confirm exactly one
  upload attempt is logged for the whole invocation, not one per component") already matched
  the implemented behavior — no edit needed (it was written during `/speckit-plan` to already
  describe this target state).

**Checkpoint**: Multi-component runs now report exactly one execution record via the
redesigned `Data` field, independent of US1's `ExecutionID`/`Flags` and US3's structured
data content.

---

## Phase 5: User Story 3 - Structured infrastructure-change data for plan/apply/deploy (Priority: P3)

**Goal**: `terraform plan`/`apply`/`deploy` execution records carry itemized
created/updated/deleted/replaced/moved/imported resources, outputs, and warnings (FR-006),
in the single `Data` field — small runs inline, large runs via the new blob-URL path
(FR-011, transparently handled by `UploadExecMetadata`, Phase 2) — using the already-existing
stdout-capture plumbing (research.md Decision 12), not a new tee mechanism.

**Independent Test**: Run `atmos terraform plan` against a component with pending
changes; confirm the uploaded execution record's `data` (inline, or fetchable via the
`/exec/data`-returned URL for a large plan) contains the resource counts/addresses visible
in the plan's own output (spec.md SC-007).

**Status**: Both halves are **done**. The multi-component half (T028) landed in the prior
`/speckit-implement` pass; the single-component half (T019-T027, research.md Decision 18)
landed this session.

**Design (research.md Decision 18)**: Inversion of control via a caller-supplied closure —
`internal/exec` never imports `pkg/ci/plugins/terraform` (which would reintroduce the
confirmed import cycle: `pkg/ci/plugins/terraform` → `internal/exec`). Instead:
1. New `internal/exec/shell_utils.go` option `WithExecMetadataParser(fn func(subCommand
   string) any) ShellCommandOption`, storing `fn` on `shellCommandConfig.execMetadataParser`,
   plus `execMetadataParserFromOpts(opts ...ShellCommandOption) func(subCommand string) any`
   — both mirror the already-shipped `WithInvokingCommand`/`invokingCommandFromOpts` pair
   exactly (same file, same pattern).
2. `cmd/terraform/plan.go`/`apply.go`/`deploy.go` move `stdoutBuf`/`stderrBuf` construction
   and `WithStdoutCapture`/`WithStderrCapture` wiring **outside** the `ciMode` conditional
   (always capture now — cheap, an in-memory buffer append; the *separate* `ciMode`-gated
   `capturedPlanOutput` CI-job-summary post-processing is untouched), and additionally pass
   `WithExecMetadataParser(func(subCommand string) any { return
   buildTerraformExecData(subCommand, ansiStrippedCombinedBuffer) })`.
3. New `cmd/terraform/utils.go` helper `buildTerraformExecData(subCommand, output string)
   any`, sharing a refactored-out `parseTerraformOutputMirror` decode step with the
   already-shipped `parseTerraformResourceChanges` (T028) — no duplicated
   `citerraform.ParseOutput`/JSON-round-trip logic — producing the combined-object shape
   (`resource_counts`/`outputs`/`warnings`/`changes`) `data-model.md`'s `TerraformExecData`
   already specifies.
4. `internal/exec/terraform.go`'s `captureExecMetadataSync` extracts the closure via
   `execMetadataParserFromOpts(opts...)` and, only when non-nil AND `IsSyncCommand` AND
   `info.NodeHooks == nil` (existing multi-component skip), calls `parser(subCommand)` once
   to obtain `data` for `proexec.CaptureSync` — never for async/non-allowlisted commands.

### Tests for User Story 3 ⚠️

- [x] T019 [P] [US3] Test asserting `execMetadataParserFromOpts` round-trips a closure passed
  via `WithExecMetadataParser` and returns `nil` when no such option was passed. **Done** —
  new file `internal/exec/shell_utils_exec_metadata_test.go`:
  `TestExecMetadataParserFromOpts_RoundTrips`, `TestExecMetadataParserFromOpts_NilWhenNotSet`.
- [x] T020 [P] [US3] Test asserting `captureExecMetadataSync`: (a) calls the parser closure
  exactly once and forwards its return value as `CaptureSync`'s `data` argument when
  `IsSyncCommand` is true and `info.NodeHooks == nil`; (b) never calls the closure for a
  non-sync-allowlisted subcommand; (c) never calls the closure when `info.NodeHooks != nil`.
  **Done** — `internal/exec/terraform_exec_metadata_flags_test.go`:
  `TestCaptureExecMetadataSync_CallsParserForSyncAllowlistedSingleComponent`,
  `TestCaptureExecMetadataSync_NeverCallsParserForNonSyncSubcommand`,
  `TestCaptureExecMetadataSync_NeverCallsParserWhenNodeHooksWired`.
- [x] T021 [P] [US3] Test asserting a `terraform plan` with a very large synthetic
  resource-change list routes through the `/exec/data`-then-`/exec` blob-URL path. **Done —
  fulfilled by Phase 2's foundational coverage** (`pkg/pro/api_client_exec_test.go`'s
  `TestUploadExecMetadata_BlobURLOverThreshold`), which already exercises exactly this
  mechanism generically (the threshold check is size-based, not command-type-based, so it
  applies uniformly regardless of which command produced the oversized `Data`); a
  `TerraformExecData`-flavored variant would exercise the identical code path with no
  additional coverage value.
- [x] T022 [P] [US3] Test asserting `buildTerraformExecData("plan"/"apply"/"deploy", output)`
  produces the combined `resource_counts`/`outputs`/`warnings`/`changes` object against the
  same real fixture T028's tests use, and returns `nil` for a non-terraform subcommand or
  empty output. **Done** — `cmd/terraform/utils_exec_metadata_test.go`:
  `TestBuildTerraformExecData_ApplySuccess`, `TestBuildTerraformExecData_DeployParsedAsApply`,
  `TestBuildTerraformExecData_NonTerraformSubcommand`.
- [x] T023 [P] [US3] Test asserting `stdoutBuf`/`stderrBuf` construction and
  `WithStdoutCapture`/`WithStderrCapture` wiring happen regardless of `ciMode`. **Done, via a
  refactor** — rather than testing `plan.go`'s inline `RunE` closure directly (not separately
  callable), the shared buffer/option construction was extracted into a new
  `cmd/terraform/utils.go` helper, `terraformCaptureShellOpts()` (called unconditionally —
  structurally provable it's not gated by `ciMode`, since `ciMode` isn't even a parameter),
  used identically by `plan.go`/`apply.go`/`deploy.go`. Covered by
  `cmd/terraform/utils_exec_metadata_test.go`:
  `TestTerraformCaptureShellOpts_AlwaysWiresCaptureAndParser`,
  `TestTerraformExecMetadataParserFunc_ReadsBuffersAtCallTime` (the parser closure itself was
  further split into `terraformExecMetadataParserFunc` for direct unit-testability).

### Implementation for User Story 3

- [x] T024 [US3] In `internal/exec/shell_utils.go`: added `WithExecMetadataParser(fn
  func(subCommand string) any) ShellCommandOption` and `execMetadataParserFromOpts(opts
  ...ShellCommandOption) func(subCommand string) any`, following the existing
  `WithInvokingCommand`/`invokingCommandFromOpts` pattern exactly (same file, adjacent to
  it). Makes T019 pass. **Done.**
- [x] T025 [US3] In `internal/exec/terraform.go`'s `captureExecMetadataSync`: added a `parser
  func(subCommand string) any` parameter (extracted at the call site via
  `execMetadataParserFromOpts(opts...)`, alongside the existing `invokingCommandFromOpts`
  extraction); after the existing `IsSyncCommand`/`info.NodeHooks != nil` gates, calls
  `parser(subCommand)` when non-nil and passes its result as `CaptureSync`'s `data` argument
  instead of the previous hardcoded `nil`. All five existing test call sites updated to the
  new signature. Makes T020 pass. **Done.**
- [x] T026 [US3] In `cmd/terraform/utils.go`: extracted the JSON-mirror decode step out of
  `parseTerraformResourceChanges` into a shared `parseTerraformOutputMirror(subCommand,
  output string) (*terraformOutputDataMirror, bool)` helper (also extended the mirror struct
  with `ResourceCounts`/`Outputs`/`HasOutputChanges`/`ChangedResult`/`Warnings`, and factored
  the per-action flattening into `terraformResourceChanges`); `parseTerraformResourceChanges`
  now calls it (T028's existing tests confirmed still green — no behavior change); added
  `buildTerraformExecData(subCommand, output string) any` using the same helper, returning
  `resource_counts`/`outputs`/`warnings`/`changes`. Makes T022 pass. **Done.**
- [x] T027 [US3] In `cmd/terraform/plan.go`/`apply.go`/`deploy.go`: replaced the
  `ciMode`-gated inline `stdoutBuf`/`stderrBuf`/`shellOpts` construction with a single
  unconditional call to a new shared helper, `terraformCaptureShellOpts()`
  (`cmd/terraform/utils.go`), which always wires `WithStdoutCapture`/`WithStderrCapture` plus
  a new `WithExecMetadataParser(terraformExecMetadataParserFunc(...))`; the
  `capturedPlanOutput`/`capturedApplyOutput`/`capturedDeployOutput` CI-job-summary
  assignments remain inside their respective `if ciMode` blocks, untouched. Removed the
  now-unused `bytes`/`internal/exec` (`e`) imports from all three files (no longer referenced
  directly). Makes T023 pass — depended on T026. **Done.**
- [x] T028 [US3] For the multi-component aggregation path (US2, `cmd/terraform/utils.go`),
  fold each node's parsed resource changes into the single aggregate record's `data` (one
  entry per component per resource action) instead of discarding it. **Done** —
  `cmd/terraform/utils.go`'s `terraformNodeHooks.recordExecResult` now calls a new
  `parseTerraformResourceChanges(subCommand, output)` helper (uses
  `pkg/ci/plugins/terraform.ParseOutput` + a local JSON-mirror struct) and folds one
  `execNodeResult{component, stack, exitCode, action, address}` entry per
  created/updated/replaced/deleted/imported/moved resource into `results`, alongside the
  always-present base per-node identity/outcome entry. Covered by
  `cmd/terraform/utils_exec_metadata_test.go`:
  `TestParseTerraformResourceChanges_ApplySuccess` (real fixture:
  `pkg/ci/plugins/terraform/testdata/stdout/apply_success.txt`),
  `TestParseTerraformResourceChanges_DeployParsedAsApply`,
  `TestParseTerraformResourceChanges_NonTerraformSubcommand`, and
  `TestTerraformNodeHooks_RecordExecResultAccumulates` (updated to assert the enriched shape).
  "deploy" is parsed with apply semantics, matching `pkg/ci/plugins/terraform`'s own
  `onAfterDeploy` override ("deploy is semantically apply for CI purposes").

**Checkpoint**: US3 is fully functional. The multi-component half (T028) — `--affected`/
`--all`/query `plan`/`apply`/`deploy` runs carry per-resource structured data, correctly
aggregated. The single-component half (T019-T027, research.md Decision 18) — a plain
`atmos terraform plan <component> -s <stack>` now also carries structured `data`
(`resource_counts`/`outputs`/`warnings`/`changes`), via a caller-supplied parser closure
threaded from `cmd/terraform` into `internal/exec`'s `captureExecMetadataSync`, with no new
import-cycle risk. Both halves verified: `go build ./...`, `atmos lint --changed`, and
`go test ./cmd/terraform/ ./internal/exec/...` (targeted exec-metadata tests; the two
`internal/exec` failures seen in a full-package run are pre-existing environment issues —
missing toolchain shim files in the test sandbox — unrelated to this work) all clean.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [x] T029 [P] Regenerated the Pact contract with the three interactions from
  `contracts/interactions.md`. **Done** — `pkg/pro/consumer_pact_test.go`'s
  `TestPact_UploadExecMetadata` (inline `data`, now includes `execution_id` and a `changes`
  array replacing the old `data_items`), `TestPact_UploadExecMetadata_NoData` (`execution_id`
  added), and a new `TestPact_UploadExecMetadata_BlobURL` (replacing the retired
  `TestPact_UploadExecMetadata_Chunked` — two chained interactions: `POST
  /api/v1/atmos/exec/data` keyed by `execution_id`, then `POST /api/v1/atmos/exec` with
  `data` as the returned URL string). Regenerated via `rm pacts/atmos-AtmosPro.json && go
  test -tags pact ./pkg/pro/...` (deleted first so stale interactions from the old test names
  don't linger in the merged output — `go test -tags pact ./pkg/pro/... -v -run
  'TestPact_UploadExecMetadata|TestPact_UploadExecData'` to run just these). Verified via a
  Python JSON walk: 12 interactions total, all current descriptions, zero occurrences of
  `batch_id`/`batch_index`/`batch_total`/`data_items` anywhere in the file.
- [x] T030 [P] Ran `go test -cover` for the touched packages and confirmed coverage.
  **Done, re-confirmed after T019-T027 landed.** `pkg/proexec`: 87.2%, `pkg/pro`: 87.2%,
  `pkg/pro/dtos`: 100% (all above the 85% floor, unchanged this session). `cmd/terraform`'s
  whole-package figure (56.4%) remains a pre-existing characteristic of that large,
  many-command, binary-dependent package — the specific functions this session added have
  solid direct coverage per `go tool cover -func`: `parseTerraformOutputMirror` 85.7%,
  `terraformResourceChanges` 91.7%, `buildTerraformExecData` 100%,
  `terraformCaptureShellOpts` 100%, `terraformExecMetadataParserFunc` 80%. `internal/exec`'s
  new `WithExecMetadataParser`/`execMetadataParserFromOpts` are at 100%, and the modified
  `captureExecMetadataSync` is at 88.9%.
- [ ] T031 Manually walk `quickstart.md` steps 1–11 end-to-end against a local Pro stub or
  test workspace, including step 9 (`ExecutionID`), step 10 (4 MB threshold / blob-URL path),
  and the new step 11 (single-component structured data). **Not done this session** —
  requires a live/stubbed Atmos Pro endpoint and a human (or a dedicated `run`/browser-driven
  session) to walk interactively; the automated test suite already exercises the same code
  paths, but this manual walkthrough is a distinct verification step deliberately left for a
  human or the `run` skill.
- [x] T032 `atmos lint --changed` and `go build ./...` across all touched files. **Done,
  re-confirmed after T019-T027 landed** — both clean.
- [ ] T033 **Carried-over open item — end-to-end regression test for the still-unresolved
  duplicate-row question** (2026-08-19 production report). Two `atexec_*` DB rows were
  observed for what should be a single `atmos terraform plan cdn -s plat-use2-dev
  --upload-status` invocation — one row (sync-path shape) with populated envelope/metrics
  but (at the time) empty `flags` (root-caused and already fixed by the shipped `Flags`
  source-of-truth fix), the other (thin shape) with `flags` populated but envelope/metrics
  fields null and a *different* `workflow_job` id. Static inspection did not turn up an
  obvious live double-fire bug in `pkg/proexec/classify.go`/`async.go`/
  `internal/exec/terraform.go`, but this was never proven with a real end-to-end run. Write
  an integration test that:
  - Drives the real `atmos terraform plan {component} -s {stack} --upload-status`
    invocation through the actual `cmd.Execute()` (`cmd/root.go`) — not just
    `RootCmd.Execute()` — since the async `proexec.CaptureAsync` hook only fires from that
    top-level wrapper. `Execute()` reads `os.Args[1:]` directly, so the test must mutate
    `os.Args` (save/restore), not just `RootCmd.SetArgs`.
  - Points `Settings.Pro.BaseURL`/`ATMOS_PRO_BASE_URL` at an `httptest.NewServer` fake, with
    `CI=true` and `ATMOS_PRO_TOKEN` set so `gateOpen` passes.
  - Runs against the existing `tests/fixtures/scenarios/terraform-generate-planfile`
    `mock`/`component-1` fixture (`tests.RequireTerraform(t)`-gated) so no real cloud
    credentials are needed.
  - Asserts the fake server received **exactly one** `POST .../atmos/exec` request (and, if
    the fixture's plan output is large enough, exactly one preceding `POST
    .../atmos/exec/data` request with a matching `execution_id`) for the invocation, with
    correctly populated `flags`/`atmos_version`/`atmos_os`/`atmos_arch`/`metrics`, AND
    **exactly one** `POST .../repos/{owner}/{repo}/instances` request (the independent
    `--upload-status` mechanism) with the correct `stack`/`component`/`exit_code`.
  - If this test passes cleanly, the production duplicate is confirmed to be a stale-build
    artifact rather than a live bug, and this task can close as verification-only.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: Empty — no new dependencies/structure.
- **Foundational (Phase 2)**: No dependencies on other phases; MUST complete before Phase
  3-5, since all three user-story phases call `buildRecord`/`CaptureSync`/`CaptureAsync`
  with the new signature and the new DTO shape.
- **US1 (Phase 3)**: Depends on Foundational (T006). Independently testable/deliverable once
  Phase 2 lands.
- **US2 (Phase 4)**: Depends on Foundational (T006, T009). Independent of US1 beyond the
  shared foundation.
- **US3 (Phase 5)**: Depends on Foundational (T006, T008) and US2 (T016/T017), since T028
  builds directly on T016's/T017's call-site shape — implement after US2 lands (matches the
  third re-plan's existing US3-depends-on-US2 ordering). The single-component tasks
  (T019-T027, this session's Decision 18) are independent of T028 (different call sites —
  `internal/exec`'s single-component path vs. `cmd/terraform`'s multi-component aggregator)
  and can proceed in parallel with it, though T026/T027 do depend on the
  `parseTerraformOutputMirror` refactor staying compatible with T028's existing tests.
- **Polish (Phase 6)**: T029-T032 depend on all three user-story phases (T030/T032 should be
  re-run once T019-T027 land, since they were measured before that work). T033 (duplicate-row
  end-to-end test) has no code dependency on US1/US2/US3 and can run in parallel with them,
  though running it after Phase 2/3 land is recommended so its assertions reflect the fixed
  `Flags`/`ExecutionID` shape.

### Parallel Opportunities

- T001-T004 (Foundational tests) are `[P]` — different files/concerns
- T014-T015 (US2 tests) are `[P]` — different files
- T019-T023 (US3 single-component tests) are `[P]` — different files
- T029-T030 (Polish) are `[P]` — independent concerns
- US2 (Phase 4) and US1 (Phase 3) can proceed in parallel once Phase 2 lands — they touch
  different call-site concerns (`ExecutionID` verification vs. multi-component gating)
- T024 (the new `ShellCommandOption`) has no dependency on T025-T028 and can start
  immediately; T025 depends on T024, T026 is independent of T024/T025, T027 depends on T026
- T033 can be worked independently of T001-T032 by a different session/agent

---

## Implementation Strategy

### MVP First (Foundational + User Story 1)

1. Complete Phase 2 (Foundational — `ExecutionID` + `Data` redesign): T001-T004 (tests) →
   T005-T011 (implementation)
2. Complete Phase 3 (US1 — `ExecutionID` verification): T012 (test) → T013 (confirm/fix)
3. **STOP and VALIDATE**: run T001-T004 and T012, confirm GREEN; run quickstart.md steps 9-10
4. This alone ships the two clarified changes (`ExecutionID`, `Data` redesign) for every
   command's base record, independent of US2/US3's still-open multi-component/structured-data
   work

### Incremental Delivery

1. Phase 2 (Foundational) → validate → ship (unblocks US1/US2/US3)
2. Phase 3 (US1 — `ExecutionID`) → validate → ship
3. Phase 4 (US2 — multi-component aggregation into `Data`) → validate (single upload) → ship
4. Phase 5 (US3):
   a. Multi-component half (T028) — **done**, shipped
   b. Single-component half (T019-T027, Decision 18) — **done**, shipped: a plain `atmos
      terraform plan <component> -s <stack>`'s execution record now carries non-nil
      structured `data`
5. Phase 6 (Polish) → Pact contract regenerated (3 interactions) — done; coverage/lint
   re-confirmed after T019-T027 landed (T030/T032) — done; T031 (quickstart walked
   end-to-end) and T033 (duplicate-row open question) remain
