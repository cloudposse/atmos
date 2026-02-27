# Atmos Manifest Schema Structure Reference

## Schema File Locations

| File | Purpose | Embedding |
|------|---------|-----------|
| `website/static/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json` | Public schema for website and IDE integration | Not embedded; deployed to `atmos.tools` |
| `pkg/datafetcher/schema/stacks/stack-config/1.0.json` | Primary schema for `atmos validate stacks` | Embedded via `//go:embed schema/*` |
| `pkg/datafetcher/schema/atmos/manifest/1.0.json` | Minimal manifest schema (fallback) | Embedded via `//go:embed schema/*` |
| `pkg/datafetcher/schema/config/global/1.0.json` | Global Atmos config schema | Embedded via `//go:embed schema/*` |
| `pkg/datafetcher/schema/vendor/package/1.0.json` | Vendor manifest schema | Embedded via `//go:embed schema/*` |

## Schema Embedding Mechanism

The file `pkg/datafetcher/atmos_fetcher.go` embeds all schema files:

```go
//go:embed schema/*
var schemaFiles embed.FS

func (a atmosFetcher) FetchData(source string) ([]byte, error) {
    source = strings.TrimPrefix(source, "atmos://")
    data, err := schemaFiles.ReadFile(source + ".json")
    if err != nil {
        return nil, ErrAtmosSchemaNotFound
    }
    return data, nil
}
```

The `atmos://` URI scheme resolves to embedded files. For example, `atmos://schema/stacks/stack-config/1.0`
resolves to `schema/stacks/stack-config/1.0.json` in the embedded filesystem.

## Top-Level Schema Structure

All manifest schemas follow this structure:

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://json.schemastore.org/atmos-manifest.json",
  "title": "JSON Schema for Atmos Stack Manifest files. Version 1.0. https://atmos.tools",
  "type": "object",
  "properties": { ... },
  "additionalProperties": true,
  "oneOf": [ ... ],
  "definitions": { ... }
}
```

### Top-Level Properties

The `properties` object maps each top-level YAML key to a `$ref` pointing to a definition:

```json
"properties": {
  "import":       { "$ref": "#/definitions/import" },
  "terraform":    { "$ref": "#/definitions/terraform" },
  "helmfile":     { "$ref": "#/definitions/helmfile" },
  "packer":       { "$ref": "#/definitions/packer" },
  "vars":         { "$ref": "#/definitions/vars" },
  "hooks":        { "$ref": "#/definitions/hooks" },
  "env":          { "$ref": "#/definitions/env" },
  "settings":     { "$ref": "#/definitions/settings" },
  "locals":       { "$ref": "#/definitions/locals" },
  "components":   { "$ref": "#/definitions/components" },
  "overrides":    { "$ref": "#/definitions/overrides" },
  "workflows":    { "$ref": "#/definitions/workflows" },
  "dependencies": { "$ref": "#/definitions/dependencies" },
  "generate":     { "$ref": "#/definitions/generate" }
}
```

Note: Not all properties are present in all schema files. The website schema is the most complete.

### Validation Logic (oneOf)

The top-level `oneOf` ensures a manifest is either a workflows file or a stack manifest:

```json
"oneOf": [
  { "required": ["workflows"] },
  {
    "anyOf": [
      { "additionalProperties": true, "not": { "required": ["workflows"] } },
      { "required": ["import"] },
      { "required": ["terraform"] },
      { "required": ["helmfile"] },
      { "required": ["packer"] },
      { "required": ["vars"] },
      { "required": ["hooks"] },
      { "required": ["env"] },
      { "required": ["settings"] },
      { "required": ["components"] },
      { "required": ["overrides"] }
    ]
  }
]
```

## Definition Catalog

### import

Array of import paths (strings) or objects with `path` and options:

```json
"import": {
  "oneOf": [
    { "type": "string", "pattern": "^!include" },
    {
      "type": "array",
      "items": {
        "oneOf": [
          { "type": "string" },
          {
            "type": "object",
            "properties": {
              "path": { "type": "string" },
              "skip_templates_processing": { "type": "boolean" },
              "ignore_missing_template_values": { "type": "boolean" },
              "skip_if_missing": { "type": "boolean" },
              "context": { "type": "object", "additionalProperties": true }
            },
            "required": ["path"]
          }
        ]
      }
    }
  ]
}
```

### components

Container for terraform, helmfile, and packer component maps:

```json
"components": {
  "oneOf": [
    { "type": "string", "pattern": "^!include" },
    {
      "type": "object",
      "additionalProperties": true,
      "properties": {
        "terraform": { "$ref": "#/definitions/terraform_components" },
        "helmfile": { "$ref": "#/definitions/helmfile_components" },
        "packer": { "$ref": "#/definitions/packer_components" }
      }
    }
  ]
}
```

### terraform / helmfile / packer (Section-Level)

Global section-level settings. Example for `terraform`:

```json
"terraform": {
  "oneOf": [
    { "type": "string", "pattern": "^!include" },
    {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "vars": { "$ref": "#/definitions/vars" },
        "hooks": { "$ref": "#/definitions/hooks" },
        "env": { "$ref": "#/definitions/env" },
        "settings": { "$ref": "#/definitions/settings" },
        "locals": { "$ref": "#/definitions/locals" },
        "command": { "$ref": "#/definitions/command" },
        "backend_type": { "$ref": "#/definitions/backend_type" },
        "backend": { "$ref": "#/definitions/backend" },
        "remote_state_backend_type": { "$ref": "#/definitions/remote_state_backend_type" },
        "remote_state_backend": { "$ref": "#/definitions/remote_state_backend" },
        "overrides": { "$ref": "#/definitions/overrides" },
        "providers": { "$ref": "#/definitions/providers" },
        "dependencies": { "$ref": "#/definitions/dependencies" },
        "generate": { "$ref": "#/definitions/generate" }
      }
    }
  ]
}
```

### terraform_components / helmfile_components / packer_components

Maps of component names (pattern-matched) to component manifests:

```json
"terraform_components": {
  "oneOf": [
    { "type": "string", "pattern": "^!include" },
    {
      "type": "object",
      "patternProperties": {
        "^[/a-zA-Z0-9-_{}. ]+$": { "$ref": "#/definitions/terraform_component_manifest" }
      },
      "additionalProperties": false
    }
  ]
}
```

The `patternProperties` key `^[/a-zA-Z0-9-_{}. ]+$` allows component names with alphanumeric
characters, hyphens, underscores, dots, spaces, slashes, and curly braces.

### terraform_component_manifest

Full Terraform component definition (website schema version):

```json
"terraform_component_manifest": {
  "oneOf": [
    { "type": "string", "pattern": "^!include" },
    {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "metadata": { "$ref": "#/definitions/metadata" },
        "component": { "$ref": "#/definitions/component" },
        "source": { "$ref": "#/definitions/source" },
        "vars": { "$ref": "#/definitions/vars" },
        "env": { "$ref": "#/definitions/env" },
        "settings": { "$ref": "#/definitions/settings" },
        "locals": { "$ref": "#/definitions/locals" },
        "command": { "$ref": "#/definitions/command" },
        "backend_type": { "$ref": "#/definitions/backend_type" },
        "backend": { "$ref": "#/definitions/backend" },
        "remote_state_backend_type": { "$ref": "#/definitions/remote_state_backend_type" },
        "remote_state_backend": { "$ref": "#/definitions/remote_state_backend" },
        "providers": { "$ref": "#/definitions/providers" },
        "hooks": { "$ref": "#/definitions/hooks" },
        "provision": { "$ref": "#/definitions/provision" },
        "dependencies": { "$ref": "#/definitions/dependencies" },
        "auth": { "$ref": "#/definitions/component_auth" },
        "generate": { "$ref": "#/definitions/generate" }
      }
    }
  ]
}
```

### metadata

Component metadata with type, inheritance, workspace, custom configuration, and locking:

```json
"metadata": {
  "oneOf": [
    { "type": "string", "pattern": "^!include" },
    {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "type": { "type": "string", "enum": ["abstract", "real"] },
        "name": { "type": "string", "description": "Logical component identity..." },
        "enabled": { "type": "boolean" },
        "component": { "type": "string", "description": "Terraform/OpenTofu/Helmfile component" },
        "inherits": {
          "oneOf": [
            { "type": "string", "pattern": "^!include" },
            { "type": "array", "uniqueItems": true, "items": { "type": "string" } }
          ]
        },
        "terraform_workspace": { "type": "string" },
        "terraform_workspace_pattern": { "type": "string" },
        "custom": {
          "oneOf": [
            { "type": "string", "pattern": "^!include" },
            { "type": "object", "additionalProperties": true }
          ]
        },
        "locked": { "type": "boolean" }
      }
    }
  ]
}
```

### settings

Settings section with validation, depends_on, spacelift, atlantis, and templates:

```json
"settings": {
  "oneOf": [
    { "type": "string", "pattern": "^!include" },
    {
      "type": "object",
      "additionalProperties": true,
      "properties": {
        "validation": { "$ref": "#/definitions/validation" },
        "depends_on": { "$ref": "#/definitions/depends_on" },
        "spacelift": { "$ref": "#/definitions/spacelift" },
        "atlantis": { "$ref": "#/definitions/atlantis" },
        "templates": { "$ref": "#/definitions/templates" }
      }
    }
  ]
}
```

Note: `settings` uses `"additionalProperties": true` to allow custom user-defined settings.

### validation / validation_manifest

Validation rules for component configurations:

```json
"validation_manifest": {
  "oneOf": [
    { "type": "string", "pattern": "^!include" },
    {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "schema_type": { "type": "string", "enum": ["jsonschema", "opa"] },
        "schema_path": { "type": "string" },
        "description": { "type": "string" },
        "disabled": { "type": "boolean" },
        "timeout": { "type": "integer", "minimum": 0 },
        "module_paths": { "type": "array", "uniqueItems": true, "items": { "type": "string" } }
      },
      "required": ["schema_type", "schema_path"]
    }
  ]
}
```

### backend_type

Enum of supported backend types:

```json
"backend_type": {
  "oneOf": [
    { "type": "string", "pattern": "^!include" },
    { "type": "string", "enum": ["local", "s3", "remote", "vault", "static", "azurerm", "gcs", "cloud"] }
  ]
}
```

### backend_manifest

Backend configuration with per-type objects:

```json
"backend_manifest": {
  "oneOf": [
    { "type": "string", "pattern": "^!include" },
    {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "local": { ... },
        "s3": { ... },
        "remote": { ... },
        "vault": { ... },
        "static": { ... },
        "azurerm": { ... },
        "gcs": { ... },
        "cloud": { ... }
      }
    }
  ]
}
```

Each backend type value is `oneOf: [!include string, object with additionalProperties: true]`.

### workflows / workflow_manifest

Workflow definitions with named steps:

```json
"workflow_manifest": {
  "oneOf": [
    { "type": "string", "pattern": "^!include" },
    {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "description": { "type": "string" },
        "stack": { "type": "string" },
        "steps": {
          "oneOf": [
            { "type": "string", "pattern": "^!include" },
            {
              "type": "array",
              "items": {
                "type": "object",
                "properties": {
                  "name": { "type": "string" },
                  "command": { "type": "string" },
                  "stack": { "type": "string" },
                  "type": { "type": "string" }
                },
                "required": ["command"]
              }
            }
          ]
        }
      },
      "required": ["steps"]
    }
  ]
}
```

### source / source_retry (Website Schema Only)

JIT vendoring source configuration:

```json
"source": {
  "oneOf": [
    { "type": "string", "pattern": "^!include" },
    { "type": "string", "description": "Go-getter compatible URI..." },
    {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "type": { "type": "string" },
        "uri": { "type": "string" },
        "version": { "type": "string" },
        "included_paths": { "type": "array", "items": { "type": "string" } },
        "excluded_paths": { "type": "array", "items": { "type": "string" } },
        "retry": { "$ref": "#/definitions/source_retry" }
      },
      "required": ["uri"]
    }
  ]
}
```

### provision / provision_workdir (Website Schema Only)

Isolated workdir provisioner:

```json
"provision": {
  "oneOf": [
    { "type": "string", "pattern": "^!include" },
    {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "workdir": { "$ref": "#/definitions/provision_workdir" }
      }
    }
  ]
}
```

### dependencies / dependencies_tools (Website Schema Only)

Tool dependency declarations:

```json
"dependencies": {
  "oneOf": [
    { "type": "string", "pattern": "^!include" },
    {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "tools": { "$ref": "#/definitions/dependencies_tools" }
      }
    }
  ]
}
```

### generate (Website Schema Only)

Declarative file generation:

```json
"generate": {
  "oneOf": [
    { "type": "string", "pattern": "^!include" },
    {
      "type": "object",
      "additionalProperties": {
        "oneOf": [
          { "type": "string", "description": "String template processed with Go templates" },
          { "type": "object", "additionalProperties": true }
        ]
      }
    }
  ]
}
```

### component_auth (Website Schema Only)

Component-level authentication with providers and identities:

```json
"component_auth": {
  "oneOf": [
    { "type": "string", "pattern": "^!include" },
    {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "providers": { "$ref": "#/definitions/auth_providers" },
        "identities": { "$ref": "#/definitions/auth_identities" }
      }
    }
  ]
}
```

Related definitions: `auth_providers`, `auth_provider`, `auth_identities`, `auth_identity`,
`auth_identity_via`, `auth_session`, `auth_console`.

## Vendor Package Schema Structure

The vendor schema (`pkg/datafetcher/schema/vendor/package/1.0.json`) is a separate schema
for `vendor.yaml` files:

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "Atmos Vendor Config",
  "fileMatch": ["vendor.{yml,yaml}", "vendor.d/**/*.{yml,yaml}"],
  "type": "object",
  "properties": {
    "apiVersion": { "enum": ["atmos/v1"] },
    "kind": { "enum": ["AtmosVendorConfig"] },
    "metadata": {
      "properties": {
        "name": { "type": "string" },
        "description": { "type": "string" }
      },
      "required": ["name", "description"]
    },
    "spec": {
      "properties": {
        "base_path": { "type": "string" },
        "imports": { "type": "array", "items": { "type": "string" } },
        "sources": {
          "type": "array",
          "items": {
            "properties": {
              "component": { "type": "string" },
              "source": { "type": "string" },
              "version": { "type": "string" },
              "targets": { "type": "array", "items": { "type": "string" } },
              "included_paths": { "type": "array", "items": { "type": "string" } },
              "excluded_paths": { "type": "array", "items": { "type": "string" } },
              "tags": { "type": "array", "items": { "type": "string" } }
            },
            "required": ["component", "source", "version", "targets"]
          }
        }
      },
      "required": ["sources"]
    }
  },
  "required": ["apiVersion", "kind", "metadata", "spec"]
}
```

## Cross-References Between Schema Files

### Feature Parity Matrix

| Definition | Website | Stack-Config | Atmos/Manifest | Config/Global |
|-----------|---------|-------------|----------------|---------------|
| import | Yes | Yes | Yes | Yes |
| components | Yes | Yes | Yes | Yes |
| terraform | Yes | Yes | Yes | Yes |
| helmfile | Yes | Yes | Yes | Yes |
| packer | Yes | Yes | Yes | Yes |
| vars | Yes | Yes | Yes | Yes |
| env | Yes | Yes | Yes | Yes |
| hooks | Yes | Yes | Yes | Yes |
| settings | Yes | Yes | Yes | Yes |
| locals | Yes | Yes | No | No |
| metadata | Yes | Yes | Yes | Yes |
| validation | Yes | Yes | Yes | Yes |
| backend_type | Yes | Yes | Yes | Yes |
| backend_manifest | Yes | Yes | Yes | Yes |
| overrides | Yes | Yes | Yes | Yes |
| workflows | Yes | Yes | Yes | Yes |
| depends_on | Yes | Yes | Yes | Yes |
| spacelift | Yes | Yes | Yes | Yes |
| atlantis | Yes | Yes | Yes | Yes |
| providers | Yes | Yes | Yes | Yes |
| templates | Yes | Yes | Yes | Yes |
| source | Yes | No | No | No |
| source_retry | Yes | No | No | No |
| provision | Yes | No | No | No |
| provision_workdir | Yes | No | No | No |
| dependencies | Yes | No | No | No |
| dependencies_tools | Yes | No | No | No |
| generate | Yes | Yes | No | No |
| component_auth | Yes | No | No | No |
| auth_* | Yes | No | No | No |
| name (top-level) | No | Yes | No | No |

### Keeping Schemas in Sync

When a feature is added:

1. Always update the **website schema** first (it is the canonical reference).
2. Then update the **stack-config schema** (used at runtime by `atmos validate stacks`).
3. Update the **atmos/manifest** and **config/global** schemas only if the feature applies
   to their respective validation domains.
4. The embedded schemas are compiled into the binary, so changes take effect at the next build.
5. The website schema is deployed when the documentation site is rebuilt.

## Adding a New Definition: Complete Example

Suppose you are adding a new `notifications` feature to Atmos components.

### 1. Define the schema

```json
"notifications": {
  "title": "notifications",
  "description": "Notification configuration for component lifecycle events",
  "oneOf": [
    {
      "type": "string",
      "pattern": "^!include"
    },
    {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "enabled": {
          "type": "boolean",
          "description": "Enable notifications for this component"
        },
        "channels": {
          "oneOf": [
            {
              "type": "string",
              "pattern": "^!include"
            },
            {
              "type": "array",
              "items": {
                "type": "string"
              },
              "description": "List of notification channel names"
            }
          ]
        },
        "events": {
          "oneOf": [
            {
              "type": "string",
              "pattern": "^!include"
            },
            {
              "type": "array",
              "items": {
                "type": "string",
                "enum": ["plan", "apply", "destroy", "drift"]
              },
              "description": "Events that trigger notifications"
            }
          ]
        }
      },
      "required": []
    }
  ]
}
```

### 2. Add to terraform_component_manifest

```json
"terraform_component_manifest": {
  "oneOf": [
    { "type": "string", "pattern": "^!include" },
    {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "metadata": { "$ref": "#/definitions/metadata" },
        "notifications": { "$ref": "#/definitions/notifications" },
        ...
      }
    }
  ]
}
```

### 3. Repeat for all schema files

Apply the same changes to all four manifest schema files (or at minimum the website and
stack-config schemas).

### 4. Validate

```shell
# Build and test
make build
atmos validate stacks
```
