# Implementation Plan: Atmos Pro Command-Execution Metadata Upload

**Branch**: `1199-pro-exec-metadata` | **Date**: 2026-08-18 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/002-pro-exec-metadata/spec.md`

**Note**: This is a second re-plan. US1/US2 shipped; the `CaptureSync`/`CaptureAsync`
double-fire regression (a single `atmos terraform plan` producing two execution records)
was already fixed on this branch (commit `63ab795af`, via the shared
`proexec.IsSyncCommand` predicate) and the `deploy`-allowlist/aggregation/US3-completion
scope from the first re-plan (2026-08-18, earlier same day) is unchanged and still
pending implementation. This revision addresses a **second, distinct** correctness gap
found via `/speckit-clarify` (2026-08-18, later same day): the two-execution-records
symptom reported in production was not fully explained by the `CaptureSync`/`CaptureAsync`
race alone — it also comes from **cross-feature duplication** between this feature's
`POST /v1/atmos/exec` and the older, independently-gated `uploadStatus`/`--upload-status`
`PATCH .../instances` mechanism (`internal/exec/pro.go`), which this feature was never
designed to coordinate with. The two paths are confirmed to stay independent (FR-003a),
but this revision fixes a live bug (`Args` always empty — `maskArgs(nil)` in
`pkg/proexec/envelope.go:55`) and tightens the wire shape of `Command`/`Args` so the two
independent records remain correlatable by content (FR-003b).

## Summary

One additional fix, layered on top of the still-pending dedup/allowlist/aggregation/US3
scope from the first re-plan:

5. **`Command`/`Args`/`Flags` shape fix (this delta)**: `pkg/proexec/envelope.go`'s
    `buildRecord` currently sends `Command = cmd.CommandPath()` (e.g.
    `"atmos terraform plan"`, including the `atmos` root) and always sends `Args = []`
    (the `maskArgs(nil)` call at line 55 never receives real arguments regardless of what
    was actually typed). Per FR-003b:
    - `Command` MUST drop the leading `atmos` root segment (`"terraform plan"`, not
      `"atmos terraform plan"`).
    - `Args` MUST carry only positional arguments (e.g. the component: `["cdn"]`),
      currently entirely unpopulated.
    - A **new** `Flags` field MUST carry the CLI flags actually passed (e.g.
      `["-s", "plat-use2-dev", "--upload-status"]`), each masked per the existing
      secret-masking used for `Args` today (FR-010) — positional args and flags are
      distinct fields, never combined into one array.
    This requires: a new `Flags []string` field on `dtos.ExecUploadRequest`
    (`pkg/pro/dtos/exec.go`), `buildRecord` splitting `cmd.Flags()`/positional `args`
    instead of calling `maskArgs(nil)`, and `Command` construction stripping the root
    segment (`strings.TrimPrefix(cmd.CommandPath(), cmd.Root().Name()+" ")` or equivalent).
    FR-003a additionally makes explicit that no code change coordinates this feature with
    `uploadStatus` — both continue to fire independently; this is a correlatable-content
    guarantee only, not a merge.

## Technical Context

**Language/Version**: Go 1.26

**Primary Dependencies**:
- `pkg/proexec` (existing, shipped — `gateOpen`, `buildRecord`, `CaptureAsync`, `CaptureSync`, `classify.IsSyncCommand`) — `envelope.go`'s `buildRecord` modified for this delta
- `pkg/pro` (existing, shipped — `AtmosProAPIClient.UploadExecMetadata`, `sendChunked`/`BatchInfo`) — `dtos.ExecUploadRequest` gains `Flags`
- `internal/exec/pro.go` (existing, unchanged by this delta — `uploadStatus`/`shouldUploadStatus`, the older `PATCH .../instances` path this feature stays independent from per FR-003a)
- `pkg/telemetry` (existing — `IsCI()`, the exec-metadata gate's CI check)
- `pkg/metrics/process` (existing, shipped)

**Storage**: N/A — unchanged; no local persistence.

**Testing**: `atmos test` (unit, table-driven, mocked `AtmosProAPIClientInterface`); new
cases for `buildRecord`: `Command` has no `atmos` prefix, `Args` contains only positional
arguments, `Flags` contains masked CLI flags, and a case asserting `Args`/`Flags` are never
combined into a single array. Existing tests asserting the old `Command`/`Args` shape
(full `atmos ...` path, always-empty `Args`) are now wrong and must be updated, not
preserved.

**Target Platform**: Linux, macOS, Windows — unchanged.

**Project Type**: CLI feature — targeted fix to already-shipped `pkg/proexec/envelope.go`
and `pkg/pro/dtos/exec.go`. No new packages.

**Performance Goals**: Unchanged (SC-003/SC-004) — this delta only changes what three
existing fields contain, not the delivery/timeout mechanics.

**Constraints**:
- No new user-facing opt-out (unchanged).
- `uploadStatus`/`--upload-status` (`internal/exec/pro.go`) is explicitly NOT modified by
  this delta (FR-003a) — no new DTO fields, no new call-site coordination, no shared
  record ID between the two mechanisms. Extending that DTO was considered and rejected
  during clarification (Option B) as unnecessary scope creep into a different,
  pre-existing endpoint this feature does not own.
- Masking: `Flags` MUST reuse the same masking path already applied to `Args`
  (`pkg/proexec/envelope.go`'s existing masking call, per FR-010) — no new masking logic.
- This corrects, not replaces, `data-model.md`'s original `ExecUploadRequest` shape
  (`Command = cmd.CommandPath()`, `Args` reserved/always-empty, "0 new DTO fields") from
  the initial 2026-08-11 plan — that shape is now superseded by FR-003b.

**Scale/Scope**: 1 new field on one existing Atmos Pro endpoint's request DTO (`Flags` on
`POST /v1/atmos/exec` — no new endpoint). 2 call-site changes: `pkg/pro/dtos/exec.go`
(new field), `pkg/proexec/envelope.go`'s `buildRecord` (Command/Args/Flags population).
This is additive to, not a replacement for, the still-pending dedup/`deploy`-allowlist/
aggregation/US3 work tracked in the same `specs/002-pro-exec-metadata/` artifacts.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Registry-Driven Extensibility | ✅ Pass | No new CLI commands/flags/endpoints; a DTO field addition on an existing endpoint. |
| II. Interface-Driven Design with DI | ✅ Pass | No interface changes; `buildRecord` remains a plain function with no new side effects. |
| III. Test-First with 80% Coverage | ✅ Pass | New table-driven cases for the `Command`/`Args`/`Flags` shape are written first, reproducing the observed always-empty-`Args` defect, per the constitution's bug-fix workflow. |
| IV. Separated I/O and UI Architecture | ✅ Pass | No new user-visible output. |
| V. Simplicity and No Over-Engineering | ✅ Pass | Rejected Option B (extending `uploadStatus`'s DTO for exact field-level equality) in favor of the smaller, narrowly-scoped `Flags` addition — avoids touching a second, unrelated upload mechanism to solve a correlation problem this feature can solve unilaterally. |

**Post-design re-check**: ✅ Pass. Phase 1 complete — `data-model.md`'s `ExecutionRecord`
table now documents `Command`/`Args`/`Flags` and the `uploadStatus` independence rule;
`contracts/interactions.md`'s request-body field list and Validation Rules now include
`flags` and the `command`/`args` shape constraints; `quickstart.md` has a manual
regression-check step (step 8). No new violations introduced.

## Project Structure

### Documentation (this feature)

```text
specs/002-pro-exec-metadata/
├── plan.md                 # This file — second re-plan, Command/Args/Flags delta
├── research.md              # Phase 0 output — Decision 13 appended for this delta
├── data-model.md            # Phase 1 output — ExecutionRecord table updated (Command/Args/Flags, uploadStatus independence note)
├── quickstart.md            # Phase 1 output — manual-validation step added for the Flags fix
├── contracts/
│   └── interactions.md      # Phase 1 output — Pact interaction's request-body field list updated with `flags`
└── tasks.md                  # Phase 2 output (/speckit-tasks) — NEEDS REGENERATION for this delta (already flagged stale by the first re-plan; this delta adds to that staleness)
```

### Source Code (repository root)

```text
pkg/pro/dtos/exec.go
└── ExecUploadRequest         # Modified — new `Flags []string` field (json:"flags"), alongside existing `Args`

pkg/proexec/envelope.go
└── buildRecord                # Modified — Command strips the `atmos` root segment; Args populated from positional
                                 arguments only (currently unpopulated); Flags (new) populated from masked CLI flags,
                                 replacing the `maskArgs(nil)` no-op at line 55

pacts/atmos-AtmosPro.json      # Regenerated — UploadExecMetadata interaction's request body gains `flags`,
                                 `command`/`args` example values updated to the new shape
```

**Structure Decision**: No new packages, no new files. This delta is intentionally the
smallest change that resolves the clarified ambiguity: one new DTO field plus one
function's field-population logic. It does not touch `internal/exec/pro.go` (the
`uploadStatus` mechanism) at all, consistent with FR-003a's explicit independence
requirement and the constitution's simplicity principle — coordinating two upload
mechanisms client-side was considered and rejected in favor of making each one
independently correct and correlatable by content.

## Complexity Tracking

> No Constitution Check violations — section intentionally empty.
