# Implementation Plan: Pro Summary Upload

**Branch**: `1197-pro-summary-upload` | **Date**: 2026-06-09 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/001-pro-summary-upload/spec.md`

## Summary

Enrich the existing `atmos terraform plan/apply --upload-status` upload path with structured CI
metadata: resource counts, error/warning status, masked output log, and component type. The CI
plugin system already provides the abstraction (`StatusDataProvider`), and the DTO, capture
wiring, and registry dispatch are **already implemented** on this branch. Two gaps remain:
preserving the literal subcommand for `deploy` invocations (FR-008a) and fetching the
server-defined output log size limit (FR-005/FR-006).

## Technical Context

**Language/Version**: Go 1.26.3

**Primary Dependencies**: cobra, viper, go.uber.org/mock/mockgen, testify/require, testify/assert

**Storage**: N/A — uploads via HTTPS PATCH to Atmos Pro API

**Testing**: `go test` with table-driven unit tests; mockgen-generated mocks for API client
and CI plugin interfaces; `make test-short` for fast iteration

**Target Platform**: Linux / macOS / Windows CLI (cross-platform, no cgo)

**Project Type**: CLI feature extension — no new packages, extends existing `pkg/ci/`,
`pkg/pro/`, and `internal/exec/`

**Performance Goals**: Upload completes within 5 seconds of command exit (SC-001)

**Constraints**: Output log ≤ 3 MB pre-encoding (default); zero sensitive value leakage (SC-002)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

| Principle | Status | Notes |
|---|---|---|
| I. Registry-Driven Architecture | ✅ Pass | CI plugins register via `ci.RegisterPlugin`; `StatusDataProvider` is an optional interface checked at runtime — no ad-hoc wiring |
| II. Interface-First, Mock-Based Testing | ✅ Pass | `StatusDataProvider` and `Plugin` interfaces defined first; `//go:generate mockgen` directive present; mocks generated |
| III. Separation of I/O and UI | ✅ Pass | Upload uses `log.Debug`/`log.Warn` (logger, not UI); data flows through `data.*` channels |
| IV. Complexity Budget | ✅ Pass | `buildCIStatusData` (12 lines), `addOutputLog` (14 lines), `buildMetadataForUpload` (6 lines) all within budget |
| V. Cross-Platform & Error Contract | ✅ Pass | Static sentinels (`errUtils.ErrFailedToUploadInstanceStatus`, etc.); `filepath.Join` patterns; no forbidden viper calls |

No violations. No Complexity Tracking entries required.

## Project Structure

### Documentation (this feature)

```text
specs/001-pro-summary-upload/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── instance-status-patch.md   # Phase 1 output
└── tasks.md             # Phase 2 output (/speckit-tasks)
```

### Source Code (files touched by this feature)

```text
internal/exec/
├── terraform_execute_helpers_exec.go  # capture wiring, buildCIStatusData, addOutputLog
├── terraform_execute_helpers.go       # handleDeploySubcommand (gap: save original SubCommand)
└── pro.go                             # uploadStatus, shouldUploadStatus (gap: deploy gate)

pkg/ci/
├── internal/plugin/types.go           # StatusDataProvider interface (complete)
├── plugins/terraform/plugin.go        # BuildStatusData, extractOutputValues (complete)
├── plugins/terraform/plugin_test.go   # unit tests (complete)
└── plugin_registry.go                 # BuildStatusData dispatcher (complete)

pkg/pro/
├── dtos/instances.go                  # InstanceStatusUploadRequest DTO (complete)
├── api_client_instance_status.go      # UploadInstanceStatus + retry (complete)
└── retry.go                           # defaultRetryConfig (3 retries — exceeds FR-007a spec of 1)
```

**Structure Decision**: Single project, existing packages only. No new packages required.
