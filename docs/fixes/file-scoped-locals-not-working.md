# Issue #1933: File-Scoped Locals Not Working

## Summary

File-scoped locals feature documented in v1.203.0 release notes and blog post was not functional. The feature code
existed but was not integrated into the stack processing pipeline, causing `{{ .locals.* }}` templates to remain
unresolved.

**Status**: ✅ Fixed

## Issue Description

**GitHub Issue**: [#1933](https://github.com/cloudposse/atmos/issues/1933)

**Symptoms**:

1. **Locals templates were not resolved**:
   ```text
   $ atmos describe component myapp -s prod
   vars:
     name: "{{ .locals.name_prefix }}-myapp"
     stage: "{{ .locals.stage }}"
   ```
   Templates showed raw `{{ .locals.* }}` instead of resolved values.

**Example Configuration** (from user's report):

```yaml
# stacks/myapp.yaml
locals:
  stage: prod
  name_prefix: "acme-{{ .locals.stage }}"

components:
  terraform:
    myapp:
      vars:
        name: "{{ .locals.name_prefix }}-myapp"
        stage: "{{ .locals.stage }}"
```

**Expected Behavior**:

```yaml
# After resolution
components:
  terraform:
    myapp:
      vars:
        name: "acme-prod-myapp"
        stage: "prod"
```

## Root Cause Analysis

The file-scoped locals feature was documented in the v1.203.0 blog post (`website/blog/2025-12-16-file-scoped-locals.mdx`)
but the implementation was incomplete:

### What Existed (Before Fix)

| Component | Location | Status |
|-----------|----------|--------|
| Locals resolver with cycle detection | `pkg/locals/resolver.go` | ✅ Complete |
| Stack locals extraction functions | `internal/exec/stack_processor_locals.go` | ✅ Complete |
| Unit tests for locals resolution | `internal/exec/stack_processor_locals_test.go` | ✅ Complete |
| Template AST utilities for `.locals.*` | `pkg/template/ast.go` | ✅ Complete |

### What Was Missing (Before Fix)

| Component | Status |
|-----------|--------|
| Integration of `ProcessStackLocals()` into stack processing pipeline | ❌ Not called |
| `.locals` context in template execution | ❌ Not provided |
| Integration tests with stack fixtures | ❌ Not present |

### Specific Code Gap

In `processYAMLConfigFileWithContextInternal()` (`internal/exec/stack_processor_utils.go`):
- Template processing happened at line ~427 via `ProcessTmpl()`
- The `context` parameter passed to `ProcessTmpl` did NOT include `.locals`
- Locals were never extracted from the raw YAML before template processing

## Solution Implemented

### 1. Added Locals Extraction Before Template Processing

Added `extractLocalsFromRawYAML()` function in `internal/exec/stack_processor_utils.go`:

```go
// extractLocalsFromRawYAML parses raw YAML content and extracts/resolves file-scoped locals.
// This function is called BEFORE template processing to make locals available during template execution.
func extractLocalsFromRawYAML(atmosConfig *schema.AtmosConfiguration, yamlContent string, filePath string) (map[string]any, error) {
    // Parse raw YAML to extract the structure.
    var rawConfig map[string]any
    if err := yaml.Unmarshal([]byte(yamlContent), &rawConfig); err != nil {
        return nil, fmt.Errorf("failed to parse YAML for locals extraction: %w", err)
    }

    // Use ProcessStackLocals which handles all scopes.
    localsCtx, err := ProcessStackLocals(atmosConfig, rawConfig, filePath)
    if err != nil {
        return nil, err
    }

    // Merge all locals into a single flat map for template processing.
    // Only merge section locals if they were explicitly defined.
    resolvedLocals := make(map[string]any)
    for k, v := range localsCtx.Global {
        resolvedLocals[k] = v
    }
    if localsCtx.HasTerraformLocals {
        for k, v := range localsCtx.Terraform {
            resolvedLocals[k] = v
        }
    }
    // ... similar for helmfile and packer sections

    return resolvedLocals, nil
}
```

### 2. Integrated Into Stack Processing Pipeline

In `processYAMLConfigFileWithContextInternal()`, added locals extraction before template processing:

```go
// Extract and resolve file-scoped locals before template processing.
if !skipTemplatesProcessingInImports {
    resolvedLocals, localsErr := extractLocalsFromRawYAML(atmosConfig, stackYamlConfig, filePath)
    if localsErr != nil {
        log.Trace("Failed to extract locals from file", "file", relativeFilePath, "error", localsErr)
        // Non-fatal: continue without locals.
    } else if resolvedLocals != nil && len(resolvedLocals) > 0 {
        // Add resolved locals to the template context.
        if context == nil {
            context = make(map[string]any)
        }
        context["locals"] = resolvedLocals
    }
}
```

### 3. Added Section Override Tracking

During testing, a bug was discovered: when sections don't define their own locals, `ProcessStackLocals` set them
to the same reference as Global. This caused helmfile/packer to overwrite terraform's values during merging.

**Fix**: Added tracking flags to `LocalsContext` in `internal/exec/stack_processor_locals.go`:

```go
type LocalsContext struct {
    Global    map[string]any
    Terraform map[string]any
    Helmfile  map[string]any
    Packer    map[string]any

    // HasTerraformLocals indicates the terraform section has its own locals defined.
    HasTerraformLocals bool

    // HasHelmfileLocals indicates the helmfile section has its own locals defined.
    HasHelmfileLocals bool

    // HasPackerLocals indicates the packer section has its own locals defined.
    HasPackerLocals bool
}
```

These flags are set in `ProcessStackLocals()` when a section explicitly defines a `locals:` key:

```go
if terraformSection, ok := stackConfigMap[cfg.TerraformSectionName].(map[string]any); ok {
    terraformLocals, err := ExtractAndResolveLocals(atmosConfig, terraformSection, ctx.Global, filePath)
    if err != nil {
        return nil, fmt.Errorf("failed to resolve terraform locals: %w", err)
    }
    ctx.Terraform = terraformLocals
    // Check if terraform section has its own locals key.
    if _, hasLocals := terraformSection[cfg.LocalsSectionName]; hasLocals {
        ctx.HasTerraformLocals = true
    }
}
```

### 4. How It Works

1. **Before template processing**: Raw YAML is parsed to extract `locals:` sections
2. **Locals resolution**: `ProcessStackLocals()` resolves locals using the existing resolver which handles:
   - Self-referencing locals (e.g., `name_prefix: "{{ .locals.namespace }}-{{ .locals.environment }}"`)
   - Dependency ordering (topological sort)
   - Cycle detection
3. **Section tracking**: Flags track which sections explicitly define locals
4. **Template context**: Resolved locals are merged (global first, then sections with explicit locals) and added to context
5. **Error handling**: Circular dependencies are logged at TRACE level; processing continues without locals

### 5. Scope Support

Locals are extracted and merged in order of specificity:
1. **Global locals** (root level `locals:` section)
2. **Section-specific locals** (`terraform:`, `helmfile:`, `packer:` sections) - only if explicitly defined

Section locals can override global locals with the same key.

**Note**: Component-level locals are NOT supported in the initial template pass because they require per-component scoping that can't be handled in a single template pass. This is a known limitation for future enhancement.

## Files Changed

| File | Change |
|------|--------|
| `internal/exec/stack_processor_utils.go` | Added `extractLocalsFromRawYAML()` function and integration |
| `internal/exec/stack_processor_locals.go` | Added `HasTerraformLocals`, `HasHelmfileLocals`, `HasPackerLocals` flags to `LocalsContext` |
| `internal/exec/stack_processor_utils_test.go` | Added 16 unit tests for `extractLocalsFromRawYAML` |
| `tests/fixtures/scenarios/locals/stacks/deploy/dev.yaml` | Updated to use global/section-level locals |
| `tests/fixtures/scenarios/locals/stacks/deploy/prod.yaml` | Updated to use global/section-level locals |
| `tests/cli_locals_test.go` | New integration tests |

## Testing

### Test Coverage

| Function | Coverage |
|----------|----------|
| `extractLocalsFromRawYAML` | **95.8%** |
| `ExtractAndResolveLocals` | **100%** |
| `ProcessStackLocals` | **100%** |
| `pkg/locals` (resolver) | **94.7%** |

### Unit Tests

New unit tests in `internal/exec/stack_processor_utils_test.go` (16 tests):

1. `TestExtractLocalsFromRawYAML_Basic` - Basic locals extraction
2. `TestExtractLocalsFromRawYAML_NoLocals` - No locals section
3. `TestExtractLocalsFromRawYAML_EmptyYAML` - Empty YAML content
4. `TestExtractLocalsFromRawYAML_InvalidYAML` - Malformed YAML
5. `TestExtractLocalsFromRawYAML_TerraformSectionLocals` - Terraform section locals
6. `TestExtractLocalsFromRawYAML_HelmfileSectionLocals` - Helmfile section locals
7. `TestExtractLocalsFromRawYAML_PackerSectionLocals` - Packer section locals
8. `TestExtractLocalsFromRawYAML_AllSectionLocals` - All sections with locals
9. `TestExtractLocalsFromRawYAML_CircularDependency` - Circular dependency detection
10. `TestExtractLocalsFromRawYAML_SelfReference` - Self-referencing locals
11. `TestExtractLocalsFromRawYAML_ComplexValue` - Complex values (maps)
12. `TestExtractLocalsFromRawYAML_SectionOverridesGlobal` - Section overrides global
13. `TestExtractLocalsFromRawYAML_TemplateInNonLocalSection` - Template in vars section
14. `TestExtractLocalsFromRawYAML_NilAtmosConfig` - Nil AtmosConfiguration
15. `TestExtractLocalsFromRawYAML_OnlyComments` - YAML with only comments
16. `TestExtractLocalsFromRawYAML_EmptyLocals` - Empty locals section

### Integration Tests

New tests in `tests/cli_locals_test.go` (4 tests):
- `TestLocalsResolutionDev` - Verifies locals resolution in dev environment
- `TestLocalsResolutionProd` - Verifies locals resolution in prod environment
- `TestLocalsDescribeStacks` - Verifies describe stacks works with locals
- `TestLocalsCircularDependency` - Verifies circular dependencies are detected gracefully

### Manual Testing

```bash
cd tests/fixtures/scenarios/locals
../../../../build/atmos describe stacks --format yaml
```

Output shows resolved locals:
```yaml
dev-us-east-1:
  components:
    terraform:
      mock/instance-1:
        backend:
          bucket: acme-dev-tfstate  # Resolved from {{ .locals.backend_bucket }}
        vars:
          app_name: acme-dev-mock-instance-1  # Resolved from {{ .locals.name_prefix }}-mock-instance-1
          bar: dev  # Resolved from {{ .locals.environment }}
```

## Bug Found During Testing

### Section Override Bug

**Problem**: When a section (terraform/helmfile/packer) doesn't define its own locals, `ProcessStackLocals` was
setting that section's locals to the same reference as Global. During merging in `extractLocalsFromRawYAML`,
helmfile/packer would overwrite terraform's values with global values because they were merged after terraform.

**Example**:
```yaml
locals:
  namespace: "global-acme"
terraform:
  locals:
    namespace: "terraform-acme"
# helmfile/packer have no locals section
```

Before fix: Result was `namespace: "global-acme"` (wrong - helmfile/packer overwrote terraform)
After fix: Result is `namespace: "terraform-acme"` (correct - only sections with explicit locals are merged)

**Solution**: Added `HasTerraformLocals`, `HasHelmfileLocals`, `HasPackerLocals` boolean flags to track which
sections explicitly define their own locals. Only merge section locals when the corresponding flag is true.

## Future Enhancements

1. **`atmos describe locals` command** - Not implemented in this fix; could be added for debugging
2. **Component-level locals** - Requires per-component template scoping for proper isolation

## References

- [File-Scoped Locals Blog Post](https://atmos.tools/changelog/file-scoped-locals/)
- [GitHub Issue #1933](https://github.com/cloudposse/atmos/issues/1933)
- [Atmos v1.203.0 Release Notes](https://github.com/cloudposse/atmos/releases/tag/v1.203.0)
