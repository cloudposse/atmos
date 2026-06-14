---
description: "Task list for Pro Summary Upload feature implementation"
---

# Tasks: Pro Summary Upload

**Input**: Design documents from `specs/001-pro-summary-upload/`

**Prerequisites**: plan.md ✅, spec.md ✅, research.md ✅, data-model.md ✅,
contracts/instance-status-patch.md ✅, quickstart.md ✅

**Note**: The majority of this feature is already implemented on this branch. Tasks focus on
two remaining gaps (FR-008a: deploy command field; FR-007a: spec alignment) plus targeted
tests for each user story and a deferred follow-up for the server-side size limit.

**Tests**: Included — the deploy command gap requires a failing test first per the bug-fix
workflow in CLAUDE.md.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1–US4)

---

## Phase 1: Setup

**Purpose**: Verify existing implementation compiles and baseline tests pass before making changes.

- [x] T001 Run `make test-short` to establish a passing baseline; confirm `TestPlugin_BuildStatusData_Plan`, `TestPlugin_BuildStatusData_Apply`, `TestBuildStatusData`, and `TestExtractOutputValues` all pass
- [x] T002 Run `go build ./...` to confirm no compilation errors on the current branch

---

## Phase 2: Foundational (Deploy Command Field Fix — FR-008a)

**Purpose**: Fix the shared gap where `handleDeploySubcommand` overwrites `info.SubCommand`
from `"deploy"` to `"apply"` before the upload runs, causing the DTO to send `"apply"` instead
of `"deploy"`. This fix affects US1, US2, and the deploy path throughout.

**⚠️ CRITICAL**: Tests T003–T004 must be written and confirmed failing before T005–T007.

- [x] T003 Write a failing test in `internal/exec/pro_test.go` that calls `uploadStatus` with `info.SubCommand = "deploy"` and asserts the DTO's `Command` field equals `"deploy"` (using a mock `AtmosProAPIClientInterface` to capture the DTO); confirm the test fails with current code
- [x] T004 Add `InvokedSubCommand string` field to `schema.ConfigAndStacksInfo` in `pkg/schema/schema.go`; document it as the original subcommand before internal conversion
- [x] T005 In `internal/exec/terraform_execute_helpers_exec.go`, add `info.InvokedSubCommand = info.SubCommand` immediately before the `handleDeploySubcommand(atmosConfig, info)` call (line ~147) so the original value is captured before conversion
- [x] T006 In `internal/exec/pro.go`, update `uploadStatus` to use `info.InvokedSubCommand` for the `Command:` field in `InstanceStatusUploadRequest`; fall back to `info.SubCommand` when `InvokedSubCommand` is empty (for callers that haven't set it)
- [x] T007 Update `shouldUploadStatus` in `internal/exec/pro.go` to also return `true` when `info.SubCommand == "deploy"` (line ~269); this adds explicit safety for any future caller that checks before conversion
- [x] T008 Confirm the test from T003 now passes; run `go test ./internal/exec/... -run TestUploadStatus -v`

**Checkpoint**: Deploy invocations now send `"deploy"` in the payload `command` field. Foundation ready.

---

## Phase 3: User Story 1 — Plan Summary in Dashboard (Priority: P1) 🎯 MVP

**Goal**: `atmos terraform plan --upload-status` uploads structured metadata (resource counts,
warnings, errors) to the Atmos Pro dashboard.

**Independent Test**: Run `atmos terraform plan vpc --stack dev-us-east-1 --upload-status`
against a stack with `settings.pro.enabled: true` and `ci.enabled: true`. The Atmos Pro
dashboard must show resource counts without opening the CI platform.

- [x] T009 [P] [US1] In `internal/exec/terraform_execute_helpers_exec_test.go`, add table-driven tests for `buildCIStatusData`: verify it returns a non-nil map with `resource_counts`, `has_changes`, and `output_log` keys for a sample plan output, and returns nil when `maskedOutput` is empty
- [x] T010 [P] [US1] In `pkg/ci/plugins/terraform/plugin_test.go`, extend `TestPlugin_BuildStatusData_Plan` to assert that `resource_counts` contains all four keys (`create`, `change`, `replace`, `destroy`) with correct values for a plan output with mixed counts
- [x] T011 [US1] In `internal/exec/terraform_execute_helpers_exec_test.go`, add a test for `buildMetadataForUpload`: assert it returns nil when `captureOutput` is false, and a non-nil map when `captureOutput` is true with non-empty output
- [x] T012 [US1] Run `go test ./internal/exec/... -run 'TestBuildCIStatusData|TestBuildMetadata' -v` to confirm new tests pass; run `make test-short` to confirm no regressions

**Checkpoint**: Plan upload path fully tested. User Story 1 independently functional.

---

## Phase 4: User Story 2 — Apply Result and Outputs (Priority: P2)

**Goal**: `atmos terraform apply --upload-status` uploads apply outcome and Terraform output
values; sensitive outputs appear as `"<MASKED>"` in the dashboard.

**Independent Test**: Run `atmos terraform apply --upload-status` on a component with sensitive
outputs. Confirm the dashboard shows apply result and `"<MASKED>"` for sensitive values.

- [x] T013 [P] [US2] In `pkg/ci/plugins/terraform/plugin_test.go`, add a table-driven test for `extractOutputValues` covering: (a) non-sensitive string value passes through, (b) sensitive value becomes `"<MASKED>"`, (c) nil map returns empty map; this is in addition to the existing tests if they don't cover all cases
- [x] T014 [P] [US2] In `pkg/ci/plugins/terraform/plugin_test.go`, add test `TestPlugin_BuildStatusData_Apply_WithOutputs` that includes a terraform apply output with both sensitive and non-sensitive output values; assert `data["outputs"]` contains `"<MASKED>"` for sensitive keys
- [x] T015 [US2] Run `go test ./pkg/ci/plugins/terraform/... -run 'TestExtractOutputValues|TestPlugin_BuildStatusData_Apply' -v` to confirm all tests pass

**Checkpoint**: Apply metadata and sensitive output masking fully tested. User Story 2 independently functional.

---

## Phase 5: User Story 3 — Masked Output Log with Truncation (Priority: P2)

**Goal**: Full masked stdout is available in the dashboard; oversized logs are tail-truncated
with a `"truncated": true` indicator.

**Independent Test**: Simulate a command output exceeding 3 MB. Confirm the upload log is
truncated from the beginning (tail preserved) and `truncated: true` is set.

- [x] T016 [P] [US3] In `internal/exec/terraform_execute_helpers_exec_test.go`, add table-driven tests for `addOutputLog` covering: (a) empty output → no keys added to map, (b) output within limit → `output_log` key added, no `truncated` key, (c) output exceeding limit → `output_log` key contains only tail, `truncated: true` is set, (d) nil data map → no-op (no panic)
- [x] T017 [US3] Run `go test ./internal/exec/... -run TestAddOutputLog -v` to confirm all cases pass; verify the test for case (c) checks that the base64-decoded value starts with the tail of the input, not the beginning

**Checkpoint**: Output log truncation behavior fully tested. User Story 3 independently functional.

---

## Phase 6: User Story 4 — Other Component Types Unaffected (Priority: P1)

**Goal**: Non-terraform component types (helmfile, packer, etc.) produce upload payloads
identical to the current CLI version — no `component_type` or `metadata` fields.

**Independent Test**: Inspect the payload produced when `BuildStatusData` is called with a
component type that has no `StatusDataProvider` implementation; assert nil is returned.

- [x] T018 [P] [US4] In `pkg/ci/plugin_registry_test.go`, add test `TestBuildStatusData_NoProvider` that registers a stub plugin without `StatusDataProvider` and asserts `ci.BuildStatusData` returns nil for that component type
- [x] T019 [P] [US4] In `internal/exec/terraform_execute_helpers_exec_test.go`, add test `TestBuildCIStatusData_CIDisabled` that calls `buildMetadataForUpload` with `captureOutput=false` and asserts the result is nil (ensuring `metadata` stays nil in the DTO when `ci.enabled` is false)
- [x] T020 [US4] Run `go test ./pkg/ci/... -run TestBuildStatusData -v` and `go test ./internal/exec/... -run TestBuildCIStatusData -v` to confirm all backward-compat tests pass

**Checkpoint**: Backward compatibility verified for all non-terraform component types.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Spec alignment, deferred issue tracking, and final validation.

- [x] T021 [P] Amend FR-007a in `specs/001-pro-summary-upload/spec.md` to read "at least one automatic retry" (current `pkg/pro/retry.go` uses `DefaultMaxRetries=3`; the spec said "exactly one" but implementation is more generous and correct — align spec to implementation)
- [x] T022 [P] Open a GitHub issue titled "feat: fetch max_output_log_bytes from server settings endpoint" describing the deferred server-settings API fetch (GET /api/v1/settings → max_output_log_bytes); add the issue number to the `## Decision 5` section in `specs/001-pro-summary-upload/research.md` — Issue #2586
- [x] T023 Run `make test-short` to confirm the full fast test suite passes with all new tests
- [x] T024 Run `make lint` to confirm no linting violations introduced by the new field and function changes

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — run immediately
- **Foundational (Phase 2)**: Depends on Phase 1 passing — BLOCKS all user story phases
- **US1 (Phase 3)**: Depends on Phase 2 — no dependency on US2/US3/US4
- **US2 (Phase 4)**: Depends on Phase 2 — can run in parallel with US1/US3/US4
- **US3 (Phase 5)**: Depends on Phase 2 — can run in parallel with US1/US2/US4
- **US4 (Phase 6)**: Depends on Phase 2 — can run in parallel with US1/US2/US3
- **Polish (Phase 7)**: Depends on all user story phases completing

### User Story Dependencies

- **US1 (P1)**: No inter-story dependency
- **US2 (P2)**: No inter-story dependency (different command path: apply vs plan)
- **US3 (P2)**: Shares `addOutputLog` infrastructure with US1/US2 but tests it independently
- **US4 (P1)**: Purely a verification phase — no new code, only new tests

### Within Each Phase

- T003 MUST fail before T005 (bug-fix workflow: test first)
- T004 → T005 (field must exist before it can be set)
- T005 → T006 (field must be captured before it can be used)
- All [P]-marked tasks within a phase have no intra-phase dependencies and can run simultaneously

---

## Parallel Example: Foundational Phase

```bash
# T003 and T004 can be worked simultaneously:
Task: "Write failing test for deploy command field (T003)"      # internal/exec/pro_test.go
Task: "Add InvokedSubCommand field to schema (T004)"           # pkg/schema/schema.go
```

```bash
# After T004: T005, T006, T007 must run sequentially
Task: "Capture InvokedSubCommand before handleDeploySubcommand (T005)"
Task: "Use InvokedSubCommand in uploadStatus DTO (T006)"
Task: "Extend shouldUploadStatus to accept deploy (T007)"
```

## Parallel Example: User Stories (after Phase 2 complete)

```bash
# All four user story phases can run in parallel:
Task: "Plan summary tests (T009–T012)"   # US1 — internal/exec/ and pkg/ci/plugins/terraform/
Task: "Apply + output tests (T013–T015)" # US2 — pkg/ci/plugins/terraform/
Task: "Output log tests (T016–T017)"     # US3 — internal/exec/
Task: "Backward compat tests (T018–T020)"# US4 — pkg/ci/ and internal/exec/
```

---

## Implementation Strategy

### MVP First (US1 + US4 — both P1)

1. Complete Phase 1: Setup (T001–T002)
2. Complete Phase 2: Foundational deploy fix (T003–T008)
3. Complete Phase 3: US1 plan summary tests (T009–T012)
4. Complete Phase 6: US4 backward compat tests (T018–T020)
5. **STOP and VALIDATE**: `make test-short` passes; deploy command field verified

### Incremental Delivery

1. Phase 1 + 2 → Foundation verified, deploy gap fixed
2. Phase 3 → Plan upload fully tested (MVP)
3. Phase 4 → Apply uploads fully tested
4. Phase 5 → Output log truncation fully tested
5. Phase 6 → Backward compatibility locked in
6. Phase 7 → Spec + issue hygiene

---

## Notes

- [P] tasks = different files, no intra-phase dependencies
- [Story] labels map tasks to user stories from spec.md
- All new tests follow the `require`/`assert` pattern from testify; use `errors.Is()` for error assertions
- Comments in new code must end with periods (godot linter)
- `perf.Track` is NOT required for test helper functions
- The server-settings size-limit endpoint (GET /api/v1/settings) is intentionally deferred to T022 GitHub issue
