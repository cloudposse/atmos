# PRD: YAML `!literal` Function

## Overview

Add a `!literal` YAML tag function that preserves values exactly as written, bypassing all template processing. This enables users to pass through template-like syntax (e.g., `{{...}}`) to downstream tools without Atmos attempting to evaluate them.

## Problem Statement

Atmos processes Go templates and Gomplate expressions in stack configurations. When users need to pass template syntax to downstream systems (Terraform, Helm, external tools), Atmos attempts to evaluate these expressions, causing errors or unexpected behavior.

### Current Workarounds

Users must resort to awkward escaping:

```yaml
# Workaround 1: Double braces (fragile, hard to read)
db_users:
  - "{{'{{external.email}}'}}"

# Workaround 2: Template escaping (verbose)
db_users:
  - "{{ `{{external.email}}` }}"
```

These workarounds are:
- **Error-prone**: Easy to get escaping wrong
- **Hard to read**: Nested braces obscure intent
- **Inconsistent**: Different escape methods for different contexts
- **Undiscoverable**: Users must search for solutions

### Real-World Use Cases

1. **Terraform templatefile()**: Pass variables to Terraform templates
   ```yaml
   vars:
     user_data_template: !literal "#!/bin/bash\necho ${hostname}"
   ```

2. **Helm values**: Pass Helm template expressions
   ```yaml
   vars:
     annotations: !literal "{{ .Values.ingress.class }}"
   ```

3. **External templating systems**: ArgoCD, Jsonnet, Kustomize overlays
   ```yaml
   vars:
     config: !literal "{{external.config_url}}"
   ```

4. **Documentation/examples**: Include template examples in configs
   ```yaml
   metadata:
     example_usage: !literal "Use {{component}} to reference components"
   ```

5. **Regex patterns**: Patterns containing brace-like syntax
   ```yaml
   vars:
     pattern: !literal "user_{id}_{timestamp}"
   ```

## Proposed Solution

Add `!literal` as a new YAML tag function that marks values as "do not process."

### Syntax

```yaml
# Single value
email: !literal "{{external.email}}"

# In a list
db_users:
  - !literal "{{external.email}}"
  - !literal "{{external.admin}}"

# Multiline
script: !literal |
  #!/bin/bash
  echo "Hello ${USER}"
  export VAR={{value}}
```

### Behavior

1. **Bypass template processing**: The value is never passed through Go template or Gomplate evaluation
2. **Preserve exact content**: Whitespace, special characters, and brace patterns are preserved exactly
3. **Early processing**: Handled at YAML parse time (like `!include`), before template evaluation
4. **Type preservation**: Returns the value as a string

### Implementation Approach

The `!literal` tag would be processed in `pkg/utils/yaml_utils.go` during early YAML processing:

1. Add `AtmosYamlFuncLiteral = "!literal"` constant
2. In `processCustomTags()`, detect `!literal` tag
3. Strip the tag and return the value as-is (no further processing)
4. Mark the value internally to skip template processing in later stages

## Why "Nice to Have" (Not Critical)

### It's Solvable Today

The workarounds exist and work. Users can escape template syntax using Go template escaping or backtick quoting. This is inconvenient but not blocking.

### Limited Audience

This primarily affects users who:
- Integrate Atmos with other templating systems
- Pass template expressions to Terraform/Helm
- Have complex multi-tool pipelines

Most Atmos users don't encounter this frequently.

### Low Frequency

Even for affected users, they typically only need this in a few specific places, not throughout their configs.

## Why It's Worth Doing

### Developer Experience

- **Discoverable**: `!literal` is self-documenting; escaping syntax is not
- **Consistent**: One clear way to say "don't process this"
- **Readable**: Intent is obvious at a glance

### Reduces Support Burden

Common question: "How do I pass `{{...}}` to Terraform without Atmos processing it?" A dedicated function provides a clear answer.

### Aligns with Existing Patterns

- Atmos already has `!include.raw` for raw file inclusion
- YAML 1.1 had `!!literal` for similar purposes
- Jinja2/Ansible users expect `raw` or `literal` constructs

### Low Implementation Cost

- Small, isolated change
- Clear semantics
- No breaking changes
- Easy to test

## Naming Alternatives Considered

| Name | Pros | Cons | Verdict |
|------|------|------|---------|
| `!literal` | Standard YAML 1.1 term, Ansible uses it, self-documenting | Slightly longer | **Selected** |
| `!raw` | Consistent with `!include.raw`, Jinja2 familiarity, short | Less universally understood | Runner-up |
| `!verbatim` | LaTeX familiarity, very explicit | Too long, less common in DevOps | Rejected |
| `!passthrough` | Clear intent | Verbose, not standard terminology | Rejected |
| `!escape` | Familiar concept | Implies character escaping (e.g., `\n`), misleading | Rejected |
| `!noop` | Programming familiarity | Not intuitive for ops users, unclear intent | Rejected |
| `!quote` | Shell familiarity | Implies quoting behavior, not template bypass | Rejected |
| `!plain` | Simple | Too vague, doesn't convey "no processing" | Rejected |
| `!static` | Clear opposite of "dynamic" | Could be confused with static files/resources | Rejected |
| `!opaque` | Technical precision | Obscure, not self-documenting | Rejected |

### Decision Rationale

**`!literal`** was chosen because:
1. YAML 1.1 spec used `!!literal` for similar purposes
2. Ansible uses `!literal` - familiar to infrastructure engineers
3. Self-documenting: "this is the literal value, don't interpret it"
4. Clear semantic meaning across programming backgrounds

**`!raw`** is the runner-up and could be added as an alias later for consistency with `!include.raw`.

## Other Alternatives Considered

### 1. Configuration-Level Disable

Add a setting to disable template processing for entire sections:

```yaml
vars:
  _template_processing: false
  user_data: "{{hostname}}"
```

**Pros**: Handles multiple values at once
**Cons**: Less granular, more complex, hidden behavior

**Decision**: Per-value `!literal` is simpler and more explicit.

### 2. Escape Sequence

Support `\{{` to escape braces:

```yaml
vars:
  email: "\{{external.email}}"
```

**Pros**: Familiar from other languages
**Cons**: Requires escaping in the middle of strings, harder to read, conflicts with YAML escaping

**Decision**: `!literal` is cleaner for the common case of "don't process this entire value."

## Success Metrics

- Reduction in GitHub issues/discussions about template escaping
- Positive user feedback on discoverability
- No increase in template processing bugs

## Documentation Requirements

1. Add `!literal` to YAML functions reference
2. Include examples in template documentation
3. Add to "Common Patterns" or FAQ section

## Out of Scope

- Partial literal (escaping only part of a string) - use existing Go template escaping
- Recursive literal (literal values in `!include` files) - handle separately if needed
- Binary/encoded content - not the purpose of this feature
