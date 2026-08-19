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

Both portions are nested together in the single `Data` value (e.g.
`{"resource_counts": {...}, "outputs": {...}, "warnings": [...], "changes": [{"action":
"created", "address": "aws_vpc.this"}, ...]}`), not split into two top-level fields.

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
| `describe affected` | Sync | Warn-and-continue | `nil` |
| All other commands | Async (fire-and-forget, bounded flush) | N/A — never affects exit code | `nil` |

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

**Multi-component invocations (FR-006a, 2026-08-18 clarification)**: When `plan`/`apply`/
`deploy` targets multiple components in one CLI invocation (`--affected`/`--all`), exactly
one `ExecutionRecord` is produced for the whole invocation — not one per component. Each
component's identity (`component`, `stack`), outcome (`exitCode`), and structured data
(created/updated/deleted/replaced/moved/imported resources, outputs, warnings) are folded
into that single record's `Data` as one entry per component (research.md Decision 17), e.g.
`{"component": "vpc", "stack": "plat-use2-dev", "exitCode": 0, "action": "created",
"address": "aws_vpc.this"}`-shaped items — `UploadExecMetadata`'s inline-vs-blob-URL size
check (FR-011, research.md Decision 16) applies transparently if the combined
multi-component `Data` is large enough to exceed the 4 MB threshold.

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

---

## State Transitions

None — each `ExecutionRecord` is a single, stateless, one-shot upload per command
invocation. There is no update/patch semantics on this endpoint (unlike
`UploadInstanceStatus`, which patches an existing instance). The `POST /v1/atmos/exec/data`
upload (when required) is likewise a single, stateless, one-shot upload that must complete
before the corresponding `/exec` request is sent — there is no retry-with-different-URL or
update semantics on the blob either.
