# Native CI Integration - CI Output Variables

> Related: [Overview](../../overview.md) | [Job Summaries](./job-summaries.md) | [Configuration](../../framework/configuration.md)

## FR-3: CI Output Variables (IMPLEMENTED)

**Requirement**: Export plan/apply results as CI output variables.

**Implementation**: The executor's `executeOutputAction()` calls `plugin.GetOutputVariables()` to get plugin-specific variables, adds common variables (`stack`, `component`, `command`, `summary`), filters by `ci.output.variables` config whitelist, and writes to `$GITHUB_OUTPUT` via the platform's `OutputWriter.WriteOutput()`. Note: `OutputHelpers` in `pkg/ci/internal/provider/output.go` provides convenience methods (`WritePlanOutputs`, `WriteApplyOutputs`) but these are NOT used by the executor — the executor calls `plugin.GetOutputVariables()` directly.

**Behavior**:
- Write to `$GITHUB_OUTPUT` in GitHub Actions
- Export standard variables: `has_changes`, `has_additions`, `has_destructions`, `artifact_key`, `plan_summary`
- Export terraform outputs after successful apply (prefixed with `output_`)
- Support filtering via `ci.output.variables` configuration

**Variables (plan)** (**IMPLEMENTED** — plugin variables from `pkg/ci/plugins/terraform/plugin.go` `GetOutputVariables()` + common variables added by `pkg/ci/executor.go`):
| Variable | Type | Source | Description |
|----------|------|--------|-------------|
| `has_changes` | bool | Plugin | Whether plan has any changes |
| `has_errors` | bool | Plugin | Whether plan had errors |
| `exit_code` | int | Plugin | Plan command exit code |
| `resources_to_create` | int | Plugin | Number of resources to create |
| `resources_to_change` | int | Plugin | Number of resources to change |
| `resources_to_replace` | int | Plugin | Number of resources to replace |
| `resources_to_destroy` | int | Plugin | Number of resources to destroy |
| `stack` | string | Executor | Stack name |
| `component` | string | Executor | Component name |
| `command` | string | Executor | Command name (e.g., "plan") |
| `summary` | string | Executor | Rendered summary markdown (if summary action ran) |

> **Note**: `OutputHelpers.WritePlanOutputs()` in `pkg/ci/internal/provider/output.go` defines a separate set of convenience variable names (`has_additions`, `has_additions_count`, etc.) but is NOT called by the executor. The executor uses `plugin.GetOutputVariables()` directly. `OutputHelpers` exists for potential future use by plugins in the callback-based architecture.

**Variables (apply)**:
| Variable | Type | Description |
|----------|------|-------------|
| `success` | bool | Whether apply succeeded |
| `output_*` | varies | Terraform outputs (flattened) |

## After `terraform plan`

```bash
# Written to $GITHUB_OUTPUT (via executor → plugin.GetOutputVariables() + executor common vars)
has_changes=true
has_errors=false
exit_code=2
resources_to_create=5
resources_to_change=2
resources_to_replace=0
resources_to_destroy=0
stack=plat-ue2-dev
component=vpc
command=plan
summary=## :recycle: Plan: `vpc` in `plat-ue2-dev`...
```

## After `terraform apply`

```bash
# Written to $GITHUB_OUTPUT
apply_exit_code=0
success=true

# Terraform outputs (using pkg/terraform/output/ formats)
# All outputs are exported in the configured format
output_vpc_id=vpc-12345678
output_subnet_ids=["subnet-1","subnet-2"]
```

The terraform outputs use the format options from `pkg/terraform/output/`:
- `FormatEnv` - Default for `$GITHUB_OUTPUT` (key=value)
- Flattening support for nested outputs
- Uppercase conversion for environment variable compatibility

## Key Design Decision: Last-Writer-Wins, No Prefix

Output variable names use simple names (`has_changes`, `plan_summary`) with **no component/stack prefix**. If two components run in the same job step, the last one's values win.

Users who need per-component isolation should use matrix strategy (one component per job) — which is the recommended workflow pattern via `describe affected --format=matrix`.

## Key Design Decision: Export Terraform Outputs After Apply

Leverage the `pkg/terraform/output/` package (from `osterman/tf-output-format` branch) to export terraform outputs after a successful apply:

```bash
# After apply, outputs are written to $GITHUB_OUTPUT
# Using the format options from pkg/terraform/output/
atmos terraform apply vpc -s plat-ue2-dev --ci
```
