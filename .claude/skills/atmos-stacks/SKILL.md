---
name: atmos-stacks
description: "Stack configuration: imports, inheritance, deep merging, locals, vars, settings, metadata, overrides, atmos.yaml setup"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Stack Configuration

Stacks are YAML configuration files that define which components to deploy, with what settings, and how they relate to each other. They separate configuration from infrastructure code, enabling the same Terraform root modules to be deployed across many environments with different settings.

## What Stacks Are

A stack manifest is a YAML file that declares components and their configuration for a specific combination of organization, tenant, account, region, and stage. Atmos discovers stack manifests based on `included_paths` and `excluded_paths` in `atmos.yaml`, then deep-merges all imported configurations to produce the final resolved state for each component.

Stacks are not Terraform workspaces, although Atmos derives workspace names from stack names. A single stack manifest can configure multiple components, and a single component can appear across many stacks with different variable values.

## atmos.yaml Stacks Configuration

The `stacks` section in `atmos.yaml` controls how Atmos discovers and names stacks:

```yaml
# atmos.yaml
stacks:
  base_path: "stacks"
  included_paths:
    - "orgs/**/*"
  excluded_paths:
    - "**/_defaults.yaml"
    - "catalog/**/*"
    - "mixins/**/*"
  name_template: "{{ .vars.tenant }}-{{ .vars.environment }}-{{ .vars.stage }}"
```

### Key Settings

- `base_path`: Root directory for all stack manifests (default: `stacks`).
- `included_paths`: Glob patterns for top-level stack manifests. Only files matching these patterns are treated as deployable stacks.
- `excluded_paths`: Glob patterns to exclude from stack discovery. Use this to exclude catalog, mixin, and `_defaults.yaml` files that are only imported, never deployed directly.
- `name_template`: Go template that computes the stack name from merged vars. This is the recommended naming approach.
- `name_pattern`: Legacy token-based pattern (e.g., `{tenant}-{environment}-{stage}`). Superseded by `name_template`.

### Stack Name Precedence

Atmos resolves the stack name using this priority (highest first):

1. `name` field in the stack manifest (explicit override)
2. `name_template` in `atmos.yaml` (Go template)
3. `name_pattern` in `atmos.yaml` (token pattern)
4. File basename (e.g., `prod.yaml` becomes `prod`)

## Stack Manifest Structure

A stack manifest can contain the following top-level sections:

```yaml
# Optional: explicit stack name override
name: "plat-ue2-prod"

# Import other configurations
import:
  - catalog/vpc/defaults
  - mixins/region/us-east-2
  - orgs/acme/plat/prod/_defaults

# Global-scope sections (apply to all components)
vars: {}
locals: {}
env: {}
settings: {}
hooks: {}
overrides: {}

# Component-type scope (apply to all components of that type)
terraform:
  vars: {}
  env: {}
  settings: {}
  hooks: {}
  backend_type: s3
  backend: {}
  providers: {}
  command: terraform
  overrides: {}

helmfile:
  vars: {}
  env: {}
  settings: {}
  hooks: {}
  command: helmfile
  overrides: {}

# Component definitions
components:
  terraform:
    vpc:
      metadata: {}
      vars: {}
      env: {}
      settings: {}
      hooks: {}
      backend_type: s3
      backend: {}
      providers: {}
      command: terraform
      auth: {}
  helmfile:
    echo-server:
      vars: {}
      env: {}
      settings: {}
```

## Configuration Sections Reference

### vars

Variables passed as inputs to Terraform, Helmfile, or Packer components. Defined at global, component-type, or component level. Deep-merged across all levels with component-level values taking precedence.

```yaml
vars:
  environment: prod
  region: us-east-1
  tags:
    Environment: Production
    ManagedBy: Atmos
```

Maps are recursively merged; lists are replaced (not appended).

### locals

File-scoped temporary variables for reducing repetition within a single YAML file. Locals do NOT inherit across file imports. They can reference each other using `{{ .locals.name }}` syntax with automatic dependency resolution.

```yaml
locals:
  namespace: acme
  name_prefix: "{{ .locals.namespace }}-{{ .vars.stage }}"

components:
  terraform:
    vpc:
      vars:
        name: "{{ .locals.name_prefix }}-vpc"
```

Locals can also access `settings`, `vars`, and `env` defined in the same file using `{{ .settings.key }}`, `{{ .vars.key }}`, and `{{ .env.KEY }}`.

### env

Environment variables set when executing components. Simple key-value pairs merged shallowly across levels.

```yaml
env:
  AWS_PROFILE: acme-prod
  TF_IN_AUTOMATION: "true"
```

### settings

Integration metadata and configuration not passed to Terraform. Used for Spacelift, Atlantis, validation, and other Atmos integrations.

```yaml
settings:
  spacelift:
    workspace_enabled: true
    autodeploy: false
  depends_on:
    - vpc
```

### metadata

Component-only section that controls Atmos behavior for that component. Cannot be used at global or component-type level.

```yaml
components:
  terraform:
    vpc:
      metadata:
        component: vpc           # Terraform root module path
        inherits:
          - vpc/defaults         # Inheritance chain
        type: abstract           # abstract or real (default)
        enabled: true            # Enable/disable component
        locked: false            # Prevent modifications
        terraform_workspace: "custom-ws"
        custom:
          owner: platform-team
```

### hooks

Lifecycle event handlers that execute actions at specific points (e.g., after `terraform apply`).

```yaml
hooks:
  store-outputs:
    events:
      - after-terraform-apply
    command: store
    name: prod/ssm
    outputs:
      vpc_id: .vpc_id
```

### command

Override the executable for a component type or specific component. Useful for OpenTofu, custom wrappers, or version-pinned binaries.

```yaml
terraform:
  command: tofu
```

### backend

Terraform backend configuration. Atmos generates `backend.tf.json` automatically.

```yaml
terraform:
  backend_type: s3
  backend:
    s3:
      bucket: acme-ue1-root-tfstate
      region: us-east-1
      encrypt: true
      use_lockfile: true
```

### providers

Terraform provider configuration. Atmos generates `providers_override.tf.json` automatically.

```yaml
terraform:
  providers:
    aws:
      region: us-east-1
      assume_role:
        role_arn: "arn:aws:iam::{{ .vars.account_id }}:role/TerraformRole"
```

### auth

Authentication configuration for cloud providers. Primarily defined in `atmos.yaml` but can be referenced at component level.

```yaml
components:
  terraform:
    vpc:
      auth:
        identity: prod-admin
```

### overrides

Scoped overrides that apply only to components defined in the current manifest and its imports (not to all components in the top-level stack). This is different from regular `vars`/`env`/`settings` which affect all components.

```yaml
overrides:
  env:
    TEST_ENV_VAR: "overridden-value"
  vars:
    custom_tag: override
  settings:
    spacelift:
      autodeploy: true
```

## Deep-Merge Behavior and Override Precedence

Atmos deep-merges configuration from multiple levels. The precedence order (lowest to highest priority):

1. Global scope (`vars:`, `env:`, `settings:`)
2. Component-type scope (`terraform.vars:`, `helmfile.env:`)
3. Base component defaults (via `metadata.inherits`, in list order)
4. Component-level scope (`components.terraform.<name>.vars:`)
5. Overrides (`overrides:`, `terraform.overrides:`)

For maps, keys are recursively merged with higher-priority values overriding lower-priority ones. For lists, the entire list at higher priority replaces the lower-priority list (lists are not appended).

## The _defaults.yaml Pattern

A common convention is to use `_defaults.yaml` files at each level of the directory hierarchy:

```
stacks/
  orgs/
    acme/
      _defaults.yaml            # Organization-wide defaults
      plat/
        _defaults.yaml          # Tenant defaults
        dev/
          _defaults.yaml        # Stage defaults
          us-east-2.yaml        # Top-level stack (deployable)
          us-west-2.yaml
        prod/
          _defaults.yaml
          us-east-2.yaml
          us-west-2.yaml
```

The underscore prefix ensures these files sort to the top of directory listings. They are excluded from stack discovery via `excluded_paths` and must be explicitly imported. Atmos has no special handling for `_defaults.yaml` -- it is purely a naming convention.

Each level imports the parent `_defaults.yaml` and adds its own defaults:

```yaml
# stacks/orgs/acme/plat/prod/_defaults.yaml
import:
  - orgs/acme/plat/_defaults

vars:
  stage: prod
  tags:
    Environment: Production
```

## Describing Stacks

Use `atmos describe stacks` to view the fully resolved configuration after all imports, inheritance, and overrides:

```bash
# View all stacks
atmos describe stacks

# Filter by stack
atmos describe stacks --stack plat-ue2-prod

# Filter by component and section
atmos describe stacks --components vpc --sections vars

# Output as JSON
atmos describe stacks --format json | jq '.["plat-ue2-prod"]'
```

Use `atmos describe component` for a single component:

```bash
atmos describe component vpc -s plat-ue2-prod
```

## YAML Functions

Atmos provides YAML functions for dynamic value resolution at runtime:

- `!terraform.output <component>/<output>` -- Read Terraform outputs from another component.
- `!terraform.state <component> <path>` -- Access Terraform state values.
- `!store <store-name> <component> <key>` -- Read from external key-value stores (SSM, Vault, etc.).
- `!env <VAR_NAME>` -- Read environment variables with optional defaults.
- `!exec <command>` -- Execute shell commands and use output.
- `!include <path>` -- Load content from external files.

## Common Patterns and Best Practices

1. **Organize by org/tenant/stage/region**: Structure stacks hierarchically so defaults cascade naturally through imports.
2. **Use catalog for component defaults**: Place reusable component configurations in `stacks/catalog/` and import them.
3. **Keep inheritance shallow**: Limit to 2-3 levels of `metadata.inherits` to maintain readability.
4. **Use `_defaults.yaml` at every level**: Define shared vars, env, and settings at the appropriate organizational level.
5. **Exclude non-deployable files**: Configure `excluded_paths` to prevent catalog, mixin, and defaults files from being treated as top-level stacks.
6. **Prefer `name_template` over `name_pattern`**: The Go template approach is more flexible and is the recommended method.
7. **Use `atmos describe stacks` liberally**: Always verify the resolved configuration before applying changes.

## References

- [references/import-patterns.md](references/import-patterns.md) -- Detailed import system: path resolution, remote imports, Go templates, context variables
- [references/inheritance-deep-merge.md](references/inheritance-deep-merge.md) -- Deep-merge algorithm, component inheritance, override precedence
