# PRD: Deferred YAML Function Evaluation in Merge

## Status
**Current**: ✅ Implemented — the Completion Plan below (Plan B, full unification) has shipped: a
real `YAMLFunctionProcessor` now resolves deferred YAML functions at Stage 3 (per-invocation,
`internal/exec/deferred_contexts.go`'s `resolveDeferredYamlFunctions`, which constructs a
`TemplateAwareYAMLProcessor`), and the result deep-merges against any
concrete override at the same path — including the mirror-precedence direction the original plan
didn't anticipate (a concrete value at a *lower*-precedence layer than the function; see
`pkg/merge/merge_yaml_functions.go`'s `fillMissingLayerValues`). The literal #2888 report
(`!labels`/`!tags` never deferred at all) is fixed via the allowlist unification in
`pkg/merge/merge_yaml_functions.go`. See [Known Gap: #2888](#known-gap-2888--deferred-values-are-never-actually-resolved-in-production)
and the [Completion Plan](#completion-plan-wiring-post-merge-resolution-plan-b) below for the
implementation this delivered.
**Date**: 2025-11-29 (original), updated 2026-08-06 (gap discovered), implemented 2026-08-06
**Version**: 2.2

## Problem Statement

### User-Reported Issue

When importing Atmos stack files that contain YAML functions (like `!template`) and attempting to override those values
with concrete types, the merge operation fails with a type mismatch error:

```text
Error: cannot override two slices with different type ([]interface {}, string)
```

### Example Scenario

```yaml
# catalog/blob-defaults.yaml
components:
  terraform:
    blob-with-list:
      vars:
        foo_list: !template '{{ toJson .settings.my_list }}'  # Will become a list
        foo_map: !template '{{ toJson .settings.my_map }}'    # Will become a map

# stacks/test.yaml
import:
  - catalog/blob-defaults

components:
  terraform:
    blob-with-list:
      vars:
        foo_list: []          # ERROR: Type mismatch during merge
        foo_map:
          a: 1                # ERROR: Type mismatch during merge
```

### Root Cause

Atmos processes YAML in a specific order:

1. **Load** - YAML files are loaded from disk
2. **Merge** - Files are deep-merged using `mergo` library (with type checking)
3. **Process Templates** - Go templates are rendered
4. **Process YAML Functions** - YAML functions are evaluated

The problem occurs because most YAML functions are processed **AFTER** merging (step 4), but during merge (step 2) they
are still represented as strings. When `mergo` encounters a type mismatch (string vs. list, string vs. map), it throws
an error because it has strict type checking enabled.

## YAML Functions Classification

### Post-Merge Functions (Require Special Handling)

These functions are processed **AFTER** merging, so during merge they appear as strings:

- `!template` - Processes JSON strings to native types
- `!terraform.output` - Gets Terraform outputs
- `!terraform.state` - Gets Terraform state
- `!store.get` / `!store` - Gets values from stores
- `!exec` - Executes commands
- `!env` - Gets environment variables

### Pre-Merge Functions (No Special Handling Needed)

These functions are processed **BEFORE** merging, during YAML loading:

- `!include` - Includes file content during YAML loading
- `!include.raw` - Includes raw file content during YAML loading

## Attempted Solution: Mergo Transformer

### Implementation Approach (v1.0 - ABANDONED)

Initially attempted to use a **custom `mergo` transformer** that allows type mismatches when one side is an Atmos YAML function string.

### Why This Approach Failed

**Critical Issue**: The transformer interferes with normal deep-merging of nested structures.

When `mergo` calls a transformer function and it returns `nil`, mergo interprets this as "field handled, skip further
processing" - even if the transformer didn't actually modify anything. This breaks deep-merging of maps like `vars`, `settings`, etc.

**Example of the problem**:
```yaml
# Base
vars:
  stage: nonprod
  config:
    key1: value1

# Override
vars:
  config:
    key2: value2
```

With the transformer, when it sees both `vars` are maps and returns `nil` (thinking `mergo` will continue),
`mergo` actually STOPS processing that field, preventing the deep-merge of `config`. Result: `.vars.stage` disappears, causing template errors.

**Conclusion**: The `mergo` transformer pattern is fundamentally incompatible with our needs because we cannot reliably
signal "I didn't handle this, please continue with normal processing."

## Proposed Solution: Deferred Merge (v2.0)

> **Historical design pseudocode.** The "Data Structures" and "Implementation Phases" subsections
> immediately below (through the end of `mergeSlices`) capture the original 2025-11-29 design
> proposal and predate the shipped implementation. They do not match the real symbol names or
> control flow. In particular:
> - `MergeContext`/`NewMergeContext`/`AddDeferred`/`IncrementPrecedence` do not exist in shipped
>   code; the real type is `DeferredMergeContext` (`pkg/merge/deferred.go`), tracked per-component
>   rather than via a single global precedence counter.
> - `ApplyDeferredMerges` calling `ProcessYAMLFunctionString` directly (Phase 3) does not reflect
>   the shipped flow. In production, `internal/exec/deferred_contexts.go`'s
>   `resolveDeferredYamlFunctions` constructs a `TemplateAwareYAMLProcessor`
>   (`internal/exec/yaml_processor.go`) and passes that processor into
>   `merge.ApplyDeferredMerges`, which then calls the processor per deferred value.
> - The `postMergeFunctions` list in `isAtmosYAMLFunction` (Phase 1) is stale: shipped code
>   (`pkg/merge/merge_yaml_functions.go`) sources this list from the canonical constants in
>   `pkg/utils/yaml_utils.go` and additionally includes `!labels`, `!labels.keys`,
>   `!labels.values`, and `!tags` — the four functions whose omission was the literal #2888 bug.
>
> See the top [Status](#status) section and the
> [Completion Plan](#completion-plan-wiring-post-merge-resolution-plan-b) below for the as-shipped
> design. The "Implementation Status" section further down is historical: it records the
> Version 2.0 pre-fix state, where every production call site passed `processor = nil` and the
> mechanism was never actually wired up (see [Known Gap: #2888](#known-gap-2888--deferred-values-are-never-actually-resolved-in-production)).

### Core Concept

Instead of trying to merge YAML function strings with concrete values during the initial merge phase, **defer the merge of YAML functions** until after they are processed:

1. **During initial merge**: Detect YAML functions, defer them (replace with placeholders), store for later merging
2. **After YAML function evaluation**: Merge the deferred YAML function values with the standard merge result
3. **Result**: Full deep-merge capability while avoiding type conflicts during the initial merge

**Key Insight**: We defer **merging** YAML functions, not their **evaluation**. YAML functions are still evaluated as the last step in Atmos's processing pipeline (as they always were). What changed is that YAML function values are now merged separately from concrete values to avoid type mismatches.

### How It Works

**Phase 1: Deferred Collection**
- Walk through all input maps during merge
- When a YAML function is detected, store it in a deferred merge context
- Track the field path and precedence order
- Replace YAML function with a placeholder to avoid type conflicts

**Phase 2: Normal Merge**
- Perform standard `mergo` merge without YAML functions (no type conflicts)
- Placeholders merge normally

**Phase 3: Process and Re-Merge**
- Process all YAML functions to get their actual values
- For each deferred field, merge all values in precedence order
- Apply final merged values to the result

### Data Structures

**Location**: `pkg/merge/deferred.go` (new file)

```go
// DeferredValue represents a value that contains a YAML function and needs
// to be processed after the initial merge.
type DeferredValue struct {
  Path       []string    // Field path (e.g., ["components", "terraform", "vpc", "vars", "config"])
  Value      interface{} // The YAML function string or the final processed value
  Precedence int         // Merge precedence (higher = later in import chain = higher priority)
  IsFunction bool        // True if Value is still a YAML function string, false if processed
}

// MergeContext tracks all deferred values during the merge process.
type MergeContext struct {
  deferredValues map[string][]*DeferredValue // Key is path joined with "."
  precedence     int                          // Current precedence counter
}

// NewMergeContext creates a new merge context for tracking deferred values.
func NewMergeContext() *MergeContext {
  return &MergeContext{
    deferredValues: make(map[string][]*DeferredValue),
    precedence:     0,
  }
}

// AddDeferred adds a deferred value to the context.
func (mc *MergeContext) AddDeferred(path []string, value interface{}) {
  key := strings.Join(path, ".")
  mc.deferredValues[key] = append(mc.deferredValues[key], &DeferredValue{
    Path:       path,
    Value:      value,
    Precedence: mc.precedence,
    IsFunction: true,
  })
}

// IncrementPrecedence increases the precedence counter (call after each import).
func (mc *MergeContext) IncrementPrecedence() {
  mc.precedence++
}
```

### Implementation Phases

#### Phase 1: Pre-Merge Detection and Deferral

**Location**: `pkg/merge/merge.go` - modify `MergeSections` function

```go
// WalkAndDeferYAMLFunctions walks through a map and defers any YAML functions.
func WalkAndDeferYAMLFunctions(ctx *MergeContext, data map[string]interface{}, basePath []string) map[string]interface{} {
  result := make(map[string]interface{})

  for key, value := range data {
    currentPath := append(basePath, key)

    // Check if this value is a YAML function string
    if strVal, ok := value.(string); ok && isAtmosYAMLFunction(strVal) {
      // Defer this value
      ctx.AddDeferred(currentPath, strVal)
      // Replace with placeholder (empty map for map types, empty slice for slice types)
      // For now, use nil as placeholder - will be determined by type after processing
      result[key] = nil
      continue
    }

    // Recursively process nested maps
    if mapVal, ok := value.(map[string]interface{}); ok {
      result[key] = WalkAndDeferYAMLFunctions(ctx, mapVal, currentPath)
      continue
    }

    // Keep all other values as-is
    result[key] = value
  }

  return result
}

func isAtmosYAMLFunction(s string) bool {
  if s == "" {
    return false
  }

  // YAML functions processed after merging (need special handling during merge).
  postMergeFunctions := []string{
    "!template",
    "!terraform.output",
    "!terraform.state",
    "!store.get",
    "!store",
    "!exec",
    "!env",
  }

  for _, fn := range postMergeFunctions {
    if strings.HasPrefix(s, fn) {
      return true
    }
  }

  return false
}
```

#### Phase 2: Normal Merge (No Type Conflicts)

```go
// After walking all maps and deferring YAML functions, perform normal merge
// No changes needed - standard mergo merge will work without type conflicts
func MergeSections(ctx *MergeContext, sections ...map[string]interface{}) (map[string]interface{}, error) {
  result := make(map[string]interface{})

  // Walk each section and defer YAML functions
  processedSections := make([]map[string]interface{}, len(sections))
  for i, section := range sections {
    processedSections[i] = WalkAndDeferYAMLFunctions(ctx, section, []string{})
    ctx.IncrementPrecedence()
  }

  // Perform normal merge (no type conflicts now)
  for _, section := range processedSections {
    if err := mergo.Merge(&result, section, mergo.WithOverride, mergo.WithTypeCheck); err != nil {
      return nil, err
    }
  }

  return result, nil
}
```

#### Phase 3: Process YAML Functions and Re-Merge

**Location**: `internal/exec/stack_processor.go` - after YAML function processing

```go
// ApplyDeferredMerges processes all deferred YAML functions and applies them to the result.
func ApplyDeferredMerges(ctx *MergeContext, result map[string]interface{}, atmosConfig schema.AtmosConfiguration) error {
  // Process each deferred field
  for pathKey, deferredValues := range ctx.deferredValues {
    // Sort by precedence (lower first, so higher precedence wins in merge)
    sort.Slice(deferredValues, func(i, j int) bool {
      return deferredValues[i].Precedence < deferredValues[j].Precedence
    })

    // Process YAML functions to get actual values
    for _, dv := range deferredValues {
      if dv.IsFunction {
        // Process the YAML function (call existing function processors)
        processedValue, err := ProcessYAMLFunctionString(dv.Value.(string), result, atmosConfig)
        if err != nil {
          return fmt.Errorf("failed to process YAML function at %s: %w", pathKey, err)
        }
        dv.Value = processedValue
        dv.IsFunction = false
      }
    }

    // Merge all values for this path (respects list_merge_strategy)
    merged, err := MergeDeferredValues(deferredValues, atmosConfig)
    if err != nil {
      return fmt.Errorf("failed to merge deferred values at %s: %w", pathKey, err)
    }

    // Apply to result at the correct path
    if err := SetValueAtPath(result, deferredValues[0].Path, merged); err != nil {
      return fmt.Errorf("failed to set value at %s: %w", pathKey, err)
    }
  }

  return nil
}

// MergeDeferredValues merges all values for a single field path.
func MergeDeferredValues(values []*DeferredValue, atmosConfig schema.AtmosConfiguration) (interface{}, error) {
  if len(values) == 0 {
    return nil, nil
  }

  // Start with first value
  result := values[0].Value

  // For simple types (string, number, bool): just override with highest precedence
  if !isMap(result) && !isSlice(result) {
    return values[len(values)-1].Value, nil
  }

  // For slices: respect list_merge_strategy
  if isSlice(result) {
    return mergeSlices(values, atmosConfig.Settings.ListMergeStrategy)
  }

  // For maps: deep-merge all values
  resultMap := result.(map[string]interface{})
  for i := 1; i < len(values); i++ {
    valueMap, ok := values[i].Value.(map[string]interface{})
    if !ok {
      // Type changed - override completely
      return values[i].Value, nil
    }

    if err := mergo.Merge(&resultMap, valueMap, mergo.WithOverride); err != nil {
      return nil, err
    }
  }

  return resultMap, nil
}

// mergeSlices merges slice values according to the configured list merge strategy.
func mergeSlices(values []*DeferredValue, strategy string) (interface{}, error) {
  switch strategy {
  case "replace":
    // Default: latest value wins
    return values[len(values)-1].Value, nil

  case "append":
    // Concatenate all lists in precedence order
    var result []interface{}
    for _, dv := range values {
      if slice, ok := dv.Value.([]interface{}); ok {
        result = append(result, slice...)
      } else {
        // Type mismatch - use latest value
        return dv.Value, nil
      }
    }
    return result, nil

  case "merge":
    // Deep-merge list items by index position
    result := values[0].Value.([]interface{})
    for i := 1; i < len(values); i++ {
      sourceSlice, ok := values[i].Value.([]interface{})
      if !ok {
        // Type mismatch - use source value
        return values[i].Value, nil
      }

      // Merge items up to length of source slice
      for idx := 0; idx < len(sourceSlice) && idx < len(result); idx++ {
        // Deep-merge if both items are maps, otherwise override
        if srcMap, ok := sourceSlice[idx].(map[string]interface{}); ok {
          if dstMap, ok := result[idx].(map[string]interface{}); ok {
            if err := mergo.Merge(&dstMap, srcMap, mergo.WithOverride); err != nil {
              return nil, err
            }
            result[idx] = dstMap
            continue
          }
        }
        // Override with source value
        result[idx] = sourceSlice[idx]
      }

      // Append remaining source items if source is longer
      if len(sourceSlice) > len(result) {
        result = append(result, sourceSlice[len(result):]...)
      }
    }
    return result, nil

  default:
    // Unknown strategy - fall back to replace
    return values[len(values)-1].Value, nil
  }
}
```

## Deep-Merge Capability with Deferred Merge

### How Deferred Merge Solves the Problem

The deferred merge approach **enables deep-merging with YAML functions**:

```yaml
# catalog/base.yaml
vars:
  config: !template '{{ toJson .settings.base }}'

# stacks/prod.yaml
import:
  - catalog/base

vars:
  config:
    custom_key: value  # With deferred merge: MERGES after processing !template
```

**Processing flow**:
1. **Import**: Both `vars.config` values are detected as YAML function (base) and map (prod)
2. **Defer**: Base YAML function is stored in merge context, placeholder used for merge
3. **Merge**: Normal merge completes without type conflicts
4. **Process**: YAML function is processed → becomes `{"base_key": "base_value"}`
5. **Re-merge**: Deep-merge `{"base_key": "base_value"}` with `{"custom_key": "value"}`
6. **Result**: `{"base_key": "base_value", "custom_key": "value"}`

### Merge Behavior by Type

**Simple types (string, number, bool)**:
- Latest value wins (override behavior)
- Example: `stage: "dev"` overrides `stage: !env STAGE`

**Lists/Slices**:
- Behavior depends on `settings.list_merge_strategy` in `atmos.yaml`
- Configurable via:
  - `atmos.yaml`: `settings.list_merge_strategy`
  - Environment variable: `ATMOS_SETTINGS_LIST_MERGE_STRATEGY`
  - Command-line flag: `--settings-list-merge-strategy`

**Available strategies**:
1. **`replace`** (default): Latest list wins (override behavior)
    - Example: `vpc_ids: ["vpc-1"]` overrides `vpc_ids: !terraform.output vpc_ids`

2. **`append`**: Lists are concatenated in import order
    - Example: `[1, 2]` + `[3, 4]` = `[1, 2, 3, 4]`

3. **`merge`**: List items are deep-merged by index position
    - Items in source list take precedence
    - Processes up to length of source list
    - Remaining destination items preserved if destination is longer

**Maps**:
- Deep-merge all values in precedence order
- Example: YAML function result merges with inline overrides
- Supports partial overrides of individual keys

### Implementation Challenges

**1. Path Tracking**:
- Need to track full path for nested structures (e.g., `components.terraform.vpc.vars.config`)
- Path must be preserved through all import levels

**2. Placeholder Strategy**:
- Using `nil` may cause issues with some merge operations
- Alternative: Use a sentinel value that can be detected and removed

**3. Circular References**:
- YAML functions may reference other YAML functions
- Need to detect and handle circular dependencies

**4. Performance Considerations**:
- Additional pass over data structures
- Memory overhead for storing deferred values
- May need optimization for large configurations

**5. List Merge Strategy Integration**:
- Must respect `settings.list_merge_strategy` from `atmos.yaml`
- Strategy can be overridden via environment variable or CLI flag
- Default strategy is `replace` for backward compatibility

## Configuration

### List Merge Strategy

The deferred merge implementation must respect the configured list merge strategy:

```yaml
# atmos.yaml
settings:
  # Specifies how lists are merged in Atmos stack manifests
  # Can also be set using 'ATMOS_SETTINGS_LIST_MERGE_STRATEGY' environment variable
  # or '--settings-list-merge-strategy' command-line argument
  list_merge_strategy: replace  # Options: replace, append, merge
```

**Strategy Details**:

1. **`replace`** (default):
    - Most recent list imported wins
    - Complete override behavior
    - Fastest performance
    - Example: `[1, 2]` + `[3, 4]` = `[3, 4]`

2. **`append`**:
    - Lists are concatenated in import order
    - Useful for accumulating values across imports
    - Example: `[1, 2]` + `[3, 4]` = `[1, 2, 3, 4]`

3. **`merge`**:
    - List items are deep-merged by index position
    - Items in source list take precedence
    - Processes up to length of source list
    - Remaining destination items preserved if destination is longer
    - Example:
    ```yaml
    # Base: [{"a": 1}, {"b": 2}]
    # Override: [{"a": 10, "c": 3}]
    # Result: [{"a": 10, "c": 3}, {"b": 2}]
    ```

**Interaction with YAML Functions**:

When YAML functions return lists, the merge strategy applies to the processed result:

```yaml
# catalog/base.yaml
vars:
  items: !template '{{ toJson .settings.base_items }}'  # Returns [{"id": 1}]

# stacks/prod.yaml
import:
  - catalog/base

settings:
  list_merge_strategy: append  # or merge

vars:
  items:
    - id: 2  # With append: [{"id": 1}, {"id": 2}]
      # With merge:  [{"id": 2}] (override first item)
      # With replace: [{"id": 2}] (default)
```

## Implementation Strategy

### Recommended Approach: Deferred Merge (v2.0)

**Breaking Change**: No (enhances existing behavior)
**Complexity**: Medium-High
**Benefit**: Enables deep-merge with YAML functions while maintaining backward compatibility

**Implementation Steps**:

1. **Create `pkg/merge/deferred.go`**: Implement MergeContext and DeferredValue data structures
2. **Modify `pkg/merge/merge.go`**: Add WalkAndDeferYAMLFunctions function
3. **Update `internal/exec/stack_processor.go`**: Integrate deferred merge into processing pipeline
4. **Add comprehensive tests**: Test all YAML function types with various merge scenarios
5. **Performance testing**: Benchmark with large configurations

### Backward Compatibility

**Current behavior preserved**:
- Simple type overrides work as before
- List/slice overrides work as before
- Map-to-map merges work as before
- YAML function to concrete type overrides work as before

**New capability added**:
- Maps can now deep-merge with YAML function results
- Multiple YAML functions at the same path merge in precedence order

**No breaking changes**:
- Existing configurations continue to work
- Users get automatic benefit of deep-merge where applicable
- Override behavior still available for simple types and lists

### Alternative Approaches Considered

#### Option 1: Opt-In Flag (Rejected)
```yaml
# atmos.yaml
settings:
  merge:
    yaml_function_mode: "deep"  # opt-in
```

**Why rejected**: Adds configuration complexity for something that should "just work"

#### Option 2: Function-Specific Annotations (Rejected)
```yaml
vars:
  config: !template.merge '{{ toJson .settings.base }}'
```

**Why rejected**: Requires users to understand implementation details, violates the principle of least surprise

#### Option 3: Current Approach (Deferred Merge)
**Why selected**:
- No configuration needed
- Backward compatible
- Intuitive behavior - merges work as users expect
- No breaking changes

## Testing

### Test Fixture
Created comprehensive test fixture at `tests/fixtures/scenarios/atmos-yaml-functions-merge/`:

```yaml
# atmos.yaml
templates:
  settings:
    enabled: true
    sprig:
      enabled: true

# stacks/catalog/blob-defaults.yaml
components:
  terraform:
    blob-with-list:
      settings:
        my_list: [1, 2, 3]
        my_map:
          b: 2
          c: 3
      vars:
        foo_list: !template '{{ toJson .settings.my_list }}'
        foo_map: !template '{{ toJson .settings.my_map }}'

# stacks/test.yaml
import:
  - catalog/blob-defaults

components:
  terraform:
    blob-with-list:
      vars:
        foo_list: []      # Should override without error
        foo_map:
          a: 1            # Should override without error
```

### Test Scenarios
1. ✅ Override `!template` list with empty list
2. ✅ Override `!template` map with different map
3. ✅ Override concrete value with `!template` function
4. ✅ Normal deep-merge when types match
5. ✅ All post-merge YAML functions handled correctly

## User-Facing Documentation Needs

### 1. Update YAML Functions Documentation
**Location**: `website/docs/core-concepts/functions/yaml/`

Add section explaining:
- How YAML functions interact with merge behavior
- Limitation: Cannot deep-merge YAML functions with concrete values
- Workaround patterns (with examples)
- When to use each pattern

### 2. Update Stack Inheritance Documentation
**Location**: `website/docs/core-concepts/stacks/`

Add section explaining:
- YAML function override behavior
- Examples of importing catalogs with YAML functions
- Best practices for organizing YAML functions in hierarchies

### 3. Migration Guide
For users experiencing merge errors or using workarounds:

```markdown
## Simplified Configuration with Deferred Merge

If you previously encountered "cannot override two slices with different type" errors
or had to use workarounds, you can now simplify your configurations.

### Before: Type Mismatch Errors

```yaml
# catalog/base.yaml
vars:
  config: !template '{{ toJson .settings.base }}'

# stacks/prod.yaml
import:
  - catalog/base

vars:
  config:
    custom_key: value  # ERROR: Type mismatch!
```

**Error**: `cannot override two slices with different type ([]interface {}, string)`

### After: Automatic Deep-Merge

```yaml
# catalog/base.yaml
vars:
  config: !template '{{ toJson .settings.base }}'  # Returns {"base": "value"}

# stacks/prod.yaml
import:
  - catalog/base

vars:
  config:
    custom_key: value  # ✓ Now deep-merges after processing!

# Result: {"base": "value", "custom_key": "value"}
```

### Before: Complex Template Merging Workaround

```yaml
# catalog/base.yaml
settings:
  base_config:
    key1: value1

vars:
  config: !template '{{ toJson .settings.base_config }}'

# stacks/prod.yaml
import:
  - catalog/base

settings:
  prod_overrides:
    key2: value2

vars:
  # Had to merge manually in template
  config: !template '{{ toJson (merge .settings.base_config .settings.prod_overrides) }}'
```

### After: Natural Override Pattern

```yaml
# catalog/base.yaml
vars:
  config: !template '{{ toJson .settings.base_config }}'

# stacks/prod.yaml
import:
  - catalog/base

vars:
  config:
    key2: value2  # ✓ Just add the override - deep-merge happens automatically!

# Result: Merges YAML function result with override
```

### Override Behavior Still Available

For simple types and lists, override behavior is preserved:

```yaml
# catalog/base.yaml
vars:
  stage: !env STAGE
  vpc_ids: !terraform.output vpc_ids

# stacks/prod.yaml
import:
  - catalog/base

vars:
  stage: "production"     # Overrides env var
  vpc_ids: ["vpc-custom"] # Overrides terraform output
```

## Implementation Checklist

### Phase 1: Core Implementation
- [ ] Create `pkg/merge/deferred.go` with MergeContext and DeferredValue
- [ ] Implement `WalkAndDeferYAMLFunctions` in `pkg/merge/merge.go`
- [ ] Modify `MergeSections` to use merge context
- [ ] Implement `ApplyDeferredMerges` in `internal/exec/stack_processor.go`
- [ ] Add helper functions (`SetValueAtPath`, `MergeDeferredValues`, `isMap`, `isSlice`)
- [ ] Implement `mergeSlices` with support for all three list merge strategies
- [ ] Handle all 7 post-merge YAML functions
- [ ] Integrate into stack processing pipeline
- [ ] Ensure `list_merge_strategy` setting is respected

### Phase 2: Testing
- [x] Create test fixture at `tests/fixtures/scenarios/atmos-yaml-functions-merge/`
- [ ] Add unit tests for deferred merge logic
- [ ] Add integration tests for all YAML function types
- [ ] Test deep-merge scenarios (map merging with YAML functions)
- [ ] Test override scenarios (simple types, lists)
- [ ] Test list merge strategies (`replace`, `append`, `merge`)
- [ ] Test list merge with YAML functions for all three strategies
- [ ] Test edge cases (circular refs, nested YAML functions)
- [ ] Performance benchmarks with large configurations
- [ ] Verify no breaking changes to existing functionality

### Phase 3: Documentation
- [x] Create PRD document (`docs/prd/yaml-function-merge-handling.md`)
- [ ] Update user-facing documentation (YAML functions)
- [ ] Update stack inheritance documentation
- [ ] Add deep-merge examples to documentation
- [ ] Document merge behavior by type
- [ ] Add a troubleshooting section
- [ ] Add examples to Atmos examples repository

### Phase 4: Release
- [ ] Code review
- [ ] Update CHANGELOG
- [ ] Consider a blog post announcement
- [ ] Create a GitHub release

## Next Steps

> **Historical / completed.** This "Next Steps" section (Immediate Actions, Implementation Order,
> Open Questions) was written before implementation began and describes the intended build order,
> not shipped file/function names. Data-structure and pre-merge-detection work (steps 1–2 below)
> shipped as `pkg/merge/deferred.go` and `pkg/merge/merge_yaml_functions.go` in the original PRD
> pass; the post-merge application and integration work (step 3) — actually wiring a real
> `YAMLFunctionProcessor` into `ApplyDeferredMerges` at a production call site — was the part left
> undone and is what the ["Known Gap: #2888"](#known-gap-2888--deferred-values-are-never-actually-resolved-in-production)
> and ["Completion Plan"](#completion-plan-wiring-post-merge-resolution-plan-b) sections below cover;
> that plan has since shipped (see the [Status](#status) note at the top of this document and the
> "Post-fix status" notes in those sections).

### Immediate Actions

1. **Remove transformer code** from `pkg/merge/merge.go` (if any remains)
2. **Begin Phase 1 implementation**: Create `pkg/merge/deferred.go`
3. **Design integration points**: Identify where in `stack_processor.go` to integrate deferred merge
4. **Set up testing infrastructure**: Create comprehensive test suite for deferred merge

### Implementation Order

1. **Data structures first** (`pkg/merge/deferred.go`):
    - MergeContext
    - DeferredValue
    - Helper methods

2. **Pre-merge detection** (`pkg/merge/merge.go`):
    - WalkAndDeferYAMLFunctions
    - Modify MergeSections to use context

3. **Post-merge application** (`internal/exec/stack_processor.go`):
    - ApplyDeferredMerges
    - Integration into processing pipeline

4. **Helper utilities**:
    - SetValueAtPath (set nested values in maps)
    - MergeDeferredValues (merge values by type)
    - isMap (type checking)

5. **Testing**:
    - Unit tests for each component
    - Integration tests for end-to-end scenarios
    - Performance benchmarks

### Open Questions

1. **Placeholder strategy**: Should we use `nil`, empty map `{}`, or a sentinel value?
2. **Circular reference detection**: How should we handle YAML functions that reference other YAML functions?
3. **Error handling**: How to report errors in deferred merge (path, precedence, etc.)?
4. **Performance optimization**: Is caching needed for large configurations?

## Implementation Status

**Status**: ⚠️ **PARTIALLY COMPLETED — the mechanism below is real and tested, but was never
connected end-to-end in production. See [Known Gap: #2888](#known-gap-2888--deferred-values-are-never-actually-resolved-in-production).**
**Date**: 2025-11-29
**Version**: 2.0

The deferred-merge *mechanism* described below (data structures, walk-and-defer, `ApplyDeferredMerges`,
`MergeDeferredValues`) was implemented and unit-tested in isolation. What was **not** completed is
the "Next Steps for Full Integration" work already listed at the bottom of this document — every
production call site in `internal/exec/stack_processor_merge.go` invokes `ApplyDeferredMerges` with
`processor = nil`, so the "process YAML functions, then deep-merge the resolved value" half of this
design has never actually run against real Atmos stacks. This section is preserved as an accurate
record of what was built; it documents implemented infrastructure, not delivered behavior.

### Files Created/Modified

#### Core Infrastructure
- **`pkg/merge/deferred.go`** (55 lines)
  - `DeferredValue` - tracks deferred YAML functions with path, value, precedence
  - `DeferredMergeContext` - manages collection of deferred values during merge
  - Helper methods for adding, tracking, and retrieving deferred values

#### Merge Functions
- **`pkg/merge/merge_yaml_functions.go`** (366 lines) - NEW FILE
  - `isAtmosYAMLFunction()` - detects 7 post-merge YAML function types
  - `WalkAndDeferYAMLFunctions()` - recursively walks maps and defers YAML functions
  - `isMap()`, `isSlice()` - type checking helpers
  - `SetValueAtPath()` - sets values at nested map paths
  - `mergeSlicesAppendStrategy()` - handles append merge strategy
  - `mergeSlicesMergeStrategy()` - handles merge strategy with deep-merging
  - `mergeSliceItems()` - deep-merges individual slice items
  - `mergeSlices()` - merges slices according to strategy (replace/append/merge)
  - `mergeDeferredMaps()` - deep-merges map values
  - `MergeDeferredValues()` - merges deferred values by type
  - `MergeWithDeferred()` - complete wrapper for deferred merge workflow
  - `ApplyDeferredMerges()` - processes and applies deferred values after merge

#### Error Handling
- **`errors/errors.go`** (additions)
  - `ErrEmptyPath` - for empty path validation
  - `ErrCannotNavigatePath` - for path navigation failures
  - `ErrUnknownListMergeStrategy` - for unknown strategy names

### Test Coverage

#### Unit Tests
- **`pkg/merge/deferred_test.go`** (135 lines, 8 test functions)
  - Tests for `DeferredMergeContext` operations
  - Path and precedence tracking
  - Deferred value management

- **`pkg/merge/merge_deferred_test.go`** (688 lines, 8 test functions, 58 test cases)
  - YAML function detection for all 7 types
  - Recursive map walking and deferral
  - Helper function tests
  - List merge strategy tests (replace, append, merge)
  - Value merging by type
  - Integration wrapper tests
  - Edge cases and error handling

#### Integration Tests
- **`tests/yaml_functions_merge_test.go`** (296 lines, 2 test functions, 10 test cases)
  - End-to-end deferred merge workflow
  - Multiple YAML function precedence
  - All 7 YAML function types
  - Nested YAML functions
  - List merge strategies with YAML functions
  - Type conflict handling
  - Nil handling

#### Test Fixtures
- **`tests/fixtures/scenarios/atmos-yaml-functions-merge/`**
  - `atmos.yaml` - Configuration with list merge strategy
  - `stacks/catalog/base.yaml` - Base catalog with YAML functions
  - `stacks/test-deferred-merge.yaml` - Test scenarios
  - `stacks/test-yaml-functions.yaml` - Comprehensive YAML function tests

#### Examples
- **`pkg/merge/example_deferred_test.go`** (177 lines, 2 examples)
  - Complete deferred merge workflow example
  - List merge strategy demonstrations

### Test Results

```
✓ Unit tests: 76 test cases passing
✓ Integration tests: 10 test cases passing
✓ Coverage: 89.9% of statements in pkg/merge
✓ All existing tests still pass
✓ Full project builds successfully
```

**Caveat (added 2026-08-06, describes the pre-fix state — see "Post-fix status" below):** these
numbers describe `pkg/merge` in isolation. Every one of these tests either passed `processor = nil`
to `ApplyDeferredMerges` or asserted the type-conflict-avoidance behavior only — none of them
exercised a real `YAMLFunctionProcessor` resolving a function and that result being deep-merged
against a concrete value from another layer, because no such processor was wired at a real call
site yet. `tests/yaml_functions_integration_test.go`'s `TestYAMLFunctionsDeferredMerge` suite runs
through the real `internal/exec` pipeline against these same fixtures, but until 2026-08-06 every
one of its subtests only asserted the merged key existed (`require.Contains(t, vars,
"template_config")`), never the merged *value* — which is exactly why it was green while the
underlying deep-merge was silently broken. See Known Gap below.

**Post-fix status (2026-08-06, later the same day):** a real `TemplateAwareYAMLProcessor` is now
wired into `ApplyDeferredMerges` at Stage 3 (`internal/exec/deferred_contexts.go`'s
`resolveDeferredYamlFunctions`, called from `internal/exec/utils.go`). The regression tests below
now assert the merged *value*, not just key presence, and pass. See the
["Completion Plan"](#completion-plan-wiring-post-merge-resolution-plan-b) section for the shipped
implementation.

### Benefits Delivered

The checklist below reflects the pre-fix state as of the original 2025-11-29 PRD pass and the
2026-08-06 gap discovery. As of the fix landing later on 2026-08-06 (commit `4b832423e3`), the
previously-❌ item is now delivered — see "Post-fix status" above.

✅ **Eliminates type conflicts** when merging YAML functions with concrete values (no crash)
✅ **Deep-merges a YAML function's resolved value with a concrete value from another layer** — this
was the actual point of the PRD ("Deep-Merge Capability with Deferred Merge" section above). It was
previously undelivered in production (concrete value silently won outright, discarding the
function's contribution — see the Known Gap section below for the historical record); it is now
delivered via the Completion Plan below.
✅ **Preserves merge order** with precedence tracking
✅ **Supports all list merge strategies** (replace, append, merge) — now verified end-to-end via the
production resolution path, not just isolated `pkg/merge` unit tests
✅ **Backwards compatible** - no changes to existing configurations needed
✅ **Well tested at the unit level** - 89.9% coverage with 76 test cases in `pkg/merge`, plus
end-to-end coverage of the resolve-and-remerge path via the regression tests added 2026-08-06
✅ **Documented** - comprehensive examples and integration guides
✅ **Performance optimized** - refactored for reduced cognitive complexity

### Usage Example

```go
import "github.com/cloudposse/atmos/pkg/merge"

// Perform merge with YAML function deferral
result, dctx, err := merge.MergeWithDeferred(&atmosConfig, inputs)
if err != nil {
    return err
}

// Process deferred YAML functions and apply to result
err = merge.ApplyDeferredMerges(dctx, result, &atmosConfig)
if err != nil {
    return err
}
```

### Integration Points

The deferred merge infrastructure is ready for integration into the stack processing pipeline:

1. **Stack Processor**: Replace `m.Merge()` calls with `m.MergeWithDeferred()`
2. **Component Merge**: Call `ApplyDeferredMerges()` after all sections are merged
3. **YAML Function Processing**: Integrate existing YAML function processors into `ApplyDeferredMerges`

See integration example in `pkg/merge/merge_yaml_functions.go` (`MergeWithDeferred` function documentation).

## Known Gap: #2888 — Deferred Values Are Never Actually Resolved in Production

> **Historical — this gap is now closed.** This section documents a bug that existed between
> 2025-11-29 (original PRD) and 2026-08-06 (fix landed, commit `4b832423e3`,
> "fix(merge): resolve deferred YAML functions and deep-merge with concrete overrides (#2888)"). It
> is kept as-written for the historical record of the investigation. For the as-shipped result, see
> the "Post-fix status" note under ["Benefits Delivered"](#benefits-delivered) and the shipped-code
> pointers in the [Completion Plan](#completion-plan-wiring-post-merge-resolution-plan-b) section
> below.

**Discovered**: 2026-08-06, via [GitHub issue #2888](https://github.com/cloudposse/atmos/issues/2888)
(`!labels`/`!tags` losing data on merge) and a field-test/investigation pass that traced the failure
to source and empirically confirmed it against the real production pipeline.

### What's actually true today

Item 2 of the "Next Steps for Full Integration" below (stack-processor integration) **was** done —
`internal/exec/stack_processor_merge.go`'s `mergeComponentConfigurations` does call
`m.MergeWithDeferred`/`m.ApplyDeferredMerges` for every deferred-eligible section (vars, settings,
env, auth, providers, required_providers, hooks, generate, test — 10 call sites total). Item 1
(connecting a real YAML function processor) **was never done**: every one of those 10 call sites
passes `processor = nil`. Per `MergeDeferredValues` (`pkg/merge/merge_yaml_functions.go:316-341`),
when `processor` is nil the deferred value stays an unresolved function *string*; the "is it a
map/slice, should I deep-merge" check at that point sees a string, takes the "simple type: highest
precedence wins" branch, and the function's own contribution is discarded outright — silently, no
error. This is functionally identical, for a *recognized* function, to what happens to an
*unrecognized* one (the literal #2888 report): both end up "last concrete value at that path wins,"
just for different mechanical reasons (nil-processor discard vs. never-deferred-in-the-first-place).

Empirically verified (details in the issue and in the regression tests added below):
- `!labels`/`!tags`/`!labels.keys`/`!labels.values` aren't in the `postMergeFunctions` allowlist
  (`pkg/merge/merge_yaml_functions.go:22-30`) at all, so they're not even deferred — the literal
  #2888 report.
- Patching that allowlist to add them does **not** fix the bug, because of the `processor=nil` gap
  above — confirmed by hand-tracing and by live reproduction against a patched-then-reverted binary.
- `!template`, which **is** in the allowlist, exhibits the byte-for-byte identical silent data loss
  against a conflicting override — confirmed live against the existing
  `tests/fixtures/scenarios/atmos-yaml-functions-merge/` fixture (`test-deep-merge` component),
  which has encoded this exact scenario since the fixture was first written (its own inline comment
  reads "This will be deep-merged with the result of `!template`") — it just was never asserted
  strongly enough to notice, until 2026-08-06 (see Test Results caveat above).

### Regression tests (added 2026-08-06, both currently RED against `main`)

- `tests/yaml_functions_integration_test.go` → `TestYAMLFunctionsDeferredMerge/deep_merges_with_yaml_functions`
  — strengthened from `require.Contains(t, vars, "template_config")` to asserting the full expected
  deep-merged map; fails today because `timeout`/`retries` (present only in the `!template` result)
  are silently dropped.
- `tests/yaml_functions_integration_test.go` → `TestYAMLFunctionsDeferredMerge/deep_merges_with_labels_and_tags_functions_(regression_for_#2888)`
  — new subtest against a new fixture component (`base-component-labels` / `test-labels-override` in
  `tests/fixtures/scenarios/atmos-yaml-functions-merge/`), asserting `!labels`/`!tags` deep-merge
  with a conflicting component-level override; fails today for the same reason.

Both should go GREEN once the completion plan below ships.

**Post-fix status:** both regression tests now pass — see the "Post-fix status" note under
["Benefits Delivered"](#benefits-delivered) above.

## Completion Plan: Wiring Post-Merge Resolution (Plan B)

> **Historical / completed.** This section was written as a forward-looking plan on 2026-08-06,
> before implementation, and is kept largely as-written for the design rationale (why Stage 2 can't
> be the resolution point, what must not regress). The actual fix landed the same day as a single
> commit (`4b832423e3`, "fix(merge): resolve deferred YAML functions and deep-merge with concrete
> overrides (#2888)") rather than as the staged PR 1 (plumbing-only) / PR 2 (behavior change) split
> described below — the two were combined into one change. The shipped implementation follows this
> plan's "Full unification" design closely: `internal/exec/deferred_contexts.go`'s
> `resolveDeferredYamlFunctions` is the Stage 3 resolution point (called from
> `internal/exec/utils.go`, after `ProcessCustomYamlTags` and before
> `postProcessTemplatesAndYamlFunctions`, matching PR 1 step 5 / PR 2 step 4 below); it constructs a
> `TemplateAwareYAMLProcessor` (`internal/exec/yaml_processor.go`, matching PR 2 step 3) and resolves
> per-section `*merge.DeferredMergeContext` bundles carried on
> `schema.ConfigAndStacksInfo.DeferredMergeContexts` (typed `any` to avoid the import cycle, matching
> PR 1 step 4). The shipped fix additionally handles the mirror-precedence direction (a concrete
> value at a *lower*-precedence layer than the function) via `pkg/merge/merge_yaml_functions.go`'s
> `fillMissingLayerValues`, which this plan did not originally anticipate — added after a new
> regression test caught it during implementation.

Two designs were evaluated for closing this gap:

- **Narrow/per-function workaround** (rejected): special-case `!labels`/`!tags` with a second,
  metadata-scoped resolve pass bolted onto `mergeComponentConfigurations`, plus a curated
  "safe subset" processor for `!store`/`!store.get`/`!exec`/`!env`, permanently leaving
  `!terraform.output`/`!terraform.state`/`!template` broken. Rejected because it adds new
  special-case machinery on top of an already two-stage pipeline instead of fixing the actual
  defect, and it doesn't fulfill this PRD's own "Next Steps for Full Integration" item 1 — it works
  around not having a real processor rather than wiring one.
- **Full unification** (selected, detailed below): finish what this PRD already set out to build —
  carry the deferred-merge context forward to the point where the whole component (including
  `metadata` and rendered `!template` output) is finally assembled, and resolve once, uniformly,
  through the `ApplyDeferredMerges`/`MergeDeferredValues`/`mergeDeferredMaps` machinery that's
  already implemented and unit-tested. No per-function special-casing, no second allowlist to keep
  in sync with `pkg/merge`'s.

### Why Stage 2 (inside `mergeComponentConfigurations`) can't be the resolution point

Three of the seven `postMergeFunctions` have cross-section or cross-stage dependencies that don't
exist yet at any point inside `mergeComponentConfigurations`:

- **`!labels`/`!tags`/`!labels.keys`/`!labels.values`** read `stackInfo.ComponentSection["metadata"]`
  (`internal/exec/yaml_func_tags.go:13-19`). `metadata` is merged via a separate plain `m.Merge`
  near the *end* of `mergeComponentConfigurations` (`internal/exec/stack_processor_merge.go:405-420`),
  after `vars` (`:86-102`) — and `stackInfo`/`ComponentSection` as a whole object doesn't exist until
  the function's caller assembles it.
- **`!template`**'s `{{ }}` rendering is not part of its tag dispatch at all —
  `processTagTemplate` (`internal/exec/yaml_func_template.go:13-31`) only `json.Unmarshal`s the
  string *after* the tag; on decode failure it silently returns the un-rendered text. The actual
  Go-template substitution happens in a separate whole-document textual pass,
  `ProcessTmplWithDatasources` (`internal/exec/utils.go:948-999`), which runs on the fully-assembled
  `configAndStacksInfo.ComponentSection` — strictly after `mergeComponentConfigurations` returns.
- **`!terraform.output`/`!terraform.state`** need `stackInfo.AuthContext`/`AuthManager` for correct
  per-component cross-account credential resolution (`internal/exec/yaml_func_terraform_output.go:103-113`).
  `mergeComponentConfigurations` runs inside a per-component goroutine
  (`internal/exec/stack_processor_process_stacks.go:1507-1523`) whose
  `ComponentProcessorOptions` carries no auth context. Resolving these early would silently use
  ambient/default credentials instead of the configured chain — a correctness/security regression,
  not just a sequencing inconvenience.

Given 3 of 7 functions are structurally blocked at Stage 2, and the remaining 4
(`!store`/`!store.get`/`!exec`/`!env`) would need Stage 3's existing cycle-detection
(`ResolutionContext`, `internal/exec/yaml_func_utils.go:39-51`) duplicated at a second call site to
resolve safely at Stage 2 anyway, uniform treatment at Stage 3 is both correctness-required for some
functions and architecturally cheaper for the rest. Special-casing "some functions resolve at Stage
2, some at Stage 3" would reintroduce exactly the per-function inconsistency this bug report is
about.

### PR 1 — Plumbing only, no behavior change

Carry each section's `*merge.DeferredMergeContext` forward instead of discarding it after the local
`ApplyDeferredMerges(..., nil)` call:

1. **`internal/exec/stack_processor_merge.go`** — `mergeComponentConfigurations` returns an
    additional `map[string]*merge.DeferredMergeContext` (keyed by section name: vars, settings, env,
    auth, providers, required_providers, hooks, generate, test) alongside the merged component map.
    Remove none of the existing `MergeWithDeferred` calls (they still correctly prevent type-conflict
    crashes during the raw merge) — just stop discarding the context afterward. `processAuthConfig`
    (`:707-728`) needs the same treatment for its second, later auth-merge pass.
2. **`internal/exec/stack_processor_process_stacks.go`** — `processComponentsInParallel` (`:1478-1541`)
    threads the returned context bundle alongside `comp` through `componentProcessResult` and the
    collection loop.
3. **`internal/exec/utils.go`** — `findStacksMapCacheEntry` (`:344-348`) and `FindStacksMap`
    (`:423-474`, and its sibling `FindStacksMapForGenerate` in `generate_adapter_funcs.go:30`) gain a
    third dimension: `map[stack]map[componentType]map[component]map[string]*merge.DeferredMergeContext`.
    This is a signature change to a widely-called exported function — audit every call site as part
    of this PR, not as a follow-up.
4. **`pkg/schema/schema.go`** — add a field to `ConfigAndStacksInfo` (`:1556`) typed `any` (not the
    concrete `*merge.DeferredMergeContext` type), mirroring the existing `AuthManager any` field
    (`:1608-1613`) and for the identical reason: `pkg/merge` imports `pkg/schema`
    (`pkg/merge/deferred.go:1-7`), so a concretely-typed field would create an import cycle.
    Type-assert at the two use sites.
5. Add a new (no-op for now) call site in `internal/exec/utils.go`, after `ProcessCustomYamlTags`
    (`:1022-1035`) and before `postProcessTemplatesAndYamlFunctions` (`:1037-1039`), that will host
    the real resolution in PR 2.

Land behind full `atmos test --full` plus a before/after perf heatmap comparison (see Performance
below) as the acceptance gate — this PR should be verifiably a no-op.

### PR 2 — The actual fix

1. **`pkg/merge/merge_yaml_functions.go`** — replace the hand-rolled `postMergeFunctions` list
    (`:22-30`) with the canonical constants already defined once in `pkg/utils/yaml_utils.go:26-63`
    (`u.AtmosYamlFuncTemplate`, `u.AtmosYamlFuncLabels`, `u.AtmosYamlFuncLabelsKeys`,
    `u.AtmosYamlFuncLabelsValues`, `u.AtmosYamlFuncTags`, plus the existing 7). `pkg/merge` already
    imports `pkg/utils` as `u` (`pkg/merge/merge.go:13`); `pkg/utils` doesn't import `pkg/merge`, so no
    cycle. This is the direct fix for the literal #2888 report and closes the "two independently
    maintained function-recognition lists" root cause as a *class* of bug, not just for these four
    functions. Curate deliberately: `!unset`/`!append`/`!include`/`!include.raw`/`!literal` are
    handled by dedicated pre-merge or merge-structural mechanisms (`pkg/merge/merge.go:468-477` for
    `!append`; parse-time resolution for `!include`/`!literal`) and must stay excluded.
2. **`internal/exec/stack_processor_merge.go`** — remove the now-pointless
    `ApplyDeferredMerges(ctx, result, mergeConfig, nil)` calls at all 9 in-function call sites (they
    currently just collapse-before-resolving; per the analysis above, keeping them as dead-weight
    `nil` calls would keep the bug alive). The per-section contexts already flow out via PR 1's return
    value.
3. **`internal/exec/yaml_processor.go`** — add a template-aware wrapper around the existing (already
    built, currently-unused-in-production) `ComponentYAMLProcessor`
    (`internal/exec/yaml_processor.go:11-59`) that Go-template-renders a deferred string via
    `ProcessTmplWithDatasources` (reusing the `componentTemplateContext` already built at
    `internal/exec/utils.go:977-989`) before delegating to `processCustomTagsWithContext`
    (`internal/exec/yaml_func_utils.go:476-496`). This is required so `!template` fails loudly on a
    render error instead of silently returning unrendered `{{ }}` text — modify the existing file, no
    new file under `internal/exec/`.
4. **`internal/exec/utils.go`** — fill in the no-op call site from PR 1 step 5: for each section in
    the recovered context bundle, resolve via the wrapper above and call
    `m.ApplyDeferredMerges(sectionDctx, componentSection[sectionName], mergeConfig, processor)`,
    reusing the same `ResolutionContext` instance Stage 3 already uses
    (`internal/exec/yaml_func_utils.go:51`) so `!terraform.output`/`!terraform.state` cycle detection
    stays consistent. `postProcessTemplatesAndYamlFunctions` (`:1355-1410`) already re-syncs the
    typed `Component*Section` fields from `ComponentSection` after mutation, so no change needed
    there.

### What must NOT change

- `pkg/merge/example_deferred_test.go`'s `ExampleMergeWithDeferred` (`:14-121`) and
  `pkg/merge/merge_deferred_test.go`'s `TestApplyDeferredMerges` (`:857-959`) both correctly document
  `processor=nil` as a supported, intentional mode (`pkg/merge/deferred.go:526-527`'s own doc
  comment) for the case where there's no *competing* concrete value at that path — leave these
  assertions as-is. The bug is specifically the case neither of these tests exercises: a concrete
  value competing with an unresolved function at the same path.

### Test plan

- The two regression tests added 2026-08-06 (`tests/yaml_functions_integration_test.go`,
  `TestYAMLFunctionsDeferredMerge/deep_merges_with_yaml_functions` and
  `.../deep_merges_with_labels_and_tags_functions_(regression_for_#2888)`) are the concrete
  red→green acceptance signal for PR 2.
- Add a mirror-precedence integration test (function at *higher* precedence than the concrete map,
  the reverse of the existing fixtures) to prove the deep-merge is symmetric.
- Add `!terraform.output`/`!store` deep-merge-against-concrete-override integration tests — existing
  fixtures cover type-conflict-avoidance for these, not the deep-merge case.
- Add a unit test for the new template-aware processor wrapper, including the negative case: it must
  fail loudly (not silently return unrendered text) if invoked without a render step.
- Add a cache-correctness regression test: two `describe component` calls for different components
  sharing a cached `stacksMap` each get their own deferred-context bundle, not a leaked/shared one
  (guards a keying bug given the parallel population in `processComponentsInParallel`).
- Fix the pre-existing, unrelated fixture gap that causes
  `TestYAMLFunctionsDeferredMerge/handles_multiple_yaml_functions_with_precedence` to `t.Skip()`
  today (`tests/yaml_functions_integration_test.go:324-326` — the `test-multiple-yaml-functions`
  stack entry has no matching base component definition in the fixture).

### Risk / rollout

- **No feature flag** — this is a correctness fix to existing, already-shipped machinery, not new
  user-facing surface.
- **This can change output for stacks currently (unknowingly) relying on the silent-drop behavior**
  — e.g. a stack whose component-level override today "wins" over a catalog's `!template`/`!labels`
  value will, after this fix, get a deep-merged result instead. Flag this prominently in the PR
  description and changelog (per the `pull-request` skill) so downstream users can audit before
  upgrading; this is a bug fix, not a purely additive feature, and needs the corresponding semver
  label.
- **Performance**: `WalkAndDeferYAMLFunctions`'s own doc comment
  (`pkg/merge/merge_yaml_functions.go:66-76`) records a tuned-hot-path history (527k recursive
  calls / 1m26s CPU across ~9k component instances in one real workload). This change doesn't touch
  that walk/detection path, but does add real cost of its own: the context bundle now flows through
  every cached `stacksMap` lookup (cheap if empty, but not free to construct/copy), and the new
  Stage 3 pass invokes `ProcessTmplWithDatasources` per deferred `!template` string rather than once
  per whole document — needs its own `perf.Track` span, a fast-path (skip the per-string render call
  if the string contains no `{{`), and a before/after heatmap comparison before merging PR 1.
  `!exec`/`!terraform.output`/etc. move earlier in the pipeline but are not invoked twice (confirmed:
  `SetValueAtPath` replaces the raw tag string in the result map once resolved, so the existing Stage
  3 dispatch is a no-op for that path afterward) — still worth a wall-clock smoke test on a
  `!exec`/`!store`-heavy stack.
- **Test suites to run**: `go test ./pkg/merge/...`, `go test ./internal/exec/...` (this package is
  on the hot path for every `atmos` command that resolves components), `go test ./tests/...` focused
  on `TestYAMLFunctionsDeferredMerge`/`TestYAMLFunctionsInLists`, and `atmos test --full` against
  example stacks using `!store`/`!exec`/`!env`/`!template`/`!labels`/`!tags`.
- Sequencing: land PR 1 first (verifiable no-op, isolates the `FindStacksMap` signature break for
  independent review/bisection), then PR 2 (the actual behavior change) gated on the test plan above.

### Follow-up tracking

This work is tracked by [GitHub issue #2888](https://github.com/cloudposse/atmos/issues/2888) — link
the implementing PR(s) to it per this repo's follow-up tracking mandate.

## References

- YAML Functions Documentation: https://atmos.tools/functions/yaml/
- Mergo Library: https://github.com/darccio/mergo
- Implementation: `pkg/merge/deferred.go`, `pkg/merge/merge_yaml_functions.go`
- Tests: `pkg/merge/*_test.go`, `tests/yaml_functions_merge_test.go`
- Examples: `pkg/merge/example_deferred_test.go`
- Test Fixture: `tests/fixtures/scenarios/atmos-yaml-functions-merge/`
- User Report: bug report about `!template` merge issues

## Future Considerations

### Extended Functionality
- Support for conditional merge (merge only if condition met)
- Merge strategies per YAML function type
- Debug mode showing merge precedence and decisions
