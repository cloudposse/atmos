# Locals: `!terraform.state` Fails with "stack is required" Error

**Date:** 2026-03-15

**Related Issues:**
- [#2080](https://github.com/cloudposse/atmos/issues/2080) — `!terraform.state` in catalog file `locals`
  fails with "stack is required" error
- User report: `!terraform.state` in locals fails because stack context is not available

**Affected Atmos Versions:** v1.200.0 through v1.209.0

**Severity:** High — `!terraform.state` and `!terraform.output` YAML functions are broken in
locals, despite being documented as supported

---

## File-Scoped Locals Architecture

### Overview

File-scoped locals is a major feature in Atmos that provides Terraform-like local variables within stack
configuration files. Unlike `vars` which inherit across imports, **locals are strictly file-scoped** and never
propagate across file boundaries. This enables users to reduce repetition and define reusable computed values
within a single file using dependency resolution with cycle detection.

### Processing Pipeline

```text
1. Stack File Loading
   Raw YAML file → Parse YAML → Extract locals sections (global, terraform, helmfile, packer)

2. Locals Resolution (for each scope: global → section → component)
   Extract "locals:" map
     → Build template context (settings, vars, env from same file)
     → Create Resolver with parent locals
     → Build dependency graph (extract .locals.X references)
     → Topological sort (Kahn's algorithm)
     → Resolve in order (YAML functions first, then Go templates)
     → Return merged locals (parent + resolved)

3. Template Processing
   Component vars/settings/env templates
     → Template context includes .locals, .settings, .vars, .env
     → Render template → Final value passed to Terraform/Helmfile

4. Output (describe locals)
   ProcessStackLocals() → LocalsContext
     → If component: merge global + section + component locals
     → If stack only: show stack-level locals with section overrides
     → YAML/JSON output
```

### Core Package: pkg/locals/resolver.go

#### Resolver Architecture

```go
type Resolver struct {
    locals                map[string]any        // Raw local definitions
    resolved              map[string]any        // Resolved local values
    dependencies          map[string][]string   // Dependency graph
    filePath              string                // For error messages
    templateContext       map[string]any        // Settings, vars, env context
    yamlFunctionProcessor YamlFunctionProcessor // YAML function callback
}
```

#### Resolution Steps

1. **Build Dependency Graph** (`buildDependencyGraph()`)
   - Extracts `.locals.X` references from template strings using AST utilities.
   - Handles strings, maps, and slices recursively.
   - Uses `pkg/template.ExtractFieldRefsByPrefix(str, "locals")`.

2. **Topological Sort with Cycle Detection** (`topologicalSort()`)
   - Uses **Kahn's algorithm** with in-degree calculation.
   - Detects cycles using **DFS** before outputting.
   - Maintains deterministic order by sorting alphabetically at each step.
   - Clear error messages showing the cycle path (e.g., `a -> b -> c -> a`).

3. **Template Resolution** (`resolveLocal()` and `resolveString()`)
   - Process YAML functions first (via callback) if string starts with `!`.
   - Then process Go templates using Sprig function library.
   - Template context includes: resolved locals, settings, vars, env.
   - Error reporting includes available locals and context keys.

#### Template Context

```go
context := map[string]any{
    "settings": ...,  // From same file
    "vars":     ...,  // From same file
    "env":      ...,  // Environment section
    "locals":   resolved,  // Currently resolved locals
}
```

### Scope Hierarchy

#### Multi-Level Scopes with Inheritance

```text
Global (root level)
  ↓ inherits
Component-Type (terraform/helmfile/packer section)
  ↓ inherits
Component-Level (inside component definition)
  ↓ inherits from base components via metadata.inherits
Final resolved locals
```

#### Merge Order (for a component)

1. Global Locals (from root level)
2. Section Locals (from terraform/helmfile/packer section)
3. Base Component Locals (via `metadata.inherits`)
4. Component's Own Locals
5. Final merged locals available to templates

#### File-Scoped vs Component-Level

**File-Scoped Locals (Global + Section):**
- Defined once per file.
- Available to all components of compatible type.
- Can be overridden by component-level locals (more specific scope wins).
- Use for: common values, naming conventions, shared computed values.

**Component-Level Locals:**
- Defined within each component block.
- Override file-scoped locals.
- Support inheritance from base components.
- Use for: component-specific customization, derived from inherited values.

### Stack Processing: internal/exec/stack_processor_locals.go

#### Main Functions

**`ProcessStackLocals()`** — Top-level orchestrator:
1. Builds template context from stack config (settings, vars, env).
2. Resolves global locals (no parent).
3. Resolves terraform section locals (inherit from global).
4. Resolves helmfile section locals (inherit from global).
5. Resolves packer section locals (inherit from global).
6. Returns `LocalsContext` with all scope levels.

**`ExtractAndResolveLocals()`** — Single scope resolver:
1. Check if locals section exists.
2. Validate it's a map.
3. Create `locals.Resolver` with template context (settings, vars, env from same file) and YAML function processor.
4. Call `Resolve(parentLocals)` for dependency ordering.

**`createYamlFunctionProcessor()`** — YAML function support:
- Wraps value in a map.
- Calls `ProcessCustomYamlTags()` to handle `!terraform.state`, `!env`, etc.
- Extracts processed result.

#### LocalsContext Struct

```go
type LocalsContext struct {
    Global              map[string]any
    Terraform           map[string]any  // Global + terraform section
    Helmfile            map[string]any  // Global + helmfile section
    Packer              map[string]any  // Global + packer section
    HasTerraformLocals  bool
    HasHelmfileLocals   bool
    HasPackerLocals     bool
}
```

**Methods:**
- `MergeForComponentType(type)` — Get merged locals for specific component type.
- `MergeForTemplateContext()` — Get all locals merged (Global -> Terraform -> Helmfile -> Packer).
- `GetForComponentType(type)` — Get finals for template usage.

### CLI Command: atmos describe locals

#### Usage

```bash
atmos describe locals [component] -s <stack>
atmos describe locals --stack deploy/dev
atmos describe locals vpc -s prod
atmos describe locals -s dev --format json
```

#### Flags

- `-s, --stack` (required) — Stack identifier (file path or logical name).
- `-f, --format` — Output format (yaml/json, default: yaml).
- `--file` — Write output to file.
- `--query` — YQ expression to filter results.

#### Output Format

**Stack-Level:**

```yaml
locals:
  namespace: acme
terraform:
  locals:
    backend_bucket: acme-tfstate
```

**Component-Level:**

```yaml
components:
  terraform:
    vpc:
      locals:
        namespace: acme
        backend_bucket: acme-tfstate
```

### Supported Features

1. **Locals referencing other locals** with topological sort resolution.
2. **Circular dependency detection** with clear error formatting (`a -> b -> c -> a`).
3. **Multi-level scopes** with inheritance from outer to inner scopes.
4. **Component inheritance** — component-level locals inherit from base components.
5. **No cross-file inheritance** — intentionally file-scoped only.
6. **Complex values** — maps, nested structures, lists with templates.
7. **Sprig functions** — `upper`, `quote`, conditionals, etc.
8. **YAML functions** — `!env`, `!terraform.state`, `!store`, `!exec`, etc.
9. **Settings/vars/env access** — `{{ .settings.X }}`, `{{ .vars.X }}`, `{{ .env.X }}` from same file.

### Key Design Decisions

#### 1. File-Scoped, Not Inherited
- Locals do NOT cross file boundaries via imports.
- Predictability: you see exactly what locals are available by reading the file.
- No hidden dependencies across files.
- Safer refactoring: changing a local doesn't break other files.
- Clear separation: use `vars` for propagating values, `locals` for file convenience.

#### 2. Topological Sort with Cycle Detection
- Kahn's algorithm for deterministic ordering.
- DFS catches cycles before processing.
- Deterministic output (alphabetical sorting at each step).
- Clear error messages showing cycle path.

#### 3. Template Context Access
- Locals can access settings, vars, env from same file.
- Enables computed values based on other file sections.
- No need to duplicate values.

#### 4. YAML Functions in Locals
- Support `!terraform.state`, `!env`, `!store`, etc.
- YAML functions resolved before templates.
- Enables sophisticated composed configurations.

#### 5. Not in Output
- Locals stripped from component output (unlike settings).
- Strictly internal convenience tools.
- Not part of the Terraform/Helmfile input.

### Limitations and Edge Cases

1. **No cross-file access:** Cannot reference locals from imported files (intentional).
2. **No dynamic key names:** Locals keys must be literals.
3. **Settings/vars dependency:** If a local references an undefined setting/var, it will template error.
4. **Windows path escaping:** Need single quotes in YAML when locals reference env vars with backslashes.
5. **Template processing required:** If locals are defined, template processing is enabled for the whole file
   (watch for `skip_templates_processing` in imports).
6. **Circular dependencies detected at resolution time:** Not at parse time.

### Implementation File Summary

| File                                      | Lines | Purpose                                                         |
|-------------------------------------------|-------|-----------------------------------------------------------------|
| `pkg/locals/resolver.go`                  | ~527  | Core dependency resolution, topological sort, cycle detection   |
| `pkg/locals/resolver_test.go`             | ~727  | 31 comprehensive test cases covering all scenarios              |
| `internal/exec/stack_processor_locals.go` | ~416  | Stack file processing, multi-scope extraction, template context |
| `internal/exec/describe_locals.go`        | ~558  | CLI command execution, stack/component locals output            |
| `cmd/describe_locals.go`                  | ~172  | Command registration, flag parsing, CLI interface               |
| `docs/prd/file-scoped-locals.md`          | ~3000 | Complete specification and design documentation                 |
| `examples/locals/`                        | -     | Working example with dev/prod stacks                            |

### Test Fixtures (13 Scenarios)

| Fixture                      | What It Tests                                                    |
|------------------------------|------------------------------------------------------------------|
| `locals`                     | Basic locals usage and resolution                                |
| `locals-advanced`            | Nested value access in settings/vars, section-specific locals    |
| `locals-circular`            | Circular dependency detection with clear error messages          |
| `locals-component-level`     | Component-level locals with inheritance via metadata.inherits    |
| `locals-conditional`         | Go template conditionals with !env in locals (new, for this fix) |
| `locals-deep-import-chain`   | Locals don't propagate through multi-level import chains         |
| `locals-env-test`            | Environment variable access in locals                            |
| `locals-file-scoped`         | File-scoping enforcement (locals stay within their file)         |
| `locals-logical-names`       | Logical stack names with name_template work correctly            |
| `locals-not-inherited`       | Explicit proof that locals are NOT inherited across imports      |
| `locals-settings-access`     | Locals can access settings from the same file                    |
| `locals-settings-cross-file` | Cross-file settings behavior with locals                         |
| `locals-yaml-functions`      | YAML functions (!env, !terraform.state, etc.) in locals          |

---

## Issue Description

### Issue 1: Go Template Conditionals in Locals (v1.200.0)

A user reports that Go template conditionals in locals are too verbose:

```yaml
locals:
  pr_number: !env PR_NUMBER
  datastream_name: '{{ if .locals.pr_number }}datastreampr{{ .locals.pr_number }}{{ else }}datastream{{ end }}'
```

**Status:** This works correctly in v1.205.0+ after the file-scoped-locals-fix release. The user was on
v1.200.0 which predated the fix. **No code change needed** — this is a version issue.

### Issue 2: `!terraform.state` in Locals (GitHub #2080)

When using `!terraform.state` with the 2-argument form in a `locals` block:

```yaml
locals:
  vpc_id: !terraform.state vpc .vpc_id

components:
  terraform:
    eks:
      vars:
        vpc_id: "{{ .locals.vpc_id }}"
```

The error:

```text
Error: invalid stack manifest: failed to process stack locals: failed to resolve global locals:
  failed to process YAML function in local "vpc_id":
  failed to describe component vpc in stack ``
  in YAML function: !terraform.state vpc .vpc_id
  stack is required; specify it on the command line using the flag --stack <stack>
```

### Issue 3: `!terraform.state` with Explicit Stack Template

User tried workarounds:

```yaml
# Attempt 1: Template in YAML function argument — invalid argument count
locals:
  my_var: !terraform.state example/componentZero {{ .stack }} '.attr["key"]'

# Attempt 2: Quoted template — hangs
locals:
  my_var: !terraform.state example/componentZero "{{ .stack }}" '.attr["key"]'
```

---

## Root Cause Analysis

### The Stack Context Gap

The root cause is in `internal/exec/stack_processor_utils.go`, function `extractLocalsFromRawYAML`:

```go
// Line 82-84:
// Note: At this early stage, stack name is not yet determined, so we pass empty string.
// YAML functions that require stack context won't work here, but Go templates will.
localsCtx, err := ProcessStackLocals(atmosConfig, rawConfig, filePath, "")
```

The stack name is passed as **empty string** (`""`) because locals extraction happens during early YAML
processing — before the stack context is fully formed. This is by design for the general case, but it
breaks YAML functions that require stack context.

### How the Empty Stack Propagates

1. `extractLocalsFromRawYAML()` passes `""` as `currentStack`
2. → `ProcessStackLocals()` passes it to `ExtractAndResolveLocals()`
3. → `createYamlFunctionProcessor()` captures the empty `currentStack`
4. → `processTagTerraformState()` receives empty `currentStack`
5. → For 2-arg form (`!terraform.state component .output`), uses `currentStack` as stack (line 82):
   ```go
   case 2:
       component = strings.TrimSpace(parts[0])
       stack = currentStack  // This is ""!
       output = strings.TrimSpace(parts[1])
   ```
6. → `stateGetter.GetState()` fails because stack is empty

### Why This Is Different From `!env`

`!env` works in locals because it doesn't need stack context — it reads environment variables directly.
`!terraform.state` with 2 args implicitly uses the current stack, which isn't available during early
processing.

---

## Fix

### Approach: Derive Stack Name from File Path Before Locals Processing

The stack name can be derived from the file path using the same logic that `describe locals` uses
(`deriveStackName`). We need to:

1. Parse the raw YAML to get the `vars` section
2. Derive the stack name from the file path + vars + atmos config
3. Pass the derived stack name to `ProcessStackLocals`

### Implementation

Two new helper functions added to `internal/exec/stack_processor_utils.go`:

**`deriveStackNameForLocals()`** — extracts vars from raw config and calls `deriveStackName()` (from
`describe_locals.go`) to compute the stack name from file path + vars + atmos config.

**`computeStackFileName()`** — strips the stacks base path prefix and file extension from an absolute
file path to produce a relative name like `deploy/dev` or `orgs/acme/plat/dev/us-east-1`.

The fix in `extractLocalsFromRawYAML()` replaces the empty string `""` with the derived stack name:

```go
// Before (broken):
localsCtx, err := ProcessStackLocals(atmosConfig, rawConfig, filePath, "")

// After (fixed):
currentStack := deriveStackNameForLocals(atmosConfig, rawConfig, filePath)
localsCtx, err := ProcessStackLocals(atmosConfig, rawConfig, filePath, currentStack)
```

### File Changes

| File                                           | Change                                                                              |
|------------------------------------------------|-------------------------------------------------------------------------------------|
| `internal/exec/stack_processor_utils.go`       | Add `deriveStackNameForLocals()`, `computeStackFileName()`; pass derived stack name |
| `internal/exec/stack_processor_utils_test.go`  | 7 new unit tests (see Test Results below)                                           |
| `tests/cli_locals_test.go`                     | 2 new integration tests for Go template conditionals                                |
| `tests/fixtures/scenarios/locals-conditional/` | New fixture with isolated stacks for conditional tests                              |

---

## Configuration Example

After the fix, this will work correctly:

```yaml
# stacks/catalog/eks/defaults.yaml
locals:
  vpc_id: !terraform.state vpc .vpc_id
  subnet_ids: !terraform.state vpc .private_subnet_ids

components:
  terraform:
    eks:
      vars:
        vpc_id: "{{ .locals.vpc_id }}"
        subnet_ids: "{{ .locals.subnet_ids }}"
```

```yaml
# stacks/orgs/acme/plat/dev/us-east-1.yaml
import:
  - catalog/eks/defaults
```

```bash
atmos terraform plan eks --stack plat-ue1-dev
# Now works! !terraform.state resolves using the stack context from the importing stack.
```

---

## Backward Compatibility

- All existing locals usage is unaffected
- `!env`, `!exec`, `!store` in locals continue to work as before
- `!terraform.state` with 3 args (explicit stack) was already working
- `!terraform.state` with 2 args (implicit stack) now works when the stack can be derived
- If the stack cannot be derived (e.g., catalog file processed without stack context), the error message
  is improved to explain the limitation

---

## Test Results

All tests pass after the fix.

### Unit Tests (`internal/exec/`)

- `TestComputeStackFileName` — 8 cases (simple path, nested org, yml, yaml.tmpl, yml.tmpl, unknown extension, no extension, nil config)
- `TestDeriveStackNameForLocals` — 3 cases (name_pattern, nil config, no vars)
- `TestExtractLocalsFromRawYAML_StackNameDerived` — confirms `!env` + Go template conditionals work
- `TestExtractLocalsFromRawYAML_GoTemplateConditionalEmpty` — confirms empty env var takes else branch
- `TestExtractLocalsFromRawYAML_TerraformStateInLocals` — mock-based: verifies derived stack name
  ("dev") is passed to `stateGetter.GetState()` instead of empty string (core fix for #2080)
- `TestExtractLocalsFromRawYAML_TerraformStateComposedLocals` — mock-based: verifies `!terraform.state`
  results can be composed with Go templates in other locals
- `TestExtractLocalsFromRawYAML_TerraformState3ArgForm` — mock-based: verifies 3-arg form with explicit
  stack still works correctly

### Integration Tests (`tests/`)

- `TestLocalsGoTemplateConditionalWithEnvSet` — end-to-end: Go template conditional with `!env` set
- `TestLocalsGoTemplateConditionalWithEnvEmpty` — end-to-end: Go template conditional with `!env` empty

### Regression

- All 20 existing `TestExtractLocals*` unit tests pass
- All `pkg/locals/` tests pass
- All 37 locals integration tests pass
