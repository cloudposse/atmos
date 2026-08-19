# Implementation Plan: Atmos Pro Command-Execution Metadata Upload

**Branch**: `1199-pro-exec-metadata` | **Date**: 2026-08-19 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/002-pro-exec-metadata/spec.md`

**Note**: This is a fourth re-plan. US1/US2/US3 and the `Command`/`Args`/`Flags`
correctness fixes (third re-plan) are already implemented and present in the current tree.
This revision covers two new `/speckit-clarify` (2026-08-19, later same day) decisions that
change the wire shape of the execution record:

1. **`ExecutionID`** — a new UUID v4, generated once per qualifying invocation, added to the
   base envelope alongside the existing `AtmosProRunID`.
2. **`Data` delivery redesign ("batch upload redo")** — the previously-shipped multi-chunk
   `DataItems`/`BatchID`/`BatchIndex`/`BatchTotal` model (`pkg/pro/api_client_exec.go`,
   `pkg/pro/dtos/exec.go`) is retired entirely and replaced with a binary choice on the
   single `Data` field: inline JSON when the whole record is under a fixed 4 MB, or a
   string URL to a blob uploaded via a new `POST /v1/atmos/exec/data` endpoint (always
   exactly one request, never chunked) when at/over 4 MB. The blob upload is keyed by
   `execution_id` so Atmos Pro can associate it with the corresponding `/exec` record.

Both changes are additive/replacing at the DTO and client level; the gate, delivery
classification (sync allowlist), timeout mechanics, multi-component aggregation point, and
`Command`/`Args`/`Flags` shape from prior re-plans are all unchanged and are not
re-litigated here.

## Summary

Add `ExecutionID` (UUID v4) to every execution record, and replace the existing
multi-chunk `DataItems` batching mechanism with a size-gated choice: `Data` is sent inline
when the whole marshaled record is under 4 MB, or uploaded once (no chunking) to
`POST /v1/atmos/exec/data` — keyed by `execution_id` — with the returned blob URL sent in
`Data`'s place on the main `POST /v1/atmos/exec` request. Both the data-upload step (when
required) and the main upload count toward the same existing delivery-timing budget already
established for sync (~10s total) and async (~2s flush) commands — no new timing surface.
The Pact consumer contract is extended/regenerated to cover both `Data` shapes plus the new
`/exec/data` interaction.

## Technical Context

**Language/Version**: Go 1.26

**Primary Dependencies**:
- `pkg/pro/dtos/exec.go` (existing, shipped) — modified: add `ExecutionID string
  \`json:"execution_id"\`` to `ExecUploadRequest`; remove `DataItems`, `BatchID`,
  `BatchIndex`, `BatchTotal` (retired); `Data json.RawMessage` is unchanged in type — its
  *content* is now either an inline JSON structure or a JSON string holding a blob URL,
  decided by the client before marshaling, never both. New DTOs: `ExecDataUploadRequest`
  (`execution_id string`, `data json.RawMessage`) and `ExecDataUploadResponse`
  (embeds `AtmosApiResponse`, adds `url string`) for the new endpoint.
- `pkg/pro/api_client_exec.go` (existing, shipped) — `UploadExecMetadata` no longer calls
  `sendChunked`/`BatchInfo`; instead it marshals the full record once, compares its byte
  length against the existing `DefaultMaxPayloadBytes`/`c.MaxPayloadBytes` (already `4 *
  1024 * 1024` — the 4 MB threshold requires no new constant), and when at/over the
  threshold calls a new `UploadExecData(dto *dtos.ExecDataUploadRequest)
  (*dtos.ExecDataUploadResponse, error)` method (`POST {BaseURL}/{BaseAPIEndpoint}/atmos/
  exec/data`, same `doWithRetry`/`getAuthenticatedRequest`/`handleAPIResponse` shape as
  every other Pro client method) before re-marshaling the record with `Data` replaced by
  the returned URL and sending it to `/exec`. Added to `AtmosProAPIClientInterface`.
- `pkg/proexec/envelope.go` (existing, shipped) — `buildRecord` gains `ExecutionID`
  generation (`uuid.New().String()`, `github.com/google/uuid` — already a direct
  dependency, already used identically by `pkg/pro/chunked_upload.go`'s `BatchID`
  generation) and drops the `dataItems`/`maskedDataItemsJSON` parameter and helper (no
  replacement needed — masking still applies to `data` exactly as today; the multi-component
  aggregation that used to populate `dataItems` now populates `data` as a JSON array
  instead, see Decision 17 in research.md).
- `pkg/proexec/sync.go` / `async.go` (existing, shipped) — `CaptureSync`/`CaptureAsync`
  signatures drop the `dataItems []any` parameter (folded into `data any`); no change to
  their timeout/flush mechanics, since the data-upload step is fully contained inside the
  existing `UploadExecMetadata` call these functions already race against a timer.
- `cmd/terraform/utils.go` (existing, shipped — multi-component aggregation, research.md
  Decision 11) — the per-component accumulator that used to build a `dataItems []any` list
  now builds a single `data` value shaped as `{summary: ..., components: [...]}` (or
  equivalent), since `DataItems` no longer exists as a separate field.
- `github.com/google/uuid` (existing direct dependency — no new dependency added).

**Storage**: N/A — unchanged; no local persistence. The new blob storage (Vercel Blob,
per the clarification) is entirely Atmos-Pro-side; the Atmos client only calls the new
`/exec/data` endpoint and stores the returned URL string in-memory before the next request.

**Testing**: `atmos test` (unit, table-driven, mocked/`httptest`-faked Atmos Pro server).
New/changed cases needed: `UploadExecMetadata` size-threshold branch (record under 4 MB →
single `/exec` call, `Data` inline; record at/over 4 MB → `/exec/data` call first, then
`/exec` with `Data` = URL string), `UploadExecData` request/response shape, `buildRecord`'s
`ExecutionID` generation (non-empty, valid UUID v4, fresh per call), and removal of the
now-dead chunking test cases (`TestSendChunked`-style cases against `ExecUploadRequest`
specifically — `pkg/pro/chunked_upload.go` itself is unchanged and still used by
`UploadAffectedStacks`/`UploadInstances`, only its use for `ExecUploadRequest` is retired).
Pact consumer suite (`pkg/pro/consumer_pact_test.go`, `//go:build pact`) gains a 10th
interaction (`UploadExecData`) and the existing 9th interaction (`UploadExecMetadata`)
splits into two example cases — inline `Data` (small record) and blob-URL `Data` (record
that exceeded 4 MB) — per the user's explicit request to generate pacts for "multiple cases
(single and batched mode)".

**Target Platform**: Linux, macOS, Windows — unchanged.

**Project Type**: CLI feature — targeted redesign of already-shipped `pkg/pro/dtos/exec.go`,
`pkg/pro/api_client_exec.go`, `pkg/proexec/envelope.go`, `pkg/proexec/sync.go`/`async.go`,
and `cmd/terraform/utils.go`'s multi-component aggregation. No new packages.

**Performance Goals**: Unchanged (SC-003/SC-004) — the data-upload step is folded into the
same existing timing budgets, not a new one.

**Constraints**:
- No new user-facing configuration surface — the 4 MB threshold is fixed, not exposed via
  `atmos.yaml`/env (per clarification), matching the spec's Assumptions ("no new
  configuration surface").
- `ExecutionID` MUST be a fresh UUID v4 per invocation (`uuid.New()`, which is
  version-4/random by default in `google/uuid`) — no reuse across retries within the same
  invocation's own request(s); if `UploadExecMetadata` retries via `doWithRetry`, the same
  already-generated `ExecutionID` is reused across retry attempts (it identifies the
  *invocation*, not the individual HTTP attempt).
- Masking (FR-010) continues to apply to `Data`'s content before either the inline-JSON
  path or the blob-upload path — i.e. masking happens once, before the size check, not
  duplicated per path.
- The `/exec/data` upload, when required, MUST happen *before* the main `/exec` request for
  the same invocation (sequential dependency — the URL must exist before it can be embedded
  in `Data`); no concurrency between the two requests is introduced.

**Scale/Scope**: 1 new field (`ExecutionID`) on 1 existing DTO, 4 fields removed
(`DataItems`/`BatchID`/`BatchIndex`/`BatchTotal`), 2 new DTOs (`ExecDataUploadRequest`/
`ExecDataUploadResponse`), 1 new client method (`UploadExecData`), 0 new packages. This
redesign touches every place that previously threaded `dataItems`/`BatchInfo` through the
exec-metadata path — smaller in field count than the multi-chunk model it replaces.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Registry-Driven Extensibility | ✅ Pass | No new CLI commands/flags; a new Atmos Pro client method (`UploadExecData`) added to the existing `AtmosProAPIClientInterface`, following the same pattern as the other five methods already on it — not a new registry. |
| II. Interface-Driven Design with DI | ✅ Pass | `UploadExecData` is added to `AtmosProAPIClientInterface` (mockgen-generated mock regenerated); `buildRecord`/`CaptureSync`/`CaptureAsync` remain plain functions with injected `pro.AtmosProAPIClientInterface`/`git.GitRepoInterface`, unchanged pattern. |
| III. Test-First with 80% Coverage | ✅ Pass | Bug-fixing/redesign workflow: write failing tests for the new threshold branch and `UploadExecData` shape before implementing (CLAUDE.md's Bug-Fixing Workflow, applied here since this replaces already-shipped, tested behavior). Removed `DataItems`-chunking tests are deleted, not left skipped (CLAUDE.md's "remove always-skipped tests"). |
| IV. Separated I/O and UI Architecture | ✅ Pass | No new user-visible output; `ui.Warningf`/`log.Debug` call sites in `sync.go`/`async.go` are unchanged. |
| V. Simplicity and No Over-Engineering | ✅ Pass | Rejected keeping both the old chunk model and the new blob model side-by-side "for compatibility" — the old model is unshipped-to-any-external-consumer (Atmos Pro provider side does not exist yet; this feature's own Pact contract is the only consumer), so there is no compatibility burden to preserve, and running two size-handling mechanisms for the same field would be exactly the kind of premature-flexibility CLAUDE.md's Simplicity principle forbids. |

**Post-design re-check**: ✅ Pass. Phase 1 complete — `data-model.md`'s `ExecutionRecord`
table now carries `ExecutionID` and the single redesigned `Data` field (old `DataItems`/
`BatchID`/`BatchIndex`/`BatchTotal` rows removed, new `ExecDataUploadRequest`/
`ExecDataUploadResponse` entities added); `contracts/interactions.md` now documents three
interactions (9: inline `Data`, 10: blob-URL `Data`, 11: `UploadExecData`) in place of the
old single chunked interaction; `quickstart.md` gained steps 9-10 for exercising
`ExecutionID` and the 4 MB threshold. No new violations introduced.

## Project Structure

### Documentation (this feature)

```text
specs/002-pro-exec-metadata/
├── plan.md                 # This file — fourth re-plan: ExecutionID + Data blob-upload redesign
├── research.md              # Phase 0 output — Decisions 15–17 appended for this delta
├── data-model.md            # Phase 1 output — ExecutionRecord/Data/new ExecDataUpload* entities updated
├── quickstart.md            # Phase 1 output — new step for exercising the 4MB threshold + blob path
├── contracts/
│   └── interactions.md      # Phase 1 output — interaction 9 split into two Data-shape cases; new interaction 10 (/exec/data)
└── tasks.md                  # Phase 2 output (/speckit-tasks) — NEEDS REGENERATION for this delta (DataItems/BatchID tasks superseded)
```

### Source Code (repository root)

```text
pkg/pro/dtos/exec.go
├── ExecUploadRequest           # Modified — +ExecutionID; -DataItems/-BatchID/-BatchIndex/-BatchTotal
├── ExecDataUploadRequest       # New — execution_id, data
└── ExecDataUploadResponse      # New — embeds AtmosApiResponse, +url

pkg/pro/api_client_exec.go
├── UploadExecMetadata          # Modified — size check (4MB) replaces sendChunked; calls
│                                  UploadExecData first when over threshold, then sends /exec
│                                  with Data replaced by the returned URL
└── UploadExecData              # New — POST /v1/atmos/exec/data, single request, no chunking

pkg/proexec/envelope.go
└── buildRecord                  # Modified — generates ExecutionID (uuid.New().String());
                                    drops dataItems param/maskedDataItemsJSON helper

pkg/proexec/sync.go
└── CaptureSync                  # Modified — drops dataItems parameter (folded into data)

pkg/proexec/async.go
└── CaptureAsync / uploadExecMetadata   # Modified — drops dataItems parameter

cmd/terraform/utils.go
└── (multi-component aggregation, research.md Decision 11)   # Modified — accumulates into
                                    a single `data` value (JSON array/object) instead of a
                                    separate dataItems list, since DataItems no longer exists
```

**Structure Decision**: No new packages, no new files beyond test files. This delta
modifies the same five call sites the third re-plan already touched (`pkg/pro/dtos/exec.go`,
`pkg/pro/api_client_exec.go`, `pkg/proexec/envelope.go`, `pkg/proexec/sync.go`/`async.go`,
`cmd/terraform/utils.go`), replacing the multi-chunk mechanism in place rather than adding a
parallel path, consistent with the constitution's simplicity principle.

## Complexity Tracking

> No Constitution Check violations — section intentionally empty.
