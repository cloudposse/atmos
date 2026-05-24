# atmos.yaml Section Routing Reference

Use this reference to choose the right skill for a top-level `atmos.yaml` section. Keep detailed
syntax and examples in the owning skill.

## Root Mechanics

| Section | Purpose | Owner |
|---|---|---|
| `base_path` | Root for resolving relative project paths | `atmos-project-layout` |
| `import` | Modular root configuration imports | `atmos-config`, `atmos-project-layout` |
| `version` | Atmos version requirements | `atmos-settings` |

## Project Structure

| Section | Purpose | Owner |
|---|---|---|
| `stacks` | Stack manifest discovery, naming, included/excluded paths | `atmos-stacks` |
| `components` | Component type paths and component runner defaults | `atmos-components`, plus the relevant component skill |
| `workflows` | Workflow file discovery path | `atmos-workflows` |
| `schemas` | JSON Schema, OPA, CUE schema base paths | `atmos-schemas`, `atmos-validation` |

## Execution Features

| Section | Purpose | Owner |
|---|---|---|
| `commands` | Custom CLI command definitions | `atmos-custom-commands` |
| `aliases` | Command aliases | `atmos-custom-commands` |
| `templates` | Go template and Gomplate processing | `atmos-templates` |
| `validate` | Validation behavior such as EditorConfig | `atmos-validation` |
| `toolchain` | Tool registries, aliases, versions, install path | `atmos-toolchain` |
| `dependencies` | Tool and component dependencies | `atmos-toolchain`, `atmos-components` |

## Platform Integrations

| Section | Purpose | Owner |
|---|---|---|
| `auth` | Providers, identities, keyring, auth integrations | `atmos-auth` |
| `stores` | External key-value stores | `atmos-stores` |
| `integrations` | Atlantis, GitHub Actions, Atmos Pro integration settings | `atmos-ci` |
| `ci` | Native CI outputs, summaries, checks, comments | `atmos-ci` |
| `vendor` | Vendoring external components | `atmos-vendoring` |
| `devcontainer` | Development containers | `atmos-devcontainer` |
| `ai` / `mcp` | AI providers, skills, MCP server/client setup | `atmos-ai` |

## Global Behavior

| Section | Purpose | Owner |
|---|---|---|
| `profiles` | Profile directories and activation | `atmos-profiles` |
| `settings` | Global CLI behavior and feature toggles | `atmos-settings` |
| `logs` | Logging level and output | `atmos-settings` |
| `errors` | Error rendering and Sentry options | `atmos-settings` |
| `env` | Global environment variables | `atmos-settings` |
| `docs` | Documentation generation settings | `atmos-settings` |
| `metadata` | Project metadata | `atmos-settings` |
| `describe` | Describe command behavior | `atmos-introspection` |

When a task spans multiple sections, start with the most specific owner. Return to `atmos-config`
only for root discovery, merge, import, or bootstrap questions.
