---
name: atmos-terraform
description: "Terraform and OpenTofu orchestration: plan/apply/deploy, workspace management, backend config, varfile generation, authentication, binary selection (terraform/tofu), mixed-binary setups"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Terraform and OpenTofu Orchestration

Atmos wraps the Terraform or OpenTofu CLI to provide stack-aware orchestration of infrastructure operations.
Instead of manually managing workspaces, backends, variable files, and authentication for each component, Atmos
resolves the full configuration from stack manifests and handles all of these concerns automatically.

Everything in this skill applies identically to **Terraform** and **OpenTofu**. The `atmos terraform` command
namespace is the same regardless of which binary is configured -- `atmos terraform plan` runs `tofu plan` when
the binary is set to `tofu`. Use the user's terminology in responses (if they say "OpenTofu," say "OpenTofu").

## Terraform or OpenTofu: Binary Selection

Atmos defaults to the `terraform` binary. Switch to OpenTofu by setting `components.terraform.command: tofu`
in `atmos.yaml`. The setting cascades through CLI > env > config > defaults precedence and can be overridden
at multiple levels for mixed setups.

### Global (whole project on OpenTofu)

```yaml
# atmos.yaml
components:
  terraform:
    command: tofu                # All atmos terraform commands invoke `tofu` instead of `terraform`
    base_path: components/terraform
```

Or via environment variable:

```bash
export ATMOS_COMPONENTS_TERRAFORM_COMMAND=tofu
atmos terraform plan vpc -s dev   # Runs: tofu plan
```

### Per-stack override

Use `terraform.overrides.command` in a stack manifest to switch the binary for everything in that stack:

```yaml
# stacks/orgs/acme/plat/prod/_defaults.yaml
terraform:
  overrides:
    command: tofu
```

### Per-component override

Set `command` on an individual component to run a single component on a different binary than the rest of
the stack (useful for legacy components that haven't been validated on OpenTofu, or new components testing
OpenTofu-specific features):

```yaml
components:
  terraform:
    legacy-vpc:
      command: terraform           # This component stays on Terraform
      vars: ...
    new-eks:
      command: tofu                # This component runs on OpenTofu
      vars: ...
```

### Per-invocation override

Pass `--terraform-command` on the CLI to override for a single command:

```bash
atmos terraform plan vpc -s dev --terraform-command=tofu
```

### Pinning the binary version

The Atmos toolchain (`atmos-toolchain` skill) manages Terraform and OpenTofu versions separately:

```yaml
# stacks/.../_defaults.yaml
dependencies:
  tools:
    terraform: "1.9.8"
    opentofu: "1.8.0"
```

Or via `.tool-versions` at the project root:

```text
terraform 1.9.8
opentofu 1.8.0
```

Then `atmos toolchain install` provisions both. When Atmos invokes the configured binary (`terraform` or
`tofu`), it uses the pinned version. See the [atmos-toolchain](../atmos-toolchain/SKILL.md) skill for full
toolchain semantics.

### OpenTofu-specific considerations

- **State encryption** (OpenTofu 1.7+) -- a Terraform-incompatible feature; if enabled, state can no longer
  be read by `terraform`. Switching back is not zero-effort.
- **`removed` blocks** -- supported by both binaries in recent versions; no Atmos-side difference.
- **Provider/module registry** -- OpenTofu uses `registry.opentofu.org` by default; pinned modules sourced
  from `registry.terraform.io` still work in OpenTofu but consult the relevant registry availability when
  the user reports a missing module.
- **`terraform.required_version`** -- OpenTofu respects this constraint; pin appropriately.
- **`terraform { backend ... }` block** -- identical syntax in both binaries; no Atmos-side change needed.

For users running mixed Terraform/OpenTofu deployments, the per-component or per-stack override pattern is
the right tool. Do not propose a project-wide switch unless the user has validated all components against
OpenTofu.

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

Terraform commands can use Atmos auth identities from `atmos.yaml` or component-level `auth.identity`.
Keep detailed provider/profile setup in `atmos-auth`; this skill should only recommend the Terraform
runtime controls:

```shell
atmos terraform plan vpc -s prod --identity prod-admin
atmos terraform plan vpc -s dev --identity ""
```

## Backend Provisioning

Backend configuration and backend provisioning are different:

- `backend_type` and `backend` tell Terraform where state lives and generate `backend.tf.json`.
- `provision.backend.enabled: true` tells Atmos to create the backend storage before first use.

Set `provision.backend.enabled: true` in stack config to auto-provision backend infrastructure,
solving the Terraform bootstrap problem. Manual provisioning is available via
`atmos terraform backend create/list/describe/update/delete`. Backend provisioning currently applies
to Terraform components. See
[references/backend-configuration.md](references/backend-configuration.md) for details.

## Source Provisioning and Workdirs

Use `source` for just-in-time component provisioning and pair it with `provision.workdir` for isolated
per-instance execution. Workdirs prevent shared `.terraform`, lockfile, backend, and varfile collisions
when the same component source is used by multiple stacks or runs.

```yaml
components:
  terraform:
    vpc:
      source:
        uri: github.com/cloudposse/terraform-aws-components//modules/vpc
        version: 1.450.0
      provision:
        workdir:
          enabled: true
```

With workdirs enabled, Atmos stages the provisioned source into the instance workdir and runs Terraform
there. Without workdirs, source provisioning targets the component path.

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

9. **Use source provisioning with workdirs** when components are pulled via `source`, especially in CI
   or any multi-stack workflow that can run concurrently.

## Additional Resources

- For the complete list of all `atmos terraform` subcommands, see [references/commands-reference.md](references/commands-reference.md)
- For backend configuration patterns (S3, GCS, Azure, remote), see [references/backend-configuration.md](references/backend-configuration.md)
