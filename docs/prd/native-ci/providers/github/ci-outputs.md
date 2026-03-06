# Native CI Integration - CI Output Variables

> Related: [Overview](../../overview.md) | [Job Summaries](./job-summaries.md) | [Configuration](../../framework/configuration.md)

## FR-3: CI Output Variables (IMPLEMENTED)

**Requirement**: Export plan/apply results as CI output variables.

**Implementation**: The plugin's `writeOutputs()` handler calls `getOutputVariables()` to get plugin-specific variables, adds common variables (`stack`, `component`, `command`, `summary`), filters by `ci.output.variables` config whitelist, and writes to `$GITHUB_OUTPUT` via the platform's `OutputWriter.WriteOutput()`. Note: `OutputHelpers` in `pkg/ci/internal/provider/output.go` provides convenience methods (`WritePlanOutputs`, `WriteApplyOutputs`) but these are NOT used by the plugin handlers — the handlers call `getOutputVariables()` directly.

**Behavior**:
- Write to `$GITHUB_OUTPUT` in GitHub Actions
- Export standard variables: `has_changes`, `has_errors`, `exit_code`, `resources_to_create`, `resources_to_change`, `resources_to_replace`, `resources_to_destroy` (plugin), plus `stack`, `component`, `command`, `summary` (executor)
- Export terraform outputs after successful apply (prefixed with `output_`) — **Phase 4, not yet implemented**
- Support filtering via `ci.output.variables` configuration

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

**Variables (apply)** (current implementation uses same variables as plan — no apply-specific variables yet):
| Variable | Type | Source | Description |
|----------|------|--------|-------------|
| `has_changes` | bool | Plugin | Whether apply had changes |
| `has_errors` | bool | Plugin | Whether apply had errors |
| `exit_code` | int | Plugin | Apply command exit code |
| `resources_to_create` | int | Plugin | Resources to create (from plan output if available) |
| `resources_to_change` | int | Plugin | Resources to change |
| `resources_to_replace` | int | Plugin | Resources to replace |
| `resources_to_destroy` | int | Plugin | Resources to destroy |
| `stack` | string | Handler | Stack name |
| `component` | string | Handler | Component name |
| `command` | string | Handler | Command name ("apply") |
| `summary` | string | Handler | Rendered summary markdown |

> **Note**: The `command` parameter in `getOutputVariables()` is accepted but not used for branching — both plan and apply return the same variable set. Terraform output export (`output_*` variables, `success` bool) is planned for Phase 4 but not yet implemented.

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
# Written to $GITHUB_OUTPUT (same variables as plan — apply-specific vars not yet implemented)
has_changes=true
has_errors=false
exit_code=0
resources_to_create=0
resources_to_change=0
resources_to_replace=0
resources_to_destroy=0
stack=plat-ue2-dev
component=vpc
command=apply
summary=## :white_check_mark: Apply: `vpc` in `plat-ue2-dev`...
```

> **Phase 4 (Not Started)**: Terraform output export after apply (`output_*` variables) is planned but not yet implemented. Will use `pkg/terraform/output/` package for formatting (flattening, uppercase conversion).

## Key Design Decision: Last-Writer-Wins, No Prefix

Output variable names use simple names (`has_changes`, `resources_to_create`) with **no component/stack prefix**. If two components run in the same job step, the last one's values win.

Users who need per-component isolation should use matrix strategy (one component per job) — which is the recommended workflow pattern via `describe affected --format=matrix`.

## Key Design Decision: Export Terraform Outputs After Apply (Phase 4 — Not Started)

Planned: leverage the `pkg/terraform/output/` package (from `osterman/tf-output-format` branch) to export terraform outputs after a successful apply. This is not yet implemented.

```bash
# After apply, terraform outputs will be written to $GITHUB_OUTPUT (Phase 4)
atmos terraform apply vpc -s plat-ue2-dev --ci
```
