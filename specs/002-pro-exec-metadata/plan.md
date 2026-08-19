# Implementation Plan: Atmos Pro Command-Execution Metadata Upload

**Branch**: `1199-pro-exec-metadata` | **Date**: 2026-08-19 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/002-pro-exec-metadata/spec.md`

**Note**: This is a third re-plan. US1/US2 shipped; the `CaptureSync`/`CaptureAsync`
double-fire regression fix (`proexec.IsSyncCommand`) and the second re-plan's
`Command`/`Args`/`Flags` shape addition (new `Flags` field, `Command` root-stripped,
`Args` populated — commit `2fe4fabe0`) are both present in the current tree. This revision
addresses a **third, distinct** correctness gap found this session while writing a
regression test against a fresh production report (2026-08-19): the second re-plan's
`Flags` field exists and is wired up, but `captureExecMetadataSync`
(`internal/exec/terraform.go`) sources it from `info.AdditionalArgsAndFlags` — a
pass-through-args collection that structurally never contains atmos-recognized flags like
`-s`/`--stack` and additionally has `--upload-status` stripped out of it before capture
runs. The result: for a typical `terraform plan ... -s ... --upload-status` invocation,
`Flags` comes back empty, exactly matching the reported production row. A regression test
(`internal/exec/terraform_exec_metadata_flags_test.go`, added this session) reproduces this
and fails on current HEAD. `/speckit-clarify` (2026-08-19) additionally resolved two shape
ambiguities this bug surfaced: `--upload-status` MUST be included in `Flags` (no
exclusions), and `Flags` MUST serialize as bare tokens as typed, not `--name value` pairs
for every flag — which also means the async path's existing `commandArgsAndFlags`
serialization needs the same correction.

A second, separate production symptom — two `atexec_*` rows for one invocation with
different `workflow_job` ids — was investigated this session but not conclusively
root-caused from static inspection alone; it is tracked as a new verification task
(`tasks.md` T026) requiring a real end-to-end test, not yet implemented, and is **not**
part of this plan's implementation scope.

## Summary

One fix, layered on top of the already-shipped dedup/allowlist/aggregation/Command-Args-Flags
work from the first and second re-plans:

6. **`Flags` source-of-truth fix (this delta)**: `captureExecMetadataSync`
   (`internal/exec/terraform.go:239`) currently passes `info.AdditionalArgsAndFlags` as
   `CaptureSync`'s `flags` argument. That field is populated by `cli_utils.go` as
   `args[componentArgIndex+1:]` — everything after the component positional argument that
   Cobra did **not** already parse as a recognized atmos flag — so `-s plat-use2-dev` is
   never in it, and `buildPlanSubcommandArgs` (`terraform_execute_helpers_args.go:36`)
   additionally strips `--upload-status` out of it before `captureExecMetadataSync` runs.
   Per the FR-003b clarification (2026-08-19), the fix must:
   - Source `Flags` from the invoking `*cobra.Command`'s own record of explicitly-set flags
     (`cmd.Flags().Visit`, `Changed == true`) — the same correct pattern the async path's
     `commandArgsAndFlags` (`pkg/proexec/async.go:109-126`) already uses for its own
     `Flags` population. This likely means threading the `*cobra.Command` down to
     `captureExecMetadataSync` (or extracting a shared helper both paths call), since
     `internal/exec/terraform.go` does not currently have access to it at that call site.
   - Preserve every flag with no exclusions, including `--upload-status`.
   - Serialize each flag as a bare token exactly as typed — a boolean/valueless flag (e.g.
     `--upload-status`) appears alone with no synthesized value; a value-bearing flag (e.g.
     `-s`/`--stack`) contributes its token and value as two separate array entries, in the
     order typed.
   - Correct the async path's `commandArgsAndFlags` (`pkg/proexec/async.go:121-123`) to
     match: it currently appends `"--"+f.Name, f.Value.String()` unconditionally, producing
     a synthesized `"true"` for every bool flag — this must become conditional on the
     flag's `Value.Type()`, skipping the value for `"bool"`-typed flags.

## Technical Context

**Language/Version**: Go 1.26

**Primary Dependencies**:
- `internal/exec/terraform.go` (existing, shipped — `captureExecMetadataSync`) — modified
  for this delta to source `Flags` from the real `*cobra.Command` instead of
  `info.AdditionalArgsAndFlags`
- `pkg/proexec/async.go` (existing, shipped — `commandArgsAndFlags`) — modified for this
  delta to serialize bool-typed flags without a synthesized value, and (if a shared helper
  is extracted) to become the single source of truth both delivery paths call
- `pkg/proexec` (existing, shipped — `gateOpen`, `buildRecord`, `CaptureAsync`,
  `CaptureSync`, `classify.IsSyncCommand`) — unchanged by this delta; `Flags` already flows
  through `buildRecord`/`ExecUploadRequest` correctly once callers pass the right data
- `github.com/spf13/pflag` (existing dependency, already imported by `async.go`) — provides
  `Flag.Value.Type()` used to detect bool-typed flags

**Storage**: N/A — unchanged; no local persistence.

**Testing**: `atmos test` (unit, table-driven, mocked/`httptest`-faked Atmos Pro server).
New regression test already added this session:
`internal/exec/terraform_exec_metadata_flags_test.go::TestCaptureExecMetadataSync_FlagsReflectRealInvocation`
(currently RED — asserts `Flags` is non-empty for an invocation shaped like the production
report). Additional cases needed: a bool-flag-only invocation asserting no synthesized
value is appended (covers the `async.go` correction), and a mixed value-bearing +
bool-flag invocation asserting both flags appear as bare tokens in order. Existing tests
that assumed `info.AdditionalArgsAndFlags` was an acceptable `Flags` source (none currently
assert on the field's *content*, only that `captureExecMetadataSync` doesn't panic — see
`TestCaptureExecMetadataSync_ComponentAndFlags`) are not invalidated by this fix but should
gain a content assertion once the fix lands.

**Target Platform**: Linux, macOS, Windows — unchanged.

**Project Type**: CLI feature — targeted fix to already-shipped
`internal/exec/terraform.go` and `pkg/proexec/async.go`. No new packages, no new DTO
fields (the `Flags` field itself already exists from the second re-plan).

**Performance Goals**: Unchanged (SC-003/SC-004) — this delta only changes what one
existing field contains, not the delivery/timeout mechanics.

**Constraints**:
- No new user-facing opt-out (unchanged).
- No DTO shape change — `Flags []string` already exists on `ExecUploadRequest`
  (`pkg/pro/dtos/exec.go`); this delta only fixes what populates it.
- Masking: `Flags` continues to reuse the existing masking path (`buildRecord`'s
  `maskArgs`, per FR-010) — no new masking logic, since the fix is only about which flags
  are collected and how they're tokenized, not how they're sanitized.
- The duplicate-`atexec_*`-row investigation (`tasks.md` T026) is explicitly **out of
  scope** for this delta — it requires a real end-to-end test not yet written, and static
  inspection this session found the client-side dedup gate (`IsSyncCommand`) structurally
  sound, so there is no confirmed code change to make yet.

**Scale/Scope**: 0 new fields, 0 new files (beyond the regression test already added).
2 call-site changes: `internal/exec/terraform.go`'s `captureExecMetadataSync` (flags
source), `pkg/proexec/async.go`'s `commandArgsAndFlags` (bool-flag serialization) — plus,
if the shared-helper approach is taken, a new small helper function shared by both. This is
additive to, not a replacement for, the already-shipped work from the first and second
re-plans.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Registry-Driven Extensibility | ✅ Pass | No new CLI commands/flags/endpoints; a call-site data-source correction on an existing, already-shipped field. |
| II. Interface-Driven Design with DI | ✅ Pass | No interface changes; `captureExecMetadataSync` gains a `*cobra.Command` parameter (or delegates to a shared helper) — still a plain function, no new side effects. |
| III. Test-First with 80% Coverage | ✅ Pass | The regression test (`TestCaptureExecMetadataSync_FlagsReflectRealInvocation`) was written and confirmed RED before this plan, per CLAUDE.md's Bug-Fixing Workflow. Additional bool-flag-serialization cases must land alongside the `async.go` fix. |
| IV. Separated I/O and UI Architecture | ✅ Pass | No new user-visible output. |
| V. Simplicity and No Over-Engineering | ✅ Pass | Rejected fixing only the stripping-order symptom (capture flags before `buildPlanSubcommandArgs` mutates `info.AdditionalArgsAndFlags`) in favor of fixing the actual wrong-data-source defect — the narrower fix would still permanently omit `-s`/`--stack` and every other atmos-recognized flag. |

**Post-design re-check**: ✅ Pass. Phase 1 complete — `data-model.md`'s `Flags` row now
documents the source-of-truth requirement and bare-token shape (research.md Decision 14);
`contracts/interactions.md`'s existing example already matched the clarified shape, no
change needed there; `quickstart.md` step 8 already asserts the exact clarified shape,
no change needed. No new violations introduced.

## Project Structure

### Documentation (this feature)

```text
specs/002-pro-exec-metadata/
├── plan.md                 # This file — third re-plan, Flags source-of-truth delta
├── research.md              # Phase 0 output — Decision 14 appended for this delta
├── data-model.md            # Phase 1 output — Flags row updated (source-of-truth, bare-token shape)
├── quickstart.md            # Phase 1 output — step 8 already matches; no change needed
├── contracts/
│   └── interactions.md      # Phase 1 output — example already matched; no change needed
└── tasks.md                  # Phase 2 output (/speckit-tasks) — T006 superseded, T008/T009 revised, new T026 added this session; NEEDS TASK REGENERATION for a clean T0xx renumbering pass if desired, but current edits are directly actionable as-is
```

### Source Code (repository root)

```text
internal/exec/terraform.go
└── captureExecMetadataSync    # Modified — flags argument to proexec.CaptureSync sourced from
                                  the invoking *cobra.Command's Flags().Visit (Changed==true),
                                  not info.AdditionalArgsAndFlags

pkg/proexec/async.go
└── commandArgsAndFlags        # Modified — flag serialization becomes conditional on
                                  pflag.Flag.Value.Type() == "bool" (skip value for bool
                                  flags), replacing the unconditional "--name value" pair
                                  append. Strong candidate to become the single shared helper
                                  captureExecMetadataSync also calls, if it can be relocated/
                                  exported without breaking the async-path-only call site.

internal/exec/terraform_exec_metadata_flags_test.go
└── TestCaptureExecMetadataSync_FlagsReflectRealInvocation   # New — regression test, added
                                  this session, RED on current HEAD, must go GREEN once the
                                  above two changes land
```

**Structure Decision**: No new packages, no new files beyond the regression test already
added. This delta is intentionally the smallest change that resolves the clarified
ambiguity: correct the data source and serialization of one already-existing field. It does
not touch `pkg/pro/dtos/exec.go` (no DTO shape change) or attempt to also fix the separate,
unconfirmed duplicate-row question (`tasks.md` T026), consistent with the constitution's
simplicity principle — investigating and fixing two independent, not-yet-both-confirmed
bugs in one delta would conflate their test coverage and risk.

## Complexity Tracking

> No Constitution Check violations — section intentionally empty.
