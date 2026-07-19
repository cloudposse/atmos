# Atmos Stack Manifest Schema Structure Reference

Detailed structure of the published Atmos stack manifest JSON Schema
(`https://atmos.tools/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json`, registered with
SchemaStore as `https://json.schemastore.org/atmos-manifest.json`). Use it to understand what
is valid where when authoring stack manifests. For validating `atmos.yaml` itself, see the
generated CLI configuration schema described in the main skill file.

## Top-Level Schema Structure

The manifest schema follows this structure:

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

Full Terraform component definition:

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

Settings section with validation, Atlantis, templates, and legacy dependency fields:

```json
"settings": {
  "oneOf": [
    { "type": "string", "pattern": "^!include" },
    {
      "type": "object",
      "additionalProperties": true,
      "properties": {
        "validation": { "$ref": "#/definitions/validation" },
        "atlantis": { "$ref": "#/definitions/atlantis" },
        "templates": { "$ref": "#/definitions/templates" }
      }
    }
  ]
}
```

Note: `settings` uses `"additionalProperties": true` to allow custom user-defined settings.
`settings.depends_on` is legacy; new examples should use `dependencies.components`.

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

### source / source_retry

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

### provision / provision_workdir

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

### dependencies / dependencies_tools

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

### generate

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

### component_auth

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

Atmos validates `vendor.yaml` files against a separate built-in vendor schema:

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
