# Implementation Plan: Atmos Pro Command-Execution Metadata Upload

**Branch**: `1199-pro-exec-metadata` | **Date**: 2026-08-20 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/002-pro-exec-metadata/spec.md`

**Note**: This is a ninth re-plan. The eighth re-plan's three terraform-side deltas (output
masking, `has_changes`/`has_errors`/`errors`, `component`/`stack`) are now confirmed
**implemented** in the current tree — `cmd/terraform/utils.go`'s `maskSensitiveOutputs`,
`buildTerraformExecData`, and `terraformOutputResultMirror` all exist and match
research.md Decisions 19-21 (re-read this session). A second 2026-08-20 `/speckit-clarify`
session, triggered by a real CI payload from `atmos-pro-qa-3` run 32412509172 showing
`errors: null`, all-zero `resource_counts`, and `outputs: {}` alongside `has_changes: true`,
surfaced five further gaps — none yet implemented:

1. **Decision 26 — list fields must never be `null`**: `buildTerraformExecData` passes
   `result.Errors`/`terraformResourceChanges(...)`'s return value straight into the `Data`
   map literal; a nil Go slice marshals to JSON `null`, contradicting data-model.md's own
   `"errors": []` example. Confirmed present in the reported payload.
2. **Decision 27 — `exit_code` as the authoritative signal**: `has_changes`/`has_errors`
   (from `OutputResult`'s top-level fields) and `resource_counts`/`outputs`/`changes` (from a
   separate parse of `OutputResult.Data`) can silently diverge when the parser detects a
   change but fails to extract itemized detail. `TerraformExecData` gains a new `exit_code`
   field so consumers have a signal independent of parse quality.
3. **Decision 28 — `exit_code` is per-component, not aggregate, for multi-component runs**:
   the multi-component path's `execNodeResult` (`cmd/terraform/utils.go:535`) already carries
   `ExitCode` per node — this decision requires the single-component `exit_code` field
   (Decision 27) to have that field as its multi-component counterpart, not a new aggregate.
4. **Decision 29 — minimal `Data` even when parsing fails entirely**: `buildTerraformExecData`
   currently returns `nil` whenever `parseTerraformOutputMirror` can't decode anything at all
   from the captured output; `exit_code` needs to be available in exactly that case, so this
   changes the return contract to always attach `Data` when an exit code is known.
5. **Decision 30, retracted during implementation → Decision 30r — `terraform deploy` keeps
   the single collapsed-as-apply shape**: Decision 30 originally proposed a `{plan, apply}`
   two-phase `Data` shape for `deploy`, on the premise that `deploy` runs plan and apply as
   two separate terraform/tofu subprocess invocations. That premise is factually wrong:
   `internal/exec/terraform.go`'s `handleDeploySubcommand` rewrites `deploy` to `apply`
   *before* any subprocess runs, so `deploy` executes exactly one subprocess with one captured
   output and one exit code — there is no independent plan-phase data to split out. Decision
   30r reverts to the single `TerraformExecData` shape for `deploy`, identical to `plan`/
   `apply`, now also carrying Decisions 26/27/29's amendments.

## Summary

Five deltas, all confined to `cmd/terraform/utils.go` (plus the aggregate-multi-component
folding it already does), fixing gaps a real CI run exposed in the already-shipped
`TerraformExecData` shape from the eighth re-plan:

1. **Null-safe lists**: `buildTerraformExecData` initializes `changes`/`warnings`/`errors` as
   non-nil `[]T{}` before the map literal, so `json.Marshal` never emits `null` for an empty
   case.
2. **`exit_code` field**: `buildTerraformExecData` gains a new `exitCode int` parameter,
   threaded from the same `errUtils.GetExitCode(execErr)` call already used at every other
   exit-code call site in this file, surfaced as a new top-level `exit_code` key.
3. **Per-component scoping**: no new plumbing needed for the multi-component path —
   `execNodeResult.ExitCode` already exists and already flows into the aggregate `Data`; this
   delta is a documentation/contract fix confirming that field fills the same role as the new
   single-component `exit_code` field, not a second exit-code concept.
4. **Always-attach minimal `Data`**: `buildTerraformExecData`'s early return
   (`if !ok { return nil }`) is replaced with a defaulted `TerraformExecData` carrying
   `version`/`exit_code`/`component`/`stack` and empty/zero/false values for everything
   `parseTerraformOutputMirror` couldn't produce.
5. **`deploy` (Decision 30r)**: no dedicated code path — `deploy` continues to flow through
   the same `buildTerraformExecData` as `plan`/`apply` (parsed with apply semantics, as
   before), so it automatically picks up deltas 1-4 with zero `deploy`-specific code.

## Technical Context

**Language/Version**: Go 1.26

**Primary Dependencies**:
- `cmd/terraform/utils.go` (existing, shipped) — `buildTerraformExecData` gains an
  `exitCode int` parameter (fourth positional, after `component, stack`) and:
  - initializes `changes := []execNodeResult{}` / ensures `warnings`/`errors` are non-nil
    before the map literal (Decision 26);
  - adds `"exit_code": exitCode` to the map literal (Decision 27);
  - loses its `if !ok { return nil }` early return in favor of building a defaulted
    `TerraformExecData` even when `parseTerraformOutputMirror` returns `(nil, false)`, as
    long as an `exitCode` is available to report (Decision 29) — the function's signature
    changes from "returns `any` (nil-able)" to "returns `any` (never nil once an invocation
    actually ran a subprocess)", though it can still legitimately return `nil` for a
    subcommand this feature doesn't cover at all (e.g. called with `subCommand` not one of
    `plan`/`apply`/`deploy`, matching today's `parseTerraformOutputMirror`'s own early
    `return nil, false` for non-terraform subcommands — that path is unaffected, since no
    exit code is meaningfully "the terraform subprocess's" for a subcommand this function
    doesn't handle at all).
  - `WithExecMetadataParser`'s closure signature changes from `func(subCommand string) any`
    to `func(subCommand string, exitCode int) any` (`internal/exec/shell_utils.go`), since
    `exitCode` isn't known when `terraformCaptureShellOpts` creates the closure (before the
    shell command runs) — it's only known where `internal/exec/terraform.go`'s
    `captureExecMetadataSync` already computes it (from `params.Err` via
    `errUtils.GetExitCode`, the same mechanism `execNodeResult.ExitCode`/`recordExecResult`
    already use) and calls the closure. `execMetadataSyncParams.Parser`'s type changes to
    match; `captureExecMetadataSync`'s existing crude `if params.Err != nil { exitCode = 1 }`
    is replaced with `errUtils.GetExitCode(params.Err)` (falling back to `1` on a non-zero
    error with no derivable code), consistent with the rest of this file's exit-code handling.
  - `execNodeResult` (multi-component path) is unchanged — its existing `ExitCode` field
    already satisfies Decision 28; only research.md/data-model.md needed updating to state
    this explicitly, no code delta.
  - No `deploy`-specific function (Decision 30r) — `deploy` reaches the same
    `buildTerraformExecData` call as `plan`/`apply` via the same closure.
- `pkg/proexec` (existing, shipped) — no changes; `exit_code`/list-normalization live entirely
  inside `TerraformExecData`'s own construction, not the generic envelope/masking layers.
- No new external dependency, no new package.

**Storage**: N/A — unchanged.

**Testing**: `atmos test` (unit, table-driven). New/changed cases:
- `TestBuildTerraformExecData_EmptyListsAreNotNull`: a run with no warnings/errors/changes
  produces `Data["changes"]`/`["warnings"]`/`["errors"]` that marshal to `[]`, never `null`.
- `TestBuildTerraformExecData_ExitCode`: `exit_code` in the returned map matches the
  `exitCode` parameter passed in, for both zero and non-zero cases.
- `TestBuildTerraformExecData_UnparseableOutputStillAttachesMinimalData`: given output
  `parseTerraformOutputMirror` can't decode at all, `buildTerraformExecData` still returns a
  non-nil map with `version`/`exit_code`/`component`/`stack` set and every other field at its
  empty/zero/false default (not `nil`).
- `TestBuildTerraformExecData_DeployParsedAsApply` (already existing): still asserts `deploy`
  produces the same shape as `apply` — Decision 30r keeps this test's premise valid; no new
  `deploy`-specific test needed beyond what already exists.
- `pkg/pro/consumer_pact_test.go`: extend interaction 9's fixture with a non-empty
  `exit_code` and an explicitly-empty-but-`[]` `errors`/`warnings`/`changes` case, per
  `contracts/interactions.md`.
- `cmd/terraform/utils_exec_metadata_test.go`'s existing `TestTerraformNodeHooks_
  RecordExecResultAccumulates` extended with an assertion that `execNodeResult.ExitCode`
  differs across two nodes in the same run (regression guard for Decision 28's
  no-new-aggregate-field claim — if this ever collapsed to one shared value, the per-component
  scoping requirement would silently break).

**Target Platform**: Linux, macOS, Windows — unchanged; pure data-plumbing, no OS-specific
behavior.

**Project Type**: CLI feature — targeted additions to two already-shipped files
(`cmd/terraform/utils.go`, `internal/exec/shell_utils.go`/`terraform.go` for the closure
signature change), no new packages, no new non-test files.

**Performance Goals**: Unchanged — Decisions 26/27/29/30r are pure data-shape corrections
with no new computation (the exit code and resource counts are already computed today; this
only changes how/whether they're surfaced). `deploy` incurs zero additional cost (Decision
30r: no second parse, no new code path).

**Constraints**:
- `exit_code` MUST be sourced from the same `errUtils.GetExitCode(...)` mechanism already
  used throughout `cmd/terraform`/`internal/exec` (e.g. `execNodeResult.ExitCode`,
  `outcome.ExitCode`) — no new exit-code derivation logic.
- List-typed fields (`changes`/`warnings`/`errors`) MUST be non-nil before marshaling for
  every code path that constructs a `TerraformExecData` map, including the new
  minimal-`Data`-on-parse-failure path (Decision 29) — not just the already-parsed-
  successfully path.
- `deploy` MUST NOT get a dedicated shape or code path (Decision 30r) — it flows through the
  identical `buildTerraformExecData` call `plan`/`apply` use, parsed with apply semantics as
  before.
- No new user-facing configuration surface — all deltas are corrections/extensions to
  already-specified, already-partially-implemented behavior, not new capabilities needing a
  flag.

**Scale/Scope**: 1 changed function signature (`buildTerraformExecData` gains `exitCode`),
1 changed return-contract (never `nil` for a covered subcommand once an exit code is known),
1 changed closure-type signature (`WithExecMetadataParser`'s `func(subCommand string) any` →
`func(subCommand string, exitCode int) any`, threaded through
`execMetadataParserFromOpts`/`execMetadataSyncParams.Parser`/`captureExecMetadataSync`).
0 new packages, 0 new non-test files. 0 breaking wire-shape changes for `plan`/`apply`/
`deploy` (additive: one new field, list-fields now guaranteed non-null, `Data` now attached in
one more edge case than before) — Decision 30r means `deploy`'s wire shape does NOT change
structurally, reversing the ninth re-plan's original (incorrect) breaking-change premise.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Registry-Driven Extensibility | ✅ Pass | No new CLI commands/flags/endpoints. All five deltas are internal shape/data corrections to an already-registered command path. |
| II. Interface-Driven Design with DI | ✅ Pass | `buildTerraformExecData` remains a pure function of its inputs (output string, exit code, identity strings) — no new interfaces needed, consistent with the eighth re-plan's own reasoning for the same file. |
| III. Test-First with 80% Coverage | ✅ Pass | New tests planned for every new/changed function before implementation (see Testing above), including a regression guard for the per-component exit-code claim (Decision 28) even though no code changes there. |
| IV. Separated I/O and UI Architecture | ✅ Pass | No new user-visible output; this data flows only into the Pro upload payload. |
| V. Simplicity and No Over-Engineering | ✅ Pass | Decision 30r rejects a `deploy`-specific two-phase shape once its premise (two real subprocess invocations) was discovered false during implementation — reusing the identical `TerraformExecData` shape for `deploy` is the minimal correct representation of what `deploy` actually does (one subprocess), not an over-engineered split modeling a distinction that doesn't exist. `exit_code`'s per-component scoping deliberately reuses `execNodeResult.ExitCode` rather than adding a parallel field, avoiding two names for the same concept. |

**Post-design re-check**: Phase 1 artifacts (data-model.md, contracts/interactions.md,
quickstart.md) updated in this same pass, including retracting Decision 30's `deploy`
two-phase interaction/shape once its premise was found false. No new violations: the
remaining four deltas extend patterns this feature already established (single-component
identity threading, Decision 21; multi-component per-node folding, Decision 17) rather than
introducing new ones, and Decision 30r removes a would-be violation rather than adding one.

## Project Structure

### Documentation (this feature)

```text
specs/002-pro-exec-metadata/
├── plan.md              # This file (/speckit-plan command output) — ninth re-plan
├── research.md          # Decisions 26-30 appended this pass
├── data-model.md        # TerraformExecData section updated: exit_code, null-safety;
│                        # deploy two-phase split retracted (Decision 30r)
├── quickstart.md        # Steps 18-21 + coverage-table rows added
├── contracts/
│   └── interactions.md  # Interaction 9's data table updated; interaction 16 retracted
└── tasks.md              # Regenerated against this re-plan (deploy tasks removed/corrected)
```

### Source Code (repository root)

```text
cmd/terraform/
├── utils.go                        # buildTerraformExecData (extended: exitCode param,
│                                    # null-safe lists, minimal-Data-on-parse-failure)
├── utils_exec_metadata_test.go     # New/extended unit tests (see Testing above)
├── plan.go / apply.go / deploy.go  # RunE unchanged — exit code now threaded via
│                                    # captureExecMetadataSync, not per-RunE plumbing
└── ...

internal/exec/
├── shell_utils.go                  # WithExecMetadataParser/execMetadataParserFromOpts:
│                                    # closure signature gains exitCode int
└── terraform.go                    # captureExecMetadataSync: exitCode via
                                     # errUtils.GetExitCode, passed to Parser(subCommand, exitCode)

pkg/pro/
└── consumer_pact_test.go           # Extended interaction 9 fixture (exit_code, empty-list case)
```

**Structure Decision**: No new files. Everything lands in already-established locations for
this feature; Decision 30r means no new `cmd/terraform/utils_deploy.go` or similar is needed.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

No unjustified violations. Decision 30's original `deploy` wire-shape break (flat →
two-phase) was retracted (Decision 30r) once its premise was found false during
implementation — nothing to justify here, since the final design introduces no breaking
change for `deploy`.
