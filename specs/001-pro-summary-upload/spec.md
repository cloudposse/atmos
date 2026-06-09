# Feature Specification: Pro Summary Upload

**Feature Branch**: `1197-pro-summary-upload`

**Created**: 2026-06-09

**Status**: Draft

**Input**: User description: "Create spec from file docs/prd/pro-summary-upload.md"

## Clarifications

### Session 2026-06-09

- Q: Can the resource count parser run against the masked output buffer and still produce accurate results, or does it need access to the original unmasked output separately? → A: Masked output is sufficient — resource counts appear on lines that never contain secrets, so `<MASKED>` substitutions never interfere with parsing.
- Q: Should the `command` field in the upload payload contain `"deploy"` (as typed) or `"apply"` (after internal conversion) when the user runs `atmos terraform deploy`? → A: `"deploy"` — preserve what the user typed for audit trail fidelity; the server receives the literal invocation.
- Q: When the server size-limit settings endpoint is unreachable and the built-in default is applied, should the user see any indication? → A: Debug log only — emit a single debug-level message; not shown in normal CI output.
- Q: Should the upload retry on transient failures before emitting a warning and proceeding? → A: Single retry — one automatic retry after a short fixed delay before warning.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - View Plan Summary in Atmos Pro Dashboard (Priority: P1)

A platform engineer runs `atmos terraform plan` with `--upload-status` in a CI pipeline. Instead
of having to leave the Atmos Pro dashboard and open GitHub Actions or GitLab CI to see what
changed, the dashboard shows the plan summary directly: how many resources will be created,
changed, replaced, or destroyed, plus any warnings and error messages.

**Why this priority**: This is the core value proposition. Without it, users must context-switch
between tools to understand every plan result, which slows down approvals and incident triage.

**Independent Test**: Run `atmos terraform plan --upload-status` against a stack with pending
changes. The Atmos Pro dashboard must display resource counts and the masked command output
without the user opening the CI platform.

**Acceptance Scenarios**:

1. **Given** a terraform plan produces changes, **When** `--upload-status` is set and
   `ci.enabled` is true, **Then** the Atmos Pro dashboard shows the number of resources to
   create, change, replace, and destroy along with any warnings.
2. **Given** a terraform plan produces no changes, **When** `--upload-status` is set and
   `ci.enabled` is true, **Then** the dashboard shows "no changes" with zero resource counts.
3. **Given** `ci.enabled` is false or `--upload-status` is absent, **When** a plan runs,
   **Then** the upload payload is identical to the current behavior (no metadata, no
   component_type) — fully backward compatible.

---

### User Story 2 - View Apply Result and Terraform Outputs in Dashboard (Priority: P2)

After a successful apply, the platform engineer can see the apply outcome and any Terraform
output values (e.g., VPC IDs, subnet lists) directly in the Atmos Pro dashboard, with sensitive
output values masked.

**Why this priority**: Apply results are the second most common approval/audit event. Terraform
outputs are frequently needed for downstream decisions; surfacing them in the dashboard eliminates
the need to grep CI logs.

**Independent Test**: Run `atmos terraform apply --upload-status`. Confirm the dashboard shows
the apply summary and that any sensitive outputs display as `<MASKED>` while non-sensitive values
are visible.

**Acceptance Scenarios**:

1. **Given** an apply completes successfully, **When** the upload runs, **Then** the dashboard
   shows the apply outcome and any Terraform output values.
2. **Given** a Terraform output is marked sensitive, **When** it is uploaded, **Then** the
   dashboard displays `<MASKED>` instead of the actual value.
3. **Given** an apply fails, **When** the upload runs, **Then** `has_errors: true` and the error
   messages are visible in the dashboard.

---

### User Story 3 - Upload Full Masked Command Output Log (Priority: P2)

The full stdout from the terraform command — already stripped of secrets — is available in the
Atmos Pro dashboard for debugging. If the log is too large, the most recent portion (tail) is
kept and the dashboard indicates the log was truncated.

**Why this priority**: Resource counts and error lists are helpful summaries, but engineers
frequently need the raw output to diagnose failures. Having it in the dashboard eliminates the
need to visit the CI platform for every debug session.

**Independent Test**: Run a terraform plan that produces verbose output. Confirm the full masked
log appears in the dashboard. Then simulate a very large output to confirm the tail is kept,
the beginning is dropped, and a truncation indicator is visible.

**Acceptance Scenarios**:

1. **Given** the command output is within size limits, **When** the upload runs, **Then** the
   full masked output log is available in the dashboard.
2. **Given** the command output exceeds the server-defined size limit, **When** the upload runs,
   **Then** the log is truncated from the beginning (tail preserved), a truncation indicator is
   set, and the upload still succeeds.
3. **Given** output capture fails, **When** the upload runs, **Then** the log field is omitted
   but the rest of the upload (resource counts, errors) still succeeds.

---

### User Story 4 - Other Component Types Are Not Broken (Priority: P1)

Users of helmfile, packer, or any future component type continue to see the same upload behavior
they have today. The new metadata fields are absent from their payloads; the server receives
identical payloads to the current CLI version.

**Why this priority**: Backward compatibility is non-negotiable. Breaking the upload contract for
non-terraform users would require a coordinated CLI + server release.

**Independent Test**: Run `atmos helmfile sync --upload-status` (or equivalent). Confirm the
upload payload contains no `component_type` or `metadata` fields.

**Acceptance Scenarios**:

1. **Given** a non-terraform component type, **When** `--upload-status` is set, **Then** the
   upload payload has no `component_type` or `metadata` fields.
2. **Given** a terraform component type where the CI plugin does not implement the summary
   interface, **When** the upload runs, **Then** the upload still succeeds with just the existing
   fields.

---

### Edge Cases

- What happens when the Atmos Pro server is unreachable during the size-limit fetch? The CLI
  falls back to a built-in default limit (3 MB pre-encoding) so the upload still proceeds. A
  debug-level log message is emitted; no warning is shown in normal CI output.
- Can a single masked output buffer serve both parsing and upload? Yes — resource count lines
  (e.g., "Plan: 3 to add") never contain secrets, so masking does not affect extraction accuracy.
- What happens when metadata construction fails (e.g., output parsing error)? The upload
  proceeds with only the existing `command` + `exit_code` fields; metadata is omitted.
- What happens if the upload itself fails? The CLI performs one automatic retry after a short
  fixed delay. If the retry also fails, the error is logged as a warning and does not cause
  the terraform command to fail.
- What happens if the `deploy` subcommand is used? Metadata is populated identically to `apply`
  (same resource counts, outputs). However, the `command` field in the payload MUST be `"deploy"`
  (not `"apply"`) to preserve the audit trail of what the user invoked.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: When `--upload-status` is set, `ci.enabled` is true, and the component type is
  terraform, the upload payload MUST include structured plan/apply metadata (resource counts,
  error status, warnings, error messages, masked output log).
- **FR-002**: When any gating condition is false (`--upload-status` absent, `ci.enabled` false,
  component type has no metadata support), the upload payload MUST be identical to the current
  payload — no new fields added.
- **FR-003**: Sensitive Terraform output values MUST be replaced with `<MASKED>` before upload.
- **FR-004**: The command stdout MUST be masked (secrets redacted) before being included in
  the upload payload. A single masked capture buffer MUST serve both output parsing (resource
  counts, errors) and the upload log — no separate unmasked capture is required, because
  resource count lines never contain secrets and are unaffected by masking.
- **FR-005**: If the masked output exceeds the server-defined size limit, the CLI MUST truncate
  from the beginning (keep the tail) and include a truncation indicator in the metadata.
- **FR-006**: If the server size-limit endpoint is unreachable, the CLI MUST fall back to a
  built-in default limit and continue the upload. The fallback MUST be recorded at debug log
  level only — no user-visible warning is emitted, so normal CI output is not polluted.
- **FR-007**: Metadata construction failures MUST be non-fatal — the upload proceeds with just
  the existing fields; the terraform command exit code is unaffected.
- **FR-008**: The upload endpoint MUST remain the existing PATCH endpoint; no new endpoints are
  introduced.
- **FR-007a**: On a transient upload failure (network error, HTTP 5xx, timeout), the CLI MUST
  perform at least one automatic retry before treating the upload as failed. After retries are
  exhausted, the failure MUST be warn-only and MUST NOT affect the terraform command exit code.
  (Current implementation uses 3 retries with exponential backoff — this exceeds the minimum
  and is the intended behavior.)
- **FR-008a**: The `command` field in the upload payload MUST contain the literal subcommand the
  user invoked (e.g., `"deploy"` when the user ran `atmos terraform deploy`), not the normalized
  internal form, so the server audit trail reflects actual user intent.
- **FR-009**: The component type identifier MUST be a top-level field on the upload payload
  (not embedded in metadata) so the server can dispatch without inspecting the metadata map.
- **FR-010**: The feature MUST be extensible: future component types (helmfile, packer) MUST be
  able to supply their own metadata keys via the same interface without changes to the upload
  wiring.

### Key Entities

- **InstanceStatusUploadRequest**: The existing upload payload DTO, extended with two optional
  fields: `component_type` (string) and `metadata` (flexible key-value map). Both are omitted
  when empty/nil.
- **StatusDataProvider**: A new interface that CI plugins implement to produce upload-ready
  metadata. Terraform is the first implementer; other component types opt in independently.
- **OutputResult**: The parsed result of a terraform command — whether changes were detected,
  whether errors occurred, resource counts, warnings, errors, and output values.
- **MaskedOutputLog**: The full command stdout after secret redaction, base64-encoded for safe
  transport, subject to size truncation.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: After running `atmos terraform plan --upload-status` in a CI pipeline, the Atmos
  Pro dashboard shows plan resource counts within 5 seconds of the command completing.
- **SC-002**: 100% of Terraform output values marked sensitive display as `<MASKED>` in the
  dashboard — zero leakage of sensitive values.
- **SC-003**: Upload payloads for non-terraform component types remain byte-for-byte identical
  to the current CLI version (verified by integration test).
- **SC-004**: When metadata construction fails, the overall terraform command exit code is
  unchanged — 0% increase in false failures.
- **SC-005**: Output logs exceeding the size limit are truncated gracefully with no upload
  failure — 100% upload success rate regardless of output size.

## Assumptions

- The Atmos Pro server will be updated to accept the extended payload fields before this CLI
  change ships; the server handles payloads with or without the new fields for backward
  compatibility.
- The `ci.enabled` configuration flag is already present in atmos.yaml; no new configuration
  keys are needed to enable this feature.
- `atmos deploy` is internally converted to `apply` for execution purposes, but the upload
  payload MUST preserve `"deploy"` in the `command` field so the server audit trail reflects
  what the user actually invoked.
- The `--upload-status` flag already exists; this feature adds metadata to the existing upload,
  not a new flag.
- The server-side settings endpoint (`GET /api/v1/settings`) returning `max_output_log_bytes`
  will be available when this feature ships; the CLI has a built-in fallback for unreachable
  endpoints.
- Helmfile and packer plugins will supply their own metadata implementations in future PRs;
  this feature only ships the terraform implementation.
