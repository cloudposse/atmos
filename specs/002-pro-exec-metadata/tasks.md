---

description: "Task list for the twelfth re-plan: TerraformExecData's logs field (base64, masked-before-encoding), single/multi-component Data unification, and the sensitive-output-flag documentation-only clarification"
---

# Tasks: Atmos Pro Command-Execution Metadata Upload — Twelfth Re-Plan (logs field / unified components shape)

**Input**: Design documents from `specs/002-pro-exec-metadata/` (plan.md twelfth re-plan, spec.md FR-006a/FR-006i/FR-010a, research.md Decisions 37-38, data-model.md "Unified shape" section, contracts/interactions.md interaction 9/10)

**Prerequisites**: plan.md (twelfth re-plan), spec.md, research.md Decisions 37-38, data-model.md

**Tests**: Included — this repo's CLAUDE.md mandates a reproduce-first bug-fixing workflow for bug fixes, and unit tests for all new/changed exported and package-private functions per Constitution Principle III.

**Organization**: All tasks serve **User Story 3** ("Structured infrastructure-change data for plan/apply/deploy", spec.md, Priority P3) — the `TerraformExecData` payload itself. Tasks are grouped by the four independent deltas within the US3 phase: (A) `logs` field (add, mask-before-encode, base64), (B) multi-component restructure (flat entries → list of full `TerraformExecData` objects), (C) single/multi-component shape unification (drop the bare-object single-component design), (D) sensitive-output-flag known-limitation documentation (no code change). `pkg/ci/plugins/terraform`'s extraction logic itself is not touched by this re-plan — its `Sensitive`-detection limitation is recorded, not fixed (out of scope, shared with Native CI).

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[US3]**: All story-phase tasks belong to User Story 3
- File paths are exact, current locations confirmed against the tree at the time this list was generated — re-verify line numbers before editing, since surrounding code may have shifted slightly.

## Path Conventions

Single Go project at repository root: `cmd/terraform/`, `pkg/pro/`, `pkg/ci/plugins/terraform/`.

---

## Phase 1: Setup

Not applicable — this re-plan extends already-shipped files only; no new project scaffolding, dependencies, or packages are introduced.

---

## Phase 2: Foundational

Not applicable — Groups A-D are independent deltas with no shared blocking prerequisite, though B and C are sequenced (C builds directly on B's per-component `TerraformExecData` entries).

---

## Phase 3: User Story 3 — Structured infrastructure-change data, final shape (Priority: P3)

**Goal**: `TerraformExecData` carries the full scoped plan/apply/deploy console text (masked, base64-encoded) as `logs`; `Data` for every `terraform plan`/`apply`/`deploy` invocation — single- or multi-component — is the identical `{"version": 1, "components": [TerraformExecData, ...]}` shape, with no per-component `version` field; and the known limitation in `outputs[*].sensitive` detection is recorded in the spec without a code change.

**Independent Test**: Run `atmos terraform plan` against a single component with pending changes and a sensitive output, in CI with Atmos Pro configured; confirm the uploaded record's `Data` is `{"version": 1, "components": [{...}]}` (one entry, no `version` inside it), that entry's `logs` field decodes (base64) to the real console text with any sensitive output's literal value redacted, and `outputs[*].value` is `<MASKED>` for the sensitive entry. Separately, run `atmos terraform plan --affected` against multiple components and confirm `components` has one entry per component, each shaped identically to the single-component case.

### Group A — `logs` field: add, mask-before-encode, base64 (FR-006i, FR-010a, research.md Decision 38)

- [X] T001 [US3] Add `logs` (base64-encoded) to `buildTerraformExecData`'s returned map in `cmd/terraform/utils.go`, replacing the field that was briefly named `raw_output` — set from `encodeLogs(output)` in the default/unparseable branch and `encodeLogs(redactSensitiveOutputsFromRawOutput(output, data.Outputs))` in the parse-succeeded branch
- [X] T002 [P] [US3] Implement `redactSensitiveOutputsFromRawOutput(text string, outputs map[string]json.RawMessage) string` in `cmd/terraform/utils.go` — replaces every literal occurrence of a string-valued, `Sensitive: true` output's own value with `iolib.MaskReplacement`; document the known limitation that the production regex parser never actually sets `Sensitive: true` (see Group D)
- [X] T003 [P] [US3] Implement `encodeLogs(text string) string` in `cmd/terraform/utils.go` — applies `iolib.MaskString` (the same Gitleaks-pattern masking `pkg/proexec/envelope.go` uses) to the plaintext, THEN base64-encodes; masking must happen before encoding since a downstream secret-pattern scan cannot see into base64-encoded bytes
- [X] T004 [US3] Unit test `TestBuildTerraformExecData_LogsWiredThroughRedactionAndEncoding` in `cmd/terraform/utils_exec_metadata_test.go` — confirms `buildTerraformExecData`'s `logs` field decodes back to expected text via the real fixture, proving the wiring (not just the standalone helpers). Depends on T001
- [X] T005 [P] [US3] Unit test `TestRedactSensitiveOutputsFromRawOutput` in `cmd/terraform/utils_exec_metadata_test.go` — string-valued sensitive output redacted everywhere it appears; non-sensitive value untouched; non-string sensitive value skipped (no single unambiguous literal form). Depends on T002
- [X] T006 [P] [US3] Unit test `TestEncodeLogs` in `cmd/terraform/utils_exec_metadata_test.go` — round-trips plaintext through base64 encode/decode. Depends on T003
- [X] T007 [US3] Update `TestBuildTerraformExecData_ApplySuccess` and `TestBuildTerraformExecData_UnparseableOutputStillAttachesMinimalData` in `cmd/terraform/utils_exec_metadata_test.go` to assert against `logs` (base64-decoded), not the old `raw_output` string field. Depends on T001

### Group B — Multi-component restructure: flat entries → list of full `TerraformExecData` objects (FR-006a, research.md Decision 37)

- [X] T008 [US3] Remove `parseTerraformResourceChanges` from `cmd/terraform/utils.go` (now unused) and change `terraformNodeHooks.recordExecResult` to call `buildTerraformExecData` directly per graph node instead of hand-flattening resource changes — `n.results` becomes `[]any`, each entry one node's full `TerraformExecData` object. `execNodeResult` reverts to its original, narrower meaning: the `{action, address}` entry type inside a single `TerraformExecData.changes` list. Depends on T001 (both are in `buildTerraformExecData`'s output shape)
- [X] T009 [US3] Update `captureMultiComponentExecMetadata` (`cmd/terraform/utils.go`) to wrap `hooks.results` via `proexec.VersionedData(terraformExecDataVersion, "components", components)` instead of sending the flat list directly as `Data`
- [X] T010 [US3] Update `TestTerraformNodeHooks_RecordExecResultAccumulates` and `TestCaptureMultiComponentExecMetadata_ExactlyOneRequestForWholeRun` in `cmd/terraform/utils_exec_metadata_test.go` for the new per-node full-object shape and the `{"version": ..., "components": [...]}` wire shape. Depends on T008, T009
- [X] T011 [P] [US3] New Pact consumer interaction `TestPact_UploadExecMetadata_MultiComponent` in `pkg/pro/consumer_pact_test.go` — asserts the `{"version": 1, "components": [TerraformExecData, TerraformExecData]}` shape for a 2-component run; regenerate `pacts/atmos-AtmosPro.json` (`go test -tags pact ./pkg/pro/...`). Depends on T009

### Group C — Single/multi-component shape unification (research.md Decision 38, direct user correction)

- [X] T012 [US3] Add `stripComponentVersion(data any) any` and `wrapComponentsData(entries ...any) any` helpers to `cmd/terraform/utils.go` — the former deletes a `buildTerraformExecData` result's own `version` key, the latter wraps one or more stripped entries as `{"version": terraformExecDataVersion, "components": [...]}` via `proexec.VersionedData`. Depends on T008 (co-locates with the multi-component restructure it generalizes)
- [X] T013 [US3] Change `terraformExecMetadataParserFunc` (`cmd/terraform/utils.go`, the single-component call site) to call `stripComponentVersion` then `wrapComponentsData` on `buildTerraformExecData`'s result before returning — single-component `Data` becomes `{"version": 1, "components": [TerraformExecData]}` (one entry), never a bare `TerraformExecData` object. Depends on T012
- [X] T014 [US3] Change `terraformNodeHooks.recordExecResult`/`captureMultiComponentExecMetadata` (`cmd/terraform/utils.go`) to call the same `stripComponentVersion`/`wrapComponentsData` helpers instead of the inline logic T008/T009 introduced, so both call sites share one code path. Depends on T012
- [X] T015 [US3] Update `TestTerraformExecMetadataParserFunc_UsesSuppliedOutput` in `cmd/terraform/utils_exec_metadata_test.go` to unwrap `{"version": ..., "components": [...]}` and assert the one entry has no `version` key. Depends on T013
- [X] T016 [P] [US3] Rewrite `TestPact_UploadExecMetadata`/`TestPact_UploadExecMetadata_BlobURL` in `pkg/pro/consumer_pact_test.go` to use the unified `{"version": 1, "components": [{...}]}` shape (previously a bare `TerraformExecData` object); regenerate `pacts/atmos-AtmosPro.json`. Depends on T013

### Group D — Sensitive-output-flag known limitation: documentation only, no code change (FR-010a, 2026-08-21 clarification)

- [X] T017 [P] [US3] Regression test `TestExtractApplyOutputs_SensitiveOutputNeverExposesRealValue` in `pkg/ci/plugins/terraform/parser_test.go` — proves `extractApplyOutputs` returns Terraform's own `<sensitive>` placeholder (never a real secret) for a sensitive output, and that `Sensitive` stays `false` (the documented, accepted gap)
- [X] T018 [P] [US3] End-to-end regression test `TestBuildTerraformExecData_SensitiveOutputNeverUploadedInAnyForm` in `cmd/terraform/utils_exec_metadata_test.go` — confirms neither `Data.components[].outputs[*].value` nor the base64-decoded `logs` field ever carries a real secret for a sensitive output, despite the `Sensitive` flag being inaccurate. Depends on T001 (needs `logs`)
- [X] T019 [US3] Amend `FR-010a` in `spec.md` with a "Known limitation" note: the regex console parser never sets `Sensitive: true` in production, so layer (1)'s redaction never triggers against real output; the no-leak property holds via Terraform's own console behavior instead. No code change — explicitly recorded as accepted, not a defect
- [X] T020 [P] [US3] Add the matching "Known limitation" note to `data-model.md`'s "Outputs masking" section, cross-referencing T017/T018's regression tests. Depends on T019

**Checkpoint**: `TerraformExecData`'s final shape — `logs` field, unconditional `components`-wrapper, no per-component `version` — is shipped and tested for both single- and multi-component invocations; the sensitive-flag limitation is recorded in the spec, not silently left undocumented.

---

## Phase 4: Polish & Cross-Cutting Concerns

- [X] T021 Run `go build ./...`, `go test ./cmd/terraform/... ./pkg/proexec/... ./internal/exec/...`, `go test -tags pact ./pkg/pro/...`, and `atmos lint --changed` — fix any findings before proceeding. Depends on all of Groups A-D
- [X] T022 Update `research.md` with Decision 37 (multi-component restructure) and Decision 38 (logs rename/encoding + unification), including alternatives-considered and the breaking-change note re: `execNodeResult`'s reverted meaning. Depends on T008, T012
- [X] T023 [P] Update `data-model.md`'s `TerraformExecData` section end-to-end: `logs` field description, the "Unified shape" paragraph replacing the old single-vs-multi-component split, the worked-example JSON. Depends on T013, T014
- [X] T024 [P] Update `contracts/interactions.md` interactions 9/10: `data` example rewritten to the `{"version": 1, "components": [...]}` wrapper, `components[*]` field table (incl. `logs`, no `version`), Validation Rules table's `version`-field row scoped to "outer wrapper only". Depends on T013, T014
- [X] T025 Regenerate `plan.md` (twelfth re-plan) summarizing all four groups, updated Technical Context/Constitution Check/Project Structure. Depends on T021-T024

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: N/A
- **Foundational (Phase 2)**: N/A
- **User Story 3 (Phase 3)**: Group A is independent. Group B depends on Group A only insofar as both touch `buildTerraformExecData`'s output shape (T008 depends on T001). Group C depends on Group B (T012 generalizes T008/T009's inline logic). Group D is independent of A/B/C except T018, which needs `logs` (T001)
- **Polish (Phase 4)**: Depends on all of Groups A-D

### Within Group A (logs field)

T001 → T004, T007 (wiring, then tests that depend on it) · T002 → T005 · T003 → T006 (helpers, then their standalone tests) — T002/T003/T005/T006 can run in parallel with each other

### Within Group B (multi-component restructure)

T008 → T009 → T010 → T011 (strictly sequential — each step builds directly on the previous)

### Within Group C (unification)

T012 → T013, T014 (both call sites adopt the shared helpers in parallel) → T015, T016 (tests follow)

### Within Group D (documentation)

T017, T018 independent (parallel) → T019 → T020

### Parallel Opportunities

- T002/T003/T005/T006 (Group A helpers + their tests) can run in parallel — different functions, no shared state
- T011 [P] relative to the rest of Group B's sequential chain — independent Pact interaction
- T016 [P] relative to Group C's core change — independent Pact test file
- T017/T018 [P] — independent test files, no shared state
- T023/T024 [P] relative to each other — independent doc files, both only depend on the code being final

---

## Parallel Example: Groups A and D (independent of each other)

```bash
# Track 1 — logs field (cmd/terraform/utils.go):
Task: "Add encodeLogs/redactSensitiveOutputsFromRawOutput and wire into buildTerraformExecData (T001-T003)"
Task: "Unit tests for the new helpers and end-to-end wiring (T004-T007)"

# Track 2 — sensitive-flag documentation (pkg/ci/plugins/terraform/, spec.md, data-model.md):
Task: "Regression tests proving no real secret ever uploaded despite the flag gap (T017-T018)"
Task: "Record the known limitation in FR-010a and data-model.md (T019-T020)"
```

---

## Implementation Strategy

### Recommended order

1. Group A (`logs` field) first — Group B's per-component entries need `buildTerraformExecData` to already include `logs`
2. Group B (multi-component restructure) next — Group C's unification helpers generalize what Group B introduces inline
3. Group C (unification) — the direct user correction that retired the bare-object single-component design
4. Group D (documentation) can land any time after Group A (T018 needs `logs`) — independent of B/C
5. Phase 4 (Polish) last, gating merge — includes regenerating every design doc to match the final shape

### Incremental delivery

Each group closes a distinct, independently-verifiable change (see plan.md's Summary) and was in fact implemented and verified incrementally within this session — Group A → Group B → Group C → Group D, with `atmos lint --changed` and the full relevant test suite run after each.

---

## Notes

- This re-plan's Group C change is a **breaking change** to the already-implemented single-component wire shape (a bare `TerraformExecData` object at the top level no longer exists) — both `pacts/atmos-AtmosPro.json` and `contracts/interactions.md` needed corresponding updates (T016, T024)
- `execNodeResult`'s meaning changed twice within this re-plan's history: Group B's predecessor (the eleventh re-plan's era) used it as a flat multi-component identity/outcome entry; Group B retires that use and reverts it to its original, narrower `{action, address}` role
- No new Pact interaction was needed for the blob-URL delivery path specifically for the unified/multi-component shape — the inline-vs-blob-URL mechanism (FR-011) is shape-agnostic and already proven by the existing single-shape blob-URL interaction (spec.md, 2026-08-21 clarification)
- Follow this repo's mandatory bug-fixing workflow only where applicable (T017/T018 are documentation-driven regression tests for an accepted limitation, not bug fixes — no fix is expected or wanted)
- Avoid platform-specific test fixtures (no hardcoded Unix paths, no shell-outs to `terraform`) — all new tests use inline fixtures per this repo's cross-platform testing conventions
