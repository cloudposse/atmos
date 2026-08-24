# Fix YAML Functions Fail with Templated Arguments When Custom Delimiters Include Quotes

**Date:** 2026-02-20

**Related Issue:** [GitHub Issue #2052](https://github.com/cloudposse/atmos/issues/2052) — `!terraform.state` fails with
`yaml: line NNN: did not find expected key` when custom delimiters include quotes (e.g., `["'{{", "}}'"]`).

**Affected Atmos Version:** v1.205.0+

**Severity:** Medium — users with custom delimiters containing single-quote characters cannot use templated arguments
in any YAML function, forcing them to use only static arguments.

**Affected YAML Functions:** All Atmos YAML functions that accept template arguments are affected:
- `!terraform.state` — Terraform state lookups
- `!terraform.output` — Terraform output lookups
- `!store` / `!store.get` — Store value lookups
- `!env` — Environment variable lookups
- `!exec` — Shell command execution
- `!include` / `!include.raw` — File inclusion
- `!random` — Random value generation
- `!template` — Template evaluation (when containing single quotes)

Functions without arguments (`!cwd`, `!repo-root`, `!aws.*`) are not directly affected since they
produce no YAML quoting conflicts, but the fix handles them generically if they were to contain
single-quote characters in the future.

## Background

Atmos supports custom template delimiters via `templates.settings.delimiters` in `atmos.yaml`. Some users configure
delimiters that include single-quote characters to avoid conflicts with YAML syntax:

```yaml
templates:
  settings:
    delimiters:
      - "'{{"
      - "}}'"
```

With these delimiters, template expressions look like `'{{ .stack }}'` instead of `{{ .stack }}`.

## Symptoms

```yaml
# Any YAML function with a templated argument fails:
vars:
  test: !terraform.state vpc '{{ .stack }}' vpc_id
  out: !terraform.output vpc '{{ .stack }}' vpc_id
  data: !store my-store '{{ .stack }}' key
  cmd: !exec echo '{{ .stack }}'
```

```text
$ atmos describe component <component> -s <stack>
yaml: line NNN: did not find expected key
```

Static arguments work fine:
```yaml
# This works:
vars:
  test: !terraform.state vpc foo
```

## Root Cause

The bug is in the template processing pipeline, specifically in how YAML serialization interacts with custom
delimiter characters. The root cause is **not specific to `!terraform.state`** — it affects ALL YAML functions
because the issue is in the YAML encoding layer, not the function execution layer.

### Processing Pipeline

1. **YAML loading**: Custom tags like `!terraform.state` are converted to plain string values
   (e.g., `"!terraform.state vpc '{{ .stack }}' vpc_id"`).
2. **YAML serialization**: The component section map is serialized to a YAML string via `ConvertToYAML`.
3. **Template processing**: The Go template engine processes the YAML string with custom delimiters.
4. **YAML parsing**: The result is parsed back to a map.

### The Conflict

At step 2, the yaml.v3 encoder must quote strings that start with `!` (YAML's tag indicator). It chooses
**single-quoted style**, which escapes internal single quotes by doubling them (`''`):

```yaml
# Input string: !terraform.state vpc '{{ .stack }}' vpc_id
# YAML-encoded (single-quoted):
test: '!terraform.state vpc ''{{ .stack }}'' vpc_id'
```

This same escaping happens for ALL YAML functions since they all start with `!`:

```yaml
# All of these get single-quoted with '' escaping:
test: '!terraform.output vpc ''{{ .stack }}'' vpc_id'
data: '!store my-store ''{{ .stack }}'' key'
cmd: '!exec echo ''{{ .stack }}'''
```

At step 3, the Go template engine with custom delimiters `'{{` and `}}'` scans the raw YAML text
looking for the delimiter patterns. It finds `'{{` within the `''{{` sequence (where the first `'` is
YAML's escape character, and the second `'` is the start of the delimiter).

After template replacement (e.g., `'{{ .stack }}'` → `nonprod`), the YAML string becomes:

```yaml
test: '!terraform.state vpc 'nonprod' vpc_id'
```

This is **invalid YAML** — the unescaped single quotes inside the single-quoted string break the parser,
producing the `did not find expected key` error.

### Why Default Delimiters Work

With default delimiters `{{` and `}}`, the YAML-escaped string is:

```yaml
test: '!terraform.state vpc ''{{ .stack }}'' vpc_id'
```

The template engine finds `{{ .stack }}` (no quotes in the delimiter pattern), and after replacement:

```yaml
test: '!terraform.state vpc ''nonprod'' vpc_id'
```

This is **valid YAML** — `''` is the proper escape for a single quote in a single-quoted string.

## Fix

### Approach

When custom delimiters contain single-quote characters, use **double-quoted YAML style** for string values
that contain single quotes. Double-quoted YAML strings don't escape single quotes, preserving the delimiter
pattern literally:

```yaml
# Double-quoted (no single-quote escaping):
test: "!terraform.state vpc '{{ .stack }}' vpc_id"
out: "!terraform.output vpc '{{ .stack }}' vpc_id"
data: "!store my-store '{{ .stack }}' key"
cmd: "!exec echo '{{ .stack }}'"
```

After template replacement:

```yaml
test: "!terraform.state vpc nonprod vpc_id"
out: "!terraform.output vpc nonprod vpc_id"
data: "!store my-store nonprod key"
cmd: "!exec echo nonprod"
```

This is **valid YAML** — the double quotes still surround the entire string.

### Implementation

Added `ConvertToYAMLPreservingDelimiters` function that:

1. Checks if custom delimiters conflict with YAML single-quote escaping.
2. If so, marshals to a `yaml.Node` tree (instead of using the default encoder).
3. Walks the node tree and forces `yaml.DoubleQuotedStyle` for scalar nodes containing single quotes.
4. Encodes the modified node tree to YAML string.

The fix is **generic** — it operates at the YAML serialization level and handles ALL scalar values
containing single quotes, regardless of which YAML function prefix they use. This means any future
YAML functions will also be automatically protected.

### Files Changed

| File | Change |
|------|--------|
| `pkg/yaml/delimiter.go` | Add `ConvertToYAMLPreservingDelimiters`, `DelimiterConflictsWithYAMLQuoting`, `EnsureDoubleQuotedForDelimiterSafety` |
| `internal/exec/utils.go` | Use `atmosYaml.ConvertToYAMLPreservingDelimiters` in template processing pipeline |
| `internal/exec/describe_stacks.go` | Use `atmosYaml.ConvertToYAMLPreservingDelimiters` in all 3 template processing sections |
| `internal/exec/terraform_generate_varfiles.go` | Use `atmosYaml.ConvertToYAMLPreservingDelimiters` in template processing |
| `internal/exec/terraform_generate_backends.go` | Use `atmosYaml.ConvertToYAMLPreservingDelimiters` in template processing |

### Tests

**Unit tests** (`pkg/yaml/delimiter_test.go`):
- `TestDelimiterConflictsWithYAMLQuoting` — 8 subtests for delimiter conflict detection
- `TestEnsureDoubleQuotedForDelimiterSafety` — 6 subtests for node style modification
- `TestConvertToYAMLPreservingDelimiters` — 10 subtests including:
  - Preserves single-quote delimiters in YAML function values
  - Falls back to standard encoding for default delimiters
  - Preserves all values correctly after double-quoting
  - Template replacement produces valid YAML with custom delimiters
  - Demonstrates that standard encoding breaks with custom delimiters
  - Handles nested maps and lists with YAML function values
- `TestAllYAMLFunctionsPreservedWithCustomDelimiters` — 12 subtests verifying every YAML function prefix:
  - `!terraform.state`, `!terraform.output`, `!store`, `!store.get`, `!env`, `!exec`,
    `!template`, `!include`, `!include.raw`, `!repo-root`, `!cwd`, `!random`
- `TestAllYAMLFunctionsTemplateReplacementWithCustomDelimiters` — 9 subtests simulating full
  template processing pipeline (serialize → replace → parse) for each YAML function
- `TestStandardEncodingBreaksAllYAMLFunctionsWithCustomDelimiters` — 18 subtests (9 functions × 2)
  proving both that standard encoding breaks AND delimiter-safe encoding works for each function

**Integration tests** (`tests/yaml_functions_custom_delimiters_test.go`):
- `TestYAMLFunctionsWithCustomDelimiters` — 2 subtests:
  - Regular templates with custom delimiters (component-1)
  - Static and templated `!terraform.state` args in a single test (component-2, core issue #2052)

Run with:
```bash
go test ./pkg/yaml/ -run TestConvertToYAMLPreserving -v
go test ./pkg/yaml/ -run TestDelimiterConflicts -v
go test ./pkg/yaml/ -run TestEnsureDoubleQuoted -v
go test ./pkg/yaml/ -run TestAllYAMLFunctions -v
go test ./pkg/yaml/ -run TestStandardEncodingBreaksAll -v
go test ./tests/ -run TestYAMLFunctionsWithCustomDelimiters -v
```
