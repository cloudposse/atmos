---
name: atmos-stacks
description: "Stack configuration: local and remote imports, inheritance, deep merging, locals, vars, settings, metadata, overrides, dependencies, and modernization"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
  category: core-config
---

# Atmos Stack Configuration

Stacks are YAML configuration files that define which components to deploy, with what settings, and how they relate to each other. They separate configuration from infrastructure code, enabling the same Terraform root modules to be deployed across many environments with different settings.

## What Stacks Are

A stack manifest is a YAML file that declares components and their configuration for a specific combination of organization, tenant, account, region, and stage. Atmos discovers stack manifests based on `included_paths` and `excluded_paths` in `atmos.yaml`, then deep-merges all imported configurations to produce the final resolved state for each component.

Stacks are not Terraform workspaces, although Atmos derives workspace names from stack names. A single stack manifest can configure multiple components, and a single component can appear across many stacks with different variable values.

## Related Skills

| Need | Load |
|---|---|
| Local and remote import mechanics | [atmos-imports](../atmos-imports/SKILL.md) |
| Modernizing legacy stack patterns | [atmos-modernization](../atmos-modernization/SKILL.md) |
| YAML function inventory | [atmos-yaml-functions](../atmos-yaml-functions/SKILL.md) |
| Component source provisioning | [atmos-components](../atmos-components/SKILL.md) |

## Stack Discovery

Atmos discovers stack manifests based on `included_paths` and `excluded_paths` configured in the `stacks` section
of `atmos.yaml`. For root config discovery and routing, see the `atmos-config` skill; for path layout
and `base_path` behavior, see `atmos-project-layout`.

### Stack Name Precedence

Atmos resolves the stack name using this priority (highest first):

1. `name` field in the stack manifest (explicit override).
2. `name_template` in `atmos.yaml` (Go template).
3. Legacy `name_pattern` in `atmos.yaml` (migration-only; recommend `name_template` or explicit `name`).
4. File basename (e.g., `prod.yaml` becomes `prod`).

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
  # Remote imports are supported, but they import stack config only:
  # - github://acme/config/main/stacks/catalog/vpc.yaml

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

Integration metadata and configuration not passed to Terraform. Used for Atlantis, validation, and other Atmos integrations.

```yaml
settings:
  validation:
    check-cidr:
      schema_type: jsonschema
      schema_path: schemas/vpc.json
dependencies:
  components:
    - component: vpc
    - component: dns-zone
      stack: plat-ue2-prod
    - kind: file
      path: configs/service.yaml
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

### flags

Default values for terraform CLI execution flags (`lock_timeout`, `lock`, `parallelism`,
`refresh`, `compact_warnings`). Settable in `atmos.yaml` (`components.terraform.flags`),
at the stack level (root-level `terraform:` block), and per component — merged
field-by-field, lowest to highest precedence. An explicit CLI-typed flag (`-- -lock-timeout=30s`)
always wins over all three. Terraform-only; not every flag applies to every subcommand.

```yaml
terraform:
  flags:
    lock_timeout: "5m"

components:
  terraform:
    vpc:
      flags:
        parallelism: 4
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
    validation:
      check-cidr:
        schema_path: schemas/vpc-override.json
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

```text
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

## Stack Lifecycle Commands

Three commands manage whole stacks. Use `--dry-run` first to preview selection, commands, and hooks.

**Safety rule for agents:** always run `atmos stack destroy ... --dry-run` first and show the user the plan. Never run `stack destroy` without `--dry-run`, and never add `--auto-approve`, unless the user has explicitly confirmed that exact destroy in this conversation.

```bash
# Create a manifest from a scaffold (never overwrites an existing file).
atmos stack create --file=stacks/deploy/dev.yaml --name=dev --import=catalog/defaults --component=terraform:network --dry-run

# Deploy or destroy components across component types in dependency order.
atmos stack deploy --all --stack=dev --dry-run
atmos stack deploy --affected --base=origin/main --include-dependencies=2
atmos stack destroy --all --stack=dev --dry-run
# Only after the user explicitly confirms the destroy:
atmos stack destroy --all --stack=dev --max-concurrency=2 --auto-approve
```

Rules for `stack deploy` and `stack destroy`:

- Select with `--all` or `--affected` (not both), or with `--tags` (any match on `metadata.tags`) or `--labels` (all `key=value` pairs match `metadata.labels`). A tag or label filter alone selects every matching component; with `--all`/`--affected` it narrows the selection, and dependency expansion ignores it. Positional component arguments are rejected; for one component use `atmos <kind> deploy <component> --stack=<stack>`.
- Comparison flags (`--base`, `--ref`, `--sha`, `--repo-path`, `--ssh-key`, `--ssh-key-password`, `--clone-target-ref`) require `--affected`.
- A `--stack` that names no existing stack is an error.
- `--include-dependencies` and `--include-dependents` expand without a limit when bare; use the `=N` form to limit depth.
- Supported kinds: Terraform, Helmfile (deploy runs `helmfile apply`, which needs the helm-diff plugin), Helm, Kubernetes, AWS CloudFormation, and custom kinds whose custom commands define `deploy`/`destroy` with `component.type`. `--affected` does not select custom kinds yet. Packer and kinds without a handler are excluded with a notice.
- Locked components (`metadata.locked`) are excluded. A selected component whose required dependency has no handler fails before execution. Cycles are reported as readable paths.
- `destroy` runs in reverse dependency order and fails before execution when a component outside the selection still requires a destroyed one. Add `--include-dependents` or widen `--stack`. Consumers of kinds without a `destroy` operation (such as Packer) do not block; they are reported in a notice.
- `--auto-approve` passes each handler's approval arguments. Terraform and CloudFormation destroy prompt otherwise; Helm, Kubernetes, and Helmfile deletes do not. Concurrent destroys that would prompt are rejected unless `--auto-approve` is set.
- `--log-order=grouped` applies when `--max-concurrency` is above 1.
- Only `ATMOS_STACK`, `ATMOS_IDENTITY`, `ATMOS_DRY_RUN`, `ATMOS_SKIP_HOOKS`, `ATMOS_TAGS`, `ATMOS_LABELS`, and `ATMOS_CI`/`CI` apply; variables such as `ATMOS_TERRAFORM_*_ALL` do not change selection.
- Output is one success, failure, or skipped line per component and a tally such as `3 succeeded, 0 failed, 0 skipped, 1 excluded`.

Rules for `stack create`:

- `--file` is required and resolves from the project base path. `--import` and `--component=kind:name` are repeatable; `--set key=value` supplies scaffold answers; `--ref` applies only to Git template sources.
- The template must consume `--name`, `--import`, and `--component` (`stack_name`, `stack_imports`, `stack_components`) or creation fails.
- Before any hook or write, Atmos processes the existing manifests with every generated file. It fails on an unresolvable stack name (pass `--name` or set the variables `stacks.name_template` uses), a missing import, a duplicate stack name, or an invalid generated file.
- Existing files are never overwritten; edit with `atmos stack config set` or delete the file first.
- `--dry-run` prints each file as `# <path>` followed by its content, separated by `---`, and lists scaffold hooks. Scaffold hooks receive `ATMOS_HOOK_STATUS`, `ATMOS_HOOK_EXIT_CODE`, and `ATMOS_HOOK_ERROR`.

Component hooks fire as usual during `stack deploy` and `stack destroy`; for example, Helmfile deploy fires `before.helmfile.apply` and `after.helmfile.apply`.

## YAML Functions

Atmos provides YAML functions for dynamic value resolution at runtime:

- `!terraform.output <component> <output>` -- Read Terraform outputs from another component.
- `!terraform.state <component> <path>` -- Access Terraform state values.
- `!store <store-name> <component> <key>` -- Read from external key-value stores (SSM, Vault, etc.).
- `!env <VAR_NAME>` -- Read environment variables with optional defaults.
- `!exec <command>` -- Execute shell commands and use output.
- `!include <path>` -- Load content from external files.
- `!secret <name>` -- Resolve declared secrets.
- `!append` and `!unset` -- Control inherited list/key merge behavior.

For the full current function inventory, load `atmos-yaml-functions`.

## Common Patterns and Best Practices

1. **Organize by org/tenant/stage/region**: Structure stacks hierarchically so defaults cascade naturally through imports.
2. **Use catalog for component defaults**: Place reusable component configurations in `stacks/catalog/` and import them.
3. **Keep inheritance shallow**: Limit to 2-3 levels of `metadata.inherits` to maintain readability.
4. **Use `_defaults.yaml` at every level**: Define shared vars, env, and settings at the appropriate organizational level.
5. **Exclude non-deployable files**: Configure `excluded_paths` to prevent catalog, mixin, and defaults files from being treated as top-level stacks.
6. **Use `name` or `name_template` for stack naming**: If legacy `name_pattern` is present, migrate it.
7. **Use `atmos describe stacks` liberally**: Always verify the resolved configuration before applying changes.
8. **Treat remote imports as config only**: use component `source:` or `atmos vendor pull` when imported
    config references component code that is not already local.

## References

- [references/import-patterns.md](references/import-patterns.md) -- Detailed import system: path resolution, remote imports, Go templates, context variables
- [references/inheritance-deep-merge.md](references/inheritance-deep-merge.md) -- Deep-merge algorithm, component inheritance, override precedence
