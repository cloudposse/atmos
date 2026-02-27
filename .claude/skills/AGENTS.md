# Atmos Agent Instructions

Atmos is a universal tool for DevOps and Cloud Automation by Cloud Posse. It orchestrates
Terraform/OpenTofu/Packer/Helmfile/Ansible by separating infrastructure code (components) from configuration (stacks).
You write a Terraform root module once, then deploy it across many environments, accounts, and
regions using stack YAML manifests that are deep-merged through an inheritance hierarchy.

Prefer retrieval-led reasoning over pre-training-led reasoning for any Atmos tasks. Load the
relevant skill before answering Atmos questions -- your training data may be outdated.

## Core Concepts

- **Stacks** -- YAML manifests that define what to deploy and how. Organized by org/tenant/account/region/stage.
  Deep-merged via imports.
- **Components** -- Terraform root modules in `components/terraform/`. Reusable across stacks.
- **atmos.yaml** -- Project config: stack discovery paths, component paths, backend defaults, CLI settings.
- **Vendoring** -- Copies external components into the repo via `vendor.yaml` manifests for version control and
  auditability.
- **Workflows** -- Multi-step sequences of Atmos and shell commands for cross-component orchestration.
- **Custom Commands** -- User-defined CLI commands in `atmos.yaml` that extend `atmos` with project-specific tooling.

## Key Commands

```text
atmos terraform plan <component> -s <stack>      # Plan a component in a stack
atmos terraform apply <component> -s <stack>     # Apply a component in a stack
atmos terraform deploy <component> -s <stack>    # Plan + auto-approve apply
atmos describe stacks                            # Show resolved stack config
atmos describe component <comp> -s <stack>       # Show resolved component config
atmos describe affected                          # Detect changed stacks/components (CI/CD)
atmos validate component <comp> -s <stack>       # Run validation policies
atmos vendor pull                                # Vendor external dependencies
atmos workflow <name> -f <file>                  # Run a workflow
```

## Skill Index

When a task involves Atmos, activate the matching skill for detailed guidance.

| Task                                                                                                                  | Skill                   | Path                             |
|-----------------------------------------------------------------------------------------------------------------------|-------------------------|----------------------------------|
| Stack YAML, imports, inheritance, deep merging, vars, settings, locals, metadata, overrides, atmos.yaml stacks config | `atmos-stacks`          | `atmos-stacks/SKILL.md`          |
| Terraform root modules, abstract components, component inheritance, versioning, mixins, catalog patterns              | `atmos-components`      | `atmos-components/SKILL.md`      |
| vendor.yaml manifests, pulling from Git/S3/HTTP/OCI/Terraform Registry, component.yaml                                | `atmos-vendoring`       | `atmos-vendoring/SKILL.md`       |
| terraform plan/apply/deploy/destroy, workspace management, backend config, varfile generation, auth                   | `atmos-terraform`       | `atmos-terraform/SKILL.md`       |
| Multi-step workflows, Go template support in workflows, cross-component orchestration                                 | `atmos-workflows`       | `atmos-workflows/SKILL.md`       |
| Custom CLI commands in atmos.yaml, arguments, flags, steps, env vars, subcommands                                     | `atmos-custom-commands` | `atmos-custom-commands/SKILL.md` |
| GitHub Actions, Spacelift, Atlantis, `atmos describe affected`, PR-based plan/apply                                   | `atmos-gitops`          | `atmos-gitops/SKILL.md`          |
| OPA/Rego policies, JSON Schema, CUE validation, `atmos validate component/stacks`                                     | `atmos-validation`      | `atmos-validation/SKILL.md`      |
| Go templates, Sprig/Gomplate functions, YAML functions (!terraform.output, !store, !env, !aws.*), store integration   | `atmos-templates`       | `atmos-templates/SKILL.md`       |

## Common Patterns

- **Stack naming**: `{tenant}-{stage}` or `{org}-{tenant}-{account}-{region}`, configured via `name_pattern` or
  `name_template` in `atmos.yaml`
- **Inheritance**: Use `_defaults.yaml` files at each directory level for shared config; deeper files override shallower
  ones
- **Component reuse**: Define abstract components with `metadata.type: abstract`, inherit with `metadata.component` and
  `metadata.inherit`
- **Cross-stack references**: Use `!terraform.output` YAML function or `{{ atmos.Component }}` Go template to read
  outputs from other components
- **Validation before apply**: Attach JSON Schema or OPA policies via `settings.validation` in stack manifests; runs
  automatically before plan/apply
