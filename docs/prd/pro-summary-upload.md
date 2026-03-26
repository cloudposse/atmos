# Pro Summary Upload to Atmos Pro

## Overview

When `--upload-status` is set and `ci.enabled` is true, Atmos should upload structured plan/apply data to Atmos Pro by extending the existing instance status upload. This enriches the Atmos Pro dashboard with resource counts, terraform outputs, warnings, and errors — all raw data that the server renders into summaries.

Today the CLI already sends the raw command and exit code via `InstanceStatusUploadRequest`. This change extends that same DTO with structured fields parsed from terraform output, gated on `ci.enabled`. No new endpoints or DTOs are needed.

## What Should Change

### 1. Extend `InstanceStatusUploadRequest` DTO

Extend the existing DTO in `pkg/pro/dtos/instances.go` with new fields for structured plan/apply data:

```go
type InstanceStatusUploadRequest struct {
    // ... existing fields ...
    AtmosProRunID string `json:"atmos_pro_run_id"`
    AtmosVersion  string `json:"atmos_version"`
    AtmosOS       string `json:"atmos_os"`
    AtmosArch     string `json:"atmos_arch"`
    GitSHA        string `json:"git_sha"`
    RepoURL       string `json:"repo_url"`
    RepoName      string `json:"repo_name"`
    RepoOwner     string `json:"repo_owner"`
    RepoHost      string `json:"repo_host"`
    Component     string `json:"component"`
    Stack         string `json:"stack"`
    Command       string `json:"command"`
    ExitCode      int    `json:"exit_code"`

    // CI contains structured plan/apply data. Populated only when ci.enabled is true.
    // Grouped into a nested struct so the entire block is omitted when CI is disabled.
    CI *InstanceStatusCI `json:"ci,omitempty"`
}

// InstanceStatusCI contains structured CI data for the instance status upload.
// Omitted entirely when ci.enabled is false, preserving backward compatibility.
type InstanceStatusCI struct {
    // HasChanges indicates whether the plan detected changes.
    HasChanges bool `json:"has_changes"`

    // HasErrors indicates whether the command had errors.
    HasErrors bool `json:"has_errors"`

    // ResourceCounts contains structured resource change counts from plan output.
    ResourceCounts *ResourceCounts `json:"resource_counts,omitempty"`

    // Outputs contains terraform output values (after successful apply).
    // Keys are output names, values are the raw output values.
    Outputs map[string]any `json:"outputs,omitempty"`

    // Warnings contains warning messages extracted from terraform output.
    Warnings []string `json:"warnings,omitempty"`

    // Errors contains error messages extracted from terraform output.
    Errors []string `json:"errors,omitempty"`

    // OutputLog contains the raw terraform stdout captured during command execution,
    // base64-encoded. This preserves the full output including ANSI formatting
    // for server-side rendering and debugging.
    OutputLog string `json:"output_log,omitempty"`
}

// ResourceCounts contains resource change counts parsed from terraform plan output.
type ResourceCounts struct {
    Create  int `json:"create"`
    Change  int `json:"change"`
    Replace int `json:"replace"`
    Destroy int `json:"destroy"`
}
```

The new `CI` field is a pointer with `omitempty` — when `ci.enabled` is false the pointer is nil and the entire block is omitted, producing identical payloads to today. The server evolves its rendering independently of CLI releases.

### 2. Extend `UploadInstanceStatus` API Client

Update `UploadInstanceStatus` in `pkg/pro/api_client_instance_status.go` to include the new fields in the PATCH payload when they are present. The endpoint remains the same:

```
PATCH /api/v1/repos/{owner}/{repo}/instances?stack={stack}&component={component}
```

The payload gains an optional `ci` block:

```json
{
  "command": "plan",
  "exit_code": 2,
  "last_run": "2026-03-26T10:00:00Z",
  "ci": {
    "has_changes": true,
    "has_errors": false,
    "resource_counts": {
      "create": 3,
      "change": 1,
      "replace": 0,
      "destroy": 2
    },
    "warnings": ["Warning: Value for undeclared variable..."],
    "errors": [],
    "output_log": "VGVycmFmb3JtIHdpbGwgcGVyZm9ybS..."
  }
}
```

For apply with outputs:

```json
{
  "command": "apply",
  "exit_code": 0,
  "last_run": "2026-03-26T10:05:00Z",
  "ci": {
    "has_changes": false,
    "has_errors": false,
    "outputs": {
      "vpc_id": "vpc-abc123",
      "subnet_ids": ["subnet-1", "subnet-2"]
    },
    "warnings": [],
    "errors": [],
    "output_log": "QXBwbHkgY29tcGxldGUhIFJlc291cmNlczo..."
  }
}
```

### 3. Populate New Fields in Upload Logic

Extend `uploadStatus()` in `internal/exec/pro.go` (or its caller `uploadCommandStatus()` in `internal/exec/terraform_execute_helpers_exec.go`) to populate the new DTO fields when `ci.enabled` is true:

1. Check `atmosConfig.CI.Enabled` — if false, leave new fields empty (existing behavior preserved).
2. Call `terraform.ParseOutput()` on the captured command output to get the `OutputResult`.
3. Build an `InstanceStatusCI` struct and map the parsed result:
   - `HasChanges` ← `result.HasChanges`
   - `HasErrors` ← `result.HasErrors`
   - `ResourceCounts` ← `result.Data.(*TerraformOutputData).ResourceCounts`
   - `Outputs` ← `result.Data.(*TerraformOutputData).Outputs` (apply only, convert `TerraformOutput.Value` to raw values)
   - `Warnings` ← `result.Data.(*TerraformOutputData).Warnings`
   - `Errors` ← `result.Errors`
   - `OutputLog` ← base64-encode the raw captured terraform stdout
4. Set `dto.CI = &ciData` on the existing `InstanceStatusUploadRequest`.

### 4. Integration Point

The integration happens in the existing `executeMainTerraformCommand()` flow. The command output must be captured and passed to the upload function. Two options:

**Option A (Preferred): Capture output at command execution.**
Add output capture to the `ExecuteShellCommand` call (via a new `ShellCommandOption` or by reading from the existing CI hook output capture) and pass it to `uploadCommandStatus()`.

**Option B: Re-invoke output parsing.**
If the output is not readily available at the upload call site, the upload function can be extended to accept the captured output string from wherever the CI hooks already capture it (e.g., stored on `ConfigAndStacksInfo`).

```go
// Upload status only when explicitly requested via --upload-status flag.
if uploadStatusFlag && shouldUploadStatus(info) {
    if uploadErr := uploadCommandStatus(atmosConfig, info, exitCode, capturedOutput); uploadErr != nil {
        return uploadErr
    }
}
```

Inside `uploadCommandStatus`, when `atmosConfig.CI.Enabled`, parse the output and populate the new fields.

### 5. Data to Upload

All fields live under the `ci` block in the payload.

**For `plan`:**

| Field | Source | Description |
|---|---|---|
| `ci.has_changes` | `result.HasChanges` | Whether the plan detected drift |
| `ci.has_errors` | `result.HasErrors` | Whether the plan had errors |
| `ci.resource_counts.create` | `TerraformOutputData.ResourceCounts.Create` | Resources to create |
| `ci.resource_counts.change` | `TerraformOutputData.ResourceCounts.Change` | Resources to change |
| `ci.resource_counts.replace` | `TerraformOutputData.ResourceCounts.Replace` | Resources to replace |
| `ci.resource_counts.destroy` | `TerraformOutputData.ResourceCounts.Destroy` | Resources to destroy |
| `ci.warnings` | `TerraformOutputData.Warnings` | Warning messages from terraform |
| `ci.errors` | `result.Errors` | Error messages |
| `ci.output_log` | base64(captured stdout) | Full terraform stdout, base64-encoded |

**For `apply`:**

| Field | Source | Description |
|---|---|---|
| `ci.has_changes` | `result.HasChanges` | Whether apply made changes |
| `ci.has_errors` | `result.HasErrors` | Whether apply had errors |
| `ci.outputs` | `TerraformOutputData.Outputs` | Terraform output values (raw) |
| `ci.warnings` | `TerraformOutputData.Warnings` | Warning messages from terraform |
| `ci.errors` | `result.Errors` | Error messages |
| `ci.output_log` | base64(captured stdout) | Full terraform stdout, base64-encoded |

**Not uploaded (server-side concern):**
- Rendered summary markdown — the server renders summaries from the structured data.

## Gating Conditions

The new fields are populated **only** when ALL of the following are true:

1. `--upload-status` flag is set (same gate as instance status upload).
2. `shouldUploadStatus(info)` returns true (pro enabled in component settings, command is plan/apply).
3. `ci.enabled` is true in the global atmos configuration.

When `ci.enabled` is false, the upload still sends the existing `command` + `exit_code` fields (backward compatible). The `ci` pointer is nil and the entire block is omitted.

## Error Handling

- Failure to parse terraform output for the `ci` block is **warn-only** — the upload proceeds with just `command` + `exit_code` (existing behavior, `ci` pointer stays nil).
- Individual fields within the `ci` block are best-effort. If output parsing partially fails, populate what is available.
- The upload itself remains fatal (matching existing behavior in `executeMainTerraformCommand`).

## API Contract

The existing PATCH endpoint is extended with optional fields. The server must handle payloads with or without the new fields for backward compatibility with older CLI versions.

Existing payload (unchanged):
```json
{
  "command": "plan" | "apply",
  "exit_code": <integer>,
  "last_run": "<ISO 8601 datetime>"
}
```

Extended payload (when ci.enabled):
```json
{
  "command": "plan" | "apply",
  "exit_code": <integer>,
  "last_run": "<ISO 8601 datetime>",
  "ci": {
    "has_changes": <boolean>,
    "has_errors": <boolean>,
    "resource_counts": {
      "create": <integer>,
      "change": <integer>,
      "replace": <integer>,
      "destroy": <integer>
    },
    "outputs": { "<key>": <any>, ... },
    "warnings": ["<string>", ...],
    "errors": ["<string>", ...],
    "output_log": "<base64-encoded string>"
  }
}
```

## Design Rationale

- **Extend existing DTO, not new one**: The data belongs to the same instance status upload. A single PATCH with optional fields is simpler than a separate endpoint and avoids race conditions between two uploads for the same command.
- **Raw data, not rendered summaries**: The CLI sends structured data (resource counts, outputs, warnings, errors). The server owns rendering — this decouples summary presentation from CLI releases and lets the dashboard evolve independently.
- **`ci.enabled` gate**: Output parsing is only meaningful when the CI subsystem is active. Without `ci.enabled`, the terraform output may not be captured in a parseable form.
- **Nested `ci` struct with pointer**: Grouping all new fields under a single `*InstanceStatusCI` pointer means the entire block is cleanly omitted when `ci.enabled` is false. Older CLIs that don't populate the field produce identical payloads.
- **Terraform outputs on apply**: Raw output values (not stringified) are sent so the server can type them correctly and use them in dashboards or downstream integrations.
- **Base64-encoded output log**: The raw terraform stdout can be large and contain ANSI escape codes, newlines, and other characters that are problematic in JSON string values. Base64 encoding ensures safe transport and lets the server decode and render as needed.
