# PRD: File-Scoped Locals in Atmos Stack Configuration

## Overview

This PRD defines a new `locals` section for Atmos stack configuration files that provides file-scoped variable definitions for cleaner, more maintainable configurations—similar to Terraform and Terragrunt locals.

## Problem Statement

Currently, when users need to define reusable values within a stack configuration file, they must either:
1. Duplicate values across multiple places in the file
2. Use `vars` or `settings` sections, which have different semantics (vars are passed to Terraform, settings are inherited across files)
3. Create additional files just to hold shared values

This leads to verbose, repetitive configurations that are harder to maintain and more error-prone.

## Solution

Introduce a `locals` section that:
- Is **file-scoped only** (never inherited across file boundaries via imports)
- Can be defined at multiple scopes within a file: global, component-type (terraform/helmfile), and component level
- Is resolved **before** other sections, making local values available for use in `vars`, `settings`, `env`, etc.
- Is **not** passed to Terraform/Helmfile (unlike `vars`)
- Is **not** visible in `atmos describe component` output (unlike `settings`)

## User Experience

### Basic Usage

```yaml
# stacks/catalog/vpc.yaml
locals:
  base_cidr: "10.0.0.0"
  environment_suffix: "prod"
  full_name: "{{ .locals.base_cidr }}-{{ .locals.environment_suffix }}"

components:
  terraform:
    vpc:
      vars:
        cidr_block: "{{ .locals.base_cidr }}/16"
        name: "vpc-{{ .locals.environment_suffix }}"
```

### Locals Referencing Other Locals

Locals can reference other locals within the same scope. References are resolved using topological sorting with cycle detection:

```yaml
locals:
  # Base values
  project: "myapp"
  environment: "prod"
  region: "us-east-1"

  # Derived values (reference other locals)
  prefix: "{{ .locals.project }}-{{ .locals.environment }}"
  full_prefix: "{{ .locals.prefix }}-{{ .locals.region }}"
  bucket_name: "{{ .locals.full_prefix }}-assets"

components:
  terraform:
    s3:
      vars:
        bucket: "{{ .locals.bucket_name }}"  # "myapp-prod-us-east-1-assets"
```

**Order doesn't matter** - locals can be defined in any order. The system builds a dependency graph and resolves them in the correct order:

```yaml
locals:
  # These work regardless of definition order
  c: "{{ .locals.a }}-{{ .locals.b }}"  # Depends on a and b
  a: "first"                              # No dependencies
  b: "{{ .locals.a }}-second"             # Depends on a
  # Resolution order: a → b → c
```

**Circular references are detected and reported:**

```yaml
locals:
  # ❌ Error: circular dependency detected: a → b → c → a
  a: "{{ .locals.c }}"
  b: "{{ .locals.a }}"
  c: "{{ .locals.b }}"
```

### Scoped Locals

```yaml
# Global locals (available to all components in this file)
locals:
  region: "us-east-1"
  account_id: "123456789012"

# Terraform-wide locals (available to all terraform components in this file)
terraform:
  locals:
    state_bucket: "terraform-state-{{ .locals.account_id }}"

  vars:
    backend_bucket: "{{ .locals.state_bucket }}"

# Component-specific locals
components:
  terraform:
    vpc:
      locals:
        vpc_name: "main-vpc-{{ .locals.region }}"
      vars:
        name: "{{ .locals.vpc_name }}"
        tags:
          Name: "{{ .locals.vpc_name }}"
```

### Locals Do NOT Inherit Across Files

```yaml
# stacks/_defaults.yaml
locals:
  shared_value: "from-defaults"  # This is ONLY available in _defaults.yaml

vars:
  some_var: "{{ .locals.shared_value }}"  # Works - same file

# stacks/deploy/prod.yaml
import:
  - _defaults

locals:
  prod_value: "prod-specific"

components:
  terraform:
    vpc:
      vars:
        # ✅ Works - prod_value is defined in this file
        name: "{{ .locals.prod_value }}"

        # ❌ Error - shared_value is NOT available (it was in _defaults.yaml, not inherited)
        # bad_ref: "{{ .locals.shared_value }}"
```

### Using YAML Functions in Locals

```yaml
locals:
  # Static values
  base_name: "myapp"

  # Computed from environment
  aws_region: !env AWS_REGION

  # Computed from other sources
  vpc_id: !terraform.output vpc/outputs/vpc_id

  # Templated values
  full_arn: !template "arn:aws:s3:::{{ .locals.base_name }}-{{ .locals.aws_region }}"

components:
  terraform:
    my-component:
      vars:
        vpc_id: "{{ .locals.vpc_id }}"
```

## Scope Resolution Order

Within a single file, locals are resolved in this order (inner scopes can reference outer scopes):

1. **Global locals** → resolved first, available everywhere in the file
2. **Component-type locals** (terraform/helmfile) → can reference global locals
3. **Component locals** → can reference global and component-type locals

```yaml
locals:
  global_val: "global"

terraform:
  locals:
    tf_val: "{{ .locals.global_val }}-terraform"

components:
  terraform:
    vpc:
      locals:
        component_val: "{{ .locals.tf_val }}-vpc"
      vars:
        name: "{{ .locals.component_val }}"  # Results in "global-terraform-vpc"
```

## Behavior Clarifications

### What Locals Are NOT

| Feature | Locals | Vars | Settings |
|---------|--------|------|----------|
| Inherited across imports | ❌ No | ✅ Yes | ✅ Yes |
| Passed to Terraform/Helmfile | ❌ No | ✅ Yes | ❌ No |
| Visible in `describe component` | ❌ No | ✅ Yes | ✅ Yes |
| Available in templates within same file | ✅ Yes | ✅ Yes | ✅ Yes |
| Purpose | File-scoped temp variables | Tool inputs | Component metadata |

### Error Handling

1. **Reference to undefined local**: Clear error message indicating the local doesn't exist
   ```text
   Error: undefined local "foo" referenced in stacks/deploy/prod.yaml

   Available locals in this file:
     - bar
     - baz

   Hint: Locals are file-scoped and do not inherit from imported files.
   ```

2. **Reference to local from imported file**: Clear error explaining locals don't inherit
   ```text
   Error: undefined local "shared_value" referenced in stacks/deploy/prod.yaml

   Hint: "shared_value" is defined in stacks/_defaults.yaml but locals do not
   inherit across files. Consider using vars or settings if cross-file sharing
   is needed.
   ```

3. **Circular reference**: Detected and reported with clear dependency chain
   ```text
   Error: circular dependency in locals at stacks/deploy/prod.yaml

   Dependency cycle detected:
     a → b → c → a

   Referenced locals:
     a: "{{ .locals.c }}"  (line 5)
     b: "{{ .locals.a }}"  (line 6)
     c: "{{ .locals.b }}"  (line 7)
   ```

### Edge Cases

**Empty locals section**: Valid, no-op
```yaml
locals: {}
```

**Locals referencing vars/settings**: Allowed, but vars/settings must be resolvable at that point
```yaml
vars:
  base: "value"

locals:
  derived: "{{ .vars.base }}-extended"  # Works if vars.base is static
```

**Component inheritance (`metadata.inherits`)**: Locals are NOT inherited through component inheritance—they are purely file-scoped
```yaml
# catalog/base.yaml
components:
  terraform:
    base-vpc:
      locals:
        base_local: "value"  # NOT inherited

# deploy/prod.yaml
components:
  terraform:
    vpc:
      metadata:
        inherits:
          - base-vpc
      vars:
        # ❌ Error - base_local is not available (was in catalog/base.yaml)
        # name: "{{ .locals.base_local }}"
```

## Implementation Considerations

### Processing Order

1. Parse YAML file
2. **Extract locals at each scope (global → component-type → component)**
3. **Build dependency graph for locals within each scope**
4. **Topologically sort and resolve locals (with cycle detection)**
5. Make resolved locals available in template context
6. Process remaining sections (vars, settings, env, etc.) with locals in context
7. Continue normal stack processing (imports, inheritance, merging)

### Locals Resolution Algorithm

For each scope (global, component-type, component):

1. **Parse phase**: Extract all local definitions without resolving templates
2. **Dependency extraction**: Scan each local's value for `{{ .locals.X }}` references
3. **Graph construction**: Build directed graph where edges represent dependencies
4. **Cycle detection**: Use DFS to detect cycles; if found, report error with full cycle path
5. **Topological sort**: Order locals so dependencies are resolved before dependents
6. **Resolution phase**: Resolve locals in sorted order, making each available for subsequent locals

```
Example: locals = {c: "{{.locals.b}}", a: "val", b: "{{.locals.a}}"}

1. Dependencies: c→[b], a→[], b→[a]
2. Topological order: [a, b, c]
3. Resolve:
   - a = "val"
   - b = "val" (a is now available)
   - c = "val" (b is now available)
```

### Cross-Scope Local References

Inner scopes inherit resolved locals from outer scopes:

```yaml
locals:
  global_val: "global"          # Scope 1: global

terraform:
  locals:
    tf_val: "{{ .locals.global_val }}-tf"  # Scope 2: can see global_val

components:
  terraform:
    vpc:
      locals:
        comp_val: "{{ .locals.tf_val }}-vpc"  # Scope 3: can see global_val AND tf_val
```

Resolution order:
1. Resolve global locals (scope 1)
2. Resolve terraform locals with global locals in context (scope 2)
3. Resolve component locals with global + terraform locals in context (scope 3)

### Template Context

Locals should be available via `.locals` in Go templates:
```yaml
locals:
  name: "example"

vars:
  full_name: "{{ .locals.name }}-component"
```

### Schema Updates

The JSON schema needs updates to allow `locals` at:
- Stack root level
- `terraform` section
- `helmfile` section
- `packer` section
- Individual component definitions

## Success Criteria

1. Users can define file-scoped variables that don't pollute `vars` or `settings`
2. Locals are clearly file-scoped and don't leak across import boundaries
3. Clear, actionable error messages for common mistakes
4. Performance impact is minimal (locals resolved once per file)
5. Works with existing YAML functions (`!template`, `!env`, `!exec`, etc.)

## Non-Goals (Out of Scope)

- Cross-file local sharing (use `vars` or `settings` for that)
- Lazy evaluation of locals (all resolved upfront)
- Locals in `atmos.yaml` (stack files only)
- Export locals to child files (opposite of file-scoped)

## Design Decisions

### Locals NOT Available in Imports

Locals are processed **after** imports are resolved. This keeps the import system simple and predictable:

```yaml
# ❌ NOT supported - imports are resolved before locals
locals:
  env: "prod"

import:
  - "catalog/{{ .locals.env }}/base.yaml"  # Error: locals not available here
```

If dynamic imports are needed, use environment variables or template context instead.

### No Special "Promote" Syntax

To use a local value in vars/settings, use standard template syntax. No special `!local` function:

```yaml
locals:
  computed: "value"

vars:
  my_var: "{{ .locals.computed }}"  # Standard approach
```

### No Name Collision (Separate Namespaces)

Locals and vars exist in separate namespaces (`.locals.*` vs `.vars.*`), so there's no collision:

```yaml
locals:
  name: "local-value"

vars:
  name: "var-value"

components:
  terraform:
    example:
      vars:
        from_local: "{{ .locals.name }}"  # "local-value"
        from_var: "{{ .vars.name }}"      # "var-value"
```

### All YAML Functions Supported

Locals support the same YAML functions as vars (`!template`, `!env`, `!exec`, `!terraform.output`, `!terraform.state`, `!store.get`). Since locals are resolved once per file, expensive operations are cached:

```yaml
locals:
  vpc_id: !terraform.output vpc/outputs/vpc_id
  region: !env AWS_REGION

components:
  terraform:
    app1:
      vars:
        vpc_id: "{{ .locals.vpc_id }}"  # Reuses cached value
    app2:
      vars:
        vpc_id: "{{ .locals.vpc_id }}"  # Same cached value
```

## Technical Implementation

### New Package: `pkg/template/` - Template AST Utilities

The locals feature requires robust Go template AST inspection. Rather than adding this to `internal/exec/template_utils.go` (which already has basic inspection via `IsGolangTemplate()`), we should create a dedicated `pkg/template/` package that:

1. **Consolidates template AST utilities** - Reusable across the codebase
2. **Follows architectural guidance** - "prefer `pkg/` over `internal/exec/`"
3. **Enables future enhancements** - Deferred evaluation, template validation, etc.

#### `pkg/template/ast.go` - Template AST Inspection

```go
package template

import (
    "text/template"
    "text/template/parse"
)

// FieldRef represents a reference to a field in a template (e.g., .locals.foo).
type FieldRef struct {
    Path []string // e.g., ["locals", "foo"] for .locals.foo
}

// ExtractFieldRefs parses a Go template string and extracts all field references.
// Handles complex expressions: conditionals, pipes, range, with blocks, nested templates.
func ExtractFieldRefs(templateStr string) ([]FieldRef, error) {
    tmpl, err := template.New("").Parse(templateStr)
    if err != nil {
        return nil, err
    }

    if tmpl.Tree == nil || tmpl.Tree.Root == nil {
        return nil, nil
    }

    var refs []FieldRef
    seen := make(map[string]bool)

    walkAST(tmpl.Tree.Root, func(node parse.Node) {
        if field, ok := node.(*parse.FieldNode); ok {
            key := fieldKey(field.Ident)
            if !seen[key] {
                refs = append(refs, FieldRef{Path: field.Ident})
                seen[key] = true
            }
        }
    })

    return refs, nil
}

// ExtractFieldRefsByPrefix extracts field references that start with a specific prefix.
// For example, ExtractFieldRefsByPrefix(tmpl, "locals") returns all .locals.X references.
func ExtractFieldRefsByPrefix(templateStr string, prefix string) ([]string, error) {
    refs, err := ExtractFieldRefs(templateStr)
    if err != nil {
        return nil, err
    }

    var result []string
    for _, ref := range refs {
        if len(ref.Path) >= 2 && ref.Path[0] == prefix {
            result = append(result, ref.Path[1])
        }
    }
    return result, nil
}

// walkAST traverses all nodes in a template AST, calling fn for each node.
func walkAST(node parse.Node, fn func(parse.Node)) {
    if node == nil {
        return
    }

    fn(node)

    switch n := node.(type) {
    case *parse.ListNode:
        if n != nil {
            for _, child := range n.Nodes {
                walkAST(child, fn)
            }
        }

    case *parse.ActionNode:
        walkAST(n.Pipe, fn)

    case *parse.PipeNode:
        if n != nil {
            for _, cmd := range n.Cmds {
                walkAST(cmd, fn)
            }
            for _, decl := range n.Decl {
                walkAST(decl, fn)
            }
        }

    case *parse.CommandNode:
        if n != nil {
            for _, arg := range n.Args {
                walkAST(arg, fn)
            }
        }

    case *parse.IfNode:
        walkAST(n.Pipe, fn)
        walkAST(n.List, fn)
        walkAST(n.ElseList, fn)

    case *parse.RangeNode:
        walkAST(n.Pipe, fn)
        walkAST(n.List, fn)
        walkAST(n.ElseList, fn)

    case *parse.WithNode:
        walkAST(n.Pipe, fn)
        walkAST(n.List, fn)
        walkAST(n.ElseList, fn)

    case *parse.TemplateNode:
        walkAST(n.Pipe, fn)

    case *parse.BranchNode:
        walkAST(n.Pipe, fn)
        walkAST(n.List, fn)
        walkAST(n.ElseList, fn)
    }
}

func fieldKey(ident []string) string {
    key := ""
    for i, s := range ident {
        if i > 0 {
            key += "."
        }
        key += s
    }
    return key
}

// HasTemplateActions checks if a string contains Go template actions.
// This is a more robust version of the existing IsGolangTemplate in internal/exec.
func HasTemplateActions(str string) (bool, error) {
    tmpl, err := template.New("").Parse(str)
    if err != nil {
        return false, err
    }

    if tmpl.Tree == nil || tmpl.Tree.Root == nil {
        return false, nil
    }

    hasActions := false
    walkAST(tmpl.Tree.Root, func(node parse.Node) {
        switch node.(type) {
        case *parse.ActionNode, *parse.IfNode, *parse.RangeNode, *parse.WithNode:
            hasActions = true
        }
    })

    return hasActions, nil
}
```

#### `pkg/template/ast_test.go` - Comprehensive Tests

```go
package template

import (
    "sort"
    "testing"

    "github.com/stretchr/testify/assert"
)

func TestExtractFieldRefsByPrefix(t *testing.T) {
    tests := []struct {
        name     string
        template string
        prefix   string
        expected []string
    }{
        {
            name:     "simple field",
            template: "{{ .locals.foo }}",
            prefix:   "locals",
            expected: []string{"foo"},
        },
        {
            name:     "multiple fields",
            template: "{{ .locals.foo }}-{{ .locals.bar }}",
            prefix:   "locals",
            expected: []string{"foo", "bar"},
        },
        {
            name:     "conditional with multiple refs",
            template: "{{ if .locals.flag }}{{ .locals.x }}{{ else }}{{ .locals.y }}{{ end }}",
            prefix:   "locals",
            expected: []string{"flag", "x", "y"},
        },
        {
            name:     "pipe expression",
            template: `{{ .locals.foo | printf "%s-%s" .locals.bar }}`,
            prefix:   "locals",
            expected: []string{"foo", "bar"},
        },
        {
            name:     "range block",
            template: "{{ range .locals.items }}{{ .locals.prefix }}-{{ . }}{{ end }}",
            prefix:   "locals",
            expected: []string{"items", "prefix"},
        },
        {
            name:     "with block - context change",
            template: "{{ with .locals.config }}{{ .name }}{{ end }}",
            prefix:   "locals",
            expected: []string{"config"}, // .name is NOT .locals.name
        },
        {
            name:     "mixed prefixes",
            template: "{{ .locals.a }}-{{ .vars.b }}-{{ .settings.c }}",
            prefix:   "locals",
            expected: []string{"a"},
        },
        {
            name:     "nested conditionals",
            template: "{{ if .locals.a }}{{ if .locals.b }}{{ .locals.c }}{{ end }}{{ end }}",
            prefix:   "locals",
            expected: []string{"a", "b", "c"},
        },
        {
            name:     "no template syntax",
            template: "just a plain string",
            prefix:   "locals",
            expected: nil,
        },
        {
            name:     "deep path",
            template: "{{ .locals.config.nested.value }}",
            prefix:   "locals",
            expected: []string{"config"}, // Only first level after prefix
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := ExtractFieldRefsByPrefix(tt.template, tt.prefix)
            assert.NoError(t, err)

            // Sort for deterministic comparison
            sort.Strings(result)
            sort.Strings(tt.expected)
            assert.Equal(t, tt.expected, result)
        })
    }
}

func TestHasTemplateActions(t *testing.T) {
    tests := []struct {
        template string
        expected bool
    }{
        {"{{ .foo }}", true},
        {"{{ if .x }}y{{ end }}", true},
        {"{{ range .items }}{{ . }}{{ end }}", true},
        {"plain text", false},
        {"no {{ braces", false},
        {"", false},
    }

    for _, tt := range tests {
        t.Run(tt.template, func(t *testing.T) {
            result, err := HasTemplateActions(tt.template)
            assert.NoError(t, err)
            assert.Equal(t, tt.expected, result)
        })
    }
}
```

This package can then be used by:
- **Locals resolver** - Extract `.locals.X` dependencies
- **Existing `IsGolangTemplate()`** - Could migrate to use `HasTemplateActions()`
- **Future features** - Template validation, dependency analysis, etc.

---

### Files to Create

#### `pkg/locals/resolver.go` - Core Locals Resolution

```go
package locals

import (
    "fmt"
    "sort"

    atmostmpl "github.com/cloudposse/atmos/pkg/template"
)

// LocalsResolver handles dependency resolution and cycle detection for locals.
type LocalsResolver struct {
    locals       map[string]any           // Raw local definitions
    resolved     map[string]any           // Resolved local values
    dependencies map[string][]string      // Dependency graph: local -> locals it depends on
    filePath     string                   // For error messages
}

// NewLocalsResolver creates a resolver for a set of locals.
func NewLocalsResolver(locals map[string]any, filePath string) *LocalsResolver {
    return &LocalsResolver{
        locals:       locals,
        resolved:     make(map[string]any),
        dependencies: make(map[string][]string),
        filePath:     filePath,
    }
}

// Resolve processes all locals in dependency order, returning resolved values.
// Returns error if circular dependency detected or undefined local referenced.
func (r *LocalsResolver) Resolve(parentLocals map[string]any) (map[string]any, error) {
    // Step 1: Build dependency graph
    if err := r.buildDependencyGraph(); err != nil {
        return nil, err
    }

    // Step 2: Topological sort with cycle detection
    order, err := r.topologicalSort()
    if err != nil {
        return nil, err
    }

    // Step 3: Resolve in order
    // Start with parent locals (from outer scope)
    for k, v := range parentLocals {
        r.resolved[k] = v
    }

    for _, name := range order {
        value, err := r.resolveLocal(name)
        if err != nil {
            return nil, err
        }
        r.resolved[name] = value
    }

    return r.resolved, nil
}

// buildDependencyGraph extracts .locals.X references using the pkg/template AST utilities.
// This handles complex expressions like conditionals, pipes, range, and with blocks.
func (r *LocalsResolver) buildDependencyGraph() error {
    for name, value := range r.locals {
        var deps []string

        // Only string values can have template references
        if strVal, ok := value.(string); ok {
            // Use pkg/template AST utilities to extract .locals.X references
            extracted, err := atmostmpl.ExtractFieldRefsByPrefix(strVal, "locals")
            if err != nil {
                // Not a valid template - no deps (will fail later during resolution)
                r.dependencies[name] = deps
                continue
            }
            deps = extracted
        }

        r.dependencies[name] = deps
    }
    return nil
}

// topologicalSort returns locals in resolution order, detecting cycles.
func (r *LocalsResolver) topologicalSort() ([]string, error) {
    // Kahn's algorithm with cycle detection.
    // inDegree[x] = number of locals that x depends on (within this scope).
    inDegree := make(map[string]int)
    for name, deps := range r.dependencies {
        count := 0
        for _, dep := range deps {
            if _, exists := r.locals[dep]; exists {
                count++
            }
        }
        inDegree[name] = count
    }

    // Start with nodes that have no dependencies
    var queue []string
    for name, degree := range inDegree {
        if degree == 0 {
            queue = append(queue, name)
        }
    }
    sort.Strings(queue) // Deterministic order

    var result []string
    for len(queue) > 0 {
        // Pop from queue
        name := queue[0]
        queue = queue[1:]
        result = append(result, name)

        // Reduce in-degree of dependents
        for dependent, deps := range r.dependencies {
            for _, dep := range deps {
                if dep == name {
                    inDegree[dependent]--
                    if inDegree[dependent] == 0 {
                        queue = append(queue, dependent)
                        sort.Strings(queue)
                    }
                }
            }
        }
    }

    // If not all nodes processed, there's a cycle
    if len(result) != len(r.locals) {
        cycle := r.findCycle()
        return nil, fmt.Errorf("circular dependency in locals at %s\n\nDependency cycle detected:\n  %s",
            r.filePath, cycle)
    }

    return result, nil
}

// findCycle uses DFS to find and return a cycle for error reporting.
func (r *LocalsResolver) findCycle() string {
    visited := make(map[string]bool)
    recStack := make(map[string]bool)
    var cyclePath []string

    var dfs func(name string) bool
    dfs = func(name string) bool {
        visited[name] = true
        recStack[name] = true
        cyclePath = append(cyclePath, name)

        for _, dep := range r.dependencies[name] {
            if _, exists := r.locals[dep]; !exists {
                continue // Skip parent scope locals
            }
            if !visited[dep] {
                if dfs(dep) {
                    return true
                }
            } else if recStack[dep] {
                // Found cycle - trim cyclePath to start at dep
                for i, n := range cyclePath {
                    if n == dep {
                        cyclePath = append(cyclePath[i:], dep)
                        return true
                    }
                }
            }
        }

        cyclePath = cyclePath[:len(cyclePath)-1]
        recStack[name] = false
        return false
    }

    for name := range r.locals {
        if !visited[name] {
            if dfs(name) {
                break
            }
        }
    }

    // Format cycle as "a → b → c → a"
    result := ""
    for i, name := range cyclePath {
        if i > 0 {
            result += " → "
        }
        result += name
    }
    return result
}

// resolveLocal resolves a single local's value using the template engine.
func (r *LocalsResolver) resolveLocal(name string) (any, error) {
    value := r.locals[name]

    // Non-string values don't need template processing
    strVal, ok := value.(string)
    if !ok {
        return value, nil
    }

    // Use existing template processing with resolved locals as context
    // This integrates with internal/exec/template_utils.go ProcessTmpl()
    context := map[string]any{
        "locals": r.resolved,
    }

    // Call existing template processor (implementation detail)
    resolved, err := processTemplate(strVal, context)
    if err != nil {
        return nil, fmt.Errorf("failed to resolve local %q in %s: %w", name, r.filePath, err)
    }

    return resolved, nil
}
```

### Files to Modify

#### 1. `pkg/config/const.go` - Add Constant

```go
// Add to existing constants
LocalsSectionName = "locals"
```

#### 2. `internal/exec/stack_processor_process_stacks_helpers.go` - Add to Processor Options

```go
// Add to ComponentProcessorOptions struct
type ComponentProcessorOptions struct {
    // ... existing fields ...

    // File-scoped locals (not inherited across imports)
    GlobalLocals    map[string]any // Resolved global locals from current file
    TerraformLocals map[string]any // Resolved terraform-section locals
    HelmfileLocals  map[string]any // Resolved helmfile-section locals
    ComponentLocals map[string]any // Resolved component-level locals
}
```

#### 3. `internal/exec/stack_processor_process_stacks.go` - Extract & Resolve Locals

Integration point in `ProcessStackConfig()`:

```go
func ProcessStackConfig(
    atmosConfig *schema.AtmosConfiguration,
    stacksBasePath string,
    // ... other params
) (map[string]any, error) {
    // ... existing code to load YAML ...

    // NEW: Extract and resolve file-scoped locals BEFORE processing other sections
    globalLocals, err := extractAndResolveLocals(stackConfigMap, cfg.LocalsSectionName, filePath, nil)
    if err != nil {
        return nil, err
    }

    // Extract terraform-section locals with global locals as parent
    terraformLocals, err := extractAndResolveLocals(
        stackConfigMap["terraform"],
        cfg.LocalsSectionName,
        filePath,
        globalLocals,
    )
    if err != nil {
        return nil, err
    }

    // ... continue with existing processing, passing locals to template context ...
}

// extractAndResolveLocals extracts locals from a config section and resolves them.
func extractAndResolveLocals(
    section any,
    key string,
    filePath string,
    parentLocals map[string]any,
) (map[string]any, error) {
    sectionMap, ok := section.(map[string]any)
    if !ok {
        return parentLocals, nil // No locals in this section
    }

    localsRaw, exists := sectionMap[key]
    if !exists {
        return parentLocals, nil
    }

    localsMap, ok := localsRaw.(map[string]any)
    if !ok {
        return nil, fmt.Errorf("locals must be a map in %s", filePath)
    }

    resolver := locals.NewLocalsResolver(localsMap, filePath)
    return resolver.Resolve(parentLocals)
}
```

#### 4. `internal/exec/stack_processor_process_stacks_helpers_extraction.go` - Component Locals

Add extraction of component-level locals:

```go
func extractComponentSections(component map[string]any, opts *ComponentProcessorOptions) error {
    // ... existing extractions for vars, settings, env ...

    // NEW: Extract component-level locals
    if localsSection, ok := component[cfg.LocalsSectionName]; ok {
        if localsMap, ok := localsSection.(map[string]any); ok {
            // Merge parent locals (global + terraform/helmfile) as context
            parentLocals := mergeMaps(opts.GlobalLocals, opts.TerraformLocals)

            resolver := locals.NewLocalsResolver(localsMap, opts.FilePath)
            resolvedLocals, err := resolver.Resolve(parentLocals)
            if err != nil {
                return err
            }
            opts.ComponentLocals = resolvedLocals
        }
    }

    return nil
}
```

#### 5. `internal/exec/template_utils.go` - Add Locals to Template Context

When building template context, include resolved locals:

```go
func buildTemplateContext(opts *ComponentProcessorOptions) map[string]any {
    return map[string]any{
        "vars":     opts.ComponentVars,
        "settings": opts.ComponentSettings,
        "env":      opts.ComponentEnv,
        // NEW: Merged locals from all scopes
        "locals":   mergeMaps(opts.GlobalLocals, opts.TerraformLocals, opts.ComponentLocals),
    }
}
```

#### 6. `internal/exec/stack_processor_merge.go` - Exclude Locals from Merge

Ensure locals are NOT merged across file boundaries:

```go
func mergeStackConfigs(base, override map[string]any) map[string]any {
    result := deepCopy(base)

    for key, value := range override {
        // NEW: Skip locals - they are file-scoped only
        if key == cfg.LocalsSectionName {
            continue
        }

        // ... existing merge logic ...
    }

    return result
}
```

#### 7. `pkg/datafetcher/schema/stacks/stack-config/1.0.json` - Schema Updates

Add `locals` to allowed sections:

```json
{
  "properties": {
    "locals": {
      "type": "object",
      "description": "File-scoped local variables for use in templates within this file",
      "additionalProperties": true
    },
    "terraform": {
      "properties": {
        "locals": {
          "type": "object",
          "description": "Terraform-scoped local variables",
          "additionalProperties": true
        }
      }
    },
    "components": {
      "properties": {
        "terraform": {
          "additionalProperties": {
            "properties": {
              "locals": {
                "type": "object",
                "description": "Component-scoped local variables",
                "additionalProperties": true
              }
            }
          }
        }
      }
    }
  }
}
```

### Existing Code to Reuse

| What | Where | How to Reuse |
|------|-------|--------------|
| Template processing | `internal/exec/template_utils.go` → `ProcessTmpl()` | Call directly for resolving local values |
| Cycle detection pattern | `internal/exec/yaml_func_resolution_context.go` | Reference for error message formatting |
| Deep map merge | `pkg/merge/merge.go` → `MergeWithDeferred()` | Merge parent + child locals |
| Section extraction | `stack_processor_process_stacks_helpers_extraction.go` | Follow same pattern for locals |
| Command registry pattern | `cmd/internal/registry.go` | Reference for any new describe subcommands |
| Flag handler | `pkg/flags/` | Use `StandardParser` for any new CLI flags |

### Architecture Notes (Post-Terraform Refactoring)

The codebase has been refactored with these patterns that locals implementation should follow:

1. **Command Registry Pattern** (`cmd/internal/registry.go`):
   - Commands implement `CommandProvider` interface
   - Register via `internal.Register()` in `init()`
   - If adding `atmos describe locals` command, follow this pattern

2. **Flag Handling** (`pkg/flags/`):
   - Use `flags.NewStandardParser()` with functional options
   - Bind to Viper for env var support
   - Example: `cmd/terraform/terraform.go` lines 42-58

3. **Package Structure**:
   - Business logic in `pkg/` (e.g., `pkg/locals/`, `pkg/template/`)
   - CLI wrappers in `cmd/` (thin, delegate to `pkg/`)
   - Follow `cmd/terraform/` structure if adding commands

4. **Component Resolution** (`pkg/component/resolver.go`):
   - New resolver pattern for component path resolution
   - Locals should integrate with this for component-scoped locals

### Test Strategy

```go
// pkg/locals/resolver_test.go

func TestLocalsResolver_SimpleResolution(t *testing.T) {
    locals := map[string]any{
        "a": "value-a",
        "b": "{{ .locals.a }}-extended",
    }
    resolver := NewLocalsResolver(locals, "test.yaml")
    result, err := resolver.Resolve(nil)

    assert.NoError(t, err)
    assert.Equal(t, "value-a", result["a"])
    assert.Equal(t, "value-a-extended", result["b"])
}

func TestLocalsResolver_CycleDetection(t *testing.T) {
    locals := map[string]any{
        "a": "{{ .locals.c }}",
        "b": "{{ .locals.a }}",
        "c": "{{ .locals.b }}",
    }
    resolver := NewLocalsResolver(locals, "test.yaml")
    _, err := resolver.Resolve(nil)

    assert.Error(t, err)
    assert.Contains(t, err.Error(), "circular dependency")
    assert.Contains(t, err.Error(), "a → b → c → a") // or similar cycle representation
}

func TestLocalsResolver_ParentScopeAccess(t *testing.T) {
    parentLocals := map[string]any{
        "global": "from-parent",
    }
    locals := map[string]any{
        "child": "{{ .locals.global }}-child",
    }
    resolver := NewLocalsResolver(locals, "test.yaml")
    result, err := resolver.Resolve(parentLocals)

    assert.NoError(t, err)
    assert.Equal(t, "from-parent-child", result["child"])
}

func TestLocalsResolver_OrderIndependent(t *testing.T) {
    // Defined in reverse dependency order
    locals := map[string]any{
        "c": "{{ .locals.b }}-c",
        "b": "{{ .locals.a }}-b",
        "a": "start",
    }
    resolver := NewLocalsResolver(locals, "test.yaml")
    result, err := resolver.Resolve(nil)

    assert.NoError(t, err)
    assert.Equal(t, "start", result["a"])
    assert.Equal(t, "start-b", result["b"])
    assert.Equal(t, "start-b-c", result["c"])
}

func TestLocalsResolver_ComplexTemplateExpressions(t *testing.T) {
    tests := []struct {
        name         string
        locals       map[string]any
        expectedDeps map[string][]string // local name -> expected dependencies
    }{
        {
            name: "conditional expression",
            locals: map[string]any{
                "result": "{{ if .locals.flag }}{{ .locals.x }}{{ else }}{{ .locals.y }}{{ end }}",
            },
            expectedDeps: map[string][]string{
                "result": {"flag", "x", "y"},
            },
        },
        {
            name: "pipe with multiple refs",
            locals: map[string]any{
                "result": `{{ .locals.foo | printf "%s-%s" .locals.bar }}`,
            },
            expectedDeps: map[string][]string{
                "result": {"foo", "bar"},
            },
        },
        {
            name: "range over local",
            locals: map[string]any{
                "result": "{{ range .locals.items }}{{ .locals.prefix }}-{{ . }}{{ end }}",
            },
            expectedDeps: map[string][]string{
                "result": {"items", "prefix"},
            },
        },
        {
            name: "with block - dot changes context",
            locals: map[string]any{
                // Inside with block, .name refers to .locals.config.name, NOT .locals.name
                "result": "{{ with .locals.config }}{{ .name }}{{ end }}",
            },
            expectedDeps: map[string][]string{
                "result": {"config"}, // Only config, not "name"
            },
        },
        {
            name: "nested conditionals",
            locals: map[string]any{
                "result": "{{ if .locals.a }}{{ if .locals.b }}{{ .locals.c }}{{ end }}{{ end }}",
            },
            expectedDeps: map[string][]string{
                "result": {"a", "b", "c"},
            },
        },
        {
            name: "sprig function with local",
            locals: map[string]any{
                "result": "{{ .locals.name | upper | quote }}",
            },
            expectedDeps: map[string][]string{
                "result": {"name"},
            },
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            resolver := NewLocalsResolver(tt.locals, "test.yaml")
            err := resolver.buildDependencyGraph()
            assert.NoError(t, err)

            for local, expectedDeps := range tt.expectedDeps {
                actualDeps := resolver.dependencies[local]
                // Sort both for comparison
                sort.Strings(actualDeps)
                sort.Strings(expectedDeps)
                assert.Equal(t, expectedDeps, actualDeps, "deps mismatch for %s", local)
            }
        })
    }
}
```

### Implementation Phases

**Phase 1: Template AST Package** (~1-2 days)
- Create `pkg/template/ast.go` with `ExtractFieldRefs()`, `ExtractFieldRefsByPrefix()`, `walkAST()`
- Create `pkg/template/ast_test.go` with comprehensive tests for complex expressions
- Add `HasTemplateActions()` as improved replacement for `IsGolangTemplate()`

**Phase 2: Locals Resolver Package** (~2-3 days)
- Create `pkg/locals/resolver.go` using `pkg/template` for dependency extraction
- Implement topological sort with Kahn's algorithm
- Implement cycle detection with DFS for clear error messages
- Comprehensive unit tests for resolution, cycles, parent scope access

**Phase 3: Stack Processor Integration** (~2-3 days)
- Add `LocalsSectionName` constant to `pkg/config/const.go`
- Modify `ComponentProcessorOptions` to carry locals at each scope
- Extract and resolve global locals in `ProcessStackConfig()`
- Extract and resolve component-type locals (terraform/helmfile sections)

**Phase 4: Component-Level Integration** (~1-2 days)
- Extract component-level locals in `extractComponentSections()`
- Add `.locals` to template context in `buildTemplateContext()`
- Ensure locals excluded from merge operations (file-scoped only)

**Phase 5: Schema & Validation** (~1 day)
- Update JSON schema to allow `locals` at all scopes
- Add validation for locals section structure

**Phase 6: Integration Tests & Documentation** (~2 days)
- End-to-end tests with real stack files
- Test file-scoped isolation (imports don't leak locals)
- Test complex template expressions with locals
- Update documentation

**Phase 7: Debugging Support** (~2-3 days)
- Implement `atmos describe locals` command
  - Follow command registry pattern from `cmd/internal/registry.go`
  - Create `cmd/describe/locals.go` implementing `CommandProvider`
  - Register via `internal.Register()` in `init()`
  - Use `pkg/flags/` for `--stack` and `--format` flags
  - Show scope hierarchy (global → terraform → component)
- Enhance error messages with available locals list
- Add typo detection ("did you mean?") using Levenshtein distance

**Optional: Migrate IsGolangTemplate** (~0.5 days)
- Update `internal/exec/template_utils.go` to use `pkg/template.HasTemplateActions()`
- Deprecate old implementation

---

## Final Implementation Plan

### Summary

| Phase | Deliverable | Files | Effort |
|-------|-------------|-------|--------|
| 1 | Template AST Package | `pkg/template/ast.go`, `pkg/template/ast_test.go` | 1-2 days |
| 2 | Locals Resolver | `pkg/locals/resolver.go`, `pkg/locals/resolver_test.go` | 2-3 days |
| 3 | Stack Processor Integration | Modify 3 files in `internal/exec/` | 2-3 days |
| 4 | Component Integration | Modify 2 files in `internal/exec/` | 1-2 days |
| 5 | Schema & Validation | `pkg/datafetcher/schema/`, `pkg/config/const.go` | 1 day |
| 6 | Integration Tests & Docs | `tests/`, `website/docs/` | 2 days |
| 7 | `atmos describe locals` Command | `cmd/describe/locals.go`, `internal/exec/describe_locals.go` | 2-3 days |
| **Total** | | **~18 files** | **~12-16 days** |

### Detailed Task Breakdown

#### Phase 1: Template AST Package (Foundation)

**Create `pkg/template/ast.go`:**
```
pkg/template/
├── ast.go           # ExtractFieldRefs, ExtractFieldRefsByPrefix, walkAST, HasTemplateActions
└── ast_test.go      # Table-driven tests for all complex template patterns
```

**Functions:**
- `ExtractFieldRefs(templateStr) → []FieldRef` - Parse template, walk AST, return all `.X.Y` refs
- `ExtractFieldRefsByPrefix(templateStr, prefix) → []string` - Filter refs by prefix (e.g., "locals")
- `walkAST(node, fn)` - Recursive AST walker handling all node types
- `HasTemplateActions(str) → bool` - Detect if string contains template actions

**Test coverage:**
- Simple field refs: `{{ .locals.foo }}`
- Multiple refs: `{{ .locals.a }}-{{ .locals.b }}`
- Conditionals: `{{ if .locals.x }}...{{ end }}`
- Pipes: `{{ .locals.foo | upper }}`
- Range: `{{ range .locals.items }}...{{ end }}`
- With (context change): `{{ with .locals.config }}{{ .name }}{{ end }}`
- Nested structures
- Invalid templates (graceful handling)

#### Phase 2: Locals Resolver Package

**Create `pkg/locals/resolver.go`:**
```
pkg/locals/
├── resolver.go      # LocalsResolver struct, Resolve, buildDependencyGraph, topologicalSort, findCycle
└── resolver_test.go # Unit tests
```

**LocalsResolver API:**
```go
resolver := NewLocalsResolver(localsMap, filePath)
resolved, err := resolver.Resolve(parentLocals)
```

**Algorithms:**
- **Dependency extraction**: Use `pkg/template.ExtractFieldRefsByPrefix(value, "locals")`
- **Topological sort**: Kahn's algorithm for resolution order
- **Cycle detection**: DFS with recursion stack for clear error paths

**Test coverage:**
- Simple resolution (no deps)
- Chained locals (a → b → c)
- Order independence (c, b, a defined but resolved as a, b, c)
- Parent scope access (component refs global)
- Cycle detection with clear error messages
- Non-string values (pass through unchanged)
- Invalid template syntax (graceful handling)

#### Phase 3: Stack Processor Integration

**Modify `pkg/config/const.go`:**
```go
LocalsSectionName = "locals"
```

**Modify `internal/exec/stack_processor_process_stacks_helpers.go`:**
```go
type ComponentProcessorOptions struct {
    // ... existing fields ...
    GlobalLocals    map[string]any
    TerraformLocals map[string]any
    HelmfileLocals  map[string]any
    ComponentLocals map[string]any
}
```

**Modify `internal/exec/stack_processor_process_stacks.go`:**
- Add `extractAndResolveLocals()` helper function
- Extract global locals early in `ProcessStackConfig()`
- Extract terraform/helmfile section locals with global as parent
- Pass locals through to component processing

#### Phase 4: Component-Level Integration

**Modify `internal/exec/stack_processor_process_stacks_helpers_extraction.go`:**
- Extract component-level locals in `extractComponentSections()`
- Merge parent scopes (global + terraform/helmfile) before resolving

**Modify `internal/exec/template_utils.go` or equivalent:**
- Add `.locals` to template context alongside `.vars`, `.settings`, `.env`

**Modify `internal/exec/stack_processor_merge.go`:**
- Skip `locals` key during merge (file-scoped only)

#### Phase 5: Schema & Validation

**Update `pkg/datafetcher/schema/stacks/stack-config/1.0.json`:**
- Add `locals` property at root level
- Add `locals` property to `terraform` section
- Add `locals` property to `helmfile` section
- Add `locals` property to `packer` section
- Add `locals` property to component definitions

**Add validation:**
- Locals must be a map
- Key names must be valid identifiers

#### Phase 6: Integration Tests & Documentation

**Create test fixtures in `tests/test-cases/`:**
```
tests/test-cases/locals/
├── atmos.yaml
├── stacks/
│   ├── _defaults.yaml      # Global locals (should NOT inherit)
│   ├── catalog/
│   │   └── vpc.yaml        # Component with locals
│   └── deploy/
│       └── prod.yaml       # Import + own locals
└── components/
    └── terraform/
        └── vpc/
```

**Test scenarios:**
1. Basic locals resolution
2. Locals referencing other locals (chained)
3. Scoped locals (global → terraform → component)
4. File isolation (import doesn't leak locals)
5. Cycle detection error
6. Undefined local error
7. Complex template expressions
8. YAML functions in locals (`!env`, `!template`)

**Documentation:**
- Add to `website/docs/core-concepts/stacks/` or similar
- Add examples to existing stack configuration docs
- Update schema documentation

### Risk Mitigation

| Risk | Mitigation |
|------|------------|
| Performance impact | Locals resolved once per file, cached for all components |
| Complex template parsing | AST-based (not regex), handles all Go template constructs |
| Breaking existing configs | `locals` is a new section, no existing configs use it |
| Confusing error messages | Provide file path, line numbers, available locals, hints |

### Debugging Locals

Since locals are file-scoped and not visible in `atmos describe component`, we provide a dedicated `atmos describe locals` command:

#### The `atmos describe locals` Command

```bash
# Show all locals for a component in a stack
atmos describe locals vpc -s prod-us-east-1

# Output as JSON
atmos describe locals vpc -s prod-us-east-1 --format json
```

**Example output:**
```yaml
# Locals for component "vpc" in stack "prod-us-east-1"
# Resolution order: global → terraform → component

global:
  region: "us-east-1"
  account_id: "123456789012"

terraform:
  state_bucket: "terraform-state-123456789012"

merged:
  region: "us-east-1"
  account_id: "123456789012"
  state_bucket: "terraform-state-123456789012"
```

#### Key Features

1. **Scope separation**: Clearly shows global, component-type, and component scopes
2. **Merged view**: Shows the final resolved locals available to templates

#### Optional: `ATMOS_DEBUG_LOCALS` Environment Variable

For verbose logging during stack processing:

```bash
ATMOS_DEBUG_LOCALS=true atmos terraform plan vpc -s prod-us-east-1
```

Output (to stderr):
```
[locals] Processing stacks/deploy/prod.yaml
[locals] Global scope: 2 locals defined
[locals]   region = "us-east-1"
[locals]   account_id = "123456789012"
[locals] Terraform scope: 1 local defined
[locals]   state_bucket = "terraform-state-{{ .locals.account_id }}"
[locals]   → resolved: "terraform-state-123456789012"
[locals] Component vpc scope: 1 local defined
[locals]   vpc_name = "main-vpc-{{ .locals.region }}"
[locals]   → resolved: "main-vpc-us-east-1"
[locals] Resolution complete: 4 locals available
```

#### Option 4: Error Messages with Available Locals

When a template fails, show which locals were available:

```
Error: template execution failed in stacks/deploy/prod.yaml

Template: "{{ .locals.vpc_naem }}"  # Typo!
Error: map has no entry for key "vpc_naem"

Available locals at this scope:
  - region: "us-east-1"
  - account_id: "123456789012"
  - state_bucket: "terraform-state-123456789012"
  - vpc_name: "main-vpc-us-east-1"    ← Did you mean this?

Hint: Check for typos in local variable names.
```

#### Recommended Implementation

| Feature | Priority | Effort | Phase |
|---------|----------|--------|-------|
| Error messages with available locals | P0 (must have) | Low | Phase 2 |
| `atmos describe locals` command | P1 (should have) | Medium | Phase 2 |
| `ATMOS_DEBUG_LOCALS` env var | P2 (nice to have) | Low | Phase 3 |

**Minimum viable debugging (Phase 2):**
- Clear error messages showing available locals on failure
- Typo detection with "did you mean?" suggestions
- `atmos describe locals` command to inspect resolved locals

### The `atmos describe locals` Command

List the resolved locals for a component in a stack:

```bash
# Show locals for a component in a stack
atmos describe locals vpc -s plat-ue2-prod

# Show locals for all stacks
atmos describe locals

# Filter by stack (logical name or file path)
atmos describe locals --stack plat-ue2-prod
atmos describe locals --stack deploy/prod

# Output as YAML (default)
atmos describe locals vpc -s plat-ue2-prod --format yaml

# Output as JSON
atmos describe locals vpc -s plat-ue2-prod --format json
```

**Example output (stack query):**
```yaml
plat-ue2-prod:
  global:
    region: us-east-2
    account_id: "123456789012"
    environment: prod
  terraform:
    state_bucket: terraform-state-123456789012
    state_key_prefix: plat-ue2-prod
  merged:
    region: us-east-2
    account_id: "123456789012"
    environment: prod
    state_bucket: terraform-state-123456789012
    state_key_prefix: plat-ue2-prod
```

**Example output (component query):**
```yaml
component: vpc
stack: plat-ue2-prod
component_type: terraform
locals:
  region: us-east-2
  account_id: "123456789012"
  environment: prod
  state_bucket: terraform-state-123456789012
  state_key_prefix: plat-ue2-prod
```

**Key Design Decisions:**

1. **Scope separation**: Clearly shows global, component-type, and component scopes
2. **Merged view**: The `merged` field shows final resolved locals available to templates
3. **JSON for tooling**: Full metadata in JSON format for programmatic access
4. **Consistent with other describe commands**: Same flags (`-s`, `--format`) as `atmos describe component`
5. **Stack name flexibility**: Accepts both logical stack names and file paths

**Implementation Notes:**

The command follows the existing `describe` command pattern:
- Located in `cmd/describe/describe_locals.go`
- Uses `CommandProvider` pattern from `cmd/internal/registry.go`
- Business logic in `internal/exec/describe_locals.go`
- Reuses `ProcessStackLocals()` from `internal/exec/stack_processor_locals.go`

### Component Registry Integration

The Atmos component registry (`pkg/component/`) provides a provider-based architecture for component types (terraform, helmfile, packer, etc.). Locals integration with this system requires careful consideration.

#### Analysis: Do Component Providers Need Locals?

**No changes needed to the `ComponentProvider` interface.** Here's why:

| Data Type | Passed to Provider? | Reason |
|-----------|---------------------|--------|
| `vars` | ✅ Yes | Passed to Terraform/Helmfile as inputs |
| `settings` | ✅ Yes | Component metadata for hooks, validation |
| `env` | ✅ Yes | Environment variables for subprocess |
| **`locals`** | ❌ **No** | Resolved during template processing, not needed at execution time |

Locals are **consumed during stack processing** to generate the final `vars`, `settings`, and `env` values. By the time a component executes, all `.locals.*` references have been resolved to their final values.

#### Execution Flow

```
Stack Processing Phase (where locals are used):
┌─────────────────────────────────────────────────────────────────┐
│ 1. Parse YAML file                                              │
│ 2. Extract & resolve locals (global → terraform → component)   │
│ 3. Process templates in vars/settings/env with locals context  │
│ 4. Merge with inherited values                                 │
│ 5. Build ConfigAndStacksInfo with resolved values              │
└─────────────────────────────────────────────────────────────────┘
                              ↓
                 ConfigAndStacksInfo
                 (vars, settings, env - all resolved)
                              ↓
Component Execution Phase (locals NOT needed):
┌─────────────────────────────────────────────────────────────────┐
│ 6. ComponentProvider.Execute(ExecutionContext)                  │
│    - ComponentConfig contains resolved vars                     │
│    - No locals references remain (all resolved)                 │
└─────────────────────────────────────────────────────────────────┘
```

#### What Changes ARE Needed

**1. `ConfigAndStacksInfo` in `pkg/schema/schema.go`:**

No new field needed for locals. The existing fields are sufficient:
- `ComponentVarsSection` - Contains resolved values (locals already applied)
- `ComponentSettingsSection` - Contains resolved values
- `ComponentEnvSection` - Contains resolved values

**2. Stack Processor (where locals are actually used):**

The changes are in `internal/exec/stack_processor_*.go`:
- Extract locals before processing templates
- Add `.locals` to template context alongside `.vars`, `.settings`
- Locals are consumed during template resolution, not stored in final output

**3. ExecutionContext in `pkg/component/provider.go`:**

No changes needed. The `ComponentConfig map[string]any` already contains the resolved vars. Locals have done their job during template processing.

#### Why NOT Add ComponentLocalsSection?

Adding `ComponentLocalsSection` to `ConfigAndStacksInfo` would be **incorrect** because:

1. **File-scoped semantics**: Locals are explicitly file-scoped. Storing them in `ConfigAndStacksInfo` (which aggregates data across files) would violate this design.

2. **Template-time only**: Locals exist only during template processing. Once templates are resolved, locals serve no purpose.

3. **Not visible in describe**: Per the PRD, locals should NOT appear in `atmos describe component` output. Storing them in `ConfigAndStacksInfo` would require explicit exclusion logic.

4. **Memory efficiency**: No need to carry locals through the execution pipeline when they're only needed during stack processing.

#### Integration Points Summary

| Location | Change Needed | Description |
|----------|---------------|-------------|
| `pkg/component/provider.go` | ❌ None | Interface unchanged |
| `pkg/component/resolver.go` | ❌ None | Path resolution unchanged |
| `pkg/schema/schema.go` (`ConfigAndStacksInfo`) | ❌ None | Locals don't need to be stored |
| `internal/exec/stack_processor_*.go` | ✅ Yes | Extract & resolve locals, add to template context |
| `internal/exec/template_utils.go` | ✅ Yes | Add `.locals` to template context |
| `pkg/locals/` (new) | ✅ Yes | New package for resolution logic |
| `pkg/template/` (new) | ✅ Yes | New package for AST utilities |

#### Future Considerations

If a future feature needs access to resolved locals at execution time (e.g., enhanced debugging, CI/CD integration), we could add:

```go
// In schema.go - only if needed for advanced features
type ConfigAndStacksInfo struct {
    // ...existing fields...

    // ComponentLocalsResolved stores the final resolved locals for debugging.
    // This is NOT used during execution - locals are consumed during template processing.
    // Only populated when ATMOS_DEBUG_LOCALS=true is set.
    ComponentLocalsResolved AtmosSectionMapType `json:"-"` // Exclude from JSON output
}
```

But this is **not needed for the initial implementation**. The `atmos describe locals` command provides all necessary debugging capabilities.

### Success Metrics

1. **Functional**: All test scenarios pass
2. **Performance**: <10ms overhead per file for typical locals (5-20 entries)
3. **UX**: Error messages clearly explain the problem and suggest fixes
4. **Debugging**: Users can inspect resolved locals without guessing
5. **Adoption**: Users can reduce config duplication by 30%+ using locals

---

## Implementation Status

### Implemented Features (v1.203.0+)

The following features have been implemented:

#### Core Locals Resolution
- ✅ File-scoped locals (not inherited across imports)
- ✅ Global-level locals (stack file root)
- ✅ Section-level locals (terraform, helmfile, packer sections)
- ✅ Locals referencing other locals
- ✅ Topological sorting for dependency resolution
- ✅ Circular dependency detection with clear error messages
- ✅ Integration with template processing (`{{ .locals.* }}`)

#### `atmos describe locals` Command
- ✅ Show all locals across stacks: `atmos describe locals`
- ✅ Filter by stack: `atmos describe locals --stack prod`
- ✅ Filter by component in stack: `atmos describe locals vpc -s prod`
- ✅ JSON output format: `atmos describe locals --format json`
- ✅ Query support: `atmos describe locals --query '.merged.namespace'`
- ✅ File output: `atmos describe locals --file output.yaml`

#### Implementation Files
- `internal/exec/stack_processor_locals.go` - LocalsContext and extraction
- `internal/exec/describe_locals.go` - DescribeLocalsExec implementation
- `cmd/describe_locals.go` - CLI command definition
- `pkg/locals/resolver.go` - Dependency resolution with cycle detection
- `errors/errors.go` - Sentinel errors for locals

### Not Yet Implemented

The following features from the PRD are planned but not yet implemented:

#### Component-Level Locals
Component-level locals (inside individual component definitions) are parsed but not resolved during the initial template pass. This is because component-level locals require per-component scoping that cannot be handled in a single template pass. Users should use global or section-level locals instead.

#### `ATMOS_DEBUG_LOCALS` Environment Variable
Verbose logging during stack processing has not been implemented.

### Design Clarifications

#### Stack Name Resolution in `describe locals`

The `--stack` flag accepts two formats that both resolve to the same underlying stack manifest file:

1. **Stack manifest file path** - Direct path relative to the stacks directory (e.g., `deploy/dev`, `prod`)
2. **Logical stack name** - The derived name based on your `atmos.yaml` stack name pattern (e.g., `prod-us-east-1`, `dev-ue2-sandbox`)

Both formats resolve to the same file and return the same locals because **locals are file-scoped**. The command returns only the locals defined in that specific stack manifest file.

**Example:** If your `atmos.yaml` has `name_pattern: "{stage}-{environment}"` and you have a file `stacks/deploy/prod-us-east-1.yaml`:
```bash
# These are equivalent - both resolve to the same file
atmos describe locals --stack deploy/prod-us-east-1
atmos describe locals --stack prod-us-east-1
```

#### Component Argument Semantics

When you specify a component with the `--stack` flag, you're asking: *"What locals would be available to this component during template processing?"*

The component argument does **not** mean the locals come from the component definition. Instead:
1. Atmos determines the component's type (terraform, helmfile, or packer)
2. Atmos merges the global locals with the corresponding section-specific locals
3. The result shows what `{{ .locals.* }}` references would resolve to for that component

**Example:**
```yaml
# stacks/deploy/prod.yaml
locals:
  namespace: acme           # Global locals

terraform:
  locals:
    backend_bucket: "{{ .locals.namespace }}-tfstate"  # Terraform section locals
```

Running `atmos describe locals vpc -s deploy/prod` (where `vpc` is a Terraform component) returns:
```yaml
component: vpc
stack: deploy/prod
component_type: terraform
locals:
  namespace: acme
  backend_bucket: acme-tfstate
```

The locals come from the **stack manifest file**, not the `vpc` component definition. The component only determines which section-specific locals (terraform/helmfile/packer) to merge.

#### File-Scoped Isolation

A key design principle: when querying locals for a stack, you get **only** the locals defined in that stack manifest file, regardless of what files it imports. This is intentional:

1. **Predictability** - You know exactly what locals are available by looking at the current file
2. **No hidden dependencies** - Locals won't mysteriously change based on import order
3. **Safer refactoring** - Renaming a local in one file won't break other files

Use `vars` or `settings` for values that should propagate across imports. Use `locals` for file-internal convenience.
