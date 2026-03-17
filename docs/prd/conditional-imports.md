# PRD: Native Conditional Imports (`import_if`)

**Status:** Proposed
**Created:** 2026-03-17
**Author:** rb
**Related PR:** https://github.com/cloudposse/atmos/pull/2202

---

## Executive Summary

Teams routinely reach for `.yaml.tmpl` files just to conditionally include a catalog entry based on a stack variable such as `stage` or `pci_scope`. The full Go-template engine is too heavy a hammer for that nail, and its use as an escape hatch works against the official best practices that warn against template-heavy stacks.

This PRD defines a lightweight native `import_if` field for the import section. The field evaluates a single Go-template expression against the stack's `vars` before any file I/O, allowing teams to express conditional imports declaratively without spinning up the template engine for the entire stack.

---

## Problem Statement

### Context

Atmos official documentation discourages using Go templates as a general-purpose escape hatch because template-heavy stacks become harder to read, harder to audit, and harder to maintain. Despite this guidance, conditional imports are a common, legitimate need:

- Include `catalog/vpc/flow-logs` only in production environments.
- Include `catalog/compliance/pci` only when the stack has `pci_scope: true`.
- Include `catalog/monitoring/enhanced` only for certain tenants.

### Current Workaround and Its Costs

The only supported workaround today is to rename the stack file to `.yaml.tmpl` and wrap the import in a Go-template `if` block:

```yaml
# stacks/deploy/prod-us-east-1.yaml.tmpl
import:
  - catalog/vpc/defaults
{{- if eq .Vars.stage "prod" }}
  - catalog/vpc/flow-logs
{{- end }}
{{- if .Vars.pci_scope }}
  - catalog/compliance/pci
{{- end }}
```

This workaround has several concrete costs:

| Cost | Description |
|------|-------------|
| **Readability** | Mixing YAML and Go-template syntax in the same file makes both harder to read |
| **Complexity rash** | Nested conditionals accumulate quickly, leading to deeply indented, hard-to-diff files |
| **Full engine overhead** | The entire template engine is invoked even when a single boolean check is all that is needed |
| **Tool compatibility** | `.yaml.tmpl` files are not recognized as YAML by editors, linters, or schema validators |
| **Best-practice conflict** | Using templates as an escape hatch directly contradicts Atmos guidance, creating confusion for onboarding engineers |

### The Gap

There is no lightweight, native way to conditionally include an import based on stack-context variables without adopting the full template-engine workaround.

---

## Proposed Solution

### Syntax

Add an optional `import_if` string field to the import object syntax. The field accepts any Go-template expression (Sprig and Gomplate functions included). Plain string imports without `import_if` are unaffected.

```yaml
vars:
  stage: prod
  pci_scope: true

import:
  - catalog/vpc/defaults                        # always included

  - path: catalog/vpc/flow-logs
    import_if: "{{ eq .stage \"prod\" }}"       # .stage shorthand

  - path: catalog/compliance/pci
    import_if: "{{ .vars.pci_scope }}"          # full .vars.key path
```

### Evaluation Semantics

The template expression is rendered to a string and then classified as **truthy** or **falsy**:

| Rendered string | Classification |
|-----------------|----------------|
| `true`, `1`, `yes` (case-insensitive) | Truthy — import is included |
| `false`, `0`, `no`, `""` (empty) | Falsy — import is skipped silently |
| Any other value | Error — surfaces a descriptive error message |


### Template Data Context

The template data object is constructed from the stack's `vars` section at the point of import resolution. Well-known top-level stack identity variables are promoted to the root of the context so both `.stage` and `.vars.stage` resolve:

| Promoted key | Source |
|--------------|--------|
| `.stage` | `vars.stage` |
| `.tenant` | `vars.tenant` |
| `.environment` | `vars.environment` |
| `.namespace` | `vars.namespace` |
| `.region` | `vars.region` |

All `vars` entries are also available under `.vars.<key>` without exception.

### Evaluation Timing

The condition is evaluated **before** glob expansion and before any file I/O. A falsy condition means the path is never resolved, never read, and leaves no trace in the merged configuration.

---

## Design Decisions

### Decision 1: Native field over template wrapper

**Chosen:** A dedicated `import_if` field in the `StackImport` struct.

**Alternatives considered:**

| Alternative | Rejection reason |
|-------------|-----------------|
| `.yaml.tmpl` with Go-template `if` blocks (status quo) | Mixes syntax, violates best practices, tool-incompatible YAML |
| A new `conditions:` block at the stack level | More verbose, requires more schema surface; `import_if` is locally scoped |
| A `when:` alias | `import_if` communicates that it gates import inclusion; `when:` is less specific |

### Decision 2: Go-template expression string (not a YAML boolean)

**Chosen:** A Go-template string that renders to a truthy/falsy value.

**Rationale:** Users already know Sprig/Gomplate from template processing elsewhere in Atmos. A plain `bool` YAML field would not support dynamic expressions (e.g., `eq .stage "prod"`). A template string is the minimal extension of familiar syntax.

**Constraint:** Only a single template expression is evaluated — it is not a full stack-template pass. This keeps evaluation isolated and fast.

### Decision 3: Promote well-known vars to root context

**Chosen:** Promote `stage`, `tenant`, `environment`, `namespace`, `region` to the root of the template data object, in addition to exposing all vars under `.vars.*`.

**Rationale:** These are the most common discriminators in conditional imports. Requiring `.vars.stage` everywhere adds noise. Promoting them reduces common-case verbosity while the `.vars.` prefix remains available for all other vars.

**Risk considered:** Name collision (a user-defined var named `stage` shadows the promoted key). Resolution: the `.vars.stage` path is always authoritative; the root-level shorthand is only a convenience alias with the same value.

### Decision 4: Evaluate before glob expansion

**Chosen:** Condition is checked before path resolution, glob expansion, and file I/O.

**Rationale:** Skipping file I/O for falsy imports is both a correctness guarantee (the file may not exist in this environment) and a performance benefit (no unnecessary filesystem access). It also means a missing file that would otherwise trigger `skip_if_missing` does not need to be present.

### Decision 5: Error on unrecognized truthy output

**Chosen:** Any template output that is not a recognized truthy/falsy value is an error.

**Rationale:** Silent mis-classification (e.g., treating `"maybe"` as truthy or falsy) would cause hard-to-debug import behaviour. A loud error surfaces the problem immediately.

---

## Functional Requirements

| ID | Requirement |
|----|-------------|
| FR-1 | `StackImport` SHALL include an optional `import_if` string field |
| FR-2 | When `import_if` is absent or empty, import behaviour SHALL be unchanged |
| FR-3 | `import_if` SHALL accept any Go-template expression using Sprig and Gomplate functions |
| FR-4 | The template data context SHALL include all `vars` under `.vars.*` |
| FR-5 | The template data context SHALL promote `stage`, `tenant`, `environment`, `namespace`, `region` to the root object |
| FR-6 | A truthy result (`true`/`1`/`yes`) SHALL include the import |
| FR-7 | A falsy result (`false`/`0`/`no`/`""`) SHALL skip the import silently |
| FR-8 | Any other rendered value SHALL return a descriptive error |
| FR-9 | Condition evaluation SHALL occur before glob expansion and any file I/O |
| FR-10 | JSON Schema manifests SHALL be updated to include the `import_if` property |

---

## Non-Goals

- **Full template pre-processing of the import section.** `import_if` evaluates a single condition expression; it does not render the entire `import:` block as a template.
- **Dynamic path construction.** The `path` field continues to be a static string (or glob). Dynamic paths should use the existing `.yaml.tmpl` approach.
- **Cross-import dependencies.** The condition context is the current stack's `vars` only; it cannot reference values from previously resolved imports.
- **Nested `import_if` composition** (AND/OR syntax). Complex multi-condition logic can be expressed with Sprig `and`/`or` functions within the single expression string.

---

## User Experience

### Before

```yaml
# stacks/deploy/prod-us-east-1.yaml.tmpl  ← must rename to .tmpl
import:
  - catalog/vpc/defaults
{{- if eq .Vars.stage "prod" }}
  - catalog/vpc/flow-logs
{{- end }}
{{- if .Vars.pci_scope }}
  - catalog/compliance/pci
{{- end }}
```

Problems: mixed syntax, not valid YAML, editor warnings, template-engine overhead.

### After

```yaml
# stacks/deploy/prod-us-east-1.yaml  ← stays .yaml
import:
  - catalog/vpc/defaults

  - path: catalog/vpc/flow-logs
    import_if: "{{ eq .stage \"prod\" }}"

  - path: catalog/compliance/pci
    import_if: "{{ .vars.pci_scope }}"
```

The file is valid YAML. Intent is clear at a glance. No template-engine ceremony.

### Additional Examples

```yaml
# Import only for specific tenant
- path: catalog/tenant-overrides/acme
  import_if: "{{ eq .tenant \"acme\" }}"

# Import only when a feature flag is enabled (using Sprig)
- path: catalog/features/new-relic
  import_if: "{{ .vars.enable_new_relic | default false }}"

# Import using a compound condition
- path: catalog/monitoring/enhanced
  import_if: "{{ and (eq .stage \"prod\") .vars.high_availability }}"

# Import using environment prefix matching
- path: catalog/us-east
  import_if: "{{ hasPrefix \"us-east\" .region }}"
```

---

## Implementation Overview

### Schema (`pkg/schema/schema.go`)

Add `ImportIf` to `StackImport`:

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

Two helper functions:

```go
// buildImportIfContext constructs the template data object from the stack's vars,
// promoting well-known identity keys to the root context.
func buildImportIfContext(stackConfigMap map[string]any) map[string]any

// evaluateImportCondition renders the import_if expression and returns true/false.
// Returns an error for any rendered value that is not a recognized truthy/falsy string.
func evaluateImportCondition(expression string, ctx map[string]any) (bool, error)
```

### Import Loop

In the import processing loop, before glob expansion:

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

Add `import_if` string property to all three manifest schema files that describe the import object.

---

## Testing Strategy

### Unit Tests

| Test | Description |
|------|-------------|
| `evaluateImportCondition` — truthy variants | `"true"`, `"1"`, `"yes"` all return `true` |
| `evaluateImportCondition` — falsy variants | `"false"`, `"0"`, `"no"`, `""` all return `false` |
| `evaluateImportCondition` — error path | Non-Boolean output returns a descriptive error |
| `evaluateImportCondition` — template expression | `{{ eq .stage "prod" }}` resolves correctly |
| `buildImportIfContext` — promotion | `stage`, `tenant`, etc. promoted to root |
| `buildImportIfContext` — override semantics | explicit `.vars.*` always available |
| `buildImportIfContext` — fallback | missing vars default to empty string |

### Integration Tests

| Test | Description |
|------|-------------|
| prod stack includes `flow-logs` | `stage: prod` → `import_if: "{{ eq .stage \"prod\" }}"` resolves to true |
| dev stack excludes `flow-logs` | `stage: dev` → same expression resolves to false |
| `pci_scope` boolean pattern | `pci_scope: true` → `import_if: "{{ .vars.pci_scope }}"` resolves to true |
| Sprig functions in expression | `hasPrefix`, `default`, `and`, `or` all work |
| Absent `import_if` unchanged | Plain string imports are unaffected |

---

## Documentation Requirements

1. **`website/docs/stacks/imports.mdx`** — Document `import_if` in the import object reference table and add a "Conditional Imports" section with examples.
2. **Blog post** — Announce the feature with before/after examples.
3. **Roadmap update** — Mark the milestone as shipped in the DX initiative.

---

## Success Metrics

- Teams can conditionally include catalog files without `.yaml.tmpl` workarounds.
- No regression in existing import behaviour (plain string and object imports without `import_if`).
- Positive signal in GitHub Discussions / Issues on template complexity.

---

## References

- PR implementing this feature: [#2202](https://github.com/cloudposse/atmos/pull/2202)
- `StackImport` struct: `pkg/schema/schema.go`
- Stack import processing: `internal/exec/stack_processor_utils.go`
- Atmos documentation on imports: `website/docs/stacks/imports.mdx`
