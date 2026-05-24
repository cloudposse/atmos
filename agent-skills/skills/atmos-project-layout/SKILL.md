---
name: atmos-project-layout
description: "Atmos project layout: base_path, relative path resolution, root stacks/components/workflows/schemas directories, atmos.d modular config, and repository path conventions"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Project Layout

Use this skill for repository layout, root paths, and how `atmos.yaml` paths resolve.

## Root Paths

`base_path` sets the root for most relative project paths. Keep it explicit when a repository does
not use the current directory as the Atmos root.

```yaml
base_path: ""

stacks:
  base_path: stacks

components:
  terraform:
    base_path: components/terraform

workflows:
  base_path: stacks/workflows

schemas:
  jsonschema:
    base_path: stacks/schemas/jsonschema
  opa:
    base_path: stacks/schemas/opa
```

## Conventional Layout

```text
.
  atmos.yaml
  atmos.d/
    stacks.yaml
    components.yaml
    auth.yaml
  components/
    terraform/
    helmfile/
    packer/
    ansible/
  stacks/
    catalog/
    workflows/
    orgs/
    schemas/
```

Use `atmos.d/` for modular root config when `atmos.yaml` gets too large. Use stack imports for stack
manifest inheritance and catalog composition; do not confuse root config imports with stack imports.

## Path Rules

- Root `base_path` controls how project paths resolve.
- Subsystem `base_path` values are normally relative to the root `base_path`.
- Workflow `--file` values are relative to `workflows.base_path`.
- Schema paths are relative to their schema base path unless an absolute path is used.
- Stack import paths are a stack-manifest concern; load `atmos-stacks` for stack inheritance details.

## Routing

| Need | Load |
|---|---|
| Config discovery, merge order, `import` of root config | `atmos-config` |
| Stack discovery, stack imports, stack naming | `atmos-stacks` |
| Component type directories and component metadata | `atmos-components` |
| Terraform backend and command-specific path behavior | `atmos-terraform` |
| Workflow file discovery | `atmos-workflows` |
| Schema directories and validation paths | `atmos-schemas`, `atmos-validation` |
| Profiles path and activation | `atmos-profiles` |

## Guardrails

- Do not force the canonical `components/terraform` layout during migrations; point `base_path` and
  component paths at the user's existing repository when that is safer.
- Prefer a small root `atmos.yaml` plus focused `atmos.d/*.yaml` files for large projects.
- Verify layout assumptions with `atmos describe config` before generating large changes.
