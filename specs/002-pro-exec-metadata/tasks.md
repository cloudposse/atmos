---

description: "Task list for Atmos Pro Command-Execution Metadata Upload — remaining work"
---

# Tasks: Atmos Pro Command-Execution Metadata Upload

**Input**: Design documents from `/specs/002-pro-exec-metadata/`

**Prerequisites**: plan.md (eighth re-plan, 2026-08-20), spec.md, research.md, data-model.md,
contracts/interactions.md, quickstart.md

**Tests**: Included — the constitution's Test-First principle (III) is NON-NEGOTIABLE and
CLAUDE.md's Bug-Fixing Workflow requires a failing regression test before any behavior
change.

**Regenerated in full** (not patched) — the previous tasks.md was written against the
fourth re-plan (`ExecutionID`/`Data` redesign) and marked US1/US2/US3's *base* work
(`ExecutionID`, multi-component aggregation, single-component `buildTerraformExecData`) as
done. That base work is confirmed still present and correct in the current tree (re-verified
this session: `cmd/terraform/utils.go`'s `buildTerraformExecData`/`terraformCaptureShellOpts`,
`internal/exec/describe_affected.go`'s `CaptureSync` call, `pkg/proexec/async.go`'s
`CaptureAsync` all exist exactly as previously shipped) and is **not** re-tasked here.

**This regeneration's scope** — seven deltas across the seventh and eighth re-plans, **none
implemented yet** (verified by reading the current code, not assumed from the plan docs):

1. FR-010a — Terraform output masking (`maskSensitiveOutputs`, research.md Decision 19)
2. FR-006 shape completeness — `has_changes`/`has_errors`/`errors` (Decision 20)
3. FR-006 — `component`/`stack` identity fields (Decision 21)
4. FR-006b — `describe affected` structured data (Decision 22)
5. FR-006c — `list instances` structured data, `--upload`-gated (Decision 23)
6. FR-005a — per-shape `version` field (Decision 24)
7. FR-013/Assumptions — 6-interaction-total Pact coverage, 3 shapes × 2 delivery modes
   (Decision 25)

**Spec-mapping note**: spec.md's User Story 3 (P3) is titled "Structured infrastructure-change
data for **plan/apply/deploy**" and its four Acceptance Scenarios only reference `terraform
plan`/`apply`. Deltas 1-3 above extend `TerraformExecData` and clearly belong to US3. Deltas
4-5 (`describe affected`, `list instances`) are required by FR-006b/FR-006c and now map to
**User Story 4** (Priority P4, "Pro inventory visibility for `describe affected` and `list
instances`"), added to spec.md in the 2026-08-20 clarification session specifically to close
this gap — every task below now carries a `[USn]` label. Delta 6 (`version`) is genuinely
cross-cutting (all three shapes) and sits in Foundational. Delta 7 (Pact coverage) is
cross-cutting test infrastructure and sits in Polish.

## Format: `[ID] [P?] [Story] Description`

---

## Phase 1: Setup

No setup tasks — no new packages, no new external dependencies. All seven deltas are edits
to already-existing files plus, at most, two new exported functions inside the
already-existing `pkg/proexec` package (plan.md Scale/Scope).

---

## Phase 2: Foundational (Blocking Prerequisites)

**Goal**: Land the two small, genuinely shared pieces of infrastructure — the per-shape
`version` field helper and the async "pending data" hand-off — before the user-story-scoped
work (Phase 5/6) that depends on them.

### Tests for Foundational Phase ⚠️

- [X] T001 [P] Test in `pkg/proexec/envelope_test.go` asserting `VersionedData(1, "stacks",
  someSlice)` returns `map[string]any{"version": 1, "stacks": someSlice}`; a second case with
  a `nil` payload still wraps correctly (`map[string]any{"version": 1, "instances": nil}`),
  no panic.
- [X] T002 [P] Test in `pkg/proexec/async_test.go` asserting: (a) `SetPendingAsyncData(x)`
  followed by a `CaptureAsync` call uses `x` as `ExecRecordInput.Data`; (b) a *second*
  `CaptureAsync` call immediately after (no intervening `SetPendingAsyncData`) sees `Data:
  nil` — proves the read-and-clear behavior prevents cross-invocation leakage (matches
  `cmd.NewTestKit(t)`-style isolation, CLAUDE.md MANDATORY); (c) never calling
  `SetPendingAsyncData` at all (today's behavior for every command except `list instances`)
  continues to produce `Data: nil`, unchanged. Use the existing fake upload client
  (`fakeUploadClient`) already present in this test file to inspect the request `CaptureAsync`
  builds.

### Implementation for Foundational Phase

- [X] T003 [P] Add `VersionedData(version int, key string, payload any) map[string]any` to
  `pkg/proexec/envelope.go` (research.md Decision 24: two genuinely identical single-key-wrap
  call sites justify one small helper; `TerraformExecData`'s multi-key shape deliberately does
  NOT use this helper — see T010). Makes T001 pass.
- [X] T004 In `pkg/proexec/async.go`: add an unexported package-level `pendingAsyncData any`
  var and an exported `SetPendingAsyncData(data any)` setter, mirroring the existing
  `currentAtmosConfig`/`SetAtmosConfig` pair in the same file exactly (same doc-comment style,
  same `defer perf.Track(nil, "proexec.SetPendingAsyncData")()` convention). In
  `CaptureAsync`, when building `ExecRecordInput` (currently `Data` is implicitly omitted from
  the struct literal, i.e. `nil`), read and clear it: `data := pendingAsyncData;
  pendingAsyncData = nil`, then set `Data: data` on the `ExecRecordInput`. Makes T002 pass.

**Checkpoint**: `pkg/proexec` now exposes the two small primitives (`VersionedData`,
`SetPendingAsyncData`) that Phase 6's `describe affected`/`list instances` tasks build on.
Phase 5 (US3 terraform deltas) does not depend on this phase — it can proceed in parallel.

---

## Phase 3: User Story 1 - Automatic visibility into CI command execution (Priority: P1) 🎯 MVP

**No remaining tasks.** `ExecutionID`, base envelope fields, and the `Flags` source-of-truth
fix are already shipped and unaffected by any of this regeneration's seven deltas (all of
which are scoped to command-specific `Data`, not the base envelope FR-003/FR-003b covers).

---

## Phase 4: User Story 2 - Reliable reporting for critical operations (Priority: P2)

**No remaining tasks.** The sync/async dedup fix, the `terraform deploy` sync-allowlist join,
and the multi-component one-record-per-invocation aggregation (FR-006a) are already shipped.
None of this regeneration's seven deltas touch delivery classification or aggregation
mechanics — they only touch what goes *inside* `Data`.

---

## Phase 5: User Story 3 - Structured infrastructure-change data for plan/apply/deploy (Priority: P3)

**Goal**: Close the three confirmed gaps in `TerraformExecData` (FR-006/FR-010a): sensitive
outputs currently ship unmasked, `has_changes`/`has_errors`/`errors` are silently discarded
by the JSON-mirror decode, and there is no `component`/`stack` identity on the payload.

**Independent Test**: Run `atmos terraform plan <component> -s <stack>` against a component
with a `sensitive = true` output and a pending change; inspect the logged request body's
`data` field and confirm `outputs[<sensitive-key>].value == "<MASKED>"`,
`has_changes/has_errors/errors` are present and correct, and `component`/`stack` match the
invocation (quickstart.md steps 12-14, 17).

### Tests for User Story 3 ⚠️

- [X] T005 [P] [US3] Table-driven test in `cmd/terraform/utils_exec_metadata_test.go` for a
  new `maskSensitiveOutputs(outputs map[string]json.RawMessage) map[string]any`: a `Sensitive:
  true` entry's `Value` becomes `pkg/io.MaskReplacement` (`"<MASKED>"`) while `Type`/
  `Sensitive` pass through unchanged; a `Sensitive: false` entry's `Value` passes through
  unchanged; a malformed/undecodable `json.RawMessage` entry defaults to masked (fail-safe,
  research.md Decision 19).
- [X] T006 [P] [US3] Extend `terraformOutputResultMirror`'s decode tests (or add new cases to
  `cmd/terraform/utils_exec_metadata_test.go`) asserting `HasChanges`/`HasErrors`/`Errors`
  decode correctly from `citerraform.ParseOutput`'s result, using the existing
  `pkg/ci/plugins/terraform/testdata/stdout/apply_success.txt` fixture (has_changes=true,
  has_errors=false) plus one error-producing fixture already present under
  `pkg/ci/plugins/terraform/testdata/stdout/` for the has_errors=true/errors-non-empty case.
- [X] T007 [P] [US3] Extend `TestBuildTerraformExecData_ApplySuccess` (and add a
  single-component-with-`component`/`stack`-args case) in
  `cmd/terraform/utils_exec_metadata_test.go` asserting `buildTerraformExecData`'s returned
  map includes: `outputs` masked per T005, `has_changes`/`has_errors`/`errors` per T006,
  `component`/`stack` when passed non-empty and *absent from the map* (not empty-string) when
  either is empty, and `"version": 1`.
- [X] T008 [P] [US3] Extend `TestTerraformCaptureShellOpts_AlwaysWiresCaptureAndParser`/
  `TestTerraformExecMetadataParserFunc_ReadsBuffersAtCallTime` in
  `cmd/terraform/utils_exec_metadata_test.go` to pass/assert `component`/`stack` flow from
  `terraformCaptureShellOpts(component, stack)` through `terraformExecMetadataParserFunc`
  into `buildTerraformExecData`'s call.
- [ ] T009 [P] [US3] Add cases to `cmd/terraform/plan_test.go`/`apply_test.go`/`deploy_test.go`
  (whichever already exercise `RunE`'s shell-opts construction) asserting
  `terraformCaptureShellOpts` is called with `args[0]` (when present) and the parsed
  `--stack`/`-s` value — not empty strings — for a single-component invocation.
- [X] T010 [P] [US3] Extend `pkg/proexec/envelope_test.go`'s
  `TestBuildRecord_SecretMaskingAppliedToData` (or add a sibling test) asserting both masking
  layers run independently: a Terraform-sensitive output masked by `maskSensitiveOutputs`
  stays masked after the Gitleaks `maskedDataJSON` pass; a separate non-sensitive output
  containing a Gitleaks-pattern-matching literal (e.g. an AWS access key shape) is still
  caught by the Gitleaks pass even though `maskSensitiveOutputs` left it untouched; a
  `version` field survives both passes unchanged.

### Implementation for User Story 3

- [X] T011 [US3] In `cmd/terraform/utils.go`: add `maskSensitiveOutputs(outputs
  map[string]json.RawMessage) map[string]any`, importing `pkg/io` for `MaskReplacement`.
  Makes T005 pass.
- [X] T012 [US3] In `cmd/terraform/utils.go`: extend `terraformOutputResultMirror` with
  `HasChanges bool \`json:"HasChanges"\``, `HasErrors bool \`json:"HasErrors"\``, `Errors
  []string \`json:"Errors"\``, decoded via the same JSON round-trip `parseTerraformOutputMirror`
  already performs (no second parse). `parseTerraformOutputMirror` returns the fuller struct
  (or its three new fields alongside the existing `Data`/`ok` return) so `buildTerraformExecData`
  can consume them. Makes T006 pass.
- [X] T013 [US3] In `cmd/terraform/utils.go`'s `buildTerraformExecData`: change signature to
  `buildTerraformExecData(subCommand, output, component, stack string) any`; call
  `maskSensitiveOutputs(data.Outputs)` in place of the current `"outputs": data.Outputs`
  pass-through; add `"has_changes"`, `"has_errors"`, `"errors"` from T012's decoded fields;
  add `"component"`/`"stack"` to the map only when the corresponding parameter is non-empty;
  add `"version": 1`. Makes T007 pass. Depends on T011, T012.
- [X] T014 [US3] In `cmd/terraform/utils.go`: change `terraformExecMetadataParserFunc(stdoutBuf,
  stderrBuf *bytes.Buffer, component, stack string) func(subCommand string) any` and
  `terraformCaptureShellOpts(component, stack string) (...)` to thread the two new parameters
  through to T013's call. Makes T008 pass. Depends on T013.
- [X] T015 [US3] In `cmd/terraform/plan.go`/`apply.go`/`deploy.go`: update each `RunE`'s
  `terraformCaptureShellOpts()` call to `terraformCaptureShellOpts(component, stack)`, where
  `component` is `args[0]` when `len(args) > 0` (empty string otherwise — the multi-component
  `--affected`/`--all` path never reaches this closure, gated out by
  `captureExecMetadataSync`'s existing `info.NodeHooks == nil` check) and `stack` is the
  already-parsed `--stack`/`-s` value each `RunE` already resolves for other purposes (no new
  parsing). Makes T009 pass. Depends on T014.

**Checkpoint**: `TerraformExecData` now masks sensitive outputs via a dedicated layer,
reports `has_changes`/`has_errors`/`errors`, carries `component`/`stack` for single-component
runs, and includes `version: 1` — closing all three US3-scoped gaps from the seventh re-plan.

---

## Phase 6: User Story 4 - Pro inventory visibility for `describe affected` and `list instances` (Priority: P4)

**Goal**: `describe affected`'s and `list instances`' execution records carry structured
`Data` reusing data already computed for their existing Pro uploads (`POST
/api/v1/affected-stacks`, `POST /api/v1/instances`), instead of always sending `Data: nil`.

**Independent Test**: Run `atmos describe affected` (no `--upload` needed) in CI with Atmos
Pro configured; confirm the logged request body's `data` is `{"version": 1, "stacks":
[...]}`. Separately, run `atmos list instances --upload` and confirm `data` is `{"version":
1, "instances": [...]}`; run `atmos list instances` without `--upload` and confirm `data` is
absent (quickstart.md steps 15-16; spec.md US4 Independent Test/Acceptance Scenarios 1-3).

### Tests for User Story 4 ⚠️

- [X] T016 [P] [US4] Extend `internal/exec/describe_affected_test.go`/`describe_affected_upload_test.go`
  asserting `executeInner`'s new `([]schema.Affected, error)` return matches the slice it
  already computes internally (no new computation to assert — a signature-shape test).
- [X] T017 [P] [US4] Extend `internal/exec/describe_affected_upload_test.go` asserting `Execute`
  passes `Data: map[string]any{"version": 1, "stacks": affected}` (via `proexec.VersionedData`)
  to `proexec.CaptureSync`'s `ExecRecordInput`, for both a `--upload` and a non-`--upload`
  invocation (the list is unconditional — no gating, unlike `list instances`). Mock/spy
  `proexec.CaptureSync` the same way this file's existing cases already do.
- [X] T018 [P] [US4] Extend `pkg/list/list_instances_upload_test.go` asserting
  `ExecuteListInstancesCmd` calls `proexec.SetPendingAsyncData(proexec.VersionedData(1,
  "instances", req.Instances))` only inside the existing `if opts.Upload { ... }` branch —
  present after an `--upload` run, absent (never called) after a non-`--upload` run. Assert by
  resetting the package's `pendingAsyncData` state before each case and inspecting it after
  (matching the test style already used for `currentAtmosConfig` in `pkg/proexec/async_test.go`),
  or via a small test-only accessor if `pendingAsyncData` needs one.

### Implementation for User Story 4

- [X] T019 [US4] In `internal/exec/describe_affected.go`: change `executeInner(a
  *DescribeAffectedCmdArgs) error` to `executeInner(a *DescribeAffectedCmdArgs)
  ([]schema.Affected, error)`, returning the already-computed `affected []schema.Affected`
  slice (the same one used for rendering and, inside the existing `if args.Upload` branch,
  `UploadAffectedStacksRequest.Stacks`) alongside its existing error — no second computation.
  Makes T016 pass. Depends on nothing (independent of Phase 5).
- [X] T020 [US4] In `internal/exec/describe_affected.go`'s `Execute`: capture `executeInner`'s
  new `affected` return value; replace the current `ExecRecordInput{Command: "describe
  affected", Flags: flags, ExitCode: exitCode}` (implicit `Data: nil`) with one that also sets
  `Data: proexec.VersionedData(1, "stacks", affected)`. Update the function's doc comment
  (currently: "Data is passed as nil — describe affected has no defined structured-data
  extension") to reflect the new behavior. Makes T017 pass. Depends on T019, T003
  (Foundational).
- [X] T021 [US4] In `pkg/list/list_instances.go`'s `ExecuteListInstancesCmd`: inside the
  existing `if opts.Upload { ... }` branch, after `req.Instances` is built (same point
  `apiClient.UploadInstances(&req)` is called, ~line 536-544), add `proexec.SetPendingAsyncData(
  proexec.VersionedData(1, "instances", req.Instances))`. Add the `pkg/proexec` import (none
  exists in this file today). Makes T018 pass. Depends on T003, T004 (Foundational).

**Checkpoint**: `describe affected` always attaches its `{version, stacks}` structured data;
`list instances` attaches `{version, instances}` only when `--upload` was passed, with zero
added cost to the plain (non-uploading) invocation. Closes US4's Acceptance Scenarios 1-3.

---

## Phase 7: Polish & Cross-Cutting Concerns

- [X] T022 [P] Regenerate the Pact contract to 15 total interactions (covering both US3's and
  US4's shapes — up from today's fewer —
  verify exact current count in `pacts/atmos-AtmosPro.json` before regenerating), adding, in
  `pkg/pro/consumer_pact_test.go`: (a) `version`/`has_changes`/`has_errors`/`errors`/
  `component`/`stack` assertions to the existing `TestPact_UploadExecMetadata`
  (interaction 9, terraform inline) and `TestPact_UploadExecMetadata_BlobURL` (interaction
  10); (b) two new test functions for `describe affected`'s shape (inline + blob-URL,
  interactions 12/13, paired with a `describe affected`-flavored `UploadExecData` interaction
  14); (c) two new test functions for `list instances`' shape (inline + blob-URL, interaction
  15 + its paired `/exec/data` interaction) — per `contracts/interactions.md`'s interactions
  12-15 and research.md Decision 25. Every new/extended `data` example's `version` field MUST
  be asserted as an exact literal `1` (`Like`, not exact-literal, is wrong here per
  contracts/interactions.md's explicit rule). Regenerate via `rm pacts/atmos-AtmosPro.json &&
  go test -tags pact ./pkg/pro/...`, then `git diff pacts/atmos-AtmosPro.json`.
- [X] T023 [P] Run `go test -cover` for every touched package (`cmd/terraform`, `pkg/proexec`,
  `internal/exec`, `pkg/list`, `pkg/pro`) and confirm each stays at/above the 85% floor
  (CLAUDE.md MANDATORY); fix any gap introduced by T011-T021 with additional table-driven
  cases rather than coverage theater.
- [X] T024 `atmos lint --changed` and `go build ./...` across all touched files.
- [X] T025 Manually walk `quickstart.md` steps 12-17 end-to-end against a local Pro stub or
  test workspace (masking, shape completeness, `component`/`stack`, `describe affected`'s
  `Data`, `list instances`' `Data` with/without `--upload`, and `version` present on all three
  shapes) — requires a live/stubbed Atmos Pro endpoint and a human or the `run` skill,
  matching the prior re-plan's T031.
- [X] T026 Docs check: none of this regeneration's seven deltas add a new user-facing CLI
  flag, command, or config surface (`describe affected`/`list instances` reuse their existing
  `--upload` flags unchanged; the only new user-visible artifact is upload *payload content*,
  not a CLI surface) — confirmed against CLAUDE.md's "All new commands/flags/parameters MUST
  have Docusaurus documentation" rule: **no `website/docs/cli/commands/` changes are
  required** for this regeneration.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: Empty.
- **Foundational (Phase 2)**: `VersionedData` (T003) and `SetPendingAsyncData` (T004) are
  required by Phase 6/US4 (T020, T021) but NOT by Phase 5 (US3's terraform deltas add
  `"version": 1` as a plain map key, deliberately not via `VersionedData` — research.md
  Decision 24). Phase 5 and Phase 2 can proceed fully in parallel.
- **US1 (Phase 3)** / **US2 (Phase 4)**: No remaining tasks — already shipped, untouched by
  this regeneration.
- **US3 (Phase 5)**: Independent of Phase 2/Phase 6 (US4). Internally sequential where noted
  (T011→T013, T012→T013, T013→T014, T014→T015); T005-T010 (tests) are `[P]` — different
  concerns/files, though several target the same file (`utils_exec_metadata_test.go`) so
  coordinate if run by parallel agents to avoid edit conflicts.
- **US4 (Phase 6)**: Depends on Phase 2 (T003, T004). T019/T020 (`describe affected`) are
  independent of T021 (`list instances`) — different files, different commands. Independent of
  Phase 5 (US3) — no shared files, no shared dependency in either direction.
- **Polish (Phase 7)**: T022 depends on Phase 5 (US3) AND Phase 6 (US4) both landing (it
  covers all three shapes). T023/T024 depend on all prior phases. T025/T026 have no code
  dependency and can run once any subset has landed, but are most meaningful after everything
  above is done.

### Parallel Opportunities

- T001-T002 (Foundational tests) are `[P]` — different files.
- T005-T010 (US3 tests) are `[P]` in intent (different concerns) but several share
  `cmd/terraform/utils_exec_metadata_test.go` — a single agent/session should own that file's
  edits to avoid clobbering, even though the tasks are conceptually independent.
- T016-T018 (US4 tests) are `[P]` — three different files/packages.
- T019 (`describe affected` signature change) has no dependency on T021 (`list instances`) —
  can proceed fully in parallel by different sessions.
- T022-T024 (Polish) are `[P]` — independent concerns.
- Phase 5 (US3) and Phase 2 (Foundational) can run fully in parallel — no shared files, no
  shared dependency in either direction. Phase 5 (US3) and Phase 6 (US4) can also run fully in
  parallel — different commands, different files.

---

## Implementation Strategy

### MVP First

There is no new MVP here — US1/US2's MVP already shipped in a prior session. The smallest
next increment is **Phase 5 alone** (US3's three terraform gaps): it's independently
testable/shippable (quickstart.md steps 12-14) and doesn't require Phase 2/6 at all.

1. Complete Phase 5 (US3 — masking, shape completeness, `component`/`stack`): T005-T010
   (tests) → T011-T015 (implementation)
2. **STOP and VALIDATE**: run the new/extended tests, confirm GREEN; walk quickstart.md steps
   12-14
3. This alone closes FR-010a/the `has_changes`/`has_errors`/`errors` gap/the `component`/
   `stack` gap for every `terraform plan`/`apply`/`deploy` invocation, independent of whether
   `describe affected`/`list instances` (Phase 6) ship in the same pass.

### Incremental Delivery

1. Phase 5 (US3 terraform deltas) → validate → ship independently (see MVP above)
2. Phase 2 (Foundational: `VersionedData`, `SetPendingAsyncData`) → validate → ship (unblocks
   Phase 6; can be done before, after, or in parallel with Phase 5)
3. Phase 6 (`describe affected` + `list instances` structured data) → validate (quickstart.md
   steps 15-16) → ship
4. Phase 7 (Polish): T022 (6-interaction Pact regeneration) only after both Phase 5 and Phase
   6 have landed, since it covers all three shapes together; T023-T026 as each phase completes
