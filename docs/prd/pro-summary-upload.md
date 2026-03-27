# Pro Summary Upload to Atmos Pro

## Problem

Atmos Pro users operating CI/CD pipelines need visibility into what each plan/apply actually did — resource counts, errors, warnings, and raw command output. Today the CLI uploads only a minimal status (command name, exit code, timestamp) via `InstanceStatusUploadRequest`. Teams using the Atmos Pro dashboard must switch to their CI platform (GitHub Actions, GitLab CI, etc.) to see the actual terraform output, parse error messages, or understand the scope of a plan. This context-switching slows down incident triage, drift review, and approval workflows.

By enriching the existing instance status upload with structured CI data, Atmos Pro can surface plan summaries, error details, and output logs directly in its dashboard — eliminating the need to leave the tool. This also enables server-side features like drift alerts, change magnitude tracking, and audit logging without requiring changes to individual CI platform integrations.

The data must be component-type-agnostic: terraform is the first implementation, but packer, helmfile, and future component types should plug into the same upload contract.

## Overview

When `--upload-status` is set and `ci.enabled` is true, Atmos uploads structured plan/apply data to Atmos Pro by extending the existing instance status DTO. The CI plugin system (`pkg/ci/internal/plugin`) already provides a component-type abstraction (`Plugin` interface, `OutputResult`, type-specific data structs). This change defines a new interface for plugins to produce upload-ready data, and wires it into the existing upload path.

## What Should Change

### 1. New Interface: `StatusDataProvider`

Define an interface in `pkg/ci/internal/plugin/` that plugins implement to produce upload-ready CI data:

```go
// StatusDataProvider is an optional interface that CI plugins can implement
// to provide structured data for the Atmos Pro status upload.
// Plugins that don't implement this interface will not contribute CI data.
type StatusDataProvider interface {
    // BuildStatusData converts parsed output into a component-type-specific
    // data structure suitable for the Pro status upload.
    // The returned value is serialized into the "ci.data" field of the upload payload.
    BuildStatusData(result *OutputResult, output string) *StatusData
}

// StatusData contains the component-type-agnostic CI data for the Pro upload.
type StatusData struct {
    // ComponentType identifies the component type (e.g., "terraform", "helmfile", "packer").
    ComponentType string `json:"component_type"`

    // HasChanges indicates whether the command detected changes.
    HasChanges bool `json:"has_changes"`

    // HasErrors indicates whether the command had errors.
    HasErrors bool `json:"has_errors"`

    // Warnings contains warning messages extracted from command output.
    Warnings []string `json:"warnings,omitempty"`

    // Errors contains error messages extracted from command output.
    Errors []string `json:"errors,omitempty"`

    // OutputLog contains the raw command stdout, passed through the IO masking
    // layer to redact secrets, then base64-encoded.
    OutputLog string `json:"output_log,omitempty"`

    // Data contains component-type-specific structured data.
    // For terraform: *TerraformStatusData
    // For helmfile: *HelmfileStatusData
    // For packer: *PackerStatusData (future)
    Data any `json:"data,omitempty"`
}
```

### 2. Terraform Implementation: `BuildStatusData`

The terraform plugin (`pkg/ci/plugins/terraform/`) implements `StatusDataProvider`:

```go
// TerraformStatusData contains terraform-specific data for the Pro upload.
type TerraformStatusData struct {
    // ResourceCounts contains resource change counts from plan output.
    ResourceCounts *ResourceCounts `json:"resource_counts,omitempty"`

    // Outputs contains terraform output values (after successful apply).
    // Keys are output names, values are the raw output values.
    Outputs map[string]any `json:"outputs,omitempty"`
}

func (p *Plugin) BuildStatusData(result *OutputResult, output string) *StatusData {
    data := &StatusData{
        ComponentType: "terraform",
        HasChanges:    result.HasChanges,
        HasErrors:     result.HasErrors,
        Errors:        result.Errors,
    }

    if tfData, ok := result.Data.(*TerraformOutputData); ok {
        data.Warnings = tfData.Warnings
        data.Data = &TerraformStatusData{
            ResourceCounts: &tfData.ResourceCounts,
            Outputs:        extractOutputValues(tfData.Outputs),
        }
    }

    // Output log is set by the caller after masking.
    return data
}
```

Other component types (helmfile, packer) implement the same interface with their own `*StatusData.Data` payloads when ready.

### 3. Extend `InstanceStatusUploadRequest` DTO

Extend the existing DTO in `pkg/pro/dtos/instances.go` with a single nested CI field:

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
    CI *StatusData `json:"ci,omitempty"`
}
```

The `CI` field is a pointer with `omitempty` — when `ci.enabled` is false the pointer is nil and the entire block is omitted, producing identical payloads to today.

### 4. Secret Masking for Output Log

The raw command output must pass through the IO masking layer before encoding:

```go
// In the upload logic:
maskedOutput := io.Mask(capturedOutput)  // Redact secrets via Gitleaks patterns
data.OutputLog = base64.StdEncoding.EncodeToString([]byte(maskedOutput))
```

The masking uses the same Gitleaks-based pattern library (120+ patterns) that protects all other Atmos output streams. This ensures that secrets (API keys, tokens, passwords) in terraform output are never sent to Atmos Pro in cleartext.

The `OutputLog` field is set by the upload caller after masking, not by the plugin's `BuildStatusData` — this keeps the masking concern in the IO layer rather than leaking it into plugin implementations.

### 5. Populate CI Data in Upload Logic

Extend `uploadCommandStatus()` in `internal/exec/terraform_execute_helpers_exec.go` to populate the CI block:

1. Check `atmosConfig.CI.Enabled` — if false, leave `CI` nil (existing behavior preserved).
2. Resolve the CI plugin for the current component type.
3. If the plugin implements `StatusDataProvider`, call `BuildStatusData(result, output)`.
4. Mask the captured output via `io.Mask()` and base64-encode it into `StatusData.OutputLog`.
5. Set `dto.CI = statusData` on the `InstanceStatusUploadRequest`.

```go
// Upload status only when explicitly requested via --upload-status flag.
if uploadStatusFlag && shouldUploadStatus(info) {
    if atmosConfig.CI.Enabled {
        if statusData, err := buildCIStatusData(atmosConfig, info, capturedOutput); err != nil {
            log.Warn("Failed to build CI status data", "error", err)
        } else {
            dto.CI = statusData
        }
    }
    if uploadErr := uploadCommandStatus(atmosConfig, info, exitCode, dto); uploadErr != nil {
        return uploadErr
    }
}
```

### 6. Integration Point

The command output must be captured and passed to the upload function. Two options:

**Option A (Preferred): Capture output at command execution.**
Add output capture to the `ExecuteShellCommand` call (via a new `ShellCommandOption` or by reading from the existing CI hook output capture) and pass it to `uploadCommandStatus()`.

**Option B: Re-invoke output parsing.**
If the output is not readily available at the upload call site, the upload function can accept the captured output string from wherever the CI hooks already capture it (e.g., stored on `ConfigAndStacksInfo`).

### 7. Data to Upload

All fields live under the `ci` block in the payload.

**Common fields (all component types):**

| Field | Source | Description |
|---|---|---|
| `ci.component_type` | plugin `GetType()` | Component type identifier |
| `ci.has_changes` | `result.HasChanges` | Whether the command detected changes |
| `ci.has_errors` | `result.HasErrors` | Whether the command had errors |
| `ci.warnings` | parsed from output | Warning messages |
| `ci.errors` | `result.Errors` | Error messages |
| `ci.output_log` | base64(io.Mask(stdout)) | Full command stdout, masked and base64-encoded |

**Terraform-specific `ci.data` (plan):**

| Field | Source | Description |
|---|---|---|
| `ci.data.resource_counts.create` | `ResourceCounts.Create` | Resources to create |
| `ci.data.resource_counts.change` | `ResourceCounts.Change` | Resources to change |
| `ci.data.resource_counts.replace` | `ResourceCounts.Replace` | Resources to replace |
| `ci.data.resource_counts.destroy` | `ResourceCounts.Destroy` | Resources to destroy |

**Terraform-specific `ci.data` (apply):**

| Field | Source | Description |
|---|---|---|
| `ci.data.outputs` | `TerraformOutputData.Outputs` | Terraform output values (raw) |

**Not uploaded (server-side concern):**
- Rendered summary markdown — the server renders summaries from the structured data.

## Gating Conditions

The `CI` block is populated **only** when ALL of the following are true:

1. `--upload-status` flag is set (same gate as instance status upload).
2. `shouldUploadStatus(info)` returns true (pro enabled in component settings, command is plan/apply).
3. `ci.enabled` is true in the global atmos configuration.
4. The CI plugin for the component type implements `StatusDataProvider`.

When any condition is false, the upload sends only the existing `command` + `exit_code` fields (backward compatible). The `ci` pointer is nil and the entire block is omitted.

## Error Handling

- Failure to build CI status data is **warn-only** — the upload proceeds with just `command` + `exit_code` (existing behavior, `ci` pointer stays nil).
- Individual fields within the `ci` block are best-effort. If output parsing partially fails, populate what is available.
- If masking fails, the output log is omitted rather than sending unmasked output.
- The upload itself remains fatal (matching existing behavior in `executeMainTerraformCommand`).

## API Contract

The existing PATCH endpoint is extended with an optional `ci` block. The server must handle payloads with or without the block for backward compatibility with older CLI versions.

```
PATCH /api/v1/repos/{owner}/{repo}/instances?stack={stack}&component={component}
```

Existing payload (unchanged):
```json
{
  "command": "plan" | "apply",
  "exit_code": <integer>,
  "last_run": "<ISO 8601 datetime>"
}
```

Extended payload (when ci.enabled, terraform plan):
```json
{
  "command": "plan",
  "exit_code": 2,
  "last_run": "2026-03-27T10:00:00Z",
  "ci": {
    "component_type": "terraform",
    "has_changes": true,
    "has_errors": false,
    "warnings": ["Warning: Value for undeclared variable..."],
    "errors": [],
    "output_log": "VGVycmFmb3JtIHdpbGwgcGVyZm9ybS...",
    "data": {
      "resource_counts": {
        "create": 3,
        "change": 1,
        "replace": 0,
        "destroy": 2
      }
    }
  }
}
```

Extended payload (when ci.enabled, terraform apply):
```json
{
  "command": "apply",
  "exit_code": 0,
  "last_run": "2026-03-27T10:05:00Z",
  "ci": {
    "component_type": "terraform",
    "has_changes": false,
    "has_errors": false,
    "warnings": [],
    "errors": [],
    "output_log": "QXBwbHkgY29tcGxldGUhIFJlc291cmNlczo...",
    "data": {
      "outputs": {
        "vpc_id": "vpc-abc123",
        "subnet_ids": ["subnet-1", "subnet-2"]
      }
    }
  }
}
```

The `ci.data` field is polymorphic — its schema depends on `ci.component_type`. The server uses the component type to deserialize the correct structure.

## Design Rationale

- **Component-type abstraction**: The `StatusDataProvider` interface lets each component type (terraform, helmfile, packer) produce its own structured data. Common fields (`has_changes`, `has_errors`, `warnings`, `errors`, `output_log`) live on `StatusData`; type-specific data lives in `StatusData.Data`. This mirrors the existing `OutputResult` / `TerraformOutputData` / `HelmfileOutputData` pattern in the CI plugin system.
- **Extend existing DTO, not new one**: The data belongs to the same instance status upload. A single PATCH with optional fields is simpler than a separate endpoint and avoids race conditions between two uploads for the same command.
- **Raw data, not rendered summaries**: The CLI sends structured data (resource counts, outputs, warnings, errors). The server owns rendering — this decouples summary presentation from CLI releases and lets the dashboard evolve independently.
- **Secret masking before upload**: The output log passes through `io.Mask()` (Gitleaks-based, 120+ patterns) before base64 encoding. This ensures secrets never leave the CLI unredacted, matching the security guarantees of all other Atmos output streams.
- **`ci.enabled` gate**: Output parsing is only meaningful when the CI subsystem is active. Without `ci.enabled`, the terraform output may not be captured in a parseable form.
- **Nested `ci` struct with pointer**: Grouping all new fields under a single pointer means the entire block is cleanly omitted when CI is disabled. Older CLIs that don't populate the field produce identical payloads.
- **Base64-encoded output log**: The raw command stdout can be large and contain ANSI escape codes, newlines, and other characters that are problematic in JSON string values. Base64 encoding ensures safe transport and lets the server decode and render as needed.
- **Polymorphic `ci.data`**: Using `any` for the data field and discriminating on `component_type` lets the server evolve per-type schemas independently. New component types can be added without changing the common upload contract.
