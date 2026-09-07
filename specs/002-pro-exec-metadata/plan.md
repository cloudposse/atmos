# Implementation Plan: Atmos Pro Command-Execution Metadata Upload

**Branch**: `1199-pro-exec-metadata` | **Date**: 2026-08-21 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/002-pro-exec-metadata/spec.md`

**Note**: This is a re-plan reflecting the feature's shipped, current state after several
2026-08-21 sessions of implementation and follow-on `/speckit-clarify` corrections against
the already-shipped `TerraformExecData` payload. The prior plan revision (see git history)
covered only the `exit_code`/buffer-scoping bug fixes (FR-006e/FR-006f); this revision folds
in three subsequent, larger changes to the same payload:

1. **`logs` field added** (FR-006i) — the scoped, ANSI-stripped plan/apply/deploy subprocess
   text is now attached to `TerraformExecData` as a base64-encoded `logs` field. Originally
   shipped as a plain-string `raw_output` field, then renamed and re-encoded (research.md
   Decision 38) once base64 was chosen: masking (Terraform-sensitive-output redaction, then
   Gitleaks-pattern masking) had to move to run directly on the plaintext, before encoding,
   since a downstream secret-pattern scan over the marshaled `Data` JSON cannot see into
   base64-encoded bytes.
2. **Multi-component `Data` restructured** (FR-006a, research.md Decision 37) — from a flat
   shared list of `{component, stack, exitCode, action, address}` entries to a list of
   complete, full-shape `TerraformExecData` objects, one per component.
3. **Single- and multi-component `Data` unified** (research.md Decision 38, direct user
   correction) — `Data` for `terraform plan`/`apply`/`deploy` is now UNCONDITIONALLY
   `{"version": 1, "components": [TerraformExecData, ...]}`; a single-component invocation
   produces a one-element `components` list, never a bare `TerraformExecData` object at the
   top level. Each `components[*]` entry omits its own `version` — redundant with the outer
   wrapper's.

A fourth, documentation-only clarification (spec.md FR-010a, 2026-08-21) recorded — without
any code change — a known, accepted limitation: the shared regex-based console parser
(`pkg/ci/plugins/terraform.extractApplyOutputs`) never actually sets a `sensitive: true` flag
on any output entry, because Terraform's own console output already replaces a sensitive
output's real value with the literal placeholder `<sensitive>` before Atmos ever captures it.
The "no real secret ever uploaded" property this feature's masking exists to guarantee still
holds — regression-tested by `TestExtractApplyOutputs_SensitiveOutputNeverExposesRealValue`
and `TestBuildTerraformExecData_SensitiveOutputNeverUploadedInAnyForm` — but via Terraform's
own console behavior, not via accurate flag-driven redaction. Fixing the parser is explicitly
out of scope (shared with Native CI, deliberately not touched by this feature).

## Summary

`TerraformExecData` — the structured `Data` payload `terraform plan`/`apply`/`deploy`
execution records attach (FR-006) — has three deltas relative to the previously-shipped
shape, all confined to `cmd/terraform/utils.go` plus the Pact consumer contract test
(`pkg/pro/consumer_pact_test.go`):

1. A new `logs` field: base64-encoded, masked-before-encoding, scoped plan/apply/deploy
   console text (`buildTerraformExecData` sets it via `encodeLogs`, which composes
   `redactSensitiveOutputsFromRawOutput` then the existing Gitleaks `iolib.MaskString`).
2. `Data` is unconditionally `{"version": 1, "components": [TerraformExecData, ...]}` for
   every `terraform plan`/`apply`/`deploy` invocation, single- or multi-component alike —
   `stripComponentVersion` and `wrapComponentsData` (`cmd/terraform/utils.go`) are the two
   small shared helpers both the single-component (`terraformExecMetadataParserFunc`) and
   multi-component (`terraformNodeHooks.recordExecResult`/`captureMultiComponentExecMetadata`)
   call sites now use, so there is exactly one code path producing the wire shape regardless
   of component count.
3. No functional/behavioral change to `resource_counts`/`outputs`/`warnings`/`changes`/
   `has_changes`/`has_errors`/`errors`/`exit_code`/`component`/`stack` — those fields and
   their masking/defaulting rules are unchanged from the prior shipped shape.

No `-json`-stream parser rewrite, no shared-parser change touching Native CI's own code
path — that direction was proposed and retracted earlier in this spec's Clarifications
(FR-006g/FR-006h, marked Retracted), and remains out of scope here.

## Technical Context

**Language/Version**: Go 1.26 (per `go.mod`; CI pins via `go-version-file: go.mod`)

**Primary Dependencies**:
- `cmd/terraform/utils.go` — `buildTerraformExecData` (per-component `TerraformExecData`
  builder), `encodeLogs`/`redactSensitiveOutputsFromRawOutput` (masking + base64 for `logs`),
  `stripComponentVersion`/`wrapComponentsData` (the shared unification helpers), and
  `terraformNodeHooks.recordExecResult`/`captureMultiComponentExecMetadata`
  (multi-component accumulation and delivery)
- `pkg/proexec` — `VersionedData` (generic `{version, key: payload}` wrapper, reused by
  `wrapComponentsData`), `CaptureSync`/`ExecRecordInput` (delivery, unaffected by this
  revision — `Data` remains an opaque `any`)
- `pkg/io` — `MaskString` (Gitleaks-pattern masking, now also invoked directly against
  `logs`'s plaintext, not only via the existing whole-blob pass in `pkg/proexec/envelope.go`)
- `pkg/ci/plugins/terraform` — `extractApplyOutputs` (the regex-based console parser
  `buildTerraformExecData` decodes through `parseTerraformOutputMirror`; unchanged by this
  revision, its `Sensitive`-detection limitation is documented, not fixed)
- `pkg/pro/dtos` — `ExecUploadRequest`/`ExecDataUploadRequest` (outer envelope, unaffected —
  `Data`/`data` remains `json.RawMessage`/opaque object)

**Storage**: N/A (no local persistence; this feature is an outbound HTTP upload to Atmos Pro)

**Testing**: `go test ./cmd/terraform/...` (unit, table-driven — `buildTerraformExecData`,
`encodeLogs`, `redactSensitiveOutputsFromRawOutput`, `stripComponentVersion`/
`wrapComponentsData`, `terraformNodeHooks.recordExecResult`, `captureMultiComponentExecMetadata`);
`go test -tags pact ./pkg/pro/...` (local-only Pact consumer contract suite — regenerates
`pacts/atmos-AtmosPro.json`, no broker); `go test ./pkg/ci/plugins/terraform/...` (the
shared console parser's own tests, including the new sensitive-output-placeholder regression
guard)

**Target Platform**: Cross-platform CLI (Linux/macOS/Windows) — this feature has no
platform-specific code path beyond the pre-existing resource-metrics `omitempty` fields

**Project Type**: CLI (single Go module, no frontend/backend split)

**Performance Goals**: N/A beyond the feature's existing SC-003/SC-004 delivery-timing
budgets (unaffected by this revision — `logs` adds bytes to `Data`, handled transparently by
the existing FR-011 4 MB inline-vs-blob-URL threshold, no new timing budget)

**Constraints**: `logs` MUST be masked on the plaintext before base64-encoding (FR-010a) —
the existing whole-blob Gitleaks pass in `pkg/proexec/envelope.go` cannot pattern-match a
secret once it is inside base64-encoded bytes, so this field cannot rely on that later pass
the way every other plain-string field in `Data` does

**Scale/Scope**: Single payload shape (`TerraformExecData`) touched; three files change
(`cmd/terraform/utils.go`, its test, `pkg/pro/consumer_pact_test.go`) plus this feature's
own spec/data-model/research/contracts documentation — no schema or database migration, no
new CLI command or flag

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|---|---|---|
| I. Registry-Driven Extensibility | N/A | No new CLI command/store provider introduced by this revision |
| II. Interface-Driven Design with DI | Pass | `buildTerraformExecData`/`encodeLogs`/`stripComponentVersion`/`wrapComponentsData` are pure functions over their inputs, not hidden singletons; existing `proexec.CaptureSync`/`ExecRecordInput` DI seams are unchanged |
| III. Test-First, 80% Coverage | Pass | Every new/changed function has a dedicated unit test (`TestEncodeLogs`, `TestRedactSensitiveOutputsFromRawOutput`, `TestBuildTerraformExecData_LogsWiredThroughRedactionAndEncoding`, `TestBuildTerraformExecData_SensitiveOutputNeverUploadedInAnyForm`, the unified-shape assertions in `TestTerraformExecMetadataParserFunc_UsesSuppliedOutput`/`TestTerraformNodeHooks_RecordExecResultAccumulates`/`TestCaptureMultiComponentExecMetadata_ExactlyOneRequestForWholeRun`), plus the Pact consumer contract (`TestPact_UploadExecMetadata`/`_MultiComponent`) |
| IV. Separated I/O and UI | N/A | This feature performs no terminal output of its own; it constructs an HTTP payload |
| V. Simplicity, No Over-Engineering | Pass | `stripComponentVersion`/`wrapComponentsData` are two small, single-purpose functions extracted specifically to eliminate the duplicated inline logic the single- and multi-component call sites previously each carried — this is a de-duplication, not a new abstraction layer |

No violations requiring justification — Complexity Tracking section is empty.

## Project Structure

### Documentation (this feature)

```text
specs/002-pro-exec-metadata/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output — Decisions 1-38
├── data-model.md        # Phase 1 output — ExecutionRecord, TerraformExecData, etc.
├── quickstart.md        # Phase 1 output — manual end-to-end + Pact regeneration steps
├── contracts/
│   └── interactions.md  # Phase 1 output — Pact interactions 1-15 (9/10 cover TerraformExecData)
└── tasks.md              # Phase 2 output (/speckit-tasks command)
```

### Source Code (repository root)

```text
cmd/terraform/
├── utils.go                       # buildTerraformExecData, encodeLogs,
│                                   # redactSensitiveOutputsFromRawOutput,
│                                   # stripComponentVersion, wrapComponentsData,
│                                   # terraformNodeHooks.recordExecResult,
│                                   # captureMultiComponentExecMetadata,
│                                   # terraformExecMetadataParserFunc
├── utils_exec_metadata_test.go    # Unit tests for all of the above
├── plan.go / apply.go / deploy.go # Wire terraformCaptureShellOpts (unchanged this revision)

pkg/proexec/
├── envelope.go                    # buildRecord, VersionedData, maskedDataJSON (unaffected —
│                                   # Data remains an opaque any/json.RawMessage)
├── sync.go / async.go             # CaptureSync/CaptureAsync (unaffected)

pkg/pro/
├── dtos/exec.go                   # ExecUploadRequest/ExecDataUploadRequest (unaffected)
├── consumer_pact_test.go          # Pact consumer interactions, incl. TestPact_UploadExecMetadata
│                                   # and TestPact_UploadExecMetadata_MultiComponent

pkg/ci/plugins/terraform/
├── parser.go                      # extractApplyOutputs (unchanged — its Sensitive-detection
│                                   # limitation is documented, not fixed, per FR-010a)
├── parser_test.go                 # TestExtractApplyOutputs_SensitiveOutputNeverExposesRealValue

internal/exec/
├── terraform.go                   # captureExecMetadataSync (unaffected — Data remains opaque)
```

**Structure Decision**: No new packages or directories. This revision is entirely
change-in-place within the existing `cmd/terraform` package (the `TerraformExecData` builder
and the two new unification helpers) plus test/contract-doc updates — consistent with
Constitution Principle V (no premature structure for a payload-shape change).

## Complexity Tracking

*No violations — table intentionally empty.*
