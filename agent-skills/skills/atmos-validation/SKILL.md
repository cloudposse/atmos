---
name: atmos-validation
description: "Policy validation: OPA/Rego policies, JSON Schema, schema manifests, and generic `atmos validate schema` glob-to-JSON-Schema file validation"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
references:
  - references/json-schema.md
  - references/opa-policies.md
---

# Atmos Validation Framework

Use this skill for Atmos validation commands and validation policy configuration: JSON Schema,
OPA/Rego, EditorConfig validation, Atmos manifest validation, and generic `atmos validate schema`
checks for arbitrary YAML files.

For detailed JSON Schema examples, read [references/json-schema.md](references/json-schema.md).
For OPA/Rego policy patterns, read [references/opa-policies.md](references/opa-policies.md).

## Validation Commands

```shell
atmos validate component vpc -s plat-ue2-prod
atmos validate stacks
atmos validate schema github-actions
```

`atmos validate component` validates a component in a stack using `settings.validation` or explicit
`--schema-path` / `--schema-type` flags.

`atmos validate stacks` validates stack YAML syntax, imports, duplicate component definitions, and
manifest schema compliance.

`atmos validate schema <key>` validates arbitrary files matched by a `schemas.<key>.matches` glob
against a JSON Schema.

## Base Paths

Configure validation schema base paths in `atmos.yaml`:

```yaml
schemas:
  jsonschema:
    base_path: stacks/schemas/jsonschema
  opa:
    base_path: stacks/schemas/opa
  atmos:
    manifest: stacks/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json
```

The `cue`, `opa`, and `jsonschema` keys under `schemas` are reserved for Atmos schema-path
configuration. Do not use those names as generic `atmos validate schema <key>` entries.

## Component Validation

Define component validation under `settings.validation`:

```yaml
components:
  terraform:
    vpc:
      settings:
        validation:
          validate-vpc-jsonschema:
            schema_type: jsonschema
            schema_path: vpc/validate-vpc-component.json
          check-vpc-opa:
            schema_type: opa
            schema_path: vpc/validate-vpc-component.rego
            module_paths:
              - catalog/constants
            timeout: 10
```

Each validation step can define:

| Property | Use |
|---|---|
| `schema_type` | `jsonschema` or `opa` |
| `schema_path` | Path relative to the matching schema base path |
| `module_paths` | OPA module paths |
| `description` | Human-readable description |
| `disabled` | Skip the step when `true` |
| `timeout` | Timeout in seconds |

## Generic File Validation

Use `atmos validate schema` for non-Atmos files, such as GitHub Actions workflows or Kubernetes
manifests:

```yaml
schemas:
  github-actions:
    schema: https://json.schemastore.org/github-workflow.json
    matches:
      - .github/workflows/*.yml
      - .github/workflows/*.yaml
```

```shell
atmos validate schema github-actions
```

## JSON Schema

JSON Schema validates structure, required fields, types, patterns, enums, and ranges. Atmos passes
the full resolved component configuration to component schemas, so schemas usually validate under
`vars`, `settings`, `env`, `backend`, or `metadata`.

Minimal component schema:

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {
    "vars": {
      "type": "object",
      "required": ["region"],
      "properties": {
        "region": { "type": "string" }
      }
    }
  }
}
```

## OPA/Rego

Atmos OPA policies use `package atmos` and collect validation errors in `errors`.

```rego
package atmos

errors[message] {
  input.vars.stage == "prod"
  input.vars.map_public_ip_on_launch == true
  message = "Public IP mapping is not allowed in prod"
}
```

OPA input is the resolved component configuration. Common fields include `input.vars`,
`input.settings`, `input.env`, `input.backend`, `input.metadata`, and Terraform CLI context fields
available during plan/apply.

## CI Guidance

Run validation early in CI with the Atmos container and direct Atmos commands:

```yaml
jobs:
  validate:
    runs-on: ubuntu-latest
    container:
      image: ghcr.io/cloudposse/atmos:${{ vars.ATMOS_VERSION }}
    steps:
      - uses: actions/checkout@v6
      - run: atmos validate stacks
```

Do not reintroduce deprecated setup actions in new examples. Route broader Native CI workflow
structure to `atmos-ci`.

## Routing

| Need | Skill |
|---|---|
| Detailed JSON Schema patterns | [references/json-schema.md](references/json-schema.md) |
| Detailed OPA/Rego policy patterns | [references/opa-policies.md](references/opa-policies.md) |
| Atmos manifest JSON Schema and IDE completion | `atmos-schemas` |
| Stack manifest structure and imports | `atmos-stacks` |
| Component configuration and inheritance | `atmos-components` |
| Native CI validation workflows | `atmos-ci` |

## Guardrails

- Prefer stack `vars` validation over ad hoc shell checks.
- Keep reusable business rules in OPA modules or JSON Schema files, not inline in CI.
- Do not validate against guessed stack names or component names; use `atmos list` or
  `atmos describe` first.
- Keep policy messages actionable and name the field that should change.
