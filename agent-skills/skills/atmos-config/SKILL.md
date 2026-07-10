---
name: atmos-config
description: "Atmos root configuration: atmos.yaml discovery, precedence, deep merging, base_path, imports, minimal bootstrap, and routing to narrower Atmos skills"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
references:
  - references/sections-reference.md
---

# Atmos Root Configuration

Use this skill for the root mechanics of `atmos.yaml`: how Atmos finds config files, merges them,
resolves project-relative paths, imports modular config, and routes section-specific work to the
right skill. Do not use this skill as a catchall for every `atmos.yaml` section.

## Configuration Discovery

Atmos searches for `atmos.yaml` in this order:

1. `--config` CLI flag or `ATMOS_CLI_CONFIG_PATH`.
2. Active profile selected by `--profile` or `ATMOS_PROFILE`.
3. Current working directory.
4. Git repository root.
5. Parent directory walk.
6. Home directory.
7. System directory.

When multiple configuration files apply, Atmos deep-merges them. More specific sources override
broader defaults.

## Root Layout

Use `base_path` for the project root that relative paths resolve from. Keep the root config small
and point subsystem paths at their owning directories:

```yaml
base_path: ""

stacks:
  base_path: stacks
  included_paths:
    - "**/*"
  excluded_paths:
    - "**/_defaults.yaml"
    - "catalog/**/*"
  name_template: "{{ .vars.stage }}"

components:
  terraform:
    base_path: components/terraform

workflows:
  base_path: stacks/workflows
```

For deeper path/layout guidance, load [atmos-project-layout](../atmos-project-layout/SKILL.md).

## Modular Imports

Use `import` to split root config into focused files:

```yaml
import:
  - atmos.d/stacks.yaml
  - atmos.d/components.yaml
  - atmos.d/auth.yaml
  - atmos.d/toolchain.yaml
```

Imported files are deep-merged into the active configuration. Keep imported files aligned with the
subsystem they configure and load the subsystem skill before changing that section.

## Routing

| Need | Load |
|---|---|
| Root discovery, merge order, imports, minimal bootstrap | stay in `atmos-config` |
| Project paths, `base_path`, path conventions, relative path resolution | `atmos-project-layout` |
| Profiles, `--profile`, `ATMOS_PROFILE`, profile directory merge behavior | `atmos-profiles` |
| Global CLI behavior, `settings`, `logs`, `errors`, `env`, `docs`, `metadata` | `atmos-settings` |
| Stack manifests, inheritance, stack imports, vars, locals, stack naming | `atmos-stacks` |
| Component structure, abstract components, metadata, component inheritance | `atmos-components` |
| Terraform/OpenTofu commands, backend defaults, Terraform component settings | `atmos-terraform` |
| Helmfile, Packer, or Ansible component behavior | `atmos-helmfile`, `atmos-packer`, `atmos-ansible` |
| Workflows section and workflow syntax | `atmos-workflows` |
| Custom commands and aliases | `atmos-custom-commands` |
| Auth providers, identities, keyring, cloud auth conventions | `atmos-auth` |
| Stores and store-backed YAML functions | `atmos-stores` |
| Tool versions, `dependencies.tools`, registries, shell/PATH integration | `atmos-toolchain` |
| Native CI, GitHub Actions, Atlantis, matrices, CI outputs | `atmos-ci` |
| Schemas and validation policy configuration | `atmos-schemas`, `atmos-validation` |
| Templates and YAML functions | `atmos-templates`, `atmos-yaml-functions` |
| Vendoring external components | `atmos-vendoring` |
| Introspection commands and querying resolved config | `atmos-introspection` |

For a compact map of top-level sections, read [references/sections-reference.md](references/sections-reference.md).

## Guardrails

- Keep `atmos-config` examples minimal; detailed subsystem examples belong in their narrower skills.
- Before editing a subsystem section, load the owning skill from the routing table.
- Prefer `atmos describe config` or `atmos describe component` when verifying merge or path behavior.
