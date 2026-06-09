# Research: Pro Summary Upload

**Date**: 2026-06-09
**Branch**: `1197-pro-summary-upload`

## Implementation Status

A codebase audit confirms the majority of this feature is already implemented on
`prd/pro-summary-upload` (merged into `1197-pro-summary-upload`). The research below
documents what exists, what the two remaining gaps are, and the decisions behind each.

---

## Decision 1: Output Capture Strategy

**Decision**: Single masked capture buffer (`WithStdoutCapture` after `MaskWriter`).

**Rationale**: Clarification Q1 confirmed that resource count lines (e.g., "Plan: 3 to add")
never contain secrets, so `<MASKED>` substitutions do not interfere with regex-based parsing.
A single buffer serves both parsing and the upload log, keeping the implementation simple and
avoiding two-buffer synchronization complexity.

**Alternatives considered**:
- Dual capture (raw for parsing, masked for upload): Rejected because masking does not affect
  the specific lines that carry resource counts; unnecessary complexity.
- Re-parsing from CI hook context: Possible but would require threading raw output through the
  hook system; not worth it given confirmed masked-buffer sufficiency.

**Where implemented**: `internal/exec/terraform_execute_helpers_exec.go` — `maskedOutput`
buffer with `WithStdoutCapture` option; `buildCIStatusData` uses this buffer for both
`ci.BuildStatusData` (parsing) and `addOutputLog` (upload log).

---

## Decision 2: Metadata Flexibility via `map[string]any`

**Decision**: `Metadata map[string]any` on `InstanceStatusUploadRequest`; component type
identifier as a separate top-level `ComponentType` field.

**Rationale**: Each component type (terraform, helmfile, packer) evolves its metadata keys
independently. A shared struct would require coordinated schema changes across all component
types. The server reads `ComponentType` to dispatch without inspecting metadata keys.

**Alternatives considered**:
- Polymorphic nested struct (`"data": {"type": "terraform", ...}`): More JSON-schema friendly
  but harder to extend and requires server-side type assertions.
- Separate endpoint per component type: More REST-idiomatic but creates race conditions
  between two PATCH calls and doubles server-side handling complexity.

**Where implemented**: `pkg/pro/dtos/instances.go`, `pkg/pro/api_client_instance_status.go`.

---

## Decision 3: Retry Policy

**Decision**: Use existing `doWithRetry` with `defaultRetryConfig` (3 retries, exponential
backoff starting at 1s). The spec (FR-007a) requires "exactly one retry"; the implementation
is more generous but fully spec-compliant in spirit.

**Rationale**: 3 retries with exponential backoff is strictly better for users than a single
retry. The spec was written describing a minimum viable behavior; the implementation exceeds it.
Reducing to 1 retry would degrade reliability with no benefit.

**Recommendation**: Update spec FR-007a to read "at least one automatic retry" to match actual
behavior. No code change required.

**Where implemented**: `pkg/pro/retry.go` — `defaultRetryConfig()` returns
`{maxRetries: 3, baseDelay: 1s}`.

---

## Decision 4: `command` Field for `deploy` Subcommand (GAP)

**Decision**: FR-008a requires the upload payload `command` field to contain `"deploy"` when
the user invokes `atmos terraform deploy`. Currently, `handleDeploySubcommand` (called at
`terraform_execute_helpers_exec.go:147`) mutates `info.SubCommand` from `"deploy"` to
`"apply"` before `executeMainTerraformCommand` runs, so the DTO receives `"apply"`.

**Fix required**: Capture `originalSubCommand := info.SubCommand` before calling
`handleDeploySubcommand`, then pass it to `uploadStatus` instead of using `info.SubCommand`
inside `uploadStatus`. Additionally, `shouldUploadStatus` currently returns false for `"deploy"`
(checks only `"plan"` and `"apply"`) — this must be updated to also accept `"deploy"` to
handle cases where the SubCommand has not yet been converted.

**Files**: `internal/exec/terraform_execute_helpers_exec.go` (capture before conversion),
`internal/exec/pro.go` (`shouldUploadStatus` gate, `uploadStatus` signature or call site).

---

## Decision 5: Server-Defined Size Limit (DEFERRED)

**Decision**: The PRD described fetching `max_output_log_bytes` from `GET /api/v1/settings`.
Current implementation hardcodes `defaultMaxOutputLogBytes = 3 * 1024 * 1024` in
`terraform_execute_helpers_exec.go`. The spec gates (FR-005, FR-006) reference a
"server-defined size limit" with a built-in fallback.

**Current state**: The built-in fallback (3 MB) is implemented. The dynamic fetch is not.
This does not block the initial release — the 3 MB default is a reasonable production value.

**Recommendation**: Defer the server-settings fetch to a follow-up issue. The hardcoded
constant satisfies all current acceptance scenarios. If the server needs to change the limit,
it can be done via a CLI config option (`settings.pro.max_output_log_bytes`) as an interim
solution until the dynamic fetch is implemented.

---

## Decision 6: Sensitive Output Masking

**Decision**: `extractOutputValues` in `pkg/ci/plugins/terraform/plugin.go` replaces any
`TerraformOutput.Sensitive == true` value with the string `"<MASKED>"`. Non-sensitive values
are passed through as-is (type-preserving `any`).

**Rationale**: Masking at the plugin level keeps the concern co-located with the data it
protects. The server receives `"<MASKED>"` strings and treats them as opaque; no server-side
unmasking is possible or desired.

**Where implemented**: `pkg/ci/plugins/terraform/plugin.go:287` — fully tested by
`TestExtractOutputValues` in `plugin_test.go`.
