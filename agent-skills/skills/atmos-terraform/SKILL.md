---
name: atmos-terraform
description: "Terraform orchestration: plan/apply/deploy, workspace management, backend config, varfile generation, authentication"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Terraform Orchestration

Atmos wraps the Terraform CLI to provide stack-aware orchestration of infrastructure operations. Instead of
manually managing workspaces, backends, variable files, and authentication for each Terraform component, Atmos
resolves the full configuration from stack manifests and handles all of these concerns automatically.

## How Atmos Orchestrates Terraform

When you run any `atmos terraform` command, Atmos performs the following sequence:

1. **Resolves stack configuration** -- Reads and deep-merges all stack manifests to produce the fully resolved
   configuration for the target component in the target stack.
2. **Generates backend configuration** -- Writes a `backend.tf.json` file in the component directory with the
   correct backend settings (S3 bucket, key, region, etc.) derived from the stack config.
3. **Generates variable file** -- Writes a `terraform.tfvars.json` file containing all `vars` defined for the
   component in the stack.
4. **Provisions backend infrastructure** -- If `provision.backend.enabled: true`, creates the backend storage
   (e.g., S3 bucket) before Terraform init.
5. **Runs `terraform init`** -- Initializes the working directory with the generated backend config. Cleans
   `.terraform/environment` first and optionally adds `-reconfigure`.
6. **Selects or creates workspace** -- Calculates the Terraform workspace name from context variables and
   selects it (or creates it if it does not exist).
7. **Executes the requested command** -- Runs `terraform plan`, `apply`, `destroy`, etc. with the generated
   varfile and any additional flags.

This means a single command like `atmos terraform plan vpc -s plat-ue2-dev` replaces what would normally
require multiple manual steps: configuring the backend, writing tfvars, running init, selecting the workspace,
and then running plan.

## Core Commands

### plan

Generates a Terraform execution plan showing what changes would be made.

```shell
atmos terraform plan <component> -s <stack>
```

By default, Atmos saves the plan to a file using the naming convention `<context>-<component>.planfile`.
This planfile can later be used with `--from-plan` to apply the exact reviewed changes.

```shell
# Basic plan
atmos terraform plan vpc -s plat-ue2-dev

# Skip planfile generation (useful for Terraform Cloud)
atmos terraform plan vpc -s dev --skip-planfile

# Plan with custom output path
atmos terraform plan vpc -s dev -out=/tmp/my-plan.tfplan

# Plan only specific resources
atmos terraform plan vpc -s dev -target=aws_subnet.private
```

### apply

Applies Terraform changes. Supports interactive approval, planfile-based apply, and auto-approve.

```shell
atmos terraform apply <component> -s <stack>
```

```shell
# Interactive apply (prompts for confirmation)
atmos terraform apply vpc -s plat-ue2-dev

# Apply from a previously generated plan
atmos terraform plan vpc -s dev
atmos terraform apply vpc -s dev --from-plan

# Apply a specific planfile
atmos terraform apply vpc -s dev --planfile /tmp/my-plan.tfplan

# Auto-approved apply (no confirmation prompt)
atmos terraform apply vpc -s dev -auto-approve
```

### deploy

Combines plan and apply with automatic approval. This is the most common command for CI/CD pipelines.

```shell
atmos terraform deploy <component> -s <stack>
```

Key differences from `apply`:
- Automatically sets `-auto-approve` -- no interactive confirmation
- Supports `--deploy-run-init` to control whether init runs
- Designed for automated, non-interactive deployments

```shell
# Deploy a component
atmos terraform deploy vpc -s plat-ue2-dev

# Deploy from a previously generated plan
atmos terraform deploy vpc -s dev --from-plan

# Deploy a specific planfile
atmos terraform deploy vpc -s dev --planfile /tmp/vpc-plan.tfplan
```

### destroy

Destroys all resources managed by a component in a stack.

```shell
atmos terraform destroy <component> -s <stack>

# With auto-approve (use with extreme caution)
atmos terraform destroy vpc -s dev -auto-approve

# Targeted destroy
atmos terraform destroy vpc -s dev -target=aws_instance.web
```

### init

Initializes the Terraform working directory. Atmos runs this automatically before plan, apply, and deploy,
so manual invocation is rarely needed.

```shell
atmos terraform init <component> -s <stack>

# Reconfigure the backend
atmos terraform init vpc -s dev -reconfigure

# Upgrade provider plugins
atmos terraform init vpc -s dev -upgrade

# Migrate state between backends
atmos terraform init vpc -s dev -migrate-state
```

## Multi-Component Operations

Atmos supports executing Terraform commands across multiple components simultaneously using filter flags.
These work with `plan`, `apply`, and `deploy`.

```shell
# All components in all stacks
atmos terraform plan --all

# All components in a specific stack
atmos terraform plan --stack prod

# Specific components across stacks
atmos terraform deploy --components vpc,eks

# Only components affected by git changes (in dependency order)
atmos terraform deploy --affected

# Affected with dependents included
atmos terraform deploy --affected --include-dependents

# Filter by YQ query expression
atmos terraform plan --query '.vars.tags.team == "eks"'

# Combine filters
atmos terraform plan --affected --stack prod

# Always preview first with --dry-run
atmos terraform deploy --all --dry-run
```

## Workspace Management

Atmos calculates Terraform workspace names from stack context variables and automatically manages workspace
selection. When you run any terraform command, Atmos:

1. Computes the workspace name from context (namespace, tenant, environment, stage, component)
2. Runs `terraform init -reconfigure`
3. Selects the workspace with `terraform workspace select` (or creates it with `terraform workspace new`
   if it does not exist)

You can also manage workspaces explicitly:

```shell
atmos terraform workspace vpc -s plat-ue2-dev
```

### Workspace Key Prefix

The recommended approach for stable workspace keys uses `metadata.name`:

```yaml
components:
  terraform:
    vpc/defaults:
      metadata:
        type: abstract
        name: vpc              # Stable identity
        component: vpc/v2      # Implementation can change
```

For advanced dynamic naming, `workspace_key_prefix` supports Go templates:

```yaml
components:
  terraform:
    vpc:
      backend:
        workspace_key_prefix: "{{.vars.namespace}}-{{.vars.environment}}-{{.vars.stage}}"
```

Configure workspace behavior in `atmos.yaml`:

```yaml
components:
  terraform:
    init_run_reconfigure: true
    workspaces_enabled: true
```

## Backend Configuration and Auto-Generation

Atmos reads `backend_type` and `backend` settings from the stack configuration and auto-generates
a `backend.tf.json` file in the component directory before running `terraform init`.

### Enabling Auto-Generation

```yaml
# atmos.yaml
components:
  terraform:
    auto_generate_backend_file: true
```

### Stack-Level Backend Config

Backend settings cascade through the stack hierarchy:

```yaml
# Organization defaults (stacks/orgs/acme/_defaults.yaml)
terraform:
  backend_type: s3
  backend:
    s3:
      bucket: acme-ue1-root-tfstate
      region: us-east-1
      encrypt: true
      use_lockfile: true

# Component override (stacks/orgs/acme/plat/prod/us-east-1.yaml)
components:
  terraform:
    special-component:
      backend_type: s3
      backend:
        s3:
          bucket: acme-ue1-prod-special-tfstate
          key: "special/terraform.tfstate"
```

### Generated File

The generated `backend.tf.json` looks like:

```json
{
  "terraform": {
    "backend": {
      "s3": {
        "bucket": "acme-ue1-root-tfstate",
        "region": "us-east-1",
        "encrypt": true,
        "use_lockfile": true,
        "key": "ue1/prod/vpc/terraform.tfstate"
      }
    }
  }
}
```

Add `backend.tf.json` to `.gitignore` since it is generated automatically.

### Manual Generation

```shell
atmos terraform generate backend vpc -s plat-ue2-dev
```

## Variable File Generation

Atmos generates `terraform.tfvars.json` from the `vars` section in the stack configuration. This happens
automatically before plan/apply/deploy, but can also be invoked manually:

```shell
atmos terraform generate varfile vpc -s plat-ue2-dev

# Output to a custom file
atmos terraform generate varfile vpc -s plat-ue2-dev -f vars.json
```

### Planfile Generation

Generate planfiles in JSON or YAML format for review or integration with tools like Checkov:

```shell
atmos terraform generate planfile vpc -s plat-ue2-dev
atmos terraform generate planfile vpc -s dev --format=json
atmos terraform generate planfile vpc -s dev --format=yaml --file=planfile.yaml
```

## Authentication Configuration

Atmos integrates authentication so components can automatically use the correct credentials.

### Global Auth in atmos.yaml

```yaml
auth:
  providers:
    acme-sso:
      kind: aws-sso
      start_url: https://acme.awsapps.com/start
      region: us-east-1

  identities:
    prod-admin:
      kind: aws
      via:
        provider: acme-sso
      principal:
        name: AdministratorAccess
        account:
          id: "333333333333"
      default: true
```

### Component-Level Identity

```yaml
components:
  terraform:
    vpc:
      auth:
        identity: prod-admin
```

### Runtime Override

```shell
# Use a specific identity
atmos terraform plan vpc -s prod --identity prod-admin

# Skip authentication
atmos terraform plan vpc -s dev --identity ""
```

## Backend Provisioning

Set `provision.backend.enabled: true` in stack config to auto-provision backend infrastructure
(S3 buckets, etc.), solving the Terraform bootstrap problem. Manual provisioning is available via
`atmos terraform backend create/list/describe/update/delete`. See
[references/backend-configuration.md](references/backend-configuration.md) for details.

## Interactive Shell

The `shell` command drops you into a shell pre-configured with all the context for a component in a stack.
Varfiles, backend config, and workspace are all set up so you can run native Terraform commands directly.

```shell
atmos terraform shell vpc -s plat-ue2-dev
```

Inside the shell:
- The working directory is the component's folder
- `terraform.tfvars.json` and `backend.tf.json` are generated
- The correct workspace is selected
- All required ENV vars are set
- `ATMOS_SHLVL` tracks shell nesting level

Customize the shell prompt in `atmos.yaml`:

```yaml
components:
  terraform:
    shell:
      prompt: "atmos [{{.Stack}}] {{.Component}} $ "
```

## Additional Commands

Atmos supports `output`, `validate`, `state`, `clean`, `console`, `fmt`, `get`, `import`, `show`,
`taint`/`untaint`, `force-unlock`, `refresh`, `graph`, and `providers` -- all standard Terraform
subcommands with the same `atmos terraform <cmd> <component> -s <stack>` syntax. For the complete
reference, see [references/commands-reference.md](references/commands-reference.md).

```shell
# Common examples
atmos terraform output vpc -s dev vpc_id
atmos terraform state list vpc -s dev
atmos terraform clean vpc -s dev
```

## Common Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--stack` | `-s` | Target Atmos stack (required for single-component) |
| `--dry-run` | | Preview without executing |
| `--skip-init` | | Skip automatic `terraform init` |
| `--from-plan` | | Apply a previously generated planfile |
| `--all` | | Target all components |
| `--affected` | | Target git-affected components |
| `--identity` | | Override authentication identity |

Use `--` to pass flags directly to Terraform: `atmos terraform plan vpc -s dev -- -refresh=false`.
For the complete flag reference, see [references/commands-reference.md](references/commands-reference.md).

## Path-Based Component Resolution

You can use filesystem paths instead of component names:

```shell
cd components/terraform/vpc
atmos terraform plan . -s dev
atmos terraform apply . -s dev
```

Supported path formats: `.`, `./component`, `../sibling`, `/absolute/path`.
If a path matches multiple components, Atmos prompts for selection in interactive mode.

## Debugging

### Describe Component

Use `atmos describe component` to see the fully resolved configuration:

```shell
atmos describe component vpc -s plat-ue2-dev
```

This shows all merged vars, backend config, workspace name, metadata, and settings.

### Dry Run

Preview what Atmos will do without executing:

```shell
atmos terraform plan vpc -s dev --dry-run
```

### Terraform Debug Logging

```shell
export TF_LOG=DEBUG
atmos terraform plan vpc -s dev
```

## Configuration in atmos.yaml

Key settings under `components.terraform` include `auto_generate_backend_file`, `init_run_reconfigure`,
`workspaces_enabled`, `deploy_run_init`, `apply_auto_approve`, and `plan.skip_planfile`. Each has a
corresponding `ATMOS_COMPONENTS_TERRAFORM_*` environment variable override. See
[references/backend-configuration.md](references/backend-configuration.md) for complete configuration details.

## Best Practices

1. **Use the two-stage plan/apply workflow for production.** Run `plan` first, review the output, then
   `apply --from-plan` to ensure exactly the reviewed changes are applied.

2. **Use `deploy` for automated pipelines.** It combines plan and apply with auto-approve, ideal for CI/CD.

3. **Always preview multi-component operations with `--dry-run`** before executing `--all` or `--affected`.

4. **Let Atmos manage backend configuration.** Set `auto_generate_backend_file: true` and define backend
   settings in stack manifests rather than hardcoding in Terraform modules.

5. **Use `atmos describe component`** to debug configuration resolution issues. It shows the fully merged
   result of all stack manifest inheritance.

6. **Add generated files to .gitignore.** The `backend.tf.json` and `terraform.tfvars.json` files are
   generated at runtime and should not be committed.

7. **Use `atmos terraform shell`** for interactive debugging. It sets up the full context so you can
   run native terraform commands directly.

8. **Enable backend provisioning** (`provision.backend.enabled: true`) to solve the Terraform bootstrap
   problem and ensure backends exist before first use.

## Additional Resources

- For the complete list of all `atmos terraform` subcommands, see [references/commands-reference.md](references/commands-reference.md)
- For backend configuration patterns (S3, GCS, Azure, remote), see [references/backend-configuration.md](references/backend-configuration.md)
