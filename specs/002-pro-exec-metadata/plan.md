# Implementation Plan: Atmos Pro Command-Execution Metadata Upload

**Branch**: `1199-pro-exec-metadata` | **Date**: 2026-08-12 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/002-pro-exec-metadata/spec.md`

## Summary

Add a new `pkg/proexec` package that, whenever Atmos is running in a recognized CI
environment **and** Atmos Pro is configured, uploads a command-execution record to a new
Atmos Pro endpoint `POST /v1/atmos/exec`. Every command gets a fire-and-forget,
best-effort-flushed async upload hooked next to the existing `telemetry.CaptureCmd` call
in `cmd/root.go`. Three commands (`terraform plan`, `terraform apply`, `describe
affected`) additionally call a synchronous variant directly from their own execution
path, bounded by a configurable (default 10s) **total** timeout, with a warn-and-continue
failure default. A new `pkg/metrics/process` package captures the `atmos` process's own
wall-time/CPU/memory/IO usage via a baseline-and-diff snapshot. `terraform plan`/`apply`
attach the already-computed `pkg/ci/internal/plugin.TerraformOutputData` as
command-specific structured data, split into a small always-sent `Data` summary
(`ResourceCounts`, `Outputs`, `Warnings`) and a potentially large, chunkable `DataItems`
list (one `{action, address}` entry per resource change) that is never truncated —
oversized `DataItems` are instead split across multiple correlated requests reusing the
existing `pkg/pro/chunked_upload.go` (`sendChunked`/`BatchInfo`) mechanism already proven
by `UploadAffectedStacks`/`UploadInstances`. The existing local-only Pact consumer
contract suite in `pkg/pro/` gains a 9th interaction for the new endpoint (including a
chunked-request example), so the generated `pacts/atmos-AtmosPro.json` can be handed to
the Atmos Pro team to implement the provider side.

## Technical Context

**Language/Version**: Go 1.26

**Primary Dependencies**:
- `pkg/pro` (existing — `AtmosProAPIClient`, DTOs, `sendChunked`/`BatchInfo` chunking helpers) — extended, not replaced
- `pkg/telemetry` (existing — `IsCI()`/`ciProvider()` reused verbatim)
- `pkg/git` (existing — `GitRepoInterface.GetLocalRepoInfo()`/`GetCurrentCommitSHA()`)
- `pkg/ci/internal/plugin` (existing — `TerraformOutputData` mapped into `Data`/`DataItems`)
- `syscall` (stdlib, Unix `Getrusage`) / `golang.org/x/sys/windows` (new dependency, Windows process metrics)
- `github.com/pact-foundation/pact-go/v2` (existing dev dependency, from 001-pact-consumer-contracts)

**Storage**: N/A — no local persistence; all data is transmitted to the Atmos Pro API.
Pact contract JSON (`pacts/atmos-AtmosPro.json`) is the only new on-disk artifact, and it
is test-generated, not runtime state.

**Testing**: `atmos test` (unit, with mocks for `AtmosProAPIClientInterface` and a fake
CI/Pro-config environment, including a multi-chunk `sendChunked` case); `go test -tags
pact ./pkg/pro/...` for the new Pact interactions — single-request and chunked — (local-
only, no CI job, per existing 001 convention).

**Target Platform**: Linux, macOS, Windows — `pkg/metrics/process` has a Unix
implementation (`syscall.Rusage`) and a reduced-fidelity Windows implementation
(wall time + process CPU times only).

**Project Type**: CLI feature — new internal packages (`pkg/proexec`,
`pkg/metrics/process`), plus targeted call-site changes in `cmd/root.go` and
`internal/exec/terraform.go` / the `describe affected` execution path. No new CLI
commands or flags.

**Performance Goals**: Async default path MUST add no more than a fixed 2-second
best-effort flush ceiling to any command's total run time (SC-004). Synchronous path MUST
add no more than the configured **total** wait ceiling (default 10s, covering every chunk
combined if `DataItems` required batching) to `terraform plan`/`apply`/`describe affected`
(SC-003).

**Constraints**:
- No new user-facing opt-out; gate is `telemetry.IsCI() && Pro-configured` only (FR-001/FR-002).
- Sync/async classification and fail-vs-warn behavior are hardcoded per command in code, not configuration (spec Assumptions).
- Secret masking MUST apply to `Args` and `Data`/`DataItems` before upload (FR-010).
- Command-specific structured data (`DataItems`) MUST NOT be truncated or dropped for size; when it exceeds `Settings.Pro.MaxPayloadBytes` it MUST be split across multiple correlated requests via `pro.sendChunked`/`BatchInfo`, reusing the existing `describe affected`/`UploadInstances` chunking mechanism (FR-011). The base envelope, `Metrics`, and small `Data` summary are never chunked and are repeated in full on every chunk request.
- The synchronous wait bound (FR-008a) applies to the complete record delivery (all chunks combined), not per chunk.

**Scale/Scope**: 1 new Atmos Pro endpoint (2 new pact interactions — single-request and
chunked — bringing the suite to 10), 2 new packages, ~3 call-site integrations (root hook
+ 2 sync commands sharing one `ExecuteTerraform` pipeline + `describe affected`).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Registry-Driven Extensibility | ✅ Pass | No new CLI commands/flags. `pkg/proexec` is a plain package, not a registry-eligible extension point — command-specific data is passed explicitly per research Decision 5, deliberately avoiding a premature registry (see Alternatives Considered there). |
| II. Interface-Driven Design with DI | ✅ Pass | `AtmosProAPIClientInterface` gains `UploadExecMetadata` and is extended, not bypassed; `pkg/proexec`'s Pro-client dependency is injected the same way `internal/exec/pro.go` already does, enabling mock-based unit tests, including a mocked multi-chunk `sendChunked` path. |
| III. Test-First with 80% Coverage | ✅ Pass | New packages ship with table-driven unit tests (gate logic, envelope assembly, `Data`/`DataItems` split, chunked delivery, timeout clamping) plus the Pact contract tests (single-request and chunked). Existing `atmos test` coverage gate applies normally (pact tests are opt-in via build tag, excluded from CodeCov per 001's precedent). |
| IV. Separated I/O and UI Architecture | ✅ Pass | All new user-visible signals (FR-009a debug-level log line) go through `pkg/logger`, matching existing `pkg/pro`/`pkg/telemetry` conventions; no direct `fmt.Print*`. |
| V. Simplicity and No Over-Engineering | ✅ Pass | No registry/interface introduced for the 2-consumer structured-data case (YAGNI, research Decision 5); chunking reuses the existing generic `sendChunked` helper rather than a new implementation (research Decision 6); files kept under 600 lines by splitting `pkg/proexec` into gate/envelope/async/sync files following the existing `pkg/pro/api_client_*.go` one-concern-per-file convention. |

**Post-design re-check**: ✅ Pass — Phase 1 design (data-model.md, contracts/) introduces
no new violations; the structured-data mechanism decision (deferred by the spec) resolved
in favor of the simplest option consistent with Principle V, and the chunking decision
(reversed after clarification) reuses existing infrastructure rather than adding a new
mechanism.

## Project Structure

### Documentation (this feature)

```text
specs/002-pro-exec-metadata/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md         # Phase 1 output
├── quickstart.md         # Phase 1 output
├── contracts/
│   └── interactions.md   # Phase 1 output — 9th/10th Pact interactions, extends 001's contract doc
└── tasks.md               # Phase 2 output (/speckit-tasks) — NEEDS REGENERATION: still reflects the superseded truncate-only design (T009 truncate.go)
```

### Source Code (repository root)

```text
pkg/proexec/                          # New package
├── gate.go                            # gateOpen(): telemetry.IsCI() && Pro-configured check
├── envelope.go                        # buildRecord(): assemble dtos.ExecUploadRequest (base + metrics + masked args/Data/DataItems)
├── async.go                           # CaptureAsync(cmd, err): fire-and-forget, 2s best-effort flush
├── sync.go                            # CaptureSync(info, exitCode, data, dataItems, opts): blocking, configurable total timeout, warn-and-continue
└── *_test.go                          # Unit tests per file, table-driven, mocked AtmosProAPIClientInterface

pkg/metrics/process/                   # New package
├── metrics.go                         # ProcessMetrics struct, Snapshot, Baseline(), Since()
├── metrics_unix.go                    # //go:build unix — syscall.Getrusage(RUSAGE_SELF, ...)
├── metrics_windows.go                 # //go:build windows — GetProcessTimes/GetProcessMemoryInfo
└── *_test.go                          # Unit tests (platform-tagged where needed)

pkg/pro/
├── dtos/exec.go                       # New — ExecUploadRequest (Data + DataItems + BatchID/BatchIndex/BatchTotal), ExecUploadResponse
├── api_client_exec.go                 # New — UploadExecMetadata: single request when it fits, else pro.sendChunked(dto.DataItems, ...) following UploadInstanceStatus/UploadAffectedStacks's pattern
├── api_client.go                      # Modified — AtmosProAPIClientInterface gains UploadExecMetadata
├── consumer_pact_test.go              # Modified — 9th (single-request) + 10th (chunked) interactions added (//go:build pact)
└── pact_helpers_test.go               # Modified if the new interactions need shared setup

pkg/schema/pro.go                      # Modified — ProSettings gains Exec ExecSettings { SyncTimeoutSeconds int }
pkg/config/load.go                     # Modified — viper binding for settings.pro.exec.sync_timeout_seconds (existing pattern)
pkg/config/const.go                    # Modified — ATMOS_PRO_EXEC_SYNC_TIMEOUT_SECONDS env var constant

cmd/root.go                            # Modified — add proexec.CaptureAsync(cmd, err) next to telemetry.CaptureCmd
internal/exec/terraform.go             # Modified — ExecuteTerraform calls proexec.CaptureSync(...) for plan/apply, mapping plugin.TerraformOutputData into Data (summary) + DataItems (resource changes)
internal/exec/describe_affected.go     # Modified (exact file TBD in tasks) — calls proexec.CaptureSync(...) with nil Data/DataItems

pacts/atmos-AtmosPro.json              # Regenerated — now 10 interactions
```

**Structure Decision**: Two new top-level `pkg/` packages (`proexec`, `metrics/process`),
consistent with the constitution's package-organization mandate (no growth of
`pkg/utils/`, purpose-built packages for new functionality). Existing `pkg/pro/` is
extended in place (new DTO file, new client-method file reusing the existing
`sendChunked` chunking helper, existing files touched only at the interface/registration
points) rather than forked, matching how `UploadAffectedStacks`/`UploadInstances` were
added historically. Call-site changes in `cmd/root.go` and `internal/exec/terraform.go`
are minimal, additive insertions at points that already exist for analogous purposes (the
telemetry hook; the plan/apply output-parsing step that already produces
`TerraformOutputData` for Native CI summaries).

**Superseded design note**: `pkg/proexec/truncate.go` and its `truncationMarker`/
`maxWarningsKept` truncate-in-place approach (from the pre-clarification design) is
removed from this plan; command-specific structured data is now split via chunking
instead of being trimmed or replaced with a marker. Any already-written code following
the old design (`pkg/proexec/truncate.go`, `pkg/pro/api_client_exec.go`'s "never
chunked" doc comment) needs to be reworked in the implementation phase to match this
plan.

## Complexity Tracking

> No Constitution Check violations — section intentionally empty.
