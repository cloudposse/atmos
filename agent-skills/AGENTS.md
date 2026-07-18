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
- **Atmos Pro** -- Control plane for affected-stack uploads, inventory, drift detection, workflow dispatch,
  stack locks, and GitHub App commits.
- **Remote Imports** -- Stack configuration can be imported from local files or remote sources (GitHub, Git,
  HTTP, S3, GCS, OCI); imports do not materialize component source code.
- **Stores** -- External key-value stores (SSM, Azure Key Vault, GCP Secret Manager, Redis, Artifactory) for
  cross-component data sharing.
- **Secrets** -- Declarative secret management via `secrets.vars`, `!secret`, and `atmos secret`.
- **Containers and Emulators** -- Stack-scoped container components and local cloud/API emulators.
- **Native Helm** -- Experimental Helm chart deployment via the Helm Go SDK (no `helm`/`helmfile` binary).
- **Native Kubernetes** -- Experimental manifest/Kustomize deployment via the Kubernetes Go SDK (no `kubectl`/`kustomize` binary).
- **Hooks** -- Lifecycle automation before/after component operations.
- **Steps** -- Shared typed actions used by workflows, custom commands, hooks, and cast recordings.
- **Compositions** -- Named service groupings that validate systems made of component instances.
- **GitOps** -- Native managed Git repositories, hooks, signed commits, and auth-aware clone/commit/push flows.
- **Workflows** -- Multi-step sequences of Atmos and shell commands for cross-component orchestration.
- **Custom Commands** -- User-defined CLI commands in `atmos.yaml` that extend `atmos` with project-specific tooling.
- **Toolchain** -- Built-in CLI tool version management via Aqua registry integration and `.tool-versions` files.
- **Version Tracker** -- Atmos-managed external versions, tracks, lock files, file managers, and CI verification.
- **AI and MCP** -- Native AI providers, AI-powered command analysis, MCP server/client integration, and agent
  workflows grounded in Atmos introspection.
- **Devcontainers** -- Native devcontainer management for standardized development environments (experimental).
- **Cast and Asciicast** -- Terminal session recording/rendering (`atmos cast play/render`, `--cast` flag,
  `type: cast`/`type: simulate` workflow steps, `.cast` demos) for docs screengrabs and CI proof-of-run artifacts.

## Key Commands

```text
atmos terraform plan <component> -s <stack>      # Plan a Terraform component
atmos terraform apply <component> -s <stack>     # Apply a Terraform component
atmos helmfile sync <component> -s <stack>       # Sync a Helmfile component (Kubernetes)
atmos packer build <component> -s <stack>        # Build a machine image
atmos ansible playbook <component> -s <stack>    # Run an Ansible playbook
atmos auth login                                 # Authenticate with configured provider
atmos pro lock --component vpc --stack prod      # Lock a stack/component through Atmos Pro
atmos pro commit -m "terraform fmt" --all        # Commit CI changes through the Atmos Pro GitHub App
atmos describe stacks                            # Show resolved stack config
atmos describe component <comp> -s <stack>       # Show resolved component config
atmos describe affected --upload                 # Detect changed stacks/components and upload to Atmos Pro
atmos list instances --upload                    # Upload instance inventory to Atmos Pro
atmos validate stacks                            # Validate stack manifests against schema
atmos validate component <comp> -s <stack>       # Run validation policies
atmos vendor pull                                # Vendor external dependencies
atmos workflow <name> -f <file>                  # Run a workflow
atmos version track status prod                  # Check managed external version status
atmos version track update prod                  # Advance locked versions within policy
atmos cast render demo.cast --output=demo.gif    # Render a cast recording to gif/mp4/html/ascii/png/jpg
atmos helm apply <component> -s <stack>          # Install or upgrade a native Helm release (experimental)
atmos kubernetes apply <component> -s <stack>    # Apply native Kubernetes manifests (experimental)
atmos container up <component> -s <stack>         # Start a stack-scoped container component
atmos emulator up <component> -s <stack>          # Start a local cloud/API emulator
atmos secret validate -c <component> -s <stack>   # Validate declared secrets are initialized
atmos git status <repo>                           # Inspect a managed Git repository
atmos composition validate <name> -s <stack>      # Validate composition service fulfillment
atmos toolchain install                          # Manual bootstrap/cache warm only; execution auto-installs declared tools
atmos ai ask "What stacks do we have?"           # Ask AI with Atmos tools and skills
atmos mcp export                                 # Export MCP server config for AI clients
atmos list stacks                                # List all stacks
atmos list components                            # List all components
atmos devcontainer shell                         # Launch dev environment (experimental)
```

## Skill Index

When a task involves Atmos, activate the matching skill for detailed guidance.

| Task                                                                                                                  | Skill                   | Path                                                                  |
|-----------------------------------------------------------------------------------------------------------------------|-------------------------|-----------------------------------------------------------------------|
| atmos.yaml root config: discovery, precedence, deep merging, imports, minimal bootstrap, routing                    | `atmos-config`          | `agent-skills/skills/atmos-config/SKILL.md`          |
| Project layout: base_path, relative paths, stacks/components/workflows/schemas directories, atmos.d config          | `atmos-project-layout`  | `agent-skills/skills/atmos-project-layout/SKILL.md`  |
| Profiles: profile directories, --profile, ATMOS_PROFILE, profile merge behavior, environment switching             | `atmos-profiles`        | `agent-skills/skills/atmos-profiles/SKILL.md`        |
| Stack YAML, imports, inheritance, deep merging, vars, settings, locals, metadata, overrides                          | `atmos-stacks`          | `agent-skills/skills/atmos-stacks/SKILL.md`          |
| Local and remote stack imports, go-getter schemes, templated imports, private GitHub imports with STS                | `atmos-imports`         | `agent-skills/skills/atmos-imports/SKILL.md`         |
| Terraform root modules, component source provisioning, abstract components, component inheritance, versioning, mixins | `atmos-components`      | `agent-skills/skills/atmos-components/SKILL.md`      |
| vendor.yaml manifests, checked-in copies from Git/S3/HTTP/OCI/Terraform Registry, component.yaml                     | `atmos-vendoring`       | `agent-skills/skills/atmos-vendoring/SKILL.md`       |
| Container components: Docker Compose migration, build/run/push/pull/up/down/logs/exec, stack-scoped containers        | `atmos-container`       | `agent-skills/skills/atmos-container/SKILL.md`       |
| Emulator components: AWS/GCP/Azure/Kubernetes/Vault/OpenBao/registry local emulators                                  | `atmos-emulator`        | `agent-skills/skills/atmos-emulator/SKILL.md`        |
| Compositions: named service groupings and `atmos composition validate`                                                | `atmos-compositions`    | `agent-skills/skills/atmos-compositions/SKILL.md`    |
| terraform plan/apply/deploy/destroy, workspace management, backend config, varfile generation                         | `atmos-terraform`       | `agent-skills/skills/atmos-terraform/SKILL.md`       |
| Terraform/OpenTofu linting: `atmos terraform lint`, TFLint configuration and rules, component toolchain pins, lifecycle hooks, SARIF CI findings | `atmos-lint` | `agent-skills/skills/atmos-lint/SKILL.md` |
| helmfile sync/apply/destroy/diff, Kubernetes deployments, EKS integration, varfile generation                         | `atmos-helmfile`        | `agent-skills/skills/atmos-helmfile/SKILL.md`        |
| Native Helm (experimental): Helm Go SDK template/diff/apply/delete, chart sources, values, repositories, provision targets | `atmos-helm`            | `agent-skills/skills/atmos-helm/SKILL.md`            |
| Native Kubernetes (experimental): Go SDK render/plan/diff/apply/deploy/delete/validate, kubectl/kustomize providers, provision targets | `atmos-kubernetes`      | `agent-skills/skills/atmos-kubernetes/SKILL.md`      |
| packer init/build/validate/inspect/output, machine image building, template management                                | `atmos-packer`          | `agent-skills/skills/atmos-packer/SKILL.md`          |
| ansible playbook execution, variable passing, inventory management, configuration management                          | `atmos-ansible`         | `agent-skills/skills/atmos-ansible/SKILL.md`         |
| Terminal asciicast demos for community-facing docs, examples, and training materials                                  | `atmos-asciicast`       | `agent-skills/skills/atmos-asciicast/SKILL.md`       |
| Shared step DSL: step types, env, output, working_directory, retry, script, workdir, cast, hook `with:` payloads     | `atmos-steps`           | `agent-skills/skills/atmos-steps/SKILL.md`           |
| Multi-step workflows, native step types, `when:` CEL conditions, `require`/`assert` preconditions, background steps, output/UI steps, cross-component orchestration | `atmos-workflows`       | `agent-skills/skills/atmos-workflows/SKILL.md`       |
| Cast recording/rendering: cast play/render, format inference, `--cast` flag, `type: cast`/`type: simulate` steps       | `atmos-cast`            | `agent-skills/skills/atmos-cast/SKILL.md`            |
| Custom CLI commands in atmos.yaml, arguments, flags, native steps, `when:` conditions, custom component types, env vars, subcommands | `atmos-custom-commands` | `agent-skills/skills/atmos-custom-commands/SKILL.md` |
| Authentication: providers, identities, keyring, login/exec/shell, Atmos Pro `github/sts`                              | `atmos-auth`            | `agent-skills/skills/atmos-auth/SKILL.md`            |
| Atmos Pro: setup, OIDC, uploads, workflow dispatch, locks, Pro commits, merge queues, drift detection                 | `atmos-pro`             | `agent-skills/skills/atmos-pro/SKILL.md`             |
| Store backends (SSM, Azure Key Vault, GCP Secret Manager, Redis, Artifactory), hooks, data sharing                    | `atmos-stores`          | `agent-skills/skills/atmos-stores/SKILL.md`          |
| Secrets: `secrets.vars`, `!secret`, backends, secret init/set/get/import/push/pull/shell/exec/validate                | `atmos-secrets`         | `agent-skills/skills/atmos-secrets/SKILL.md`         |
| Hooks: lifecycle events, hook kinds, `kind: step`/`kind: steps`, `when:` conditions, scoping, `--skip-hooks`, toolchain-aware checks and uploads | `atmos-hooks`           | `agent-skills/skills/atmos-hooks/SKILL.md`           |
| JSON Schema for stack manifests, IDE auto-completion, schema updates for new features, validation                     | `atmos-schemas`         | `agent-skills/skills/atmos-schemas/SKILL.md`         |
| Atmos CI: Native CI, GitHub Actions containers, collapsible log groups, Atlantis, affected/all matrices, OIDC profiles, cache, Pro dispatch | `atmos-ci`              | `agent-skills/skills/atmos-ci/SKILL.md`              |
| Cache: CI cache and Terraform registry cache                                                                         | `atmos-cache`           | `agent-skills/skills/atmos-cache/SKILL.md`           |
| OPA/Rego policies, JSON Schema validation, `atmos validate component/stacks`, generic `atmos validate schema` for any file against any JSON Schema | `atmos-validation`      | `agent-skills/skills/atmos-validation/SKILL.md`      |
| YAML functions: !terraform.state, !store, !secret, !emulator, !git.*, !include, !append, !unset, !aws.*, !literal    | `atmos-yaml-functions`  | `agent-skills/skills/atmos-yaml-functions/SKILL.md`  |
| Go templates, Sprig/Gomplate functions, atmos.Component, atmos.GomplateDatasource, template configuration            | `atmos-templates`       | `agent-skills/skills/atmos-templates/SKILL.md`       |
| Design patterns: stack organization, component catalogs, inheritance, configuration composition, version management   | `atmos-design-patterns` | `agent-skills/skills/atmos-design-patterns/SKILL.md` |
| Toolchain management: declarative dependencies, automatic installs, .tool-versions, Aqua registries, aliases         | `atmos-toolchain`       | `agent-skills/skills/atmos-toolchain/SKILL.md`       |
| Version Tracker: version tracks, lock files, managed external dependency versions, file managers, CI verification    | `atmos-version`         | `agent-skills/skills/atmos-version/SKILL.md`         |
| Introspection: describe component/stacks/affected/dependents, list stacks/components/instances, querying, provenance | `atmos-introspection`   | `agent-skills/skills/atmos-introspection/SKILL.md`   |
| Diagnostics: JSONL event streams for subprocess start/end/output and debugging execution                             | `atmos-diagnostics`     | `agent-skills/skills/atmos-diagnostics/SKILL.md`     |
| Global settings: settings, logs, errors, env, docs, metadata, version requirements, terminal color/theme          | `atmos-settings`        | `agent-skills/skills/atmos-settings/SKILL.md`        |
| GitOps: managed repositories, Git hooks, signed commits, clone/pull/status/diff/commit/push, fork-PR trust gate    | `atmos-git`             | `agent-skills/skills/atmos-git/SKILL.md`             |
| AI and MCP: providers, skills, agent workflows, MCP server/client setup, auth-wrapped tools, toolchain-aware export  | `atmos-ai`              | `agent-skills/skills/atmos-ai/SKILL.md`              |
| Devcontainers: start/stop/attach/exec/shell, Docker/Podman, identity integration, instance management (experimental) | `atmos-devcontainer`    | `agent-skills/skills/atmos-devcontainer/SKILL.md`    |
| AWS EKS: update kubeconfig, kubectl exec tokens, EKS auth integrations                                             | `atmos-aws-eks`         | `agent-skills/skills/atmos-aws-eks/SKILL.md`         |
| AWS ECR: registry login, ECR auth integrations, Docker credential writes                                          | `atmos-aws-ecr`         | `agent-skills/skills/atmos-aws-ecr/SKILL.md`         |
| AWS compliance: Security Hub standards, compliance reports, CIS AWS, PCI DSS, SOC2, HIPAA, NIST                   | `atmos-aws-compliance`  | `agent-skills/skills/atmos-aws-compliance/SKILL.md`  |
| AWS security: analyze findings, map to components/stacks, structured remediation                                  | `atmos-aws-security`    | `agent-skills/skills/atmos-aws-security/SKILL.md`    |
| Migrating to Atmos from native Terraform/OpenTofu or Terraform Workspaces: layout, workspace mapping, remote-state bridge | `atmos-migration`       | `agent-skills/skills/atmos-migration/SKILL.md`       |
| Atmos Modernization: replace deprecated patterns with current Atmos naming, CI, Pro, auth, secrets, and dependencies | `atmos-modernization`   | `agent-skills/skills/atmos-modernization/SKILL.md`   |

## Common Patterns

- **Stack naming**: use explicit stack `name` for exceptions or `stacks.name_template` for computed names.
  If legacy `name_pattern` is present, recommend migrating it to `name_template`.
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
- **CLI help**: `atmos <command> --help=usage` shows just the usage line and embedded invocation examples --
  the fastest way for an agent to learn correct syntax without wading through full flag docs. `--help=flags`
  shows command-specific flags only (no inherited/global flags); `--help=all` shows the complete page. See
  `atmos-introspection` for details.
- **Custom commands vs. workflows**: a custom command is a new named CLI verb (`atmos <name>`) a user invokes
  directly, best for a single reusable operation with its own arguments/flags. A workflow (`atmos workflow
  <name>`) is a multi-step sequence run by workflow name, best for cross-component orchestration where steps
  share context and DAG-style dependencies matter more than a clean CLI surface. If you're choosing between
  them for a new task: does it need its own flags/arguments and get invoked ad hoc? Custom command. Is it a
  fixed sequence of steps across one or more components/stacks? Workflow.
- **Toolchain**: Use `dependencies.tools` for stack/component/workflow/custom-command requirements and
  `.tool-versions` for repo-wide developer shell defaults. Atmos auto-installs and injects missing tools
  during execution; reserve `atmos toolchain install` for shell bootstrap, cache warming, or troubleshooting.
- **Version tracks**: use `version.dependencies`, `version.tracks`, and `versions.lock.yaml` for external
  versions Atmos must resolve, lock, apply to files, and verify. Use `atmos version track update` for
  policy-aware advancement and `atmos version track verify` in CI.
- **Workflow/custom command steps**: Prefer native typed steps over large inline shell scripts. Repeated
  `echo`, shell loops, ad hoc sleeps, and hand-formatted output usually mean the step should become
  `atmos`, `toast`, `table`, `parallel`, `matrix`, `wait`, `container`, `emulator`, `http`, or another
  typed step. Keep shell for short glue commands, external CLIs, terminal-native tools, or checked-in scripts.
- **AI/MCP**: Configure AI providers and MCP servers in `atmos.yaml`; use `atmos mcp export` so AI clients get
  auth-wrapped MCP commands and toolchain-aware `PATH` entries.
- **Dependencies**: use `dependencies.components` for component, file, and folder dependencies. Treat
  `settings.depends_on` as legacy migration syntax only.
- **Remote imports**: `import:` can reference local files or remote sources; remote imports pull stack
  configuration only. They do not vendor component source. Use component `source:` or `atmos vendor pull`
  when the referenced component code must be materialized.
- **Native CI**: use the Atmos container and direct `atmos` commands; replace deprecated
  `cloudposse/github-action-atmos*` wrapper actions with Native CI workflows. Prefer Atmos
  `dependencies.tools` over Terraform/OpenTofu setup actions such as `hashicorp/setup-terraform`
  or `opentofu/setup-opentofu`. When a CI step needs a tool, first declare it on the owning
  component, workflow, or custom command instead of adding a preinstall step.
- **Atmos Pro drift detection**: use `settings.pro.drift_detection.enabled` and `atmos terraform plan
  --upload-status`; do not build new scheduled drift workflows from deprecated wrapper actions.
- **GitHub STS**: for private GitHub `vendor`, component `source`, remote `import`, or Terraform modules
  in CI, configure `auth.providers.<name>.kind: atmos/pro` and `auth.integrations.<name>.kind: github/sts`.
- **Check for an existing subsystem before hand-rolling one**: this table is reactive -- it only helps once
  you already know what to search for. Before writing custom shell logic, a bespoke script, or a manual
  process, scan Core Concepts and the Skill Index above for a native feature that already does it. Atmos
  agents most often miss: toolchain-managed tool installs (`dependencies.tools`/`.tool-versions`, not a
  hand-installed binary), vendoring (`vendor.yaml`, not a manual copy), CI/Terraform-registry caching, custom
  tool registries, secrets/stores (not env files or plaintext config), Gomplate/Sprig template functions (not
  string concatenation), generic schema validation (`atmos validate schema` against any file/JSON Schema, not
  a hand-rolled linter), theming/color control (`atmos theme`, `--no-color`/`--force-color`, not raw ANSI
  codes), and the custom-commands-vs-workflows choice above. If a task feels repetitive or manual, assume
  Atmos already has a typed way to do it and check before writing new code.
