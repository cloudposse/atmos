# PRD: YAML Function Merge Handling

## Status
**Current**: Implemented
**Date**: 2025-01-23
**Version**: 1.0

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

The problem occurs because most YAML functions are processed **AFTER** merging (step 4), but during merge (step 2) they are still represented as strings. When `mergo` encounters a type mismatch (string vs list, string vs map), it throws an error because it has strict type checking enabled.

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

## Solution

### Implementation Approach
Instead of trying to process YAML functions before merging (which breaks template rendering), we implemented a **custom `mergo` transformer** that allows type mismatches when one side is an Atmos YAML function string.

### How It Works

1. **Detect Type Mismatches** - Transformer only intervenes when types don't match
2. **Check for YAML Functions** - If one side is a YAML function string, allow override
3. **Preserve Normal Merging** - When types match, normal deep-merge continues

### Code Implementation

**Location**: `pkg/merge/merge.go`

```go
// yamlFunctionTransformer is a custom mergo transformer that allows Atmos YAML function strings
// to be overridden by any type during merge. This is necessary because most YAML functions
// are processed AFTER merging (except !include and !include.raw), so during merge they are
// still strings, but after processing they become the actual type (list, map, etc.).
type yamlFunctionTransformer struct{}

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

func (t *yamlFunctionTransformer) Transformer(typ reflect.Type) func(dst, src reflect.Value) error {
	return func(dst, src reflect.Value) error {
		// Only intervene if there's a TYPE MISMATCH and one side is an Atmos YAML function string.

		// Check if types match - if so, let mergo handle normally.
		if dst.Kind() == src.Kind() {
			return nil
		}

		// Types don't match - check if one side is an Atmos YAML function string.

		// Case 1: Destination is a YAML function string, source is a different type.
		if dst.Kind() == reflect.String && isAtmosYAMLFunction(dst.String()) {
			if dst.CanSet() {
				dst.Set(src)
			}
			return nil
		}

		// Case 2: Source is a YAML function string, destination is a different type.
		if src.Kind() == reflect.String && isAtmosYAMLFunction(src.String()) {
			if dst.CanSet() {
				dst.Set(src)
			}
			return nil
		}

		// Type mismatch but neither side is a YAML function - let mergo handle.
		return nil
	}
}
```

**Integration**: Transformer is registered in merge options:
```go
opts = append(opts,
	mergo.WithOverride,
	mergo.WithTypeCheck,
	mergo.WithTransformers(&yamlFunctionTransformer{}))
```

## Deep-Merge Limitation

### The Problem
The transformer solution prevents deep-merging when YAML functions are involved. For example:

```yaml
# catalog/base.yaml
vars:
  config: !template '{{ toJson .settings.base }}'

# stacks/prod.yaml
import:
  - catalog/base

vars:
  config:
    custom_key: value  # This will REPLACE the !template, not merge with it
```

### Why This Cannot Be "Fixed"
This is a **fundamental architectural constraint**, not a bug:

1. **Temporal Problem**: At merge time (T1), the YAML function result is unknown
2. **Type Safety**: We cannot merge an unknown value with a known value
3. **Processing Order**: Functions execute after merge (T2), so at T1 we only have strings

### Impact Assessment
- **Override behavior works**: Users can replace YAML function values completely
- **Deep-merge blocked**: Cannot partially merge into YAML function results
- **Workarounds exist**: Users can achieve desired outcomes through different patterns

## Recommendations

### Short-Term: Documentation and Workarounds

**Document the limitation clearly** in user-facing documentation with workaround patterns:

#### Pattern 1: Separate Concerns
```yaml
# catalog/base.yaml
vars:
  vpc_config: !template '{{ toJson .settings.vpc }}'

# stacks/prod.yaml
import:
  - catalog/base

vars:
  vpc_config: !template '{{ toJson .settings.vpc }}'  # Override completely
  custom_config:                                       # Your additions
    my_custom_value: xyz
```

#### Pattern 2: Merge at Template Time
```yaml
# catalog/base.yaml
settings:
  base_config:
    key1: value1
    key2: value2

vars:
  config: !template '{{ toJson .settings.base_config }}'

# stacks/prod.yaml
import:
  - catalog/base

settings:
  vpc_overrides:
    key3: value3

vars:
  # Merge within the template
  config: !template '{{ toJson (merge .settings.base_config .settings.vpc_overrides) }}'
```

#### Pattern 3: Explicit Override
```yaml
# catalog/base.yaml
vars:
  vpc_list: !terraform.output vpc_ids

# stacks/prod.yaml
import:
  - catalog/base

vars:
  # Completely replace (not merge)
  vpc_list: ["vpc-custom1", "vpc-custom2"]
```

### Long-Term: Architecture Options

#### Option 1: Two-Phase Merge (Recommended for Atmos 2.0)
**Breaking Change**: Yes
**Complexity**: High
**Benefit**: Enables deep-merge with YAML functions

**Approach**:
1. Phase 1: Merge all concrete values
2. Phase 2: Process YAML functions
3. Phase 3: Re-merge processed values into Phase 1 result

**Considerations**:
- Requires reworking merge pipeline
- Breaking change for users relying on current override behavior
- Significantly more complex implementation
- Performance impact needs evaluation

#### Option 2: Opt-In Deep-Merge Flag
**Breaking Change**: No
**Complexity**: Medium
**Benefit**: Backward compatible, gradual migration

**Approach**:
```yaml
# atmos.yaml
settings:
  merge:
    yaml_function_mode: "override"  # default (current behavior)
    # yaml_function_mode: "deep"    # opt-in deep-merge
```

#### Option 3: Function-Specific Merge Annotations
**Breaking Change**: No
**Complexity**: Medium
**Benefit**: Fine-grained control

**Approach**:
```yaml
vars:
  config: !template.merge '{{ toJson .settings.base }}'  # Deep-merge hint
  other: !template '{{ toJson .settings.other }}'        # Override (default)
```

#### Option 4: Status Quo + Documentation
**Breaking Change**: No
**Complexity**: Low
**Benefit**: Simplicity, clear mental model

**Approach**: Keep current behavior, improve documentation with workaround patterns

**Recommended**: This option for current release (1.x), consider Option 1 for Atmos 2.0

## Testing

### Test Fixture
Created comprehensive test fixture at `tests/fixtures/scenarios/atmos-template-merge-issue/`:

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
For users experiencing merge errors:

```markdown
## Migrating from Manual Workarounds

If you previously encountered "cannot override two slices with different type" errors,
you can now simplify your configurations:

### Before (Manual Workaround)
You had to avoid YAML functions in base catalogs or use complex template merging.

### After (Fixed)
You can now freely override YAML function values:

\`\`\`yaml
# catalog/base.yaml
vars:
  vpc_list: !template '{{ toJson .settings.vpcs }}'

# stacks/prod.yaml
import:
  - catalog/base

vars:
  vpc_list: ["custom-vpc"]  # Now works!
\`\`\`

Note: This is an OVERRIDE, not a merge. See [workarounds](#) for deep-merge patterns.
```

## Implementation Checklist

- [x] Implement `yamlFunctionTransformer` in `pkg/merge/merge.go`
- [x] Handle all 7 post-merge YAML functions
- [x] Create test fixture for reproduction
- [x] Add comprehensive code documentation
- [x] Verify no breaking changes to existing functionality
- [ ] Update user-facing documentation (YAML functions)
- [ ] Update stack inheritance documentation
- [ ] Add migration guide
- [ ] Add examples to Atmos examples repository
- [ ] Consider blog post announcement

## References

- YAML Functions Documentation: https://atmos.tools/functions/yaml/
- Mergo Library: https://github.com/imdario/mergo
- Related Code: `pkg/merge/merge.go`, `internal/exec/yaml_func_template.go`
- Test Fixture: `tests/fixtures/scenarios/atmos-template-merge-issue/`

## Future Considerations

- Evaluate two-phase merge architecture
- Consider breaking changes for improved YAML function handling
- Performance profiling of merge operations at scale
