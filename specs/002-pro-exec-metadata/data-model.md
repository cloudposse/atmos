# Data Model: Atmos Pro Command-Execution Metadata Upload

**Feature**: 002-pro-exec-metadata
**Date**: 2026-08-11

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
| `AtmosProRunID` | `string` | Reused from the existing `ATMOS_PRO_RUN_ID` env var convention (same as `InstanceStatusUploadRequest`) — correlation ID, per Clarification Q4 |
| `AtmosVersion` | `string` | `pkgversion.Version` |
| `AtmosOS` | `string` | `runtime.GOOS` |
| `AtmosArch` | `string` | `runtime.GOARCH` |
| `Command` | `string` | Subcommand path with the leading `atmos` root stripped, e.g. `"terraform plan"` (not `"atmos terraform plan"`) — FR-003b, 2026-08-18 (2nd) clarification |
| `Args` | `[]string` | Positional arguments only, e.g. `["cdn"]` — previously always empty (`maskArgs(nil)` bug in `envelope.go:55`), now populated per FR-003b |
| `Flags` | `[]string` | **New** — every CLI flag actually passed, masked, no exclusions (`--upload-status` included), as bare tokens exactly as typed — e.g. `["-s", "plat-use2-dev", "--upload-status"]`, never `["--upload-status", "true"]` for a bool flag; kept separate from `Args`, never combined — FR-003b, 2026-08-19 clarification. MUST be sourced from the invoking command's own record of explicitly-set flags (e.g. Cobra's `cmd.Flags().Visit`), never from a pass-through/leftover-args collection that cannot contain atmos-recognized flags in the first place — see research.md Decision 14 |
| `ExitCode` | `int` | |
| `GitSHA` | `string` | |
| `RepoURL`, `RepoName`, `RepoOwner`, `RepoHost` | `string` | From `git.GitRepoInterface.GetLocalRepoInfo()`, matching `InstanceStatusUploadRequest` |
| `Metrics` | `ResourceUsageMetrics` | Always present (Unix-only sub-fields `omitempty`) |
| `Data` | `any` (`json.RawMessage` after marshal) | Command-specific structured *summary* data (bounded/small — e.g. `ResourceCounts`, `Outputs`, `Warnings`); `nil`/absent for most commands. Never chunked. |
| `DataItems` | `[]json.RawMessage`, `omitempty` | Command-specific structured *bulk* data — the potentially large, chunkable array (e.g. one `{action, address}` entry per created/updated/deleted/replaced/moved/imported resource for `terraform plan`/`apply`); `nil`/absent for most commands. Split across correlated requests via `pro.sendChunked` when it would exceed `MaxPayloadBytes` (FR-011) |
| `BatchID` | `string`, `omitempty` | Present only on chunked requests — `pro.BatchInfo.BatchID`, correlates all chunks of one `ExecutionRecord`'s `DataItems` |
| `BatchIndex` | `*int`, `omitempty` | Present only on chunked requests — 0-based chunk index |
| `BatchTotal` | `*int`, `omitempty` | Present only on chunked requests — total chunk count |

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

### TerraformExecData (one concrete `Data`/`DataItems` shape, for `terraform plan`/`apply`/`deploy`)

Derived from the already-merged `pkg/ci/internal/plugin.TerraformOutputData` structure,
split across the two fields by size:
- `Data` (small, always sent in full): `ResourceCounts`, `Outputs`, `HasOutputChanges`,
  `ChangedResult`, `Warnings`.
- `DataItems` (potentially large, chunkable): one `{action, address}` entry per resource
  in `CreatedResources`, `UpdatedResources`, `ReplacedResources`, `DeletedResources`,
  `MovedResources`, `ImportedResources` — `action` is the source field name
  (`"created"`/`"updated"`/`"replaced"`/`"deleted"`/`"moved"`/`"imported"`), `address` is
  the resource address string.

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
into that single record's `DataItems` as one entry per component, e.g.
`{"component": "vpc", "stack": "plat-use2-dev", "exitCode": 0, "action": "created",
"address": "aws_vpc.this"}`-shaped items — the existing chunking mechanism (FR-011)
applies transparently if the combined multi-component `DataItems` list is large enough to
exceed `MaxPayloadBytes`. See research.md Decision 11 for the aggregation-point rationale.

---

## Validation Rules

| Rule | Detail |
|------|--------|
| Gate check | `proexec.gateOpen` MUST be evaluated fresh per invocation; never cached across commands within the same process |
| Secret masking | `Args`, `Flags`, and `Data` MUST pass through the existing Gitleaks-based masking (`pkg/io` masking) before marshaling, consistent with FR-010 |
| Command/Args/Flags shape | `Command` MUST exclude the `atmos` root segment; `Args` MUST hold only positional arguments; `Flags` MUST hold only CLI flags — never combined into one array (FR-003b) |
| Independence from `uploadStatus` | This endpoint MUST NOT be skipped, merged, or made conditional on whether `uploadStatus`/`--upload-status` (`internal/exec/pro.go`) also fires for the same invocation, and vice versa — the two mechanisms remain fully independent (FR-003a); only `Command`/`Args`/`Flags` content is kept correlatable |
| Payload size | Marshaled body (envelope + `Metrics` + `Data`) MUST be compared against `Settings.Pro.MaxPayloadBytes` (falling back to the existing Pro default). When it fits, the record is sent as a single request. When `DataItems` pushes it over the limit, `DataItems` is split across multiple correlated requests via `pro.sendChunked`/`BatchInfo`, each carrying the full envelope/`Metrics`/`Data`; the envelope, `Metrics`, and `Data` themselves are never truncated or chunked (FR-011) |
| Sync timeout | `CaptureSync` MUST bound its **total** wait — across all chunk requests combined, if `DataItems` required batching — to `max(Settings.Pro.Exec.SyncTimeoutSeconds, 10)` seconds |
| Async flush ceiling | `CaptureAsync` MUST bound its wait to a fixed 2 seconds, not configurable |
| Correlation ID | `AtmosProRunID` MUST be sourced identically to `internal/exec/pro.go: uploadStatus` (`os.Getenv("ATMOS_PRO_RUN_ID")`) for consistency across both upload paths |

---

## State Transitions

None — each `ExecutionRecord` is a single, stateless, one-shot upload per command
invocation. There is no update/patch semantics on this endpoint (unlike
`UploadInstanceStatus`, which patches an existing instance).
