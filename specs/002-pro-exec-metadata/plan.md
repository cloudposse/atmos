# Implementation Plan: Atmos Pro Command-Execution Metadata Upload

**Branch**: `1199-pro-exec-metadata` | **Date**: 2026-08-18 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/002-pro-exec-metadata/spec.md`

**Note**: This is a re-plan. US1 (async default upload) and US2 (synchronous base-envelope
upload) shipped and are live in production. This revision addresses (a) a correctness
defect found in production data — sync-allowlisted commands were producing **two**
execution records per invocation instead of one — and (b) scope changes from the
2026-08-18 `/speckit-clarify` session: multi-component `--affected`/`--all` runs must
report one aggregate record instead of one per component, and `terraform deploy` joins
the synchronous allowlist with its own structured data. It also corrects the previously
filed blocker for US3 ([#2924](https://github.com/cloudposse/atmos/issues/2924)): the
raw stdout capture US3 needs already exists and is already wired for CI mode in
`cmd/terraform/plan.go`/`apply.go` — the real gap is plumbing, not a new tee mechanism.

## Summary

Three fixes to the already-shipped `pkg/proexec`/`internal/exec/terraform.go` delivery
path, plus completion of the previously-blocked User Story 3:

1. **Dedup fix (regression)**: `cmd/root.go`'s unconditional `proexec.CaptureAsync(cmd,
   err)` call must be skipped for commands on the synchronous allowlist, so each
   qualifying invocation produces exactly one execution record (FR-007), not two. This
   requires a single shared classification function callable from both `cmd/root.go` and
   `internal/exec/terraform.go`/`describe affected`'s call sites, replacing the
   `internal/exec`-only `isExecMetadataSyncSubcommand`.
2. **Allowlist expansion**: `terraform deploy` joins the synchronous allowlist
   (`plan`/`apply`/`deploy`/`describe affected`) and gets the same structured
   infrastructure-change data as `plan`/`apply` (FR-006/FR-007, 2026-08-18 clarifications).
3. **Multi-component aggregation**: `atmos terraform plan/apply/deploy --affected`/`--all`
   currently uploads one execution record per graph node (since `ExecuteTerraform` runs
   per component and `captureExecMetadataSync` lives inside it). Per FR-006a, this must
   become exactly one aggregate record per CLI invocation, with each component's identity,
   outcome, and structured data folded into that single record's `DataItems`.
4. **US3 completion (previously blocked)**: attach itemized created/updated/deleted/
   replaced/moved/imported resources, output values, and warnings to `plan`/`apply`/
   `deploy` execution records. The blocker recorded in #2924 ("`ExecuteTerraform` never
   captures raw stdout, needs a new `MultiWriter` tee across the shared pipeline") is
   **only partially accurate**: `internal/exec/shell_utils.go`'s `WithStdoutCapture`/
   `WithStderrCapture` `ShellCommandOption`s already implement exactly that tee, and
   `cmd/terraform/plan.go`/`apply.go` (and `deploy.go`) already use them today — gated on
   a *different* CI flag (`ciMode`) than the exec-metadata gate (`telemetry.IsCI() &&
   Pro-configured`), and the captured buffer is a package-level var consumed only by
   `PostRunE`'s Native-CI job-summary hooks, never threaded down into `ExecuteTerraform`'s
   `captureExecMetadataSync`. The fix is to thread that already-captured, already
   ANSI-stripped buffer (or a second capture using the same existing option, gated on the
   exec-metadata gate instead of `ciMode`) into `CaptureSync`'s `data`/`dataItems`
   arguments via the now-public `terraform.ParsePlanOutput`/`ParseApplyOutput`, not to
   invent new tee infrastructure.

## Technical Context

**Language/Version**: Go 1.26

**Primary Dependencies**:
- `pkg/proexec` (existing, shipped — `gateOpen`, `buildRecord`, `CaptureAsync`, `CaptureSync`) — extended, not replaced
- `pkg/pro` (existing, shipped — `AtmosProAPIClient.UploadExecMetadata`, `sendChunked`/`BatchInfo`)
- `pkg/telemetry` (existing — `IsCI()`, the exec-metadata gate's CI check)
- `pkg/ci` (existing — `IsCI()`, a *separate* CI-detection function gating today's `plan`/`apply` stdout capture; this plan clarifies which of the two gates the US3 capture must key off)
- `internal/exec/shell_utils.go` (existing — `WithStdoutCapture`/`WithStderrCapture` `ShellCommandOption`s; reused, not replaced, for US3)
- `pkg/ci/plugins/terraform` (existing, public — `ParsePlanOutput`/`ParseApplyOutput(output string) *plugin.OutputResult`, confirmed callable from `internal/exec` via `any`-typed `OutputResult.Data`, per #2924's own investigation)
- `pkg/metrics/process` (existing, shipped)

**Storage**: N/A — unchanged from the original plan; no local persistence.

**Testing**: `atmos test` (unit, table-driven, mocked `AtmosProAPIClientInterface`); new
cases for the dedup fix (assert `CaptureAsync` is a no-op for sync-allowlisted commands),
the aggregation change (assert one record, not N, for a multi-component graph run), the
`deploy` allowlist addition, and US3's stdout-capture plumbing.

**Target Platform**: Linux, macOS, Windows — unchanged.

**Project Type**: CLI feature — bug-fix and scope-extension changes to already-shipped
`pkg/proexec`, `internal/exec/terraform.go`, `cmd/root.go`, `cmd/terraform/{plan,apply,deploy}.go`,
and `cmd/terraform/utils.go` (multi-component graph hooks). No new packages.

**Performance Goals**: Unchanged (SC-003/SC-004) — the aggregation change must not turn a
multi-component run's per-node timing into a serialization point; components still execute
concurrently, only the *upload* is deferred and combined until the whole graph run completes.

**Constraints**:
- No new user-facing opt-out (unchanged).
- The dedup fix and aggregation change are corrections to already-merged code — they must
  not regress US1/US2's existing test coverage; existing tests asserting per-component
  records or dual-record behavior are now wrong and must be updated, not preserved.
- `deploy`'s structured data reuses the same `TerraformExecData` shape as `plan`/`apply`
  (FR-006) — no new DTO fields.
- US3's stdout capture MUST NOT change streaming/TTY/masking behavior for any terraform
  subcommand outside `plan`/`apply`/`deploy` — reusing the existing, already-scoped
  `WithStdoutCapture` call sites keeps this risk bounded to code paths already proven safe
  for exactly this purpose.

**Scale/Scope**: 0 new Atmos Pro endpoints (still `POST /v1/atmos/exec`, no DTO shape
change beyond what US3 already specified in data-model.md). ~6 call-site changes: a new
shared classification function, `cmd/root.go`'s dedup skip, `cmd/terraform/deploy.go`'s
allowlist join, `cmd/terraform/utils.go`'s per-node→aggregate change for the graph path,
and `internal/exec/terraform.go`'s US3 wiring.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Registry-Driven Extensibility | ✅ Pass | No new CLI commands/flags; `deploy` joining a fixed, code-defined allowlist is the same pattern as `plan`/`apply`, not a new extension point. |
| II. Interface-Driven Design with DI | ✅ Pass | No interface changes; the shared classification function is a plain predicate (`isExecMetadataSyncCommand(subCommand string) bool`), injected nowhere because it has no side effects — consistent with the existing `isExecMetadataSyncSubcommand`. |
| III. Test-First with 80% Coverage | ✅ Pass | Regression tests for the dedup bug and aggregation change are written first (reproducing the observed double-record behavior), per the constitution's bug-fix workflow. |
| IV. Separated I/O and UI Architecture | ✅ Pass | No new user-visible output; unchanged from original plan. |
| V. Simplicity and No Over-Engineering | ✅ Pass | The US3 fix explicitly rejects #2924's proposed new `MultiWriter`-tee-across-the-shared-pipeline mechanism in favor of reusing the existing, narrowly-scoped `WithStdoutCapture` call sites already present in `plan.go`/`apply.go`/`deploy.go` — smaller blast radius, no new abstraction. The aggregation change replaces N per-node uploads with 1, which is strictly simpler than what it replaces, not more complex. |

**Post-design re-check**: Pending Phase 1 (data-model.md, contracts/ updates for the
aggregated multi-component `DataItems` shape and the `deploy` allowlist addition).

## Project Structure

### Documentation (this feature)

```text
specs/002-pro-exec-metadata/
├── plan.md                 # This file — re-plan for the dedup/aggregation/deploy/US3 delta
├── research.md              # Phase 0 output — Decisions 10–12 appended for this delta
├── data-model.md            # Phase 1 output — Delivery Classification table updated (deploy, aggregation)
├── quickstart.md            # Phase 1 output — updated manual-validation steps for the 3 fixes
├── contracts/
│   └── interactions.md      # Phase 1 output — Pact interaction updated if the aggregated multi-component DataItems shape needs a dedicated example
└── tasks.md                  # Phase 2 output (/speckit-tasks) — NEEDS REGENERATION for this delta
```

### Source Code (repository root)

```text
pkg/proexec/
├── gate.go                  # Unchanged
├── classify.go               # NEW — shared IsSyncCommand(commandPath string) bool, single source of truth for the sync allowlist (plan/apply/deploy/describe affected), replacing internal/exec's private isExecMetadataSyncSubcommand
├── envelope.go               # Unchanged
├── async.go                  # Modified — CaptureAsync no-ops early when classify.IsSyncCommand(cmd) is true (dedup fix)
├── sync.go                   # Unchanged (signature already supports data/dataItems from US2's rework)
└── *_test.go                 # New table-driven cases: CaptureAsync no-ops for each sync-allowlisted command; classify.IsSyncCommand matrix

internal/exec/terraform.go
├── captureExecMetadataSync    # Modified — subCommand check now delegates to proexec/classify.go; deploy added; wired to US3's data/dataItems once the capture plumbing (below) lands
└── isExecMetadataSyncSubcommand  # REMOVED — superseded by pkg/proexec/classify.go

cmd/root.go
└── Execute()                 # Modified — proexec.CaptureAsync(cmd, err) call now preceded by a classify.IsSyncCommand(cmd.CommandPath()) skip (dedup fix)

cmd/terraform/
├── plan.go                   # Modified — existing ciMode-gated WithStdoutCapture buffer additionally (or separately) feeds ExecuteTerraform's US3 data via a new opts param, decoupled from the ciMode/Native-CI gate
├── apply.go                  # Modified — same as plan.go
├── deploy.go                 # Modified — joins the sync allowlist; gains the same stdout-capture wiring as plan.go/apply.go
└── utils.go                  # Modified — terraformNodeHooks.AfterWithWriters's per-node CI-hook path (runCIHooksForNode) is joined by a new aggregation collector: per-node results accumulate across the graph run and a single proexec.CaptureSync call fires once after the whole graph completes, replacing today's per-node captureExecMetadataSync calls

pkg/ci/plugins/terraform/parser.go
└── ParsePlanOutput/ParseApplyOutput  # Unchanged (already public per #2924's investigation) — now actually called from internal/exec, not just pkg/ci

pacts/atmos-AtmosPro.json           # Unchanged shape; regenerated only if contracts/interactions.md's aggregated-multi-component example is added as a distinct interaction
```

**Structure Decision**: No new packages. This delta is intentionally scoped as targeted
fixes to already-shipped files plus one new small file (`pkg/proexec/classify.go`) that
centralizes the sync-allowlist predicate so `cmd/root.go` and `internal/exec/terraform.go`
can no longer drift out of sync the way they did to produce the dedup bug — the single
biggest structural lesson from this delta. The multi-component aggregation moves
`captureExecMetadataSync`'s effective call site from "once per graph node" (inside
`ExecuteTerraform`, which recurs per node) to "once per graph run" (in
`cmd/terraform/utils.go`, which owns the graph's lifecycle) — this is the one place in the
codebase that already knows when a multi-component run starts and ends
(`wasMultiComponentExecution`), so it is the natural aggregation point rather than a new
one.

**US3 blocker correction**: Per the research phase below (Decision 12), issue #2924's
proposed fix (`MultiWriter`-based tee added to `ExecuteTerraform`'s shared pipeline) is
superseded — `WithStdoutCapture`/`WithStderrCapture` already exist and are already used
for this exact purpose in `plan.go`/`apply.go`/`deploy.go`, just gated on the wrong
condition and not threaded to the right call site. #2924 should be updated or closed as
"solved differently" once this plan's US3 tasks land.

## Complexity Tracking

> No Constitution Check violations — section intentionally empty.
