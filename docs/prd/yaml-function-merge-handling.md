# PRD: YAML Function Merge Handling

## Status
**Current**: In Progress - Designing Improved Solution
**Date**: 2025-01-27
**Version**: 2.0 (Draft)

## Problem Statement

### User-Reported Issue

When importing Atmos stack files that contain YAML functions (like `!template`) and attempting to override those values
with concrete types, the merge operation fails with a type mismatch error:

```
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

### Core Concept

Instead of trying to merge YAML functions during the initial merge phase, **defer them** and merge after processing:

1. **During merge**: Detect YAML functions, store all values for affected fields, replace with placeholders
2. **After YAML function processing**: Re-merge the processed values with stored overrides
3. **Result**: Full deep-merge capability while avoiding type conflicts

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

		// Merge all values for this path
		merged, err := MergeDeferredValues(deferredValues)
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
func MergeDeferredValues(values []*DeferredValue) (interface{}, error) {
	if len(values) == 0 {
		return nil, nil
	}

	// Start with first value
	result := values[0].Value

	// For simple types (string, number, bool) and slices: just override with highest precedence
	if !isMap(result) {
		return values[len(values)-1].Value, nil
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
- Latest value wins (override behavior)
- Example: `vpc_ids: ["vpc-1"]` overrides `vpc_ids: !terraform.output vpc_ids`

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
- [ ] Add helper functions (`SetValueAtPath`, `MergeDeferredValues`, `isMap`)
- [ ] Handle all 7 post-merge YAML functions
- [ ] Integrate into stack processing pipeline

### Phase 2: Testing
- [x] Create test fixture at `tests/fixtures/scenarios/atmos-yaml-functions-merge/`
- [ ] Add unit tests for deferred merge logic
- [ ] Add integration tests for all YAML function types
- [ ] Test deep-merge scenarios (map merging with YAML functions)
- [ ] Test override scenarios (simple types, lists)
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

## References

- YAML Functions Documentation: https://atmos.tools/functions/yaml/
- Mergo Library: https://github.com/imdario/mergo
- Related Code: `pkg/merge/merge.go`, `internal/exec/yaml_func_template.go`
- Test Fixture: `tests/fixtures/scenarios/atmos-yaml-functions-merge/`
- User Report: bug report about `!template` merge issues

## Future Considerations

### Extended Functionality
- Support for conditional merge (merge only if condition met)
- Merge strategies per YAML function type
- Debug mode showing merge precedence and decisions
