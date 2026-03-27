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
    // BuildStatusData converts parsed output into a map of key-value pairs
    // for the Pro status upload. Each component type decides its own keys.
    // The returned map is serialized as-is into the "ci" field of the upload payload.
    BuildStatusData(result *OutputResult, output string) map[string]any
}
```

The `ci` field on the DTO is `map[string]any` — a flexible bag of data that each component type populates with whatever keys it needs. This avoids a rigid shared struct and lets each component type evolve its schema independently. The only convention is that plugins should include a `"component_type"` key so the server can dispatch.

### 2. Terraform Implementation: `BuildStatusData`

The terraform plugin (`pkg/ci/plugins/terraform/`) implements `StatusDataProvider`:

```go
func (p *Plugin) BuildStatusData(result *OutputResult, output string) map[string]any {
    data := map[string]any{
        "component_type": "terraform",
        "has_changes":    result.HasChanges,
        "has_errors":     result.HasErrors,
        "errors":         result.Errors,
    }

    if tfData, ok := result.Data.(*TerraformOutputData); ok {
        data["warnings"] = tfData.Warnings
        data["resource_counts"] = map[string]int{
            "create":  tfData.ResourceCounts.Create,
            "change":  tfData.ResourceCounts.Change,
            "replace": tfData.ResourceCounts.Replace,
            "destroy": tfData.ResourceCounts.Destroy,
        }
        data["outputs"] = extractOutputValues(tfData.Outputs)
    }

    // "output_log" and "truncated" are set by the caller after capture/truncation.
    return data
}

// extractOutputValues converts TerraformOutput map to raw values.
// Sensitive outputs are replaced with "<MASKED>" to prevent secret leakage.
func extractOutputValues(outputs map[string]TerraformOutput) map[string]any {
    result := make(map[string]any, len(outputs))
    for key, out := range outputs {
        if out.Sensitive {
            result[key] = "<MASKED>"
        } else {
            result[key] = out.Value
        }
    }
    return result
}
```

Other component types (helmfile, packer) implement the same interface with their own keys when ready. For example, a helmfile plugin might include `"releases"` instead of `"resource_counts"`.

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

    // CI contains structured plan/apply data as a flexible map.
    // Each component type populates its own keys. Omitted when ci.enabled is false.
    CI map[string]any `json:"ci,omitempty"`
}
```

The `CI` field is `map[string]any` with `omitempty` — when `ci.enabled` is false the map is nil and the entire block is omitted, producing identical payloads to today. Each component type decides what keys to include; the only convention is a `"component_type"` key for server-side dispatch.

### 4. Secret Masking for Output Log

The raw command output must pass through the IO masking layer before encoding:

`ExecuteShellCommand` already supports `WithStdoutCapture(w io.Writer)` which tees output into a buffer **after** the `MaskWriter` layer. This means the captured output is already masked — no explicit `Masker().Mask()` call is needed.

```go
// In the command execution:
var capturedOutput bytes.Buffer
err := ExecuteShellCommand(..., WithStdoutCapture(&capturedOutput))

// In the upload logic — output is already masked by MaskWriter:
data.OutputLog = base64.StdEncoding.EncodeToString(capturedOutput.Bytes())
```

The `OutputLog` field is set by the upload caller, not by the plugin's `BuildStatusData` — this keeps the capture/masking concern in the IO layer rather than leaking it into plugin implementations.

### 5. Populate CI Data in Upload Logic

Extend `uploadCommandStatus()` in `internal/exec/terraform_execute_helpers_exec.go` to populate the CI block:

1. Check `atmosConfig.CI.Enabled` — if false, leave `CI` nil (existing behavior preserved).
2. Resolve the CI plugin for the current component type via `ci.GetPlugin(info.Command)`.
3. If the plugin implements `StatusDataProvider`, call `BuildStatusData(result, output)` to get `map[string]any`.
4. Add `"output_log"` key: base64-encode the captured output (already masked by `WithStdoutCapture`).
5. If truncated, add `"truncated": true`.
6. Set `dto.CI = ciData` on the `InstanceStatusUploadRequest`.

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

### 6. Output Log Size Limits

The output log can be large (verbose providers, many resources). To prevent oversized payloads, the CLI truncates the output log before base64 encoding.

**Max size is server-defined:** The CLI fetches the max payload size from the Atmos Pro API at upload time (or caches it). The server returns the limit via a settings/configuration endpoint. This lets the server control the limit without requiring CLI updates.

**Truncation behavior:**
- If the masked output exceeds the server-defined max size, truncate from the **beginning** (keep the tail — the most useful part: plan summary, apply result, errors).
- Add a `"truncated": true` key to the CI map so the server knows the log is incomplete.
- If the server is unreachable for settings, fall back to a built-in default (e.g., 3MB pre-encoding, which becomes ~4MB after base64).

**Server settings endpoint:**
```
GET /api/v1/settings
```
Response (relevant fields):
```json
{
  "max_output_log_bytes": 3145728
}
```

The CLI caches this value for the duration of the command execution. If the endpoint is unavailable, the built-in default is used.

### 7. Integration Point

The upload needs two views of the command output:

1. **Raw output** — for parsing (resource counts, warnings, errors, terraform outputs). The parser's regex patterns expect unmodified terraform output; masked strings (`<MASKED>`) could break extraction.
2. **Masked output** — for the `output_log` field. Secrets must be redacted before upload.

`ExecuteShellCommand` supports both via its option system:
- `WithStdoutCapture(w io.Writer)` captures output **after** `MaskWriter` (masked).
- A second capture buffer can be added **before** `MaskWriter` to get raw output for parsing.

Alternatively, if the CI hooks already capture raw output into `HookContext.Output`, reuse that for parsing and only use `WithStdoutCapture` for the masked log.

```go
var maskedOutput bytes.Buffer
err := ExecuteShellCommand(..., WithStdoutCapture(&maskedOutput))

// For parsing: use raw output from CI hook context or pre-mask capture
result := terraform.ParseOutput(rawOutput, info.SubCommand)

// For upload: use masked capture
data.OutputLog = base64.StdEncoding.EncodeToString(maskedOutput.Bytes())
```

Pass both the parsed result and the masked buffer to `uploadCommandStatus()`.

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
| `ci.output_log` | base64(captured stdout) | Full command stdout, masked via `WithStdoutCapture`, base64-encoded |
| `ci.truncated` | size check | Whether the output log was truncated due to size limits |

**Terraform-specific keys (plan):**

| Field | Source | Description |
|---|---|---|
| `ci.resource_counts.create` | `ResourceCounts.Create` | Resources to create |
| `ci.resource_counts.change` | `ResourceCounts.Change` | Resources to change |
| `ci.resource_counts.replace` | `ResourceCounts.Replace` | Resources to replace |
| `ci.resource_counts.destroy` | `ResourceCounts.Destroy` | Resources to destroy |

**Terraform-specific keys (apply):**

| Field | Source | Description |
|---|---|---|
| `ci.outputs` | `TerraformOutputData.Outputs` | Terraform output values (sensitive values masked) |

**Not uploaded (server-side concern):**
- Rendered summary markdown — the server renders summaries from the structured data.

## Gating Conditions

The `CI` block is populated **only** when ALL of the following are true:

1. `--upload-status` flag is set (same gate as instance status upload).
2. `shouldUploadStatus(info)` returns true (pro enabled in component settings, command is plan/apply).
3. `ci.enabled` is true in the global atmos configuration.
4. The CI plugin for the component type implements `StatusDataProvider`.

When any condition is false, the upload sends only the existing `command` + `exit_code` fields (backward compatible). The `ci` map is nil and the entire block is omitted.

**Note on `deploy`:** The `deploy` subcommand is internally converted to `apply` by `handleDeploySubcommand()` before the upload logic runs. For Atmos Pro, deploy and apply are identical — no special handling is needed.

## Error Handling

- Failure to build CI status data is **warn-only** — the upload proceeds with just `command` + `exit_code` (existing behavior, `ci` map stays nil).
- Individual fields within the `ci` block are best-effort. If output parsing partially fails, populate what is available.
- If output capture fails, the output log is omitted rather than blocking the upload.
- The upload itself remains fatal (matching existing behavior in `executeMainTerraformCommand`).

## API Contract

The existing PATCH endpoint is extended with an optional `ci` map. The server must handle payloads with or without the map for backward compatibility with older CLI versions.

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
    "resource_counts": {
      "create": 3,
      "change": 1,
      "replace": 0,
      "destroy": 2
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
    "outputs": {
      "vpc_id": "vpc-abc123",
      "subnet_ids": ["subnet-1", "subnet-2"]
    }
  }
}
```

The `ci` map is a flat bag of keys. The server reads `component_type` to know which other keys to expect. Each component type owns its key namespace.

## Related PRDs

- **[Instance Status Raw Upload](instance-status-raw-upload.md)** — defines the base `InstanceStatusUploadRequest` DTO, the PATCH endpoint, exit code interpretation, and the `--upload-status` flag. This PRD extends that foundation with the optional `ci` map.

## Design Rationale

- **Component-type abstraction via `map[string]any`**: The `StatusDataProvider` interface returns `map[string]any`, giving each component type full flexibility over its keys. Terraform includes `resource_counts` and `outputs`; helmfile might include `releases`; packer might include `build_artifacts`. No shared struct constrains them — only the convention of including `component_type` for server-side dispatch.
- **Extend existing DTO, not new one**: The data belongs to the same instance status upload. A single PATCH with optional fields is simpler than a separate endpoint and avoids race conditions between two uploads for the same command.
- **Raw data, not rendered summaries**: The CLI sends structured data (resource counts, outputs, warnings, errors). The server owns rendering — this decouples summary presentation from CLI releases and lets the dashboard evolve independently.
- **Secret masking before upload**: The output log is captured via `WithStdoutCapture`, which tees output after the `MaskWriter` layer. The captured buffer is already masked — no additional masking call is needed. This reuses the same masking pipeline that protects all other Atmos output streams.
- **`ci.enabled` gate**: Output parsing is only meaningful when the CI subsystem is active. Without `ci.enabled`, the terraform output may not be captured in a parseable form.
- **`map[string]any` with `omitempty`**: A nil map is cleanly omitted from JSON. Older CLIs that don't populate the field produce identical payloads. No rigid struct means no breaking changes when adding new keys.
- **Base64-encoded output log**: The raw command stdout can be large and contain ANSI escape codes, newlines, and other characters that are problematic in JSON string values. Base64 encoding ensures safe transport and lets the server decode and render as needed.
- **No polymorphic nesting**: Instead of a shared struct with a polymorphic `data` field, the entire `ci` map is the component's canvas. This is simpler to serialize, simpler to extend, and the server just reads the `component_type` key to know what other keys to expect.
