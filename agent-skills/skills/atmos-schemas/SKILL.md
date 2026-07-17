---
name: atmos-schemas
description: "JSON Schema for Atmos: stack-manifest and atmos.yaml config schemas, IDE auto-completion, validate stacks/schema/config, SchemaStore integration"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.1.0"
---

# Atmos JSON Schema System

## Overview

Atmos uses JSON Schema (Draft 2020-12) to validate configuration files, provide IDE
auto-completion, and catch configuration errors early. Two schemas cover the core file types:

1. **Stack manifest schema** -- Validates stack YAML manifests (`stacks/**`). Published at
   `https://atmos.tools/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json` and registered
   with SchemaStore as `https://json.schemastore.org/atmos-manifest.json`. Used by
   `atmos validate stacks` and by IDEs for auto-completion.

2. **CLI configuration schema** -- Validates `atmos.yaml` itself (every configuration section
   the CLI reads). Published at
   `https://atmos.tools/schemas/atmos/atmos-config/1.0/atmos-config.json`. Used by
   `atmos validate schema config` and by IDEs for auto-completion. This schema is generated
   from the Atmos configuration code, so it always matches the options the installed release
   actually supports -- including YAML function alternatives (like `logs: !include shared.yaml`)
   and descriptions for hover text.

Users can also validate arbitrary YAML files against arbitrary schemas via `schemas.<key>`
entries in `atmos.yaml` (see "Custom Schema Validation" below), and `vendor.yaml` manifests
are validated against a built-in vendor schema.

### Floating vs. Pinned Schema URLs

Both schemas are published in two forms:

- **Floating** -- `.../atmos-manifest/1.0/atmos-manifest.json` and
  `.../atmos-config/1.0/atmos-config.json` always reflect the latest Atmos release.
- **Pinned** -- `.../atmos-manifest/<atmos-version>/atmos-manifest.json` and
  `.../atmos-config/<atmos-version>/atmos-config.json` are immutable snapshots of the schema
  exactly as it shipped with that release. Pin when a team upgrades the Atmos binary on its own
  schedule and must not silently absorb schema changes. Pinned URLs only exist for versions
  released after pinning shipped; earlier releases only have the floating `1.0` path.

## Validating atmos.yaml (CLI Configuration)

Out of the box -- no configuration required -- `atmos validate schema` validates `atmos.yaml`
(including hidden `.atmos.yaml` variants), `atmos.d/**` fragments, and project-local profile
files against the built-in configuration schema:

```shell
atmos validate schema           # validates atmos.yaml, atmos.d/**, profiles/** (and everything in `schemas`)
atmos validate schema config    # validates only the atmos.yaml schema entry
atmos config validate           # alias for `atmos validate schema config`
```

Fragments and profiles are partial configurations; the schema requires no specific fields, so
they validate standalone.

Print the schema itself (for inspection, or to commit a copy into a repository):

```shell
atmos config schema                       # print to stdout
atmos config schema schemas/atmos-config.json   # write to a file
```

### Overriding the Built-In Entry

Define your own `schemas.config` entry to change the schema or the matched files:

```yaml
schemas:
  config:
    # Pin to a specific Atmos release's schema instead of the embedded one.
    schema: "https://atmos.tools/schemas/atmos/atmos-config/1.219.0/atmos-config.json"
    matches:
      - "atmos.yaml"
      - "atmos.d/**/*.yaml"
```

### Editor Integration for atmos.yaml

Add a `yaml-language-server` modeline at the top of `atmos.yaml` for auto-completion and
inline validation:

```yaml
# yaml-language-server: $schema=https://atmos.tools/schemas/atmos/atmos-config/1.0/atmos-config.json
base_path: "./"
```

## Validating Stack Manifests

### `atmos validate stacks`

Validates all stack manifests against the Atmos manifest JSON Schema:

```shell
# Use default embedded schema
atmos validate stacks

# Use local schema file
atmos validate stacks --schemas-atmos-manifest schemas/atmos/atmos-manifest/1.0/atmos-manifest.json

# Use remote schema
atmos validate stacks --schemas-atmos-manifest https://atmos.tools/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json
```

This command checks:
- YAML syntax errors in all manifest files (template files `.yaml.tmpl`, `.yml.tmpl` are excluded)
- Import validity -- correct references, no self-imports, valid data types
- Schema validation of all manifest sections against the JSON Schema
- Component duplication -- same component in same stack defined in multiple files with different configs

### Schema Configuration in atmos.yaml

```yaml
schemas:
  # JSON Schema for validating component configurations
  jsonschema:
    base_path: "stacks/schemas/jsonschema"

  # OPA policies for component validation
  opa:
    base_path: "stacks/schemas/opa"

  # JSON Schema for validating Atmos stack manifests themselves
  atmos:
    manifest: "stacks/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json"
    # Also supports URLs:
    # manifest: "https://atmos.tools/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json"
```

Configuration precedence for the manifest schema:
1. `--schemas-atmos-manifest` CLI flag
2. `ATMOS_SCHEMAS_ATMOS_MANIFEST` environment variable
3. `schemas.atmos.manifest` in `atmos.yaml`
4. Default embedded schema (matches the installed Atmos version)

## Custom Schema Validation

`atmos validate schema` also validates arbitrary files against custom schema validators
configured in `atmos.yaml`:

```yaml
schemas:
  my_custom_key:
    schema: !import https://example.com/schema.json
    matches:
      - folder/*.yaml
```

```shell
atmos validate schema
atmos validate schema my_custom_key
```

## IDE Integration

### JetBrains IDEs

JetBrains IDEs (IntelliJ, WebStorm, GoLand) automatically download schemas from SchemaStore.
The Atmos manifest schema is registered with `$id: https://json.schemastore.org/atmos-manifest.json`.

To manually associate: Settings > Languages & Frameworks > Schemas and DTDs > JSON Schema Mappings,
add the schema URL and map it to your stack YAML files (and `atmos.yaml` for the config schema).

### VS Code

Enable SchemaStore in VS Code settings for YAML files:

```json
{
  "yaml.schemaStore.enable": true
}
```

For manual association, add to `.vscode/settings.json`:

```json
{
  "yaml.schemas": {
    "https://atmos.tools/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json": [
      "stacks/**/*.yaml",
      "stacks/**/*.yml"
    ],
    "https://atmos.tools/schemas/atmos/atmos-config/1.0/atmos-config.json": [
      "atmos.yaml",
      "atmos.yml"
    ]
  }
}
```

Alternatively, use `yaml-language-server` modelines per file (works in any editor with a YAML
language server, no workspace settings required).

## Stack Manifest Structure Overview

The manifest schema defines these top-level properties:

- `import` -- Import section (array of strings or objects with `path`)
- `terraform` -- Global Terraform settings (vars, env, settings, backend, etc.)
- `helmfile` -- Global Helmfile settings
- `packer` -- Global Packer settings
- `vars` -- Global variables
- `hooks` -- Lifecycle hooks
- `env` -- Environment variables
- `settings` -- Settings including validation, atlantis, templates, and custom metadata
- `locals` -- File-scoped local variables for templates (do not inherit across imports)
- `components` -- Component definitions (terraform, helmfile, packer)
- `overrides` -- Override section
- `workflows` -- Workflow definitions
- `dependencies` -- Tool dependencies (tools with versions)
- `generate` -- Declarative file generation

Every section also accepts an `!include` string, so configuration can be loaded from external
files (e.g. `vars: !include shared/vars.yaml`); the schema models this alternative everywhere.
The `atmos.yaml` config schema likewise models Atmos YAML function alternatives (`!include`,
`!env`, ...) for every section.

## Reference Files

- [Stack manifest schema structure reference](references/schema-structure.md) -- Detailed
  structure and definitions of the published manifest schema
- Manifest schema: `https://atmos.tools/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json`
- Config schema: `https://atmos.tools/schemas/atmos/atmos-config/1.0/atmos-config.json`
- Schema configuration docs: https://atmos.tools/cli/configuration/schemas
- Validate stacks docs: https://atmos.tools/cli/commands/validate/stacks
- Validate schema docs: https://atmos.tools/cli/commands/validate/schema
- Print config schema docs: https://atmos.tools/cli/commands/config/config-schema
