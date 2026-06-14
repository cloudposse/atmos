# Data Model: Pro Summary Upload

**Date**: 2026-06-09

## Entities

### InstanceStatusUploadRequest (extended)

The upload payload DTO. Lives in `pkg/pro/dtos/instances.go`.

| Field | Type | Required | Source | Notes |
|---|---|---|---|---|
| `atmos_pro_run_id` | string | no | `ATMOS_PRO_RUN_ID` env var | CI run identifier |
| `atmos_version` | string | yes | `pkg/version.Version` | |
| `atmos_os` | string | yes | `runtime.GOOS` | |
| `atmos_arch` | string | yes | `runtime.GOARCH` | |
| `git_sha` | string | no | git HEAD | Empty string on failure |
| `repo_url` | string | yes | git remote | |
| `repo_name` | string | yes | git remote | |
| `repo_owner` | string | yes | git remote | |
| `repo_host` | string | yes | git remote | |
| `component` | string | yes | `info.Component` | Stack component name |
| `stack` | string | yes | `info.Stack` | Stack name |
| `command` | string | yes | original `info.SubCommand` | Literal user invocation (`"deploy"` preserved, not normalized to `"apply"`) |
| `exit_code` | int | yes | command exit code | |
| `component_type` | string | no (omitempty) | `info.ComponentType` | `"terraform"` when present; omitted for types without `StatusDataProvider` |
| `metadata` | map[string]any | no (omitempty) | `buildCIStatusData()` | Nil when `ci.enabled` false or metadata construction fails |

### Metadata Map (terraform plan)

Populated by `terraform.Plugin.BuildStatusData` + `addOutputLog`.

| Key | Type | Source | Notes |
|---|---|---|---|
| `has_changes` | bool | parsed exit code / output | true when resources will change |
| `has_errors` | bool | `OutputResult.HasErrors` | |
| `warnings` | []string | `TerraformOutputData.Warnings` | May be empty |
| `errors` | []string | `OutputResult.Errors` | May be empty |
| `resource_counts` | map[string]int | `TerraformOutputData.ResourceCounts` | Keys: `create`, `change`, `replace`, `destroy` |
| `output_log` | string | base64(maskedOutput tail) | Omitted when output capture fails |
| `truncated` | bool | size check | Present and `true` only when log was truncated |

### Metadata Map (terraform apply)

Same as plan, with `resource_counts` replaced by:

| Key | Type | Source | Notes |
|---|---|---|---|
| `outputs` | map[string]any | `TerraformOutputData.Outputs` | Sensitive values replaced with `"<MASKED>"` |

### OutputResult

Internal struct. Lives in `pkg/ci/internal/plugin/types.go`. Not serialized directly.

| Field | Type | Notes |
|---|---|---|
| `ExitCode` | int | Authoritative signal for changes/errors |
| `HasChanges` | bool | Plan: true when exit code 2 or output indicates changes |
| `HasErrors` | bool | true when exit code 1 or error text detected |
| `Errors` | []string | Extracted error messages |
| `Data` | any | Type-asserted to `*TerraformOutputData` for terraform |

### TerraformOutputData

Component-specific parsed data. Lives in `pkg/ci/internal/plugin/types.go`.

| Field | Type | Notes |
|---|---|---|
| `ResourceCounts` | ResourceCounts | Create/Change/Replace/Destroy counts |
| `Warnings` | []string | Full warning block text |
| `Outputs` | map[string]TerraformOutput | After apply; key = output name |

### TerraformOutput

| Field | Type | Notes |
|---|---|---|
| `Value` | any | String/number/bool/list/map |
| `Type` | string | `"string"`, `"number"`, `"bool"`, `"list"`, `"map"`, `"object"` |
| `Sensitive` | bool | When true, `Value` is replaced with `"<MASKED>"` before upload |

## Data Flow

```
atmos terraform plan --upload-status
        │
        ▼
executeCommandPipeline (terraform_execute_helpers_exec.go)
        │
        ├─► capture originalSubCommand  ← GAP: must save BEFORE handleDeploySubcommand
        │
        ├─► handleDeploySubcommand      (mutates info.SubCommand: "deploy" → "apply")
        │
        ├─► WithStdoutCapture(&maskedOutput)   ← tees stdout after MaskWriter
        │
        ├─► ExecuteShellCommand / terraform runs
        │
        ├─► exitCode = resolveExitCode(err)
        │
        ├─► buildMetadataForUpload(captureOutput, info, maskedOutput.Bytes())
        │        │
        │        └─► ci.BuildStatusData(componentType, maskedOutput, subCommand)
        │                │
        │                └─► terraform.Plugin.BuildStatusData(output, command)
        │                        ├─► ParseOutput → OutputResult + TerraformOutputData
        │                        ├─► resource_counts, has_changes, has_errors, errors, warnings
        │                        └─► extractOutputValues (sensitive → "<MASKED>")
        │
        ├─► addOutputLog(data, maskedOutput, 3MB limit)
        │        ├─► truncate from beginning if > 3MB
        │        └─► base64 encode tail → data["output_log"]
        │
        └─► uploadCommandStatus(atmosConfig, info, exitCode, metadata)
                 │
                 └─► uploadStatus(info, exitCode, componentType, metadata, client, gitRepo)
                          │
                          └─► InstanceStatusUploadRequest{Command: originalSubCommand, ...}
                                   │
                                   └─► PATCH /api/v1/repos/{owner}/{repo}/instances
```

## Gating Conditions

All four must be true for `metadata`/`component_type` to be populated:

1. `uploadStatusFlag` — `--upload-status` flag present
2. `shouldUploadStatus(info)` — `settings.pro.enabled: true` in component AND subcommand is
   `"plan"`, `"apply"`, or `"deploy"` ← **gap: `"deploy"` currently missing from this gate**
3. `atmosConfig.CI.Enabled` — `ci.enabled: true` in atmos.yaml
4. Component type implements `StatusDataProvider` — checked by `ci.BuildStatusData` via registry

When any condition is false: `metadata` is nil, `component_type` is empty string — both omitted
from JSON due to `omitempty`. Payload is identical to pre-feature behavior.
