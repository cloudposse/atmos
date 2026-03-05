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
- **Components** -- Terraform/Helmfile/Packer/Ansible implementations in `components/<type>/`. Reusable across stacks.
- **atmos.yaml** -- Project config: stack discovery paths, component paths, backend defaults, CLI settings.
- **Vendoring** -- Copies external components into the repo via `vendor.yaml` manifests for version control and
  auditability.
- **Authentication** -- Multi-provider auth system with SSO, SAML, OIDC, identity chaining, and keyring storage.
- **Stores** -- External key-value stores (SSM, Azure Key Vault, GCP Secret Manager, Redis, Artifactory) for
  cross-component data sharing.
- **Workflows** -- Multi-step sequences of Atmos and shell commands for cross-component orchestration.
- **Custom Commands** -- User-defined CLI commands in `atmos.yaml` that extend `atmos` with project-specific tooling.
- **Toolchain** -- Built-in CLI tool version management via Aqua registry integration and `.tool-versions` files.
- **Devcontainers** -- Native devcontainer management for standardized development environments (experimental).

## Key Commands

```text
atmos terraform plan <component> -s <stack>      # Plan a Terraform component
atmos terraform apply <component> -s <stack>     # Apply a Terraform component
atmos helmfile sync <component> -s <stack>       # Sync a Helmfile component (Kubernetes)
atmos packer build <component> -s <stack>        # Build a machine image
atmos ansible playbook <component> -s <stack>    # Run an Ansible playbook
atmos auth login                                 # Authenticate with configured provider
atmos describe stacks                            # Show resolved stack config
atmos describe component <comp> -s <stack>       # Show resolved component config
atmos describe affected                          # Detect changed stacks/components (CI/CD)
atmos validate stacks                            # Validate stack manifests against schema
atmos validate component <comp> -s <stack>       # Run validation policies
atmos vendor pull                                # Vendor external dependencies
atmos workflow <name> -f <file>                  # Run a workflow
atmos toolchain install                          # Install tools from .tool-versions
atmos list stacks                                # List all stacks
atmos list components                            # List all components
atmos devcontainer shell                         # Launch dev environment (experimental)
```

## Skill Index

When a task involves Atmos, activate the matching skill for detailed guidance.

| Task                                                                                                                  | Skill                   | Path                                                                  |
|-----------------------------------------------------------------------------------------------------------------------|-------------------------|-----------------------------------------------------------------------|
| atmos.yaml project config: all sections, discovery, merging, base paths, settings, imports, profiles                 | `atmos-config`          | `agent-skills/skills/atmos-config/SKILL.md`          |
| Stack YAML, imports, inheritance, deep merging, vars, settings, locals, metadata, overrides                          | `atmos-stacks`          | `agent-skills/skills/atmos-stacks/SKILL.md`          |
| Terraform root modules, abstract components, component inheritance, versioning, mixins, catalog patterns              | `atmos-components`      | `agent-skills/skills/atmos-components/SKILL.md`      |
| vendor.yaml manifests, pulling from Git/S3/HTTP/OCI/Terraform Registry, component.yaml                                | `atmos-vendoring`       | `agent-skills/skills/atmos-vendoring/SKILL.md`       |
| terraform plan/apply/deploy/destroy, workspace management, backend config, varfile generation                         | `atmos-terraform`       | `agent-skills/skills/atmos-terraform/SKILL.md`       |
| helmfile sync/apply/destroy/diff, Kubernetes deployments, EKS integration, varfile generation                         | `atmos-helmfile`        | `agent-skills/skills/atmos-helmfile/SKILL.md`        |
| packer init/build/validate/inspect/output, machine image building, template management                                | `atmos-packer`          | `agent-skills/skills/atmos-packer/SKILL.md`          |
| ansible playbook execution, variable passing, inventory management, configuration management                          | `atmos-ansible`         | `agent-skills/skills/atmos-ansible/SKILL.md`         |
| Multi-step workflows, Go template support in workflows, cross-component orchestration                                 | `atmos-workflows`       | `agent-skills/skills/atmos-workflows/SKILL.md`       |
| Custom CLI commands in atmos.yaml, arguments, flags, steps, env vars, subcommands                                     | `atmos-custom-commands` | `agent-skills/skills/atmos-custom-commands/SKILL.md` |
| Authentication: providers (SSO/SAML/OIDC/GCP), identities (AWS/Azure/GCP), keyring, login/exec/shell                  | `atmos-auth`            | `agent-skills/skills/atmos-auth/SKILL.md`            |
| Store backends (SSM, Azure Key Vault, GCP Secret Manager, Redis, Artifactory), hooks, data sharing                    | `atmos-stores`          | `agent-skills/skills/atmos-stores/SKILL.md`          |
| JSON Schema for stack manifests, IDE auto-completion, schema updates for new features, validation                     | `atmos-schemas`         | `agent-skills/skills/atmos-schemas/SKILL.md`         |
| GitHub Actions, Spacelift, Atlantis, `atmos describe affected`, PR-based plan/apply                                   | `atmos-gitops`          | `agent-skills/skills/atmos-gitops/SKILL.md`          |
| OPA/Rego policies, JSON Schema validation, `atmos validate component/stacks`                                          | `atmos-validation`      | `agent-skills/skills/atmos-validation/SKILL.md`      |
| YAML functions: !terraform.state, !terraform.output, !store, !store.get, !env, !exec, !include, !aws.*, !literal     | `atmos-yaml-functions`  | `agent-skills/skills/atmos-yaml-functions/SKILL.md`  |
| Go templates, Sprig/Gomplate functions, atmos.Component, atmos.GomplateDatasource, template configuration            | `atmos-templates`       | `agent-skills/skills/atmos-templates/SKILL.md`       |
| Design patterns: stack organization, component catalogs, inheritance, configuration composition, version management   | `atmos-design-patterns` | `agent-skills/skills/atmos-design-patterns/SKILL.md` |
| Toolchain management: install/exec/search tools, .tool-versions, Aqua registries, custom registries, aliases         | `atmos-toolchain`       | `agent-skills/skills/atmos-toolchain/SKILL.md`       |
| Introspection: describe component/stacks/affected/dependents, list stacks/components/instances, querying, provenance | `atmos-introspection`   | `agent-skills/skills/atmos-introspection/SKILL.md`   |
| Devcontainers: start/stop/attach/exec/shell, Docker/Podman, identity integration, instance management (experimental) | `atmos-devcontainer`    | `agent-skills/skills/atmos-devcontainer/SKILL.md`    |

## Common Patterns

- **Stack naming**: `{tenant}-{stage}` or `{org}-{tenant}-{account}-{region}`, configured via `name_pattern` or
  `name_template` in `atmos.yaml`.
- **Inheritance**: Use `_defaults.yaml` files at each directory level for shared config; deeper files override shallower
  ones.
- **Component reuse**: Define abstract components with `metadata.type: abstract`, inherit with `metadata.component` and
  `metadata.inherits`.
- **Cross-stack references**: Use `!terraform.output` YAML function or `{{ atmos.Component }}` Go template to read
  outputs from other components.
- **Data sharing via stores**: Write outputs to stores with hooks after apply, read them with `!store` YAML function.
- **Authentication**: Configure providers and identities in `atmos.yaml`, use `atmos auth login` to authenticate,
  `atmos auth shell` for authenticated sessions.
- **Validation before apply**: Attach JSON Schema or OPA policies via `settings.validation` in stack manifests; runs
  automatically before plan/apply.
- **Schema validation**: Use `atmos validate stacks` to validate manifests against the Atmos JSON Schema.
- **Introspection**: Use `atmos describe component` and `atmos list stacks/components` to query the project before
  generating configuration -- never guess at stack names or component configs.
- **Toolchain**: Declare tool versions in `.tool-versions`, configure registries in `atmos.yaml`, run
  `atmos toolchain install` to set up the project.
