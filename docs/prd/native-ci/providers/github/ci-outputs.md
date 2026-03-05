# Native CI Integration - CI Output Variables

> Related: [Overview](../../overview.md) | [Job Summaries](./job-summaries.md) | [Configuration](../../framework/configuration.md)

## FR-3: CI Output Variables

**Requirement**: Export plan/apply results as CI output variables.

**Behavior**:
- Write to `$GITHUB_OUTPUT` in GitHub Actions
- Export standard variables: `has_changes`, `has_additions`, `has_destructions`, `artifact_key`, `plan_summary`
- Export terraform outputs after successful apply (prefixed with `output_`)
- Support filtering via `ci.output.variables` configuration

**Variables (plan)**:
| Variable | Type | Description |
|----------|------|-------------|
| `has_changes` | bool | Whether plan has any changes |
| `has_additions` | bool | Whether plan creates resources |
| `has_destructions` | bool | Whether plan destroys resources |
| `additions_count` | int | Number of resources to create |
| `changes_count` | int | Number of resources to change |
| `destructions_count` | int | Number of resources to destroy |
| `artifact_key` | string | Planfile storage key |
| `plan_summary` | string | Human-readable summary |

**Variables (apply)**:
| Variable | Type | Description |
|----------|------|-------------|
| `success` | bool | Whether apply succeeded |
| `output_*` | varies | Terraform outputs (flattened) |

## After `terraform plan`

```bash
# Written to $GITHUB_OUTPUT
has_changes=true
has_additions=true
has_destructions=false
additions_count=5
changes_count=2
destructions_count=0
artifact_key=plat-ue2-dev/vpc/abc123.tfplan
plan_exit_code=2
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
