# Implementation Plan: Atmos Pro Command-Execution Metadata Upload

**Branch**: `1199-pro-exec-metadata` | **Date**: 2026-08-19 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/002-pro-exec-metadata/spec.md`

**Note**: This is a fifth re-plan. `ExecutionID`, the `Data` inline-or-blob-URL redesign
(fourth re-plan), and the multi-component half of US3 (structured infrastructure-change
data folded into the aggregate record) are already implemented and present in the current
tree. This revision covers the one piece explicitly left open at the end of that
implementation pass: **threading parsed `terraform plan`/`apply`/`deploy` structured data
into the *single-component* synchronous execution record** (`internal/exec`'s
`captureExecMetadataSync`), which a confirmed Go import cycle prevents from calling the CI
plugin's output parser directly.

## Summary

Add a new `ShellCommandOption`, `WithExecMetadataParser`, that lets `cmd/terraform`
(`plan.go`/`apply.go`/`deploy.go` — which can safely import `pkg/ci/plugins/terraform`,
unlike `internal/exec`) hand `internal/exec`'s `captureExecMetadataSync` a closure that
produces the parsed `TerraformExecData` on demand. `internal/exec` never imports the parser
itself — it only invokes the closure, exactly mirroring the existing
`WithInvokingCommand`/`invokingCommandFromOpts` pattern already used for `Flags` sourcing.
Decouple `WithStdoutCapture`/`WithStderrCapture` construction from the `ciMode` gate in
`plan.go`/`apply.go`/`deploy.go` (capture is cheap — an in-memory buffer append — so this
carries negligible cost, matching research.md Decision 12's own prior reasoning), so the
buffer is always available for the new parser closure to read, regardless of whether Native
CI job summaries are also enabled.

## Technical Context

**Language/Version**: Go 1.26

**Primary Dependencies**:
- `internal/exec/shell_utils.go` (existing, shipped) — new `WithExecMetadataParser(fn
  func(subCommand string) any) ShellCommandOption`, storing `fn` on
  `shellCommandConfig.execMetadataParser`, and a new `execMetadataParserFromOpts(opts
  ...ShellCommandOption) func(subCommand string) any` extractor, mirroring
  `WithInvokingCommand`/`invokingCommandFromOpts` exactly.
- `internal/exec/terraform.go` (existing, shipped — `captureExecMetadataSync`) — modified to
  accept the extracted parser closure (via the existing `opts ...ShellCommandOption` already
  in scope at its call site) and, when non-nil, call `parser(subCommand)` to obtain `data
  any` for `proexec.CaptureSync`, instead of always passing `nil`.
- `cmd/terraform/plan.go`/`apply.go`/`deploy.go` (existing, shipped) — modified to: (a)
  always construct `stdoutBuf`/`stderrBuf` and pass `WithStdoutCapture`/`WithStderrCapture`,
  decoupled from the `ciMode` conditional (the separate `capturedPlanOutput`/CI-job-summary
  post-processing stays `ciMode`-gated — unrelated, independently-configured feature, per
  research.md Decision 12's rationale for keeping the two gates distinct); (b) pass a new
  `WithExecMetadataParser` closure that lazily reads `stdoutBuf`/`stderrBuf` (by the time
  `captureExecMetadataSync` invokes it, `executeCommandPipeline` has already finished writing
  into them) and calls a new `buildTerraformExecData` helper.
- `cmd/terraform/utils.go` (existing, shipped this session's Phase 4/5 work) — refactor:
  extract the JSON-mirror decoding step already used by `parseTerraformResourceChanges` into
  a shared `parseTerraformOutputMirror(subCommand, output string)
  (*terraformOutputDataMirror, bool)` helper, so both `parseTerraformResourceChanges`
  (multi-component, flat per-resource entries) and the new `buildTerraformExecData`
  (single-component, one combined object) decode via the same code path — no duplicated
  `citerraform.ParseOutput`/JSON-round-trip logic.
- `citerraform "github.com/cloudposse/atmos/pkg/ci/plugins/terraform"` (existing direct
  dependency, already imported by `cmd/terraform/utils.go` this session) — no new dependency.

**Storage**: N/A — unchanged.

**Testing**: `atmos test` (unit, table-driven). New/changed cases needed:
`execMetadataParserFromOpts`/`WithExecMetadataParser` round-trip (mirrors the existing
`invokingCommandFromOpts` test shape), `captureExecMetadataSync` calling the parser only for
sync-allowlisted commands and folding its result into `CaptureSync`'s `data` argument (a new
case alongside the existing `TestCaptureExecMetadataSync_SkipsPerNodeWhenNodeHooksWired`),
`buildTerraformExecData`'s combined-object shape against the same
`pkg/ci/plugins/terraform/testdata/stdout/apply_success.txt` fixture already used by this
session's multi-component tests, and a `cmd/terraform/plan_test.go` case asserting
`stdoutBuf`/`WithStdoutCapture` are wired regardless of `ciMode`.

**Target Platform**: Linux, macOS, Windows — unchanged.

**Project Type**: CLI feature — targeted addition to already-shipped
`internal/exec/shell_utils.go`, `internal/exec/terraform.go`, `cmd/terraform/plan.go`/
`apply.go`/`deploy.go`, `cmd/terraform/utils.go`. No new packages, no new DTO/wire-shape
changes (the wire shape for single-component `Data` was already specified by FR-006/
data-model.md's `TerraformExecData` in the fourth re-plan — this delta only makes
`internal/exec` able to populate it for the single-component path).

**Performance Goals**: Unchanged — the always-on stdout/stderr capture is an in-memory
`bytes.Buffer` append with no additional I/O, negligible relative to a real
`terraform plan`/`apply` subprocess run (research.md Decision 12).

**Constraints**:
- `internal/exec` MUST NOT gain a new import of `pkg/ci/plugins/terraform` or
  `pkg/ci/internal/plugin` — the whole point of this design is avoiding the confirmed import
  cycle (`pkg/ci/plugins/terraform` → `internal/exec`) by inversion of control (a
  caller-supplied closure), not by restructuring package boundaries.
- The parser closure is invoked **at most once per invocation**, only when
  `proexec.IsSyncCommand` is true for that subcommand (mirrors the existing gate order in
  `captureExecMetadataSync` — the `info.NodeHooks != nil` multi-component skip still runs
  first) — never for async/non-allowlisted commands, so there is no added parsing cost for
  the overwhelming majority of `atmos` invocations.
- No new user-facing configuration surface — this is purely an internal wiring change; FR-006
  already mandates the behavior, Assumptions already defer the "how" to implementation.

**Scale/Scope**: 1 new `ShellCommandOption` + extractor function (mirrors an existing
pattern exactly), 1 new helper function (`buildTerraformExecData`) reusing already-shipped
parsing logic via a small refactor (no new parsing code, just de-duplication), 3 call-site
changes (`plan.go`/`apply.go`/`deploy.go`'s shellOpts construction), 1 call-site change
(`captureExecMetadataSync`). 0 new packages, 0 new wire-shape/DTO changes.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Registry-Driven Extensibility | ✅ Pass | No new CLI commands/flags/endpoints; a new functional option on an already-established options pattern (`ShellCommandOption`), not a new extensibility mechanism. |
| II. Interface-Driven Design with DI | ✅ Pass | The parser is injected as a plain closure (`func(string) any`), exactly matching how `WithInvokingCommand` already injects the `*cobra.Command` — no new interface needed since there is exactly one implementer (`cmd/terraform`) and no test double is required (`captureExecMetadataSync`'s existing tests just pass `nil` or a stub closure). |
| III. Test-First with 80% Coverage | ✅ Pass | New tests planned for every new function (`WithExecMetadataParser`/extractor, `captureExecMetadataSync`'s parser-invocation branch, `buildTerraformExecData`, the decoupled-from-`ciMode` capture wiring) before implementation, per CLAUDE.md's Bug-Fixing/feature workflow. |
| IV. Separated I/O and UI Architecture | ✅ Pass | No new user-visible output. |
| V. Simplicity and No Over-Engineering | ✅ Pass | Rejected restructuring `pkg/ci`'s package boundaries (e.g. extracting the parser into a new leaf package) as a larger, riskier change than a single caller-supplied closure; rejected introducing a new interface type for a single implementer (YAGNI, matches the existing `WithInvokingCommand` precedent exactly rather than inventing a second, inconsistent pattern). |

**Post-design re-check**: ✅ Pass. Phase 1 complete — `data-model.md`'s `TerraformExecData`
section now notes how the single-component path populates it (no wire-shape change, so
`contracts/interactions.md` needs no edit); `quickstart.md` gained a step for manually
verifying single-component structured data. No new violations introduced.

## Project Structure

### Documentation (this feature)

```text
specs/002-pro-exec-metadata/
├── plan.md                 # This file — fifth re-plan: single-component US3 wiring
├── research.md              # Phase 0 output — Decision 18 appended for this delta
├── data-model.md            # Phase 1 output — TerraformExecData note updated (no wire-shape change)
├── quickstart.md            # Phase 1 output — new step for single-component structured data
├── contracts/
│   └── interactions.md      # Unchanged — no wire-shape change, single-component Data already matches the documented inline/blob-URL shapes
└── tasks.md                  # Phase 2 output (/speckit-tasks) — T019/T020/T022/T023 NEED REGENERATION against this concrete design
```

### Source Code (repository root)

```text
internal/exec/shell_utils.go
├── WithExecMetadataParser        # New — injects func(subCommand string) any
└── execMetadataParserFromOpts     # New — extractor, mirrors invokingCommandFromOpts

internal/exec/terraform.go
└── captureExecMetadataSync        # Modified — calls the extracted parser closure (if any)
                                      when IsSyncCommand and info.NodeHooks == nil, passing
                                      its result as CaptureSync's data argument

cmd/terraform/plan.go
cmd/terraform/apply.go
cmd/terraform/deploy.go
└── RunE                            # Modified — stdoutBuf/stderrBuf + WithStdoutCapture/
                                      WithStderrCapture construction decoupled from ciMode;
                                      new WithExecMetadataParser(buildTerraformExecData)
                                      passed into shellOpts

cmd/terraform/utils.go
├── parseTerraformOutputMirror      # New — extracted shared decode step (refactor, no new
│                                     parsing logic) used by both call sites below
├── parseTerraformResourceChanges   # Modified — now calls parseTerraformOutputMirror
└── buildTerraformExecData          # New — single-component combined-object shape
                                      (resource_counts/outputs/warnings/changes), using
                                      parseTerraformOutputMirror
```

**Structure Decision**: No new packages, no new files beyond test files. This delta adds one
new option/extractor pair (following an existing, proven pattern exactly) and one new helper
function that reuses already-shipped parsing/decoding logic via a small refactor, rather than
introducing a second, divergent parsing path or restructuring `pkg/ci`'s package boundaries —
consistent with the constitution's simplicity principle.

## Complexity Tracking

> No Constitution Check violations — section intentionally empty.
