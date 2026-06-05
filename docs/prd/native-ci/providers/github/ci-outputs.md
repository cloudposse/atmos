# Native CI Integration - CI Output Variables

> Related: [Overview](../../overview.md) | [Job Summaries](./job-summaries.md) | [Configuration](../../framework/configuration.md)

## FR-3: CI Output Variables (IMPLEMENTED)

**Requirement**: Export plan/apply results as CI output variables.

**Implementation**: The plugin's `writeOutputs()` handler calls `getOutputVariables()` to get plugin-specific variables, adds common variables (`stack`, `component`, `command`, `summary`), filters native CI variables by `ci.output.variables` config whitelist, then exports terraform outputs (which bypass the whitelist), and writes to `$GITHUB_OUTPUT` via the platform's `OutputWriter.WriteOutput()`. Note: `OutputHelpers` in `pkg/ci/internal/provider/output.go` provides convenience methods (`WritePlanOutputs`, `WriteApplyOutputs`) but these are NOT used by the plugin handlers — the handlers call `getOutputVariables()` directly.

**Behavior**:
- Write to `$GITHUB_OUTPUT` in GitHub Actions
- Export standard variables: `has_changes`, `has_errors`, `exit_code`, `resources_to_create`, `resources_to_change`, `resources_to_replace`, `resources_to_destroy` (plugin), plus `stack`, `component`, `command`, `summary` (handler)
- Export `success` variable for apply commands (`true`/`false`)
- Export terraform outputs after successful apply (prefixed with `output_`) — flattened via `FlattenMap()`, nested outputs use `_` separator (e.g., `output_config_host`)
- **Terraform outputs bypass the `ci.output.variables` whitelist** — they are always included
- Native CI variables support filtering via `ci.output.variables` configuration
- Terraform output fetch failures are warn-only (do not fail the apply command)

**Variables (plan)** (**IMPLEMENTED** — plugin variables from `pkg/ci/plugins/terraform/plugin.go` `getOutputVariables()` + common variables added by `writeOutputs()` handler):
| Variable | Type | Source | Description |
|----------|------|--------|-------------|
| `has_changes` | bool | Plugin | Whether plan has any changes |
| `has_errors` | bool | Plugin | Whether plan had errors |
| `exit_code` | int | Plugin | Plan command exit code |
| `resources_to_create` | int | Plugin | Number of resources to create |
| `resources_to_change` | int | Plugin | Number of resources to change |
| `resources_to_replace` | int | Plugin | Number of resources to replace |
| `resources_to_destroy` | int | Plugin | Number of resources to destroy |
| `stack` | string | Handler | Stack name |
| `component` | string | Handler | Component name |
| `command` | string | Handler | Command name (e.g., "plan") |
| `summary` | string | Handler | Rendered summary markdown (if summary was enabled) |

> **Note**: `OutputHelpers.WritePlanOutputs()` in `pkg/ci/internal/provider/output.go` defines a separate set of convenience variable names (`has_additions`, `has_additions_count`, etc.) but is NOT called by the plugin handlers. The handlers call `getOutputVariables()` directly. `OutputHelpers` exists for potential future use.

**Variables (apply)** (**IMPLEMENTED** — includes apply-specific `success` variable and terraform `output_*` variables):
| Variable | Type | Source | Description |
|----------|------|--------|-------------|
| `has_changes` | bool | Plugin | Whether apply had changes |
| `has_errors` | bool | Plugin | Whether apply had errors |
| `exit_code` | int | Plugin | Apply command exit code |
| `success` | bool | Plugin | Whether apply succeeded (`!has_errors`) |
| `resources_to_create` | int | Plugin | Resources to create (from plan output if available) |
| `resources_to_change` | int | Plugin | Resources to change |
| `resources_to_replace` | int | Plugin | Resources to replace |
| `resources_to_destroy` | int | Plugin | Resources to destroy |
| `stack` | string | Handler | Stack name |
| `component` | string | Handler | Component name |
| `command` | string | Handler | Command name ("apply") |
| `summary` | string | Handler | Rendered summary markdown |
| `output_*` | any | Handler | Terraform outputs, flattened with `output_` prefix (e.g., `output_vpc_id`, `output_config_host`) |

> **Note**: The `success` variable is only present for apply commands. The `output_*` variables are fetched via `tfoutput.GetComponentOutputs()` after a successful apply. Nested outputs are flattened using `FlattenMap()` with `_` separator (e.g., `{"config": {"host": "localhost"}}` → `output_config_host=localhost`). Arrays use numeric indices (e.g., `output_subnet_ids_0`). Terraform outputs bypass the `ci.output.variables` whitelist — they are always included.

## After `terraform plan`

```bash
# Written to $GITHUB_OUTPUT (via plugin handler writeOutputs() → getOutputVariables() + common vars)
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
# Written to $GITHUB_OUTPUT (includes apply-specific success and terraform output_* variables)
has_changes=true
has_errors=false
exit_code=0
success=true
resources_to_create=0
resources_to_change=0
resources_to_replace=0
resources_to_destroy=0
stack=plat-ue2-dev
component=vpc
command=apply
summary=## :white_check_mark: Apply: `vpc` in `plat-ue2-dev`...
output_vpc_id=vpc-0123456789abcdef0
output_subnet_ids_0=subnet-abc123
output_subnet_ids_1=subnet-def456
output_config_host=localhost
output_config_port=3000
```

## Key Design Decision: Last-Writer-Wins, No Prefix

Output variable names use simple names (`has_changes`, `resources_to_create`) with **no component/stack prefix**. If two components run in the same job step, the last one's values win.

Users who need per-component isolation should use matrix strategy (one component per job) — which is the recommended workflow pattern via `describe affected --format=matrix`.

## Key Design Decision: Terraform Outputs Bypass Whitelist (IMPLEMENTED)

Terraform `output_*` variables are added **after** the `ci.output.variables` whitelist filter. This means:

- **Native CI variables** (`has_changes`, `stack`, `component`, etc.) are subject to the whitelist — only variables listed in `ci.output.variables` are exported.
- **Terraform outputs** (`output_*`) are **always included** regardless of the whitelist configuration.

**Rationale**: Users don't always know terraform output names upfront. The whitelist is designed to control the well-known native CI variables. Terraform outputs are inherently dynamic and should always be available to downstream CI jobs.

**Implementation**: `writeOutputs()` in `handlers.go` applies `filterVariables()` to native CI variables first, then adds terraform outputs afterward. The `getTerraformOutputs()` method fetches outputs via `tfoutput.GetComponentOutputs()` with `skipInit=true` (terraform is already initialized from the apply), flattens them via `tfoutput.FlattenMap()` with `"output"` prefix and `"_"` separator.

**Error handling**: If terraform output fetching fails (e.g., no state, executor not initialized), a warning is logged but the apply command is not failed. This is consistent with the warn-only pattern for CI output operations.

```bash
# After apply, all terraform outputs are automatically exported
atmos terraform apply vpc -s plat-ue2-dev --ci
# → output_vpc_id=vpc-0123456789abcdef0
# → output_config_host=localhost  (nested: config.host)
# → output_subnet_ids_0=subnet-abc123  (array index 0)
```
