# Feature Specification: Atmos Pro Command-Execution Metadata Upload

**Feature Branch**: `1199-pro-exec-metadata`

**Created**: 2026-08-11

**Status**: Draft

**Input**: User description: "Add Atmos Pro command-execution metadata upload. When Atmos detects it is running in CI AND Atmos Pro is configured, every atmos command execution sends metadata to a new Atmos Pro API endpoint POST /v1/atmos/exec. Payload includes a base envelope (version, OS, arch, command, args, exit code, git info), resource usage metrics (wall time, CPU time, peak memory, page faults, context switches, block I/O), and per-command additional structured JSON data (e.g. terraform plan/apply attaches created/updated/deleted/replaced resources, outputs, warnings/errors, and per-component logs). Delivery is synchronous and blocking for a hardcoded allowlist of commands (terraform plan/apply, describe affected) and asynchronous/fire-and-forget for all other commands, with both the sync/async choice and the strict/best-effort failure behavior decided per-command in code, not via user-facing settings. Testing requirement: extend the existing local-only Pact consumer contract test suite with a 9th interaction for POST /v1/atmos/exec covering the full payload shape, so the generated contract can be handed to the Atmos Pro team to implement the provider side."

## Clarifications

### Session 2026-08-11

- Q: For asynchronous (fire-and-forget) commands, should the CLI process wait briefly to ensure the upload has at least been dispatched before exiting, or exit immediately with no delivery guarantee? → A: Short bounded wait — best-effort flush with a small timeout before exiting, to maximize delivery without materially slowing the command.
- Q: For synchronous commands (`terraform plan`, `terraform apply`, `describe affected`), how long should Atmos wait for the execution-record upload to be confirmed before treating delivery as failed? → A: A moderate default ceiling (~10 seconds), consistent with typical HTTP client timeout conventions, and this default MUST be user-configurable (increasable) via Atmos configuration.
- Q: Should a failed asynchronous execution-metadata upload ever be visible to the user, or remain completely silent by default? → A: Silent by default; only surfaced at debug/verbose log level, consistent with existing telemetry failure handling.
- Q: Should each execution record carry a unique identifier for correlation (CI run, dedup, related records)? → A: Yes — reuse the existing Atmos Pro run identifier already used by other Pro uploads, rather than inventing a new ID scheme.
- Q: When an execution record's command-specific structured data is too large for a single request, what should be chunked/batched across multiple correlated requests? → A: Only the command-specific structured-data field is split across multiple correlated requests, reusing the existing chunked-upload/batch-correlation mechanism already used for `describe affected` uploads; the base envelope and resource-usage metrics are small and are always sent in full, never truncated or dropped.
- Q: For synchronous commands, does the ~10s upload-confirmation wait (FR-008a) apply per chunk or to the whole record? → A: To the whole record (all chunks combined), ~10s total, not per chunk.

### Session 2026-08-18

- Q: For a command on the synchronous allowlist (`terraform plan`, `terraform apply`, `describe affected`), should the async default-path upload (FR-002/FR-009) still also fire, or should sync delivery replace it for that invocation? → A: Mutually exclusive — sync-allowlisted commands are exempted from the async default path entirely; each qualifying invocation produces exactly one execution record, delivered via its command's classified path (sync or async), never both.
- Q: For a multi-component `atmos terraform plan --affected`/`--all` run (many components in one CLI invocation), should each per-component graph node produce its own execution record, or should the invocation produce one aggregate record? → A: Aggregate — one execution record per CLI invocation; per-component results (including each component's identity, outcome, and structured data) are folded into that single record's structured data, not sent as separate records.
- Q: Should `atmos terraform deploy` (which already has the same CI-mode stdout-capture wiring as `plan`/`apply`) join the synchronous execution-record allowlist, or stay outside it? → A: Add `deploy` to the synchronous allowlist alongside `plan`/`apply`/`describe affected`.
- Q: Since `terraform deploy` typically wraps plan+apply internally, should its execution record carry deploy's own structured infrastructure-change data (FR-006), or just the base envelope like `describe affected`? → A: `deploy` gets structured infrastructure-change data too, same as `plan`/`apply` — FR-006 extended to include it.
- Q: For a `terraform plan`/`apply` invocation where the pre-existing `uploadStatus`/`--upload-status` PATCH mechanism (`internal/exec/pro.go`, `PATCH .../instances`) also fires — independently of this feature's `POST /v1/atmos/exec` — should the client coordinate/merge the two paths, or remain fully independent? → A: Remain independent — the Atmos Pro backend owns any cross-record correlation/dedup between the two record types; the Atmos client MUST NOT merge, skip, or make the two paths mutually exclusive. The one client-side guarantee is that the `Command` and `Args` values reported by the new execution record MUST be correlatable with what `uploadStatus` reports for the same invocation, so Atmos Pro can correlate the two records by content even without a shared record ID.
- Q: `InstanceStatusUploadRequest.Command` is a bare subcommand (`"plan"`) and has separate `Component`/`Stack` fields with no `Args` counterpart, while `ExecUploadRequest.Command` was `cmd.CommandPath()` (`"atmos terraform plan"`) with a separate, currently-always-empty `Args` field — what concrete `Command`/`Args` shape should `ExecUploadRequest` use so the two paths are correlatable? → A: `ExecUploadRequest.Command` MUST be the subcommand path without the `atmos` root (e.g. `"terraform plan"`, not `"atmos terraform plan"`). `ExecUploadRequest.Args` MUST hold only the positional arguments (e.g. the component identifier: `["cdn"]`), replacing the current always-empty `Args`. A new, separate `ExecUploadRequest.Flags` field MUST hold the CLI flags actually passed (e.g. `["-s", "plat-use2-dev", "--upload-status"]`), each passed through the existing secret-masking (FR-010) — positional args and flags are kept in distinct fields, not combined into one array.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Automatic visibility into CI command execution (Priority: P1)

As an organization running Atmos in CI with Atmos Pro configured, I want every `atmos` command invocation to automatically report its execution outcome (what ran, exit code, environment, resource consumption) to Atmos Pro, so that our team has a complete, automatic audit trail of infrastructure operations without any extra CI configuration.

**Why this priority**: This is the foundational capability — without it, no execution data reaches Atmos Pro at all. It delivers value the moment it ships: visibility into CI activity that previously required manually parsing CI logs.

**Independent Test**: Run any `atmos` command (e.g. `atmos version`, `atmos describe stacks`) in a CI environment with Atmos Pro configured, and confirm a corresponding execution record appears on the Atmos Pro side with correct version, command, exit code, and resource-usage fields. Can be fully tested without any per-command structured data or synchronous delivery.

**Acceptance Scenarios**:

1. **Given** Atmos is running inside a recognized CI provider and Atmos Pro is configured with valid credentials, **When** any `atmos` command completes (success or failure), **Then** an execution record containing the command's identity, exit code, environment info, and resource-usage metrics is sent to Atmos Pro.
2. **Given** Atmos is running on a developer's local machine (not CI), **When** any `atmos` command completes, **Then** no execution record is sent to Atmos Pro.
3. **Given** Atmos is running in CI but Atmos Pro is not configured, **When** any `atmos` command completes, **Then** no execution record is sent to Atmos Pro.
4. **Given** Atmos Pro is unreachable or returns an error, **When** a command whose upload is fire-and-forget completes, **Then** the command's own exit code and output are unaffected by the delivery failure.

---

### User Story 2 - Reliable reporting for critical operations (Priority: P2)

As an organization relying on Atmos Pro to track infrastructure changes, I want the outcome of critical operations (`terraform plan`, `terraform apply`, `terraform deploy`, `describe affected`) to be confirmed as recorded by Atmos Pro before the command finishes, so that a CI pipeline never reports success for an infrastructure change that Atmos Pro failed to record.

**Why this priority**: Builds directly on User Story 1's delivery mechanism; only meaningful once basic reporting exists. It matters most for the highest-stakes commands, which is why it is scoped to a small, well-defined set rather than all commands.

**Independent Test**: Run `atmos terraform plan` (or `apply`, `deploy`, or `describe affected`) in CI with Atmos Pro configured, and verify the command does not exit until the execution record upload has completed (or has been explicitly handled as a failure per that command's configured behavior).

**Acceptance Scenarios**:

1. **Given** `atmos terraform plan` runs in CI with Atmos Pro configured and reachable, **When** the plan completes, **Then** the command does not exit until its execution record has been accepted by Atmos Pro.
2. **Given** `atmos terraform apply` runs in CI with Atmos Pro configured but unreachable, **When** the apply completes, **Then** the command follows its own configured failure behavior (either fails with a clear error, or proceeds with a warning) — never silently proceeds without one of the two.
3. **Given** a command outside the critical allowlist (e.g. `atmos validate stacks`) runs in CI with Atmos Pro configured, **When** the command completes, **Then** it exits immediately without waiting for the execution record upload to finish.
4. **Given** `atmos terraform plan` runs in CI with Atmos Pro configured, **When** the command completes, **Then** exactly one execution record is produced for that invocation — delivered via the synchronous path — and no separate asynchronous execution record is also sent for the same invocation.

---

### User Story 3 - Structured infrastructure-change data for plan/apply/deploy (Priority: P3)

As an organization reviewing infrastructure changes made through CI, I want `terraform plan`/`apply`/`deploy` execution records to include the specific resources created, updated, deleted, and the resulting outputs, so that I can see what changed in Atmos Pro without re-reading raw CI logs.

**Why this priority**: Adds significant analytical value but depends on User Stories 1 and 2 already being in place; the base execution record is useful on its own, and this enriches it further for the commands that produce the richest data.

**Independent Test**: Run `atmos terraform plan` against a component with pending changes, and verify the execution record sent to Atmos Pro includes the counts and identities of resources to be created/updated/deleted/replaced, along with any output values and warnings/errors, not just the pass/fail status.

**Acceptance Scenarios**:

1. **Given** `atmos terraform plan` detects resources to be created, updated, and deleted, **When** the execution record is sent, **Then** it includes itemized lists (or counts) of created, updated, deleted, and replaced resources.
2. **Given** `atmos terraform apply` produces new or changed output values, **When** the execution record is sent, **Then** those output values are included in the record.
3. **Given** a `terraform plan` run produces warnings or errors, **When** the execution record is sent, **Then** those warnings/errors are included alongside the plan's per-component status.
4. **Given** a command with no defined structured-data extension (e.g. `atmos list components`), **When** its execution record is sent, **Then** the structured-data portion is simply absent, and the base envelope is still sent normally.

---

### Edge Cases

- What happens when the CI environment is detected but Atmos Pro authentication has expired or is rejected (401)? The existing Atmos Pro client's token-refresh/retry behavior applies; if it still cannot authenticate, the upload is treated as a failed delivery per that command's configured behavior (fail vs. warn-and-continue).
- What happens on an operating system where fine-grained resource metrics (e.g. peak memory, page faults) are unavailable (e.g. Windows)? The execution record is still sent with the fields that are available (e.g. wall time), and unavailable fields are omitted rather than blocking delivery.
- What happens when the additional structured JSON data for a command is very large (e.g. a plan touching thousands of resources)? The command-specific structured-data field is split across multiple correlated requests using the existing chunked-upload/batch-correlation mechanism already used for `describe affected` uploads (base envelope and resource-usage metrics are small and always sent in full); the complete data set is delivered without truncation or loss.
- What happens if a command's arguments or structured data would otherwise contain secret values? Consistent with Atmos's existing secret-masking behavior, sensitive values must never be included in the uploaded payload.
- What happens when a command is interrupted (e.g. Ctrl-C) mid-execution? Best-effort delivery of a partial/aborted execution record is acceptable; a synchronous command being interrupted must not hang indefinitely waiting on delivery.
- What happens when the same CI job runs many `atmos` commands in sequence (e.g. a workflow)? Each command execution produces its own independent execution record.
- What happens when a single `atmos terraform plan`/`apply` CLI invocation targets multiple components in one run (`--affected`/`--all`)? The invocation still produces exactly one execution record overall, not one per component; each component's identity, outcome, and any structured data (FR-006) are folded into that single record's command-specific structured-data field.
- What happens when both this feature's execution-record upload and the pre-existing `uploadStatus`/`--upload-status` PATCH mechanism (a separate, independently-gated upload path used for lightweight instance-status tracking) fire for the same `terraform plan`/`apply` invocation? Both proceed independently — the Atmos client does not merge, skip, or coordinate between them, and any cross-record correlation or deduplication of the two resulting records is the Atmos Pro backend's responsibility. The one client-side guarantee is that this feature's execution record reports its `Command` as the bare subcommand path (e.g. `"terraform plan"`), its `Args` as the invocation's positional arguments (e.g. the component), and its `Flags` as the CLI flags actually passed (FR-003b), keeping it correlatable in content with `uploadStatus`'s `Command`/`Component`/`Stack` fields for that same invocation, even without a shared record ID.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST determine whether the current invocation qualifies for execution-metadata upload by checking that both (a) Atmos is running inside a recognized CI environment, and (b) Atmos Pro is configured with usable credentials.
- **FR-002**: System MUST NOT send any execution metadata when either condition in FR-001 is not met, and this behavior MUST require no additional user configuration beyond the existing CI and Atmos Pro setup.
- **FR-003**: For every qualifying command execution, system MUST report a base execution record containing: Atmos version, operating system, architecture, the full command invoked, any command-specific additional arguments, the command's exit code, source-control identification (e.g. commit/repository info), and the existing Atmos Pro run identifier, consistent with what other Atmos Pro uploads already capture.
- **FR-003a**: This feature's execution-record upload (`POST /v1/atmos/exec`) and the pre-existing `uploadStatus`/`--upload-status` PATCH mechanism (`internal/exec/pro.go`) are separate, independently-gated upload paths. System MUST NOT merge them, skip either one because the other fired, or otherwise make them mutually exclusive; any cross-record correlation or deduplication between the two record types on the Atmos Pro side is out of scope for this feature.
- **FR-003b**: The execution record's `Command` field MUST be the subcommand path without the leading `atmos` root segment (e.g. `"terraform plan"`, not `"atmos terraform plan"`). The `Args` field MUST hold only the invocation's positional arguments (e.g. the component identifier: `["cdn"]`); it MUST NOT be left empty for an invocation that received positional arguments. A separate `Flags` field MUST hold the CLI flags actually passed (e.g. `["-s", "plat-use2-dev", "--upload-status"]`), each passed through the existing secret-masking (FR-010) — positional args and flags MUST be kept in distinct fields, not combined into one array. This shape keeps `Command`/`Args`/`Flags` correlatable in practice with `uploadStatus`'s bare-subcommand `Command` and its separate `Component`/`Stack` fields for the same invocation, even though the two upload paths remain fully independent per FR-003a.
- **FR-004**: System MUST report resource-usage metrics for the command's execution, including at minimum wall-clock duration and CPU time, with additional metrics (peak memory, page faults, context switches, block I/O) included whenever the host platform makes them available.
- **FR-005**: System MUST support commands attaching additional, command-specific structured data to their execution record; commands that do not define such data MUST still have their base execution record and resource-usage metrics sent normally.
- **FR-006**: System MUST attach structured infrastructure-change data (created/updated/deleted/replaced resources, output values, warnings/errors) to the execution record for `terraform plan`, `terraform apply`, and `terraform deploy`.
- **FR-006a**: When a single `terraform plan`/`apply` CLI invocation targets multiple components in one run (e.g. `--affected`/`--all`), System MUST report exactly one execution record for the whole invocation, not one per component; each component's identity, outcome, and structured data (FR-006) MUST be folded into that single record's command-specific structured-data field rather than sent as separate, independent execution records.
- **FR-007**: System MUST treat `terraform plan`, `terraform apply`, `terraform deploy`, and `describe affected` as commands whose execution-record delivery blocks command completion (synchronous), and MUST treat all other commands as fire-and-forget (asynchronous), matching each command's fixed, code-defined classification. These two delivery paths are mutually exclusive per invocation: a command on the synchronous allowlist MUST NOT also receive an asynchronous default-path upload for the same invocation — every qualifying command execution produces exactly one execution record, never two.
- **FR-008**: For each command classified as synchronous, the command's own implementation MUST define whether a delivery failure causes the command itself to fail, or is treated as a non-fatal warning that still allows the command to complete; system MUST guarantee one of these two outcomes occurs (never a silent, indefinite hang).
- **FR-008a**: System MUST bound how long a synchronous command waits for its complete execution-record upload (including all chunks, if the structured data required batching) to be confirmed, defaulting to approximately 10 seconds total, and this wait duration MUST be configurable by the user to a longer value.
- **FR-009**: For commands classified as asynchronous, a delivery failure MUST NOT alter the command's own exit code or block its completion. The process MUST perform a short, bounded best-effort wait to maximize the chance the upload is dispatched before exiting, rather than exiting with no delivery guarantee at all.
- **FR-009a**: An asynchronous upload failure MUST be silent to the user by default (no warning/error printed), surfaced only at debug/verbose log level, consistent with existing telemetry failure handling.
- **FR-010**: System MUST exclude or mask any secret/sensitive values from command arguments and structured data before inclusion in an execution record, consistent with Atmos's existing secret-masking behavior.
- **FR-011**: System MUST NOT truncate or drop command-specific structured data due to payload size. When an execution record's command-specific structured-data field exceeds the platform's payload size limit, System MUST split that field across multiple correlated `POST /v1/atmos/exec` requests, reusing the existing chunked-upload/batch-correlation mechanism already used for `describe affected` uploads, tagged so Atmos Pro can reassemble the complete data set. The base envelope and resource-usage metrics are not subject to chunking and MUST always be sent in full.
- **FR-012**: System MUST authenticate execution-record uploads using the same credential/token mechanism already used for other Atmos Pro API calls, including automatic token refresh on expiry.
- **FR-013**: A verifiable, versioned description of the exact request and response shape for the execution-metadata upload MUST exist and be kept up to date as the payload evolves, in a form that the Atmos Pro team can use to implement and validate the receiving endpoint independently of the Atmos codebase.

### Key Entities

- **Execution Record**: Represents a single `atmos` command invocation's reported outcome — exactly one record per CLI invocation, including a multi-component `--affected`/`--all` run (FR-006a), never one record per component. Includes identity (bare subcommand path, positional arguments, CLI flags, exit code — FR-003b), environment (Atmos version, OS, architecture), source-control context, resource-usage metrics, an optional command-specific structured-data payload (which, for a multi-component invocation, itself contains the per-component breakdown), and the existing Atmos Pro run identifier already used by other Pro uploads, for correlation across a CI run and deduplication of retried uploads.
- **Resource Usage Metrics**: Represents how much time and system resources a command consumed while running — wall-clock duration, CPU time, and (where available) peak memory, page faults, context switches, and block I/O.
- **Command Structured Data**: Represents optional, command-specific enrichment attached to an Execution Record. For `terraform plan`/`apply`/`deploy`, this is the set of created/updated/deleted/replaced resources, output values, and warnings/errors produced by that run. When this data exceeds the platform's payload size limit, it is split across multiple correlated requests (never truncated or dropped) using the same chunking/batch-correlation mechanism as `describe affected` uploads; the base envelope and resource-usage metrics are unaffected and always delivered in full.
- **Execution Contract**: Represents the agreed, verifiable shape of the data exchanged between Atmos and Atmos Pro for execution-metadata upload, independent of either side's internal implementation.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of `atmos` command executions that run in a recognized CI environment with Atmos Pro configured produce a corresponding execution record on the Atmos Pro side, with no additional setup beyond existing CI/Pro configuration.
- **SC-002**: 0% of command executions outside CI, or in CI without Atmos Pro configured, produce any outbound execution-metadata traffic.
- **SC-003**: For `terraform plan`, `terraform apply`, `terraform deploy`, and `describe affected`, 100% of runs either confirm successful delivery of their complete execution record — including all chunks, if its structured data required batching — within the configured total wait period (~10 seconds by default), or exit through one of the two explicitly defined failure paths (fail command / warn and continue) — never an indefinite wait.
- **SC-004**: For all other commands, execution-metadata delivery adds no more than a brief, bounded delay to command completion as perceived by the user (fire-and-forget with a short best-effort flush, not a blocking wait for confirmation).
- **SC-005**: An outage or error response from Atmos Pro causes zero unexpected exit-code changes for asynchronous commands, and causes only the explicitly configured behavior (fail or warn) for synchronous commands.
- **SC-006**: A reviewer on the Atmos Pro team can determine the exact request/response shape for execution-metadata upload from a single, versioned artifact, without reading Atmos's Go source code.
- **SC-007**: For a `terraform plan` run with pending changes, 100% of the created/updated/deleted/replaced resource counts and output values visible in the plan's own output are also present in its execution record.

## Assumptions

- "Recognized CI environment" reuses Atmos's existing CI-detection behavior (environment-variable-based provider detection) rather than introducing a new detection mechanism.
- "Atmos Pro configured" means a usable authentication path already exists (a static token, or GitHub OIDC plus workspace identification) via Atmos's existing Atmos Pro configuration — no new configuration surface is introduced by this feature.
- The set of commands treated as synchronous (`terraform plan`, `terraform apply`, `terraform deploy`, `describe affected`) is fixed by this feature's initial implementation; commands are not user-configurable as synchronous or asynchronous.
- The mechanism by which an individual command supplies its structured data (FR-005) is an internal implementation concern and is intentionally not specified here; it must not require any user-facing configuration.
- Resource-usage metrics that are platform-specific (e.g. detailed memory/IO counters available on Unix-like systems but not Windows) are best-effort and their absence on unsupported platforms is expected, not an error.
- The "Execution Contract" (FR-013) is delivered as a consumer contract test artifact, following the same local-only, no-broker pattern already established for Atmos's other Atmos Pro API interactions.
- This feature does not define what Atmos Pro does with received execution records (storage, retention, display) — that is out of scope and owned by the Atmos Pro application. This includes any correlation or deduplication Atmos Pro may perform between this feature's execution records and records produced by the separate, pre-existing `uploadStatus`/`--upload-status` mechanism (FR-003a/FR-003b); the Atmos client's only obligation is to keep `Command`/`Args` consistent across both paths.
- No new user-facing opt-out flag is introduced; the feature is active whenever the CI + Pro-configured condition (FR-001) is met. The single new user-facing setting is the synchronous-upload wait timeout (FR-008a), which only lengthens the default wait and cannot be used to disable delivery.
