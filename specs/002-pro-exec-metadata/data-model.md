# Data Model: Atmos Pro Command-Execution Metadata Upload

**Feature**: 002-pro-exec-metadata
**Date**: 2026-08-11 (revised 2026-08-19 — ExecutionID, Data blob-upload redesign)

---

## Core Entities

### ExecGate

The runtime decision of whether an execution record should be produced at all.

| Field | Type | Description |
|-------|------|-------------|
| `CIDetected` | `bool` | `telemetry.IsCI()` |
| `ProConfigured` | `bool` | Static token present, or full OIDC settings (`RequestURL`, `RequestToken`, `WorkspaceID`) present |
| `Open` | `bool` | `CIDetected && ProConfigured` — the single condition gating all delivery |

No new entity is persisted for this — it is a pure function evaluated per invocation
(`proexec.gateOpen`).

### ExecutionRecord (`dtos.ExecUploadRequest`)

Represents a single `atmos` command invocation's reported outcome — the request body
for `POST /v1/atmos/exec`.

| Field | Type | Notes |
|-------|------|-------|
| `ExecutionID` | `string` (UUID v4) | **New** — a fresh UUID generated once per qualifying invocation (`uuid.New().String()`), distinct from `AtmosProRunID`: this uniquely identifies *this* execution record, while `AtmosProRunID` correlates records across a CI run. Also keys the correlated `POST /v1/atmos/exec/data` upload when `Data` requires out-of-band delivery — FR-003c, research.md Decision 15 |
| `AtmosProRunID` | `string` | Reused from the existing `ATMOS_PRO_RUN_ID` env var convention (same as `InstanceStatusUploadRequest`) — correlation ID, per Clarification Q4 |
| `AtmosVersion` | `string` | `pkgversion.Version` |
| `AtmosOS` | `string` | `runtime.GOOS` |
| `AtmosArch` | `string` | `runtime.GOARCH` |
| `Command` | `string` | Subcommand path with the leading `atmos` root stripped, e.g. `"terraform plan"` (not `"atmos terraform plan"`) — FR-003b, 2026-08-18 (2nd) clarification |
| `Args` | `[]string` | Positional arguments only, e.g. `["cdn"]` — previously always empty (`maskArgs(nil)` bug in `envelope.go:55`), now populated per FR-003b |
| `Flags` | `[]string` | Every CLI flag actually passed, masked, no exclusions (`--upload-status` included), using each flag's canonical long name — e.g. `["--stack", "plat-use2-dev", "--upload-status"]`, never `["--upload-status", "true"]` for a bool flag; kept separate from `Args`, never combined — FR-003b, 2026-08-19 clarifications. Shorthand form (`-s`) and invocation order are NOT preserved — array order follows Cobra's flag-iteration order, and this is an accepted tradeoff, not a defect. MUST be sourced from the invoking command's own record of explicitly-set flags (e.g. Cobra's `cmd.Flags().Visit`), never from a pass-through/leftover-args collection that cannot contain atmos-recognized flags in the first place — see research.md Decision 14 |
| `ExitCode` | `int` | |
| `GitSHA` | `string` | |
| `RepoURL`, `RepoName`, `RepoOwner`, `RepoHost` | `string` | From `git.GitRepoInterface.GetLocalRepoInfo()`, matching `InstanceStatusUploadRequest` |
| `Metrics` | `ResourceUsageMetrics` | Always present (Unix-only sub-fields `omitempty`) |
| `Data` | `any` (`json.RawMessage` after marshal) | Command-specific structured data — the single field for both "small summary" and "potentially large bulk" content (the old `Data`/`DataItems` split no longer exists — FR-011, research.md Decision 16). On the wire it is **always exactly one of two shapes**: an inline JSON structure (object/array), when the whole marshaled record is under 4 MB; or a JSON string holding a blob URL returned by `POST /v1/atmos/exec/data`, when the whole record is at/over 4 MB. `nil`/absent for commands with no structured-data extension. |

**Removed in this revision** (research.md Decision 16): `DataItems`, `BatchID`,
`BatchIndex`, `BatchTotal` — the multi-chunk model these fields supported is retired.
`pkg/pro/chunked_upload.go` itself is unchanged and remains in use by
`UploadAffectedStacks`/`UploadInstances`; only its use for `ExecUploadRequest` is retired.

### ExecDataUploadRequest / ExecDataUploadResponse (`dtos.ExecDataUploadRequest`/`dtos.ExecDataUploadResponse`)

**New** — the request/response body for `POST /v1/atmos/exec/data`, called by
`UploadExecMetadata` only when the full `ExecutionRecord` would be at/over 4 MB (FR-011,
research.md Decision 16). Always exactly one request per invocation that needs it — never
chunked.

| Field | Type | Notes |
|-------|------|-------|
| `ExecutionID` (request) | `string` (UUID v4) | `json:"execution_id"` — the same value as the corresponding `ExecutionRecord.ExecutionID`, keying this upload to that record |
| `Data` (request) | `json.RawMessage` | `json:"data"` — the masked, marshaled command-specific structured data (same content that would otherwise have gone inline into `ExecutionRecord.Data`) |
| `AtmosApiResponse` (response, embedded) | — | `Success`, `Status`, `TraceID`, etc. — same envelope as every other Pro response |
| `URL` (response) | `string` | `json:"url"` — the blob's retrievable URL, to be set as `ExecutionRecord.Data`'s content (as a JSON string) on the subsequent `POST /v1/atmos/exec` call |

### ResourceUsageMetrics (embedded in `ExecutionRecord`)

| Field | Type | Notes |
|-------|------|-------|
| `WallTimeMS` | `int64` | Always present, all platforms |
| `UserCPUTimeMS` | `int64` | Always present, all platforms |
| `SystemCPUTimeMS` | `int64` | Always present, all platforms |
| `MaxRSSBytes` | `int64` | `omitempty` — Unix only |
| `MinorPageFaults`, `MajorPageFaults` | `int64` | `omitempty` — Unix only |
| `InBlockOps`, `OutBlockOps` | `int64` | `omitempty` — Unix only |
| `VolCtxSwitches`, `InvolCtxSwitches` | `int64` | `omitempty` — Unix only |

### TerraformExecData (one concrete `Data` shape, for `terraform plan`/`apply`/`deploy`)

Derived from the already-merged `pkg/ci/internal/plugin.TerraformOutputData` structure.
Previously split across `Data` (small, always inline) and `DataItems` (potentially large,
chunkable) — as of research.md Decision 16, both are folded into the single `Data` field;
`UploadExecMetadata`'s size check (inline vs. blob-URL) handles the large case uniformly:

- Small/summary portion: `ResourceCounts`, `Outputs`, `HasOutputChanges`, `ChangedResult`,
  `Warnings`.
- Bulk portion: one `{action, address}` entry per resource in `CreatedResources`,
  `UpdatedResources`, `ReplacedResources`, `DeletedResources`, `MovedResources`,
  `ImportedResources` — `action` is the source field name (`"created"`/`"updated"`/
  `"replaced"`/`"deleted"`/`"moved"`/`"imported"`), `address` is the resource address string.
- `logs` (string, base64-encoded — Decision 38): the same ANSI-stripped,
  already-scoped output text `buildTerraformExecData` itself parses
  (FR-006f/Decision 32) — only the final `plan`/`apply` (or `deploy`'s
  internal `apply`) subprocess invocation's own stdout+stderr, never
  `terraform init`'s or `terraform workspace select`'s. Always included, even
  when the covered subcommand's output can't be parsed into the other fields
  (Decision 29) — the actual captured text in that case too (base64-encoded),
  not an empty string, since a failed/unparseable run's raw text is exactly
  the case a consumer needs it most. Applies uniformly to both the
  single-component shape and every per-component entry in the multi-component
  `{"components": [...]}` list (Decision 37) — each component's own `logs`
  lives inside its own `TerraformExecData` entry. Masking happens on the
  plaintext, before encoding (`encodeLogs`, `cmd/terraform/utils.go`): when
  parsing succeeds, `redactSensitiveOutputsFromRawOutput` first replaces every
  literal occurrence of a string-valued, Terraform-sensitive-flagged output's
  own value with the mask placeholder (spec.md FR-010a, extended by the
  2026-08-21 clarification session), then `iolib.MaskString` — the same
  Gitleaks-pattern masking `pkg/proexec/envelope.go` applies to the rest of
  `Data` — is applied directly to the plaintext here, before base64-encoding
  (Decision 38: required because once encoded, envelope.go's later whole-blob
  pass can no longer pattern-match secrets inside this field's encoded
  bytes). When parsing fails, no output values are known, so only the
  Gitleaks-pattern masking step applies to the plaintext before encoding.
- Top-level status portion (research.md Decision 20): `has_changes` (bool), `has_errors`
  (bool), `errors` (array of string) — decoded from `citerraform.ParseOutput`'s top-level
  `OutputResult.HasChanges`/`HasErrors`/`Errors` fields, which the parser already computes
  correctly but which the pre-Decision-20 mirror struct silently discarded (it only decoded
  the nested `Data` field).
- Top-level identity portion (research.md Decision 21): `component` (string), `stack`
  (string) — sourced from `cmd/terraform`'s already-resolved call-site data (the positional
  component argument, the parsed `--stack` value), not from a second parse of the captured
  output. Omitted (not empty-string) when unknown.
- `buildTerraformExecData`'s own map literal includes a `version` field (research.md
  Decision 24), but — as of Decision 38 — this is stripped from every entry before it becomes
  one item in the outer `components` list (see "Unified shape" below); `buildTerraformExecData`
  itself is never sent as `Data` directly, so this internal `version` never actually reaches
  the wire.
- Top-level `exit_code` (research.md Decisions 27-29): the terraform/tofu subprocess's own
  process exit code — the authoritative pass/fail/parse-completeness signal, distinct from
  the base `ExecutionRecord.ExitCode` (`atmos`'s own process exit code). Each component's own
  entry in the `components` list carries its own `exit_code`, never a single aggregate
  top-level value — true whether the list has one entry or many. Always populated —
  including when itemized parsing fails entirely, in which case the entry is still returned
  with `exit_code`/`component`/`stack` set and every unparseable field defaulted (Decision
  29), rather than that component's entry being omitted.
- List-typed fields (`changes`, `warnings`, `errors`) MUST serialize as `[]`, never `null`,
  when empty (research.md Decision 26) — `buildTerraformExecData` MUST initialize these as
  non-nil zero-length slices before the map literal is built, not pass a possibly-nil slice
  straight through from `terraformResourceChanges`/`result.Warnings`/`result.Errors`.
- **Provenance amendments (research.md Decisions 31-32/35-36, 2026-08-21 sessions)**:
  `exit_code` MUST be captured pre-CI-remap (Decision 31) — not the value `mapCIExitCode`
  substitutes for Atmos's own process exit code. `-detailed-exitcode` is added to `plan`
  only, never to `apply`/`deploy`'s internal `apply` invocation (Decision 35 — version-
  support risk on older pinned terraform/tofu binaries), so `exit_code`'s value space is
  0/1/2 for `plan` but stays plain 0/1 for `apply`/`deploy`. Adding `-detailed-exitcode` to
  `plan` MUST NOT change Atmos's own process exit code (Decision 36 — a local
  neutralization independent of the global `ci.enabled` switch, not a widening of that
  switch's gate). `resource_counts`/`outputs`/`changes`/
  `warnings`/`errors`/`has_changes`/`has_errors` MUST be extracted from output scoped to only
  that invocation's own subprocess (Decision 32), never a buffer shared with `terraform init`/
  `terraform workspace select`. Extraction stays regex-based (`ParsePlanOutput`/
  `ParseApplyOutput`, same as Native CI) — no shape change to `TerraformExecData` itself, only
  a correctness fix to how its fields are populated and what text they're populated from.

All portions are nested together in one `TerraformExecData` object (e.g.
`{"resource_counts": {...}, "outputs": {...}, "warnings": [], "changes":
[{"action": "created", "address": "aws_vpc.this"}, ...], "has_changes": true, "has_errors":
false, "errors": [], "exit_code": 0, "component": "vpc", "stack": "plat-use2-dev",
"logs": "UGxhbjogMiB0byBhZGQuLi4="}` — no `version` field, per Decision 38), not split into
multiple top-level `Data`-sibling fields. This object is never sent as `Data` on its own; it
is always exactly one entry in the unified `components` list `Data` itself is (see below).

**`terraform deploy` uses this identical shape, not a split view (research.md Decision 30r,
correcting the retracted Decision 30)**: `deploy` continues to be parsed with apply semantics
(`if parseCommand == "deploy" { parseCommand = "apply" }`, `cmd/terraform/utils.go`) and
produces one `TerraformExecData` object, same as `plan`/`apply`. A prior design
(Decision 30) proposed splitting `deploy`'s `Data` into `{"version": 1, "component": ...,
"stack": ..., "plan": {...}, "apply": {...}}` on the premise that `deploy` runs plan and
apply as two separate terraform/tofu subprocess invocations — that premise was discovered
false during implementation: `internal/exec/terraform.go`'s `handleDeploySubcommand`
rewrites `deploy` to `apply` *before* any subprocess runs, so `deploy` executes exactly one
subprocess, with one captured output stream and one exit code. There is no independent
plan-phase output/exit-code for Atmos to report separately, so the single-object shape is
correct, not a workaround.

**Unified shape — single- and multi-component invocations are structurally identical
(research.md Decisions 37/38, spec.md Session 2026-08-21, supersedes Decision 17's
flat-entries shape AND the earlier "single-component Data is a bare TerraformExecData
object" design)**: `Data` for `terraform plan`/`apply`/`deploy` is ALWAYS `{"version": 1,
"components": [TerraformExecData, ...]}` — never a bare `TerraformExecData` object at the
top level, regardless of whether the invocation targeted one component or many. A
single-component invocation (`atmos terraform plan vpc -s dev`) produces a `components` list
of exactly one entry; a multi-component `--affected`/`--all` run produces one entry per
component. There is no other distinction between the two cases — same wrapper, same
per-entry shape, same masking, same size-threshold handling. This retires the earlier,
now-superseded design where a single-component invocation's `Data` was the `TerraformExecData`
object itself with `version` at its own top level.

Each entry in `components` omits its own `version` field (Decision 38) — redundant with the
outer wrapper's `"version": 1`; `stripComponentVersion` (`cmd/terraform/utils.go`) deletes the
key from the map `buildTerraformExecData` returns before it is wrapped. Both call sites —
`terraformExecMetadataParserFunc` (single-component) and `terraformNodeHooks.recordExecResult`
(multi-component) — call `buildTerraformExecData` per component and then
`wrapComponentsData`/`stripComponentVersion` to produce this same shape, so there is exactly
one code path for the wire shape regardless of component count.
`captureMultiComponentExecMetadata` wraps the accumulated list with
`wrapComponentsData(components...)` (itself `proexec.VersionedData(terraformExecDataVersion,
"components", entries)`) before attaching it as the record's `Data`; the single-component path
wraps its one entry the same way, via the same helper.

**Population (research.md Decision 17/18)**: For a single-component invocation,
`internal/exec/terraform.go`'s `captureExecMetadataSync` populates each component's entry via
a caller-supplied parser closure threaded in from `cmd/terraform/plan.go`/`apply.go`/
`deploy.go` (`WithExecMetadataParser`, a new `ShellCommandOption`) — `internal/exec` never
imports the CI plugin's parser directly, since doing so would reintroduce a confirmed import
cycle (`pkg/ci/plugins/terraform` → `internal/exec`); implemented (Decision 18). This closure
is `terraformExecMetadataParserFunc`, which calls `buildTerraformExecData` then wraps its
result via `wrapComponentsData`/`stripComponentVersion` into the same unified shape the
multi-component path produces.

**`Outputs` masking (research.md Decision 19, FR-010a)**: Each entry in `Outputs` is
`{value, sensitive}` — no `type` field. `type` is omitted rather than shipped as a
permanently-blank string: on this feature's apply path, `Outputs` is only ever populated by
`pkg/ci/plugins/terraform.extractApplyOutputs` (the regex-based console parser), which has no
type information available in apply console text and so never sets it. Before
`buildTerraformExecData` returns, a `maskSensitiveOutputs` pass replaces `value` with
`pkg/io.MaskReplacement` (`"<MASKED>"`) for any entry where `sensitive` is `true` (or which
fails to decode — treated as sensitive by default), leaving `sensitive` and all non-sensitive
entries' `value` untouched. This is independent of, and runs strictly before,
`pkg/proexec/envelope.go`'s existing Gitleaks-pattern masking pass over the whole marshaled
`Data` blob — both layers always execute; the Terraform-`sensitive` layer exists because a
sensitive-flagged value need not match any Gitleaks-recognizable secret pattern. Since
Decision 37/38's restructure, every `components[]` entry — whether from a single-component or
a multi-component invocation — is built by the same `buildTerraformExecData` call and so
carries its own masked `Outputs`; this is no longer a single-component-only concern.

**Sensitive-output detection**: `extractApplyOutputs` sets `Sensitive: true` whenever an
output's parsed value is exactly Terraform's own literal placeholder text `<sensitive>` — the
only value Terraform ever prints for a sensitive output in apply console text (it never prints
the real value there), so the placeholder itself is a reliable signal. `maskSensitiveOutputs`
then replaces that placeholder with the shared `pkg/io.MaskReplacement` value, so a sensitive
output's `value` in `Outputs` and in the redacted `logs` field never shows Terraform's own
distinct `<sensitive>` string, only the one masking placeholder used throughout `Data`.
Regression-tested by `TestExtractApplyOutputs_SensitiveOutputNeverExposesRealValue` and
`TestBuildTerraformExecData_SensitiveOutputNeverUploadedInAnyForm`.

### AffectedStacksExecData (`Data` shape for `describe affected`, research.md Decision 22)

`Data = proexec.VersionedData(1, "stacks", affected)`, i.e. `{"version": 1, "stacks":
[schema.Affected, ...]}`. `affected` is the exact `[]schema.Affected` slice `describe
affected` already computes for every invocation (rendering, and — when `--upload` fires —
`UploadAffectedStacksRequest.Stacks`) — `executeInner` now returns it (previously discarded
after use) so `Execute` can attach it without a second resolution pass. Each `schema.Affected`
entry carries its own identity (`component`, `component_type`, `stack`, etc.), the reason it
is affected, its dependents, and its settings — the same fields already sent to
`POST /api/v1/affected-stacks`. Fields already present in the execution record's base envelope
(`repo_url`/`repo_name`/`repo_owner`/`repo_host`, `git_sha`) are NOT duplicated here — this
shape carries only `version` and `stacks`. Unlike `list instances` (below), `Data` is attached
regardless of whether `--upload` was passed, since `affected` is always computed.

### InstancesExecData (`Data` shape for `atmos list instances`, research.md Decision 23)

`Data = proexec.VersionedData(1, "instances", req.Instances)`, i.e. `{"version": 1,
"instances": [dtos.UploadInstance, ...]}` — present **only** when the invocation's `--upload`
flag was passed (the `[]UploadInstance` list is not built otherwise, and this shape MUST NOT
force that computation just to populate `Data`). Each `UploadInstance` entry carries
`component`, `stack`, `component_type`, and `settings` — the same fields already sent to
`POST /api/v1/instances`. Populated via a new `proexec.SetPendingAsyncData` hand-off (since
`list instances` is not sync-allowlisted — FR-007 — and the generic async hook,
`cmd/root.go`'s `proexec.CaptureAsync`, has no command-specific knowledge of its own): called
inside `ExecuteListInstancesCmd`'s existing `--upload` branch, immediately after
`req.Instances` is built; read and cleared by `CaptureAsync` when it assembles that
invocation's `ExecRecordInput`, so a value never leaks into a later invocation within the same
process.

### ExecUploadResponse

| Field | Type | Notes |
|-------|------|-------|
| `AtmosApiResponse` (embedded) | — | `Success`, `Status`, `TraceID`, etc. — same envelope as every other Pro response |

---

## Delivery Classification

| Command | Sync/Async | Failure behavior | Structured `Data` |
|---|---|---|---|
| `terraform plan` | Sync | Warn-and-continue (does not fail the plan) | `TerraformOutputData` |
| `terraform apply` | Sync | Warn-and-continue (does not fail the apply) | `TerraformOutputData` |
| `terraform deploy` | Sync | Warn-and-continue (does not fail the deploy) | `TerraformOutputData` |
| `describe affected` | Sync | Warn-and-continue | `AffectedStacksExecData` — unconditional (research.md Decision 22) |
| `atmos list instances` (`--upload` passed) | Async (fire-and-forget, bounded flush) | N/A — never affects exit code | `InstancesExecData` — via `SetPendingAsyncData` (research.md Decision 23) |
| All other commands (incl. `list instances` without `--upload`) | Async (fire-and-forget, bounded flush) | N/A — never affects exit code | `nil` |

The specific fail-vs-warn choice per synchronous command (FR-008) defaults to
**warn-and-continue** for all four sync commands — a delivery outage must never turn
a successful `terraform apply`/`deploy` into a failed CI run. This is a code-level
default, not a user setting, consistent with the spec's Assumptions.

**Sync/async exclusivity (FR-007, 2026-08-18 clarification)**: A command classified as
sync above MUST NOT also receive the async default-path upload for the same invocation.
Both `cmd/root.go` (async call site) and `internal/exec/terraform.go`/`describe affected`
(sync call site) MUST consult the same shared predicate (`proexec.IsSyncCommand`, research
Decision 10) so the two paths cannot independently drift — this was the root cause of a
production defect where a single `atmos terraform plan` produced two execution records.

**Multi-component invocations (FR-006a, 2026-08-18 clarification, restructured by research.md
Decisions 37/38, spec.md Session 2026-08-21)**: When `plan`/`apply`/`deploy` targets multiple
components in one CLI invocation (`--affected`/`--all`), exactly one `ExecutionRecord` is
produced for the whole invocation — not one per component. `Data` is `{"version": 1,
"components": [TerraformExecData, ...]}` — the identical unified shape a single-component
invocation also uses (see "Unified shape" above), just with more than one entry: one complete
object per component (identity, outcome, itemized resource changes, outputs, warnings,
`logs`, etc., minus its own `version` field per Decision 38), not the retired flat shared list
of `{component, stack, exitCode, action, address}` entries. `UploadExecMetadata`'s
inline-vs-blob-URL size check (FR-011, research.md Decision 16) applies transparently if the
combined multi-component `Data` is large enough to exceed the 4 MB threshold.

---

## Validation Rules

| Rule | Detail |
|------|--------|
| Gate check | `proexec.gateOpen` MUST be evaluated fresh per invocation; never cached across commands within the same process |
| Secret masking | `Args`, `Flags`, and `Data` MUST pass through the existing Gitleaks-based masking (`pkg/io` masking) before marshaling, consistent with FR-010 — masking happens once, before the inline-vs-blob size check, not duplicated per delivery path |
| Command/Args/Flags shape | `Command` MUST exclude the `atmos` root segment; `Args` MUST hold only positional arguments; `Flags` MUST hold only CLI flags — never combined into one array (FR-003b) |
| Independence from `uploadStatus` | This endpoint MUST NOT be skipped, merged, or made conditional on whether `uploadStatus`/`--upload-status` (`internal/exec/pro.go`) also fires for the same invocation, and vice versa — the two mechanisms remain fully independent (FR-003a); only `Command`/`Args`/`Flags` content is kept correlatable |
| Payload size / Data delivery | `UploadExecMetadata` MUST marshal the whole record (envelope + `Metrics` + `Data` inline) once and compare its byte length against `Settings.Pro.MaxPayloadBytes` (falling back to `DefaultMaxPayloadBytes` = 4 MB). Under the threshold: send the record as-is, `Data` inline. At/over the threshold: call `UploadExecData` once (never chunked) with `{execution_id, data}`, then send the record to `/exec` with `Data` replaced by the returned `url` (JSON string). The envelope and `Metrics` are never affected by this and are always sent in full (FR-011) |
| Sync timeout | `CaptureSync` MUST bound its **total** wait — across both the `/exec/data` upload (if required) and the main `/exec` call combined — to `max(Settings.Pro.Exec.SyncTimeoutSeconds, 10)` seconds (FR-011a) |
| Async flush ceiling | `CaptureAsync` MUST bound its wait — across both the `/exec/data` upload (if required) and the main `/exec` call combined — to a fixed 2 seconds, not configurable (FR-011a); if the sequence can't complete in time, the record is dropped silently (FR-009a) |
| Correlation ID | `AtmosProRunID` MUST be sourced identically to `internal/exec/pro.go: uploadStatus` (`os.Getenv("ATMOS_PRO_RUN_ID")`) for consistency across both upload paths |
| ExecutionID | MUST be a fresh UUID v4 (`uuid.New().String()`) generated once per invocation inside `buildRecord`; MUST be reused (not regenerated) across `doWithRetry` retry attempts for the same invocation's request(s); MUST be sent as `execution_id` on `ExecDataUploadRequest` when that upload is required, so Atmos Pro can associate the blob with the corresponding `ExecutionRecord` |
| `Data.version` | Every structured `Data` shape MUST include its own top-level `version` field (plain integer, starting at `1`, independent per shape — FR-005a, research.md Decision 24). The outer `ExecutionRecord`/`ExecDataUploadRequest` envelope is NOT versioned |
| Pending async data | `proexec.SetPendingAsyncData` MUST be cleared by `CaptureAsync` on read (`data := pendingAsyncData; pendingAsyncData = nil`), never left for the caller to clear — a value set by one invocation MUST NOT leak into a later invocation's `ExecRecordInput.Data` within the same process (research.md Decision 23) |

---

## State Transitions

None — each `ExecutionRecord` is a single, stateless, one-shot upload per command
invocation. There is no update/patch semantics on this endpoint (unlike
`UploadInstanceStatus`, which patches an existing instance). The `POST /v1/atmos/exec/data`
upload (when required) is likewise a single, stateless, one-shot upload that must complete
before the corresponding `/exec` request is sent — there is no retry-with-different-URL or
update semantics on the blob either.
