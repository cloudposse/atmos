---
name: atmos-schemas
description: "JSON Schema for Atmos stack manifests: IDE auto-completion, manifest validation, schema updates for new features, SchemaStore integration"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos JSON Schema System

## Overview

Atmos uses JSON Schema (Draft 2020-12) to validate stack manifests, provide IDE auto-completion, and
catch configuration errors early. The schema system has three layers:

1. **Website manifest schema** -- Published at `website/static/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json`,
   served at `https://atmos.tools/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json`, and registered with SchemaStore
   as `https://json.schemastore.org/atmos-manifest.json`. This is the public-facing schema for IDE integration.

2. **Embedded schemas** -- Located under `pkg/datafetcher/schema/`, compiled into the Atmos binary via Go `embed`.
   These are the schemas Atmos uses at runtime for validation. There are multiple embedded schemas:
   - `pkg/datafetcher/schema/atmos/manifest/1.0.json` -- Minimal manifest schema (fallback).
   - `pkg/datafetcher/schema/stacks/stack-config/1.0.json` -- Stack configuration validation schema. This is
     the primary schema used by `atmos validate stacks`.
   - `pkg/datafetcher/schema/config/global/1.0.json` -- Global Atmos configuration schema.
   - `pkg/datafetcher/schema/vendor/package/1.0.json` -- Vendor package manifest schema.

3. **User-provided schema** -- Users can override the embedded schema by specifying a path or URL in `atmos.yaml`
   under `schemas.atmos.manifest`, or via `--schemas-atmos-manifest` flag or `ATMOS_SCHEMAS_ATMOS_MANIFEST` env var.

## Schema Files and Their Locations

### Website Schema (Public)

**Path:** `website/static/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json`

This is deployed to `https://atmos.tools/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json` and is the
canonical public schema. It uses the SchemaStore `$id`:

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://json.schemastore.org/atmos-manifest.json",
  "title": "JSON Schema for Atmos Stack Manifest files. Version 1.0. https://atmos.tools"
}
```

### Embedded Schemas (Runtime)

**Path:** `pkg/datafetcher/schema/`

The `pkg/datafetcher/atmos_fetcher.go` uses `//go:embed schema/*` to embed all schema files into the binary.
The `atmosFetcher.FetchData()` method resolves `atmos://` URIs to embedded schema files by stripping the prefix
and appending `.json`.

Directory structure:

```text
pkg/datafetcher/schema/
  atmos/manifest/1.0.json          -- Minimal manifest schema
  stacks/stack-config/1.0.json     -- Full stack config validation schema
  config/global/1.0.json           -- Global atmos.yaml config schema
  vendor/package/1.0.json          -- Vendor manifest schema
```

### Vendor Package Schema

**Path:** `pkg/datafetcher/schema/vendor/package/1.0.json`

Validates `vendor.yaml` files with `apiVersion`, `kind`, `metadata`, and `spec` sections:

```json
{
  "fileMatch": ["vendor.{yml,yaml}", "vendor.d/**/*.{yml,yaml}"],
  "properties": {
    "apiVersion": { "enum": ["atmos/v1"] },
    "kind": { "enum": ["AtmosVendorConfig"] },
    "metadata": { "required": ["name", "description"] },
    "spec": { "required": ["sources"] }
  },
  "required": ["apiVersion", "kind", "metadata", "spec"]
}
```

## Schema Configuration in atmos.yaml

```yaml
schemas:
  # JSON Schema for validating component configurations
  jsonschema:
    base_path: "stacks/schemas/jsonschema"

  # OPA policies for component validation
  opa:
    base_path: "stacks/schemas/opa"

  # JSON Schema for validating Atmos manifests themselves
  atmos:
    manifest: "stacks/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json"
    # Also supports URLs:
    # manifest: "https://atmos.tools/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json"
```

Configuration precedence:
1. `--schemas-atmos-manifest` CLI flag
2. `ATMOS_SCHEMAS_ATMOS_MANIFEST` environment variable
3. `schemas.atmos.manifest` in `atmos.yaml`
4. Default embedded schema (compiled into the binary)

## Validating Stacks

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

### `atmos validate schema`

Validates files against custom schema validators configured in `atmos.yaml`:

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
add the schema URL and map it to your stack YAML files.

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
    ]
  }
}
```

## How to Update Schemas When Adding New Features

When you add a new feature to Atmos that introduces new configuration keys in stack manifests,
you MUST update the JSON Schema files. Failure to do so causes validation errors for users who
rely on `atmos validate stacks` or IDE auto-completion.

### Which Schema Files to Update

For feature work, always update the **website schema** and **stack-config schema** first.
Then update the other two when the new keys or structure apply to their domains.

| File | Purpose |
|------|---------|
| `website/static/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json` | Public schema (website, SchemaStore, IDE) |
| `pkg/datafetcher/schema/stacks/stack-config/1.0.json` | Primary embedded schema for `validate stacks` |
| `pkg/datafetcher/schema/atmos/manifest/1.0.json` | Minimal embedded manifest schema |
| `pkg/datafetcher/schema/config/global/1.0.json` | Global config validation schema |

The **website schema** and the **stack-config schema** are the most complete and feature-rich.
Update **atmos/manifest** and **config/global** when adding top-level or structural changes that
affect their respective domains.

For vendor manifest changes, update:
- `pkg/datafetcher/schema/vendor/package/1.0.json`

### Step-by-Step: Adding a New Top-Level Property

1. **Add the property reference in the top-level `properties` object** in each schema file:

```json
{
  "properties": {
    "existing_prop": { "$ref": "#/definitions/existing_prop" },
    "my_new_prop": { "$ref": "#/definitions/my_new_prop" }
  }
}
```

2. **Add the definition** in the `definitions` section. Follow the Atmos `!include` pattern:

```json
{
  "definitions": {
    "my_new_prop": {
      "title": "my_new_prop",
      "description": "Description of the new property",
      "oneOf": [
        {
          "type": "string",
          "pattern": "^!include"
        },
        {
          "type": "object",
          "additionalProperties": false,
          "properties": {
            "field_one": {
              "type": "string",
              "description": "Description of field_one"
            },
            "field_two": {
              "type": "boolean",
              "description": "Description of field_two"
            }
          },
          "required": []
        }
      ]
    }
  }
}
```

3. **If the property should make a manifest valid on its own**, add it to the `oneOf` > `anyOf` array:

```json
{
  "oneOf": [
    { "required": ["workflows"] },
    {
      "anyOf": [
        { "required": ["import"] },
        { "required": ["my_new_prop"] }
      ]
    }
  ]
}
```

4. **Update all four schema files** with the same changes.

### Step-by-Step: Adding a Property to a Component Manifest

To add a new property at the component level (inside `components.terraform.<name>`):

1. **Add the property to `terraform_component_manifest`** (and/or `helmfile_component_manifest`,
   `packer_component_manifest` as appropriate):

```json
{
  "definitions": {
    "terraform_component_manifest": {
      "oneOf": [
        { "type": "string", "pattern": "^!include" },
        {
          "type": "object",
          "additionalProperties": false,
          "properties": {
            "metadata": { "$ref": "#/definitions/metadata" },
            "my_new_field": { "$ref": "#/definitions/my_new_field" }
          }
        }
      ]
    }
  }
}
```

2. **Create the definition** for your new field as shown above.

### Step-by-Step: Adding a Property to an Existing Definition

To add a new field to an existing definition (e.g., adding a field to `metadata`):

1. **Locate the definition** in the `definitions` section.

2. **Add the property** inside the object variant of the `oneOf`:

```json
{
  "definitions": {
    "metadata": {
      "oneOf": [
        { "type": "string", "pattern": "^!include" },
        {
          "type": "object",
          "additionalProperties": false,
          "properties": {
            "type": { "type": "string", "enum": ["abstract", "real"] },
            "enabled": { "type": "boolean" },
            "my_new_metadata_field": {
              "type": "string",
              "description": "Description of the new metadata field"
            }
          }
        }
      ]
    }
  }
}
```

### The !include Pattern

Every definition in the Atmos schema supports the `!include` YAML tag pattern. This means every
definition uses `oneOf` with the first option being a string matching `^!include` and the second
being the actual type definition:

```json
"oneOf": [
  {
    "type": "string",
    "pattern": "^!include"
  },
  {
    "type": "object",
    ...
  }
]
```

Always include this pattern when creating new definitions. It enables users to use `!include` directives
to load sections from external files.

### JSON Schema 2020-12 Quick Reference

Common patterns used in Atmos schemas:

```json
// String with enum
{ "type": "string", "enum": ["value1", "value2"] }

// Boolean
{ "type": "boolean", "description": "Flag description" }

// Integer with minimum
{ "type": "integer", "minimum": 0 }

// Array of strings with uniqueness
{ "type": "array", "uniqueItems": true, "items": { "type": "string" } }

// Object with free-form keys
{ "type": "object", "additionalProperties": true }

// Object with pattern-matched keys referencing a definition
{
  "type": "object",
  "patternProperties": {
    "^[/a-zA-Z0-9-_{}. ]+$": { "$ref": "#/definitions/my_definition" }
  },
  "additionalProperties": false
}

// Flexible type (string or number)
{ "anyOf": [{ "type": "number" }, { "type": "string" }] }

// Reference to another definition
{ "$ref": "#/definitions/my_definition" }
```

### Differences Between Schema Files

The four manifest schema files are mostly identical but have some differences:

- **Website schema** (`website/static/`) -- The most complete. Includes `locals`, `dependencies`,
  `generate`, `provision`, `source`, `auth`, and `component_auth` definitions. Has `source_retry`,
  `auth_providers`, `auth_identities`, `auth_identity`, `auth_identity_via`, `auth_session`,
  and `auth_console` definitions.

- **Stack-config schema** (`pkg/datafetcher/schema/stacks/`) -- Has `name` as a top-level property
  (with description: "Logical name for this stack"). Has `locals` definition. May include
  additional definitions like `name` in the `metadata` section. Missing some newer definitions
  that are in the website schema (e.g., `generate`, `provision`, `source`, `auth`).

- **Atmos manifest schema** (`pkg/datafetcher/schema/atmos/`) -- Minimal. Does not have `locals`,
  `dependencies`, `generate`, `provision`, `source`, or `auth` definitions.

- **Global config schema** (`pkg/datafetcher/schema/config/`) -- Similar to atmos manifest, used
  for global config validation.

When adding new features, the minimum required updates are the **website schema** and the
**stack-config schema**. Also update **atmos/manifest** when manifest-level validation is affected,
and **config/global** when global config validation is affected.

### Checklist for Schema Updates

When adding a new Atmos feature with configuration keys:

- [ ] Add the definition to `website/static/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json`
- [ ] Add the definition to `pkg/datafetcher/schema/stacks/stack-config/1.0.json`
- [ ] Add the definition to `pkg/datafetcher/schema/atmos/manifest/1.0.json` (if applicable)
- [ ] Add the definition to `pkg/datafetcher/schema/config/global/1.0.json` (if applicable)
- [ ] Add property references in top-level `properties` or component manifest `properties`
- [ ] Include the `!include` pattern in `oneOf` for all new object definitions
- [ ] Add `description` fields for IDE auto-completion hover text
- [ ] Test with `atmos validate stacks` to ensure no regressions
- [ ] Verify IDE auto-completion works with the updated schema

## Schema Structure Overview

The manifest schema defines these top-level properties (each referencing a definition):

- `import` -- Import section (array of strings or objects with `path`)
- `terraform` -- Global Terraform settings (vars, env, settings, backend, etc.)
- `helmfile` -- Global Helmfile settings
- `packer` -- Global Packer settings
- `vars` -- Global variables
- `hooks` -- Lifecycle hooks
- `env` -- Environment variables
- `settings` -- Settings including validation, depends_on, spacelift, atlantis, templates
- `locals` -- File-scoped local variables for templates (do not inherit across imports)
- `components` -- Component definitions (terraform, helmfile, packer)
- `overrides` -- Override section
- `workflows` -- Workflow definitions
- `dependencies` -- Tool dependencies (tools with versions)
- `generate` -- Declarative file generation

Key definitions in the `definitions` section:

| Definition | Description |
|-----------|-------------|
| `terraform_components` | Map of Terraform component names to manifests |
| `terraform_component_manifest` | Single Terraform component (metadata, vars, backend, hooks, etc.) |
| `helmfile_components` | Map of Helmfile component names to manifests |
| `helmfile_component_manifest` | Single Helmfile component |
| `packer_components` | Map of Packer component names to manifests |
| `packer_component_manifest` | Single Packer component |
| `metadata` | Component metadata (type, enabled, component, inherits, workspace, custom, locked) |
| `settings` | Settings with validation, depends_on, spacelift, atlantis, templates |
| `validation` / `validation_manifest` | Validation rules (schema_type, schema_path, module_paths) |
| `backend_type` | Backend type enum (local, s3, remote, vault, static, azurerm, gcs, cloud) |
| `backend` / `backend_manifest` | Backend config per type |
| `overrides` | Override section (command, vars, env, settings, providers) |
| `workflows` / `workflow_manifest` | Workflow definitions with steps |
| `depends_on` / `depends_on_manifest` | Dependency declarations |
| `spacelift` | Spacelift integration settings |
| `atlantis` | Atlantis integration settings |
| `source` / `source_retry` | JIT vendoring source configuration |
| `provision` / `provision_workdir` | Isolated workdir provisioner |
| `dependencies` / `dependencies_tools` | Tool dependency declarations |
| `component_auth` | Component-level auth (providers, identities) |
| `generate` | Declarative file generation (string templates or objects) |

## Reference Files

- [Schema structure reference](references/schema-structure.md) -- Detailed schema structure and definitions
- Website schema: `website/static/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json`
- Embedded schemas: `pkg/datafetcher/schema/`
- Schema embedding: `pkg/datafetcher/atmos_fetcher.go`
- Schema configuration docs: `website/docs/cli/configuration/schemas.mdx`
- Validate stacks docs: `website/docs/cli/commands/validate/validate-stacks.mdx`
- Validate schema docs: `website/docs/cli/commands/validate/validate-schema.mdx`
