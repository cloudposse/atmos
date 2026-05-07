# PRD: Native Conditional Imports (`import_if`)

**Status:** Proposed
**Created:** 2026-03-17
**Author:** rb @nitrocode
**Related PR:** https://github.com/cloudposse/atmos/pull/2202

---

## Problem Statement

Atmos documentation discourages using Go templates as an escape hatch because template-heavy stacks are harder to read and maintain. Despite this, teams routinely rename stack files to `.yaml.tmpl` just to conditionally include a catalog entry based on a stack variable like `stage` or `pci_scope`.

The `.yaml.tmpl` workaround has concrete costs:

| Cost | Description |
|------|-------------|
| **Readability** | Mixing YAML and Go-template syntax makes both harder to read |
| **Maintenance burden** | Nested conditionals accumulate quickly, producing hard-to-diff files |
| **Tool compatibility** | `.yaml.tmpl` files are not recognized as YAML by editors, linters, or schema validators |
| **Best-practice conflict** | Template use as an escape hatch contradicts Atmos guidance, confusing new contributors |

There is no lightweight, native way to conditionally include an import without the full template-engine workaround.

---

## Proposed Solution

Add an optional `import_if` string field to the `StackImport` struct. The field holds a Go-template expression (Sprig and Gomplate functions supported) that is evaluated against the stack's `vars` before any file I/O. Plain string imports without `import_if` are unaffected.

```yaml
vars:
  stage: prod
  pci_scope: true

import:
  - catalog/vpc/defaults                    # always included

  - path: catalog/vpc/flow-logs
    import_if: '{{ eq .stage "prod" }}'     # .stage shorthand

  - path: catalog/compliance/pci
    import_if: '{{ .vars.pci_scope }}'      # full .vars.key path
```

> Use single-quoted YAML strings (`'...'`) to avoid backslash-escaping double quotes inside the expression.

### Evaluation Semantics

The expression is rendered to a string and classified:

| Rendered output | Result |
|-----------------|--------|
| `true`, `1`, `yes` (case-insensitive) | Include the import |
| `false`, `0`, `no`, `""` (empty string) | Skip silently |
| Any other value | Error with descriptive message |

A referenced var that does not exist in `vars` resolves to an empty string, which is falsy. This is explicit Atmos behavior: unlike standard Go templates (which error on missing map keys), the context is pre-populated so all known vars default to `""` rather than triggering a template error.

### Template Data Context

The context is built from the stack's `vars` at the time of import resolution. Well-known identity keys are promoted to the root so `.stage` and `.vars.stage` both work:

| Root key | Source |
|----------|--------|
| `.stage` | `vars.stage` |
| `.tenant` | `vars.tenant` |
| `.environment` | `vars.environment` |
| `.namespace` | `vars.namespace` |
| `.region` | `vars.region` |

All `vars` entries are available under `.vars.<key>`. The `context:` field on the same import entry is **not** merged into the `import_if` context; it remains only for template rendering inside the imported file.

### Evaluation Timing

The condition is checked before path resolution, glob expansion, and file I/O. A falsy result means the path is never read and leaves no trace in the merged configuration. A file that does not exist in the current environment does not need `skip_if_missing` when `import_if` is already falsy for that environment.

---

## Design Decisions

### Decision 1: Native field over template wrapper

**Chosen:** A dedicated `import_if` field in `StackImport`.

| Alternative | Rejection reason |
|-------------|-----------------|
| `.yaml.tmpl` with `if` blocks (status quo) | Mixes syntax, tool-incompatible, violates best practices |
| `conditions:` block at stack level | More schema surface; `import_if` is locally scoped to each entry |
| `when:` alias | Less self-descriptive than `import_if` |

### Decision 2: Go-template expression string

**Chosen:** A Go-template string that renders to a truthy/falsy value.

Users already know Sprig/Gomplate from template processing elsewhere in Atmos. A plain `bool` YAML field cannot express dynamic conditions (`eq .stage "prod"`). A template expression is the minimal familiar extension. Only the `import_if` field is evaluated as a template; the rest of the stack file is unaffected.

### Decision 3: Promote well-known vars to root context

**Chosen:** Promote `stage`, `tenant`, `environment`, `namespace`, `region` to the root of the context.

These are the most common discriminators in conditional imports. The `.vars.` prefix remains available for all other vars. If a promoted key is absent from `vars`, it defaults to an empty string (falsy).

**Risk:** If a user defines a custom var with the same name as a promoted key, both `.stage` and `.vars.stage` resolve to the same value (the var), so there is no ambiguity. The promoted key is never sourced from anything other than `vars.stage`.

### Decision 4: Evaluate before glob expansion

**Chosen:** Condition checked before path resolution, glob expansion, and file I/O.

Skipping file I/O for falsy imports is a correctness guarantee (the file may not exist in that environment) and avoids unnecessary filesystem access. It also removes the need to combine `import_if` with `skip_if_missing` for environment-specific files.

### Decision 5: Error on unrecognized output

**Chosen:** Any rendered value that is not a recognized truthy/falsy string is an error.

Silent misclassification (treating `"maybe"` as truthy or falsy) would produce hard-to-debug import behavior. A loud error surfaces the problem immediately.

---

## Functional Requirements

| ID | Requirement |
|----|-------------|
| FR-1 | `StackImport` SHALL include an optional `import_if` string field |
| FR-2 | When `import_if` is absent or empty, import behavior SHALL be unchanged |
| FR-3 | `import_if` SHALL accept any Go-template expression using Sprig and Gomplate functions |
| FR-4 | The template context SHALL include all `vars` under `.vars.*` |
| FR-5 | The template context SHALL promote `stage`, `tenant`, `environment`, `namespace`, `region` to the root |
| FR-6 | A truthy result (`true`/`1`/`yes`) SHALL include the import |
| FR-7 | A falsy result (`false`/`0`/`no`/`""`) SHALL skip the import silently |
| FR-8 | Any other rendered value SHALL return a descriptive error |
| FR-9 | Condition evaluation SHALL occur before glob expansion and any file I/O |
| FR-10 | The `context:` field on the same import entry SHALL NOT be merged into the `import_if` context |
| FR-11 | A var referenced in `import_if` that is absent from `vars` SHALL resolve to empty string (falsy) |
| FR-12 | JSON Schema manifests SHALL be updated to include the `import_if` string property |

---

## Non-Goals

- Full template pre-processing of the `import:` block. `import_if` evaluates one condition expression per entry.
- Dynamic `path` construction. The `path` field remains a static string or glob. Dynamic path generation is a separate feature request.
- Cross-import dependencies. The context is the current stack's `vars` only; values from previously resolved imports are not available.

---

## User Experience

### Before

```yaml
# stacks/deploy/prod-us-east-1.yaml.tmpl  <- must rename to .tmpl
import:
  - catalog/vpc/defaults
{{- if eq .Vars.stage "prod" }}
  - catalog/vpc/flow-logs
{{- end }}
{{- if .Vars.pci_scope }}
  - catalog/compliance/pci
{{- end }}
```

### After

```yaml
# stacks/deploy/prod-us-east-1.yaml  <- stays .yaml
import:
  - catalog/vpc/defaults

  - path: catalog/vpc/flow-logs
    import_if: '{{ eq .stage "prod" }}'

  - path: catalog/compliance/pci
    import_if: '{{ .vars.pci_scope }}'
```

### Additional Examples

```yaml
# Tenant-specific override
- path: catalog/tenant-overrides/acme
  import_if: '{{ eq .tenant "acme" }}'

# Feature flag with default (Sprig)
- path: catalog/features/new-relic
  import_if: '{{ .vars.enable_new_relic | default false }}'

# Compound condition
- path: catalog/monitoring/enhanced
  import_if: '{{ and (eq .stage "prod") .vars.high_availability }}'

# Region prefix match
- path: catalog/us-east
  import_if: '{{ hasPrefix "us-east" .region }}'
```

---

## Implementation Overview

### Schema (`pkg/schema/schema.go`)

```go
type StackImport struct {
    Path                        string              `yaml:"path" json:"path" mapstructure:"path"`
    Context                     AtmosSectionMapType `yaml:"context" json:"context" mapstructure:"context"`
    SkipTemplatesProcessing     bool                `yaml:"skip_templates_processing,omitempty" ...`
    IgnoreMissingTemplateValues bool                `yaml:"ignore_missing_template_values,omitempty" ...`
    SkipIfMissing               bool                `yaml:"skip_if_missing,omitempty" ...`
    ImportIf                    string              `yaml:"import_if,omitempty" json:"import_if,omitempty" mapstructure:"import_if"`
}
```

### Condition Evaluator (`internal/exec/stack_processor_utils.go`)

```go
// buildImportIfContext constructs template data from stack vars,
// promoting well-known identity keys to the root.
func buildImportIfContext(stackConfigMap map[string]any) map[string]any

// evaluateImportCondition renders the import_if expression and returns true/false.
// Returns an error for any rendered value that is not a recognized truthy/falsy string.
func evaluateImportCondition(expression string, ctx map[string]any) (bool, error)
```

### Import Loop (before glob expansion)

```go
if imp.ImportIf != "" {
    ctx := buildImportIfContext(stackConfigMap)
    include, err := evaluateImportCondition(imp.ImportIf, ctx)
    if err != nil {
        return nil, fmt.Errorf("import_if on %q: %w", imp.Path, err)
    }
    if !include {
        continue
    }
}
```

### JSON Schema (`pkg/datafetcher/schema/`)

Add `import_if` as a string property to all three manifest schema files that describe the import object.

---

## Testing Strategy

### Unit Tests

| Test | Description |
|------|-------------|
| `evaluateImportCondition` - truthy variants | `"true"`, `"1"`, `"yes"` all return `true` |
| `evaluateImportCondition` - falsy variants | `"false"`, `"0"`, `"no"`, `""` all return `false` |
| `evaluateImportCondition` - error path | Unrecognized output returns a descriptive error |
| `evaluateImportCondition` - template expression | `{{ eq .stage "prod" }}` resolves correctly |
| `buildImportIfContext` - promotion | `stage`, `tenant`, etc. promoted to root |
| `buildImportIfContext` - vars namespace | all vars available under `.vars.*` |
| `buildImportIfContext` - missing var | absent var resolves to empty string |

### Integration Tests

| Test | Description |
|------|-------------|
| prod stack includes `flow-logs` | `stage: prod` causes `import_if: '{{ eq .stage "prod" }}'` to resolve to true |
| dev stack excludes `flow-logs` | `stage: dev` causes same expression to resolve to false |
| `pci_scope` boolean | `pci_scope: true` causes `import_if: '{{ .vars.pci_scope }}'` to resolve to true |
| Sprig functions | `hasPrefix`, `default`, `and`, `or` work correctly |
| Plain import unchanged | String imports without `import_if` are unaffected |

---

## Documentation Requirements

1. `website/docs/stacks/imports.mdx` - Add `import_if` to the import object reference and a "Conditional Imports" section with examples.
2. Blog post - Announce the feature with before/after examples.
3. Roadmap update - Mark the milestone as shipped.

---

## References

- PR: [#2202](https://github.com/cloudposse/atmos/pull/2202)
- `StackImport` struct: `pkg/schema/schema.go`
- Stack import processing: `internal/exec/stack_processor_utils.go`
- Import docs: `website/docs/stacks/imports.mdx`
