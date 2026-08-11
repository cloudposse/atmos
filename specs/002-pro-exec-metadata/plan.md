# Implementation Plan: Atmos Pro Command-Execution Metadata Upload

**Branch**: `1199-pro-exec-metadata` | **Date**: 2026-08-11 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/002-pro-exec-metadata/spec.md`

## Summary

Add a new `pkg/proexec` package that, whenever Atmos is running in a recognized CI
environment **and** Atmos Pro is configured, uploads a command-execution record to a new
Atmos Pro endpoint `POST /v1/atmos/exec`. Every command gets a fire-and-forget,
best-effort-flushed async upload hooked next to the existing `telemetry.CaptureCmd` call
in `cmd/root.go`. Three commands (`terraform plan`, `terraform apply`, `describe
affected`) additionally call a synchronous variant directly from their own execution
path, bounded by a configurable (default 10s) timeout, with a warn-and-continue failure
default. A new `pkg/metrics/process` package captures the `atmos` process's own
wall-time/CPU/memory/IO usage via a baseline-and-diff snapshot. `terraform plan`/`apply`
attach the already-computed `pkg/ci/internal/plugin.TerraformOutputData` as
command-specific structured data. The existing local-only Pact consumer contract suite in
`pkg/pro/` gains a 9th interaction for the new endpoint, so the generated
`pacts/atmos-AtmosPro.json` can be handed to the Atmos Pro team to implement the provider
side.

## Technical Context

**Language/Version**: Go 1.26

**Primary Dependencies**:
- `pkg/pro` (existing — `AtmosProAPIClient`, DTOs, retry/chunking helpers) — extended, not replaced
- `pkg/telemetry` (existing — `IsCI()`/`ciProvider()` reused verbatim)
- `pkg/git` (existing — `GitRepoInterface.GetLocalRepoInfo()`/`GetCurrentCommitSHA()`)
- `pkg/ci/internal/plugin` (existing — `TerraformOutputData` reused as structured data)
- `syscall` (stdlib, Unix `Getrusage`) / `golang.org/x/sys/windows` (new dependency, Windows process metrics)
- `github.com/pact-foundation/pact-go/v2` (existing dev dependency, from 001-pact-consumer-contracts)

**Storage**: N/A — no local persistence; all data is transmitted to the Atmos Pro API.
Pact contract JSON (`pacts/atmos-AtmosPro.json`) is the only new on-disk artifact, and it
is test-generated, not runtime state.

**Testing**: `atmos test` (unit, with mocks for `AtmosProAPIClientInterface` and a fake
CI/Pro-config environment); `go test -tags pact ./pkg/pro/...` for the new Pact
interaction (local-only, no CI job, per existing 001 convention).

**Target Platform**: Linux, macOS, Windows — `pkg/metrics/process` has a Unix
implementation (`syscall.Rusage`) and a reduced-fidelity Windows implementation
(wall time + process CPU times only).

**Project Type**: CLI feature — new internal packages (`pkg/proexec`,
`pkg/metrics/process`), plus targeted call-site changes in `cmd/root.go` and
`internal/exec/terraform.go` / the `describe affected` execution path. No new CLI
commands or flags.

**Performance Goals**: Async default path MUST add no more than a fixed 2-second
best-effort flush ceiling to any command's total run time (SC-004). Synchronous path MUST
add no more than the configured wait ceiling (default 10s) to `terraform plan`/`apply`/
`describe affected` (SC-003).

**Constraints**:
- No new user-facing opt-out; gate is `telemetry.IsCI() && Pro-configured` only (FR-001/FR-002).
- Sync/async classification and fail-vs-warn behavior are hardcoded per command in code, not configuration (spec Assumptions).
- Secret masking MUST apply to `Args` and `Data` before upload (FR-010).
- Payload MUST respect `Settings.Pro.MaxPayloadBytes` via truncation, not chunking (FR-011, research Decision 6).

**Scale/Scope**: 1 new Atmos Pro endpoint (1 new pact interaction, bringing the suite to
9), 2 new packages, ~3 call-site integrations (root hook + 2 sync commands sharing one
`ExecuteTerraform` pipeline + `describe affected`).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Registry-Driven Extensibility | ✅ Pass | No new CLI commands/flags. `pkg/proexec` is a plain package, not a registry-eligible extension point — command-specific data is passed explicitly per research Decision 5, deliberately avoiding a premature registry (see Alternatives Considered there). |
| II. Interface-Driven Design with DI | ✅ Pass | `AtmosProAPIClientInterface` gains `UploadExecMetadata` and is extended, not bypassed; `pkg/proexec`'s Pro-client dependency is injected the same way `internal/exec/pro.go` already does, enabling mock-based unit tests. |
| III. Test-First with 80% Coverage | ✅ Pass | New packages ship with table-driven unit tests (gate logic, envelope assembly, truncation, timeout clamping) plus the Pact contract test. Existing `atmos test` coverage gate applies normally (pact tests are opt-in via build tag, excluded from CodeCov per 001's precedent). |
| IV. Separated I/O and UI Architecture | ✅ Pass | All new user-visible signals (FR-009a debug-level log line) go through `pkg/logger`, matching existing `pkg/pro`/`pkg/telemetry` conventions; no direct `fmt.Print*`. |
| V. Simplicity and No Over-Engineering | ✅ Pass | No registry/interface introduced for the 2-consumer structured-data case (YAGNI, research Decision 5); files kept under 600 lines by splitting `pkg/proexec` into gate/envelope/async/sync files following the existing `pkg/pro/api_client_*.go` one-concern-per-file convention. |

**Post-design re-check**: ✅ Pass — Phase 1 design (data-model.md, contracts/) introduces
no new violations; the structured-data mechanism decision (deferred by the spec) resolved
in favor of the simplest option consistent with Principle V.

## Project Structure

### Documentation (this feature)

```text
specs/002-pro-exec-metadata/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md         # Phase 1 output
├── quickstart.md         # Phase 1 output
├── contracts/
│   └── interactions.md   # Phase 1 output — 9th Pact interaction, extends 001's contract doc
└── tasks.md               # Phase 2 output (/speckit-tasks)
```

### Source Code (repository root)

```text
pkg/proexec/                          # New package
├── gate.go                            # gateOpen(): telemetry.IsCI() && Pro-configured check
├── envelope.go                        # buildRecord(): assemble dtos.ExecUploadRequest (base + metrics + masked args/data)
├── async.go                           # CaptureAsync(cmd, err): fire-and-forget, 2s best-effort flush
├── sync.go                            # CaptureSync(info, exitCode, data, opts): blocking, configurable timeout, warn-and-continue
├── truncate.go                        # payload-size clamp against Settings.Pro.MaxPayloadBytes (FR-011)
└── *_test.go                          # Unit tests per file, table-driven, mocked AtmosProAPIClientInterface

pkg/metrics/process/                   # New package
├── metrics.go                         # ProcessMetrics struct, Snapshot, Baseline(), Since()
├── metrics_unix.go                    # //go:build unix — syscall.Getrusage(RUSAGE_SELF, ...)
├── metrics_windows.go                 # //go:build windows — GetProcessTimes/GetProcessMemoryInfo
└── *_test.go                          # Unit tests (platform-tagged where needed)

pkg/pro/
├── dtos/exec.go                       # New — ExecUploadRequest, ExecUploadResponse
├── api_client_exec.go                 # New — UploadExecMetadata, following UploadInstanceStatus's pattern
├── api_client.go                      # Modified — AtmosProAPIClientInterface gains UploadExecMetadata
├── consumer_pact_test.go              # Modified — 9th interaction added (//go:build pact)
└── pact_helpers_test.go               # Modified if the new interaction needs shared setup

pkg/schema/pro.go                      # Modified — ProSettings gains Exec ExecSettings { SyncTimeoutSeconds int }
pkg/config/load.go                     # Modified — viper binding for settings.pro.exec.sync_timeout_seconds (existing pattern)
pkg/config/const.go                    # Modified — ATMOS_PRO_EXEC_SYNC_TIMEOUT_SECONDS env var constant

cmd/root.go                            # Modified — add proexec.CaptureAsync(cmd, err) next to telemetry.CaptureCmd
internal/exec/terraform.go             # Modified — ExecuteTerraform calls proexec.CaptureSync(...) for plan/apply, passing plugin.TerraformOutputData
internal/exec/describe_affected.go     # Modified (exact file TBD in tasks) — calls proexec.CaptureSync(...) with nil Data

pacts/atmos-AtmosPro.json              # Regenerated — now 9 interactions
```

**Structure Decision**: Two new top-level `pkg/` packages (`proexec`, `metrics/process`),
consistent with the constitution's package-organization mandate (no growth of
`pkg/utils/`, purpose-built packages for new functionality). Existing `pkg/pro/` is
extended in place (new DTO file, new client-method file, existing files touched only at
the interface/registration points) rather than forked, matching how every other Pro
endpoint was added historically. Call-site changes in `cmd/root.go` and
`internal/exec/terraform.go` are minimal, additive insertions at points that already
exist for analogous purposes (the telemetry hook; the plan/apply output-parsing step that
already produces `TerraformOutputData` for Native CI summaries).

## Complexity Tracking

> No Constitution Check violations — section intentionally empty.
