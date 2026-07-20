---
name: atmos-validation
description: "Validate Atmos projects, components, arbitrary JSON Schema inputs, EditorConfig, and GitHub Actions; use affected-file selection and native CI annotations"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.1.0"
references:
  - references/json-schema.md
  - references/opa-policies.md
  - references/project-validation.md
---

# Atmos Validation Framework

Use this skill for every Atmos validation command and validation policy configuration: aggregate
project validation, configuration and stack schemas, JSON Schema, OPA/Rego, EditorConfig,
GitHub Actions workflows, affected-file selection, and native CI reporting.

For detailed JSON Schema examples, read [references/json-schema.md](references/json-schema.md).
For OPA/Rego policy patterns, read [references/opa-policies.md](references/opa-policies.md).
For project-wide, affected, and CI validation behavior, read
[references/project-validation.md](references/project-validation.md).

## Start with the Validation Target

| Need | Command |
|---|---|
| Run every project validator | `atmos validate` |
| Validate only changed project inputs | `atmos validate --affected` |
| Validate `atmos.yaml` | `atmos validate config` or `atmos config validate` |
| Validate stack manifests | `atmos validate stacks` or `atmos stack validate` |
| Validate `.editorconfig` rules | `atmos validate editorconfig` |
| Lint GitHub Actions workflows with actionlint | `atmos ci validate` or `atmos validate ci` |
| Validate arbitrary files against JSON Schema | `atmos validate schema <key>` |
| Validate resolved component input | `atmos validate component <component> -s <stack>` |

Start with the narrowest command that matches the change. Use the aggregate command for a
repository gate because it reports every applicable validator instead of stopping at the first one.

## Validation Commands

```shell
atmos validate component vpc -s plat-ue2-prod
atmos validate stacks
atmos validate schema github-actions
atmos validate --affected --format rich
```

`atmos validate component` validates a component in a stack using `settings.validation` or explicit
`--schema-path` / `--schema-type` flags.

`atmos validate stacks` validates stack YAML syntax, imports, duplicate component definitions, and
manifest schema compliance.

`atmos validate schema <key>` validates arbitrary files matched by a `schemas.<key>.matches` glob
against a JSON Schema.

`atmos validate` aggregates configuration, stack, EditorConfig, and GitHub Actions validation when
those project inputs exist. It is the preferred CI entry point for repository-wide validation.

## Affected Validation and Exclusions

Use `--affected` to select repository inputs changed from the merge base. In GitHub pull requests,
Atmos reads the PR base; locally, pass `--base <ref>` when the default base is not appropriate.

```shell
atmos validate --affected --base origin/main --format rich
atmos config validate --affected
atmos stack validate --affected
atmos validate ci --affected
```

Use repeatable repository-relative glob exclusions to keep deliberately-invalid test fixtures out
of production validation. `--exclude` works with the aggregate command and the config, stacks,
schema, and GitHub Actions validators whether they run all inputs or only affected inputs.

```shell
atmos validate --affected --exclude 'tests/fixtures/**' --format rich
atmos validate schema github-actions --exclude 'tests/fixtures/**'
atmos validate ci --exclude 'tests/fixtures/**'
```

Use slash-separated repository globs. Do not use an exclusion to hide a deployable project path;
run intentional negative fixtures in a dedicated test or annotation E2E check instead.

`atmos validate editorconfig --exclude` is its established EditorConfig regular-expression flag,
not the generic repository-glob exclusion. The aggregate command still applies its repository-glob
`--exclude` before invoking the EditorConfig validator.

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
      - run: atmos validate --affected --format rich --exclude 'tests/fixtures/**'
```

Enable native CI output in `atmos.yaml` when validation runs in CI:

```yaml
ci:
  enabled: true
  annotations:
    enabled: true
```

With native CI enabled, aggregate validation writes an Atmos job summary, including passed,
skipped, and failed validators. Findings from validators that support source annotations (including
GitHub Actions/actionlint) emit GitHub Actions workflow commands; a successful validation stays
visible through the green check and job summary rather than creating success annotations. Use
`--format rich` for readable CI diagnostics. The actionlint SARIF format is intentionally
side-effect free, so use it when a separate SARIF uploader owns annotations.

Do not reintroduce deprecated setup actions in new examples. Route broader Native CI workflow
structure, event triggers, permissions, and actionlint annotation E2E tests to `atmos-ci`.

## Routing

| Need | Skill |
|---|---|
| Detailed JSON Schema patterns | [references/json-schema.md](references/json-schema.md) |
| Detailed OPA/Rego policy patterns | [references/opa-policies.md](references/opa-policies.md) |
| Aggregate, affected, exclusions, and native CI behavior | [references/project-validation.md](references/project-validation.md) |
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
