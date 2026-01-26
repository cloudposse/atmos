# Describe Affected / atmos.Component Path Resolution Issue

**Date:** 2026-01-25

## Summary

This document describes an issue affecting `describe affected`, `describe stacks`, and `atmos.Component` template
function in Atmos v1.195.0+. The issue causes errors when Atmos is run from a non-project-root directory or when
using the `--chdir` flag.

| Issue                                     | Status   | Description                                                       |
|-------------------------------------------|----------|-------------------------------------------------------------------|
| `backend.tf.json: no such file or directory` | ✅ Fixed | `atmos.Component` template function fails with path error         |
| Relative `component_path` resolution      | ✅ Fixed | `buildComponentInfo` returns relative path, consumers expect absolute |
| `--chdir` flag interaction                | ✅ Fixed | Issue exposed by working directory changes in v1.195.0            |

---

## User-Reported Issue

### Error Message

```console
template: describe-stacks-all-sections:72:21 ... at <atmos.Component>: error calling Component: open ... backend.tf.json: no such file or directory
```

### Observed Behavior

- The workflow fails when using `atmos.Component` template function in stack configurations.
- The same SHAs and workflow succeed on older versions (v1.180.0, v1.188.0).
- Reverting to v1.180.0 or v1.188.0 resolves the issue.
- Commands affected: `atmos describe stacks`, `atmos describe affected`, any template processing using `atmos.Component`.

### Environment

- Atmos v1.195.0 (regression introduced)
- Working versions: v1.180.0, v1.188.0

---

## Root Cause Analysis

### The Bug: Path Resolution Mismatch

The `atmos.Component` template function fails because of a **path resolution mismatch** between how paths are stored
and how they are used:

#### 1. Path Generation (describe_stacks.go)

`buildComponentInfo()` in `internal/exec/describe_stacks.go` (line 1076) returns `component_path` as a **relative path**:

```go
// buildComponentInfo constructs the component_info map with component_path for a component.
// The component_path is returned as a relative path from the project root using forward slashes.
func buildComponentInfo(atmosConfig *schema.AtmosConfiguration, componentSection map[string]any, componentKind string) map[string]any {
    // ...
    basePath := getComponentBasePath(atmosConfig, componentKind)  // e.g., "components/terraform"
    // ...
    relativePath := filepath.ToSlash(filepath.Clean(filepath.Join(parts...)))
    componentInfo[cfg.ComponentPathSectionName] = relativePath  // e.g., "components/terraform/vpc"
}
```

#### 2. Path Consumption (pkg/terraform/output/backend.go)

The `defaultBackendGenerator.GenerateBackendIfNeeded()` uses this path directly:

```go
func (g *defaultBackendGenerator) GenerateBackendIfNeeded(config *ComponentConfig, ...) error {
    // ...
    backendFileName := filepath.Join(config.ComponentPath, "backend.tf.json")  // Uses relative path!
    // ...
    if err := u.WriteToFileAsJSON(backendFileName, backendConfig, filePermission); err != nil {
        return err  // ERROR: "no such file or directory"
    }
}
```

#### 3. The Problem

When the relative path `components/terraform/vpc/backend.tf.json` is used:
- It resolves relative to the **current working directory (CWD)**
- NOT relative to the project root (`atmosConfig.BasePath`)
- If CWD differs from project root (e.g., `--chdir`, running from subdirectory, CI environment), the path doesn't exist

### Flow Diagram

```
User Template: {{ atmos.Component "vpc" "dev-ue1" }}
        ↓
componentFunc() [template_funcs_component.go]
        ↓
ExecuteDescribeComponent() → sections["component_info"]["component_path"] = "components/terraform/vpc" (RELATIVE)
        ↓
tfoutput.ExecuteWithSections()
        ↓
ExtractComponentConfig() → config.ComponentPath = "components/terraform/vpc" (RELATIVE)
        ↓
GenerateBackendIfNeeded() → filepath.Join("components/terraform/vpc", "backend.tf.json")
        ↓
WriteToFileAsJSON() → open("components/terraform/vpc/backend.tf.json")
        ↓
ERROR: "no such file or directory" (CWD is not project root!)
```

---

## Why This Affects v1.195.0

### Changes Between v1.180.0 and v1.195.0

The following changes may have exposed this latent bug:

| PR/Commit | Description | Impact |
|-----------|-------------|--------|
| #1644 | `Add global --chdir flag for changing working directory` | Changed how working directory is handled before command execution |
| #1639 | `Atmos Performance Optimization` | Added caching that may preserve incorrect paths |

The `--chdir` flag (#1644) processes before all other operations including configuration loading. This change may have
exposed the path resolution bug that was previously masked by consistent CWD state.

---

## Code Analysis

### Where the Path Should Be Absolute

In contrast to `buildComponentInfo`, other parts of the codebase correctly use absolute paths:

**ExecuteTerraform (internal/exec/terraform.go:215):**
```go
componentPath, err := u.GetComponentPath(&atmosConfig, "terraform", info.ComponentFolderPrefix, info.FinalComponent)
```

**GetComponentPath (pkg/utils/component_path_utils.go:255):**
```go
func GetComponentPath(...) (string, error) {
    // ...
    // Ensure base path is absolute.
    if !filepath.IsAbs(basePath) {
        absPath, err := filepath.Abs(basePath)
        if err != nil {
            return "", err
        }
        basePath = absPath
    }
    // ...
    return cleanPath, nil  // Returns ABSOLUTE path
}
```

### Pattern Throughout Codebase

The consistent pattern for path construction is:
```go
filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Terraform.BasePath, component)
```

This is used in:
- `internal/exec/terraform.go`
- `internal/exec/helmfile.go`
- `internal/exec/packer.go`
- `internal/exec/stack_utils.go`
- `internal/exec/describe_affected_utils_2.go`
- `internal/exec/vendor_component_utils.go`

---

## Proposed Fix Options

### Option 1: Fix at Source (buildComponentInfo)

Change `buildComponentInfo` to return an absolute path:

```go
func buildComponentInfo(atmosConfig *schema.AtmosConfiguration, componentSection map[string]any, componentKind string) map[string]any {
    // ...
    basePath := getComponentBasePath(atmosConfig, componentKind)
    if basePath == "" {
        return componentInfo
    }

    // Build path parts, filtering empty strings.
    parts := []string{atmosConfig.BasePath, basePath}  // Prepend atmosConfig.BasePath
    if componentFolderPrefix != "" {
        parts = append(parts, componentFolderPrefix)
    }
    parts = append(parts, finalComponent)

    // Return absolute path.
    absPath := filepath.Clean(filepath.Join(parts...))
    componentInfo[cfg.ComponentPathSectionName] = absPath  // Now absolute

    return componentInfo
}
```

**Pros:**
- Fixes all consumers at once
- Consistent with other path handling in codebase

**Cons:**
- May break consumers expecting relative paths
- Changes behavior for all `component_info.component_path` users

### Option 2: Fix at Consumer (pkg/terraform/output)

Resolve the relative path against `atmosConfig.BasePath` before use:

```go
func (e *Executor) execute(ctx context.Context, atmosConfig *schema.AtmosConfiguration, ...) (map[string]any, error) {
    // ...
    config, err := ExtractComponentConfig(sections, component, stack, ...)
    if err != nil {
        return nil, err
    }

    // Resolve relative component path against atmosConfig.BasePath
    if !filepath.IsAbs(config.ComponentPath) {
        config.ComponentPath = filepath.Join(atmosConfig.BasePath, config.ComponentPath)
    }

    // Now safe to use config.ComponentPath
    // ...
}
```

**Pros:**
- Minimal change, localized to affected code
- Doesn't change `component_path` format for other consumers

**Cons:**
- Need to apply fix in multiple places that use `component_path`

### Option 3: Use GetComponentPath Utility

Use the existing `utils.GetComponentPath()` function instead of reading from sections:

```go
func (e *Executor) execute(ctx context.Context, atmosConfig *schema.AtmosConfiguration, ...) (map[string]any, error) {
    // Get component name and folder prefix from sections
    componentName := extractComponentName(sections)
    folderPrefix := extractFolderPrefix(sections)
    componentType := extractComponentType(sections)

    // Use existing utility for consistent path resolution
    componentPath, err := u.GetComponentPath(atmosConfig, componentType, folderPrefix, componentName)
    if err != nil {
        return nil, err
    }

    config.ComponentPath = componentPath
    // ...
}
```

**Pros:**
- Reuses battle-tested utility function
- Guarantees absolute path

**Cons:**
- More invasive change
- Requires extracting component info from sections

---

## Implemented Fix

**Option 2 (Fix at Consumer)** was implemented because:

1. It's a minimal, targeted fix
2. It doesn't change the `component_path` format that other consumers may rely on
3. It can be done without affecting the describe output format

### Changes Made

**File: `pkg/terraform/output/executor.go`**

Added path resolution after `ExtractComponentConfig()` in the `execute()` function:

```go
// Step 2.5: Resolve relative component path against atmosConfig.BasePath.
// The component_path from sections may be relative (e.g., "components/terraform/vpc").
// When running with --chdir or from a non-project-root directory, we need to
// resolve this path against the configured base path to ensure file operations
// (backend generation, terraform init) work correctly.
if config.ComponentPath != "" && !filepath.IsAbs(config.ComponentPath) {
    config.ComponentPath = filepath.Join(atmosConfig.BasePath, config.ComponentPath)
    log.Debug("Resolved relative component path", "component", component, "stack", stack, "path", config.ComponentPath)
}
```

This ensures that when `ComponentPath` is relative (e.g., `components/terraform/vpc`), it's resolved against
`atmosConfig.BasePath` before being used for file operations like writing `backend.tf.json`.

---

## User Workarounds

Until a fix is released, users can try:

### Workaround 1: Pin to Older Version

```bash
atmos version install 1.188.0
atmos version use 1.188.0
```

### Workaround 2: Avoid --chdir Flag

Run Atmos from the project root directory instead of using `--chdir`:

```bash
# Instead of:
atmos --chdir /path/to/project describe stacks

# Use:
cd /path/to/project && atmos describe stacks
```

### Workaround 3: Use !terraform.state Instead of atmos.Component

Replace `atmos.Component` template function with `!terraform.state` YAML function:

```yaml
# Before (template function):
vpc_id: "{{ (atmos.Component \"vpc\" .stack).outputs.vpc_id }}"

# After (YAML function - recommended):
vpc_id: !terraform.state vpc vpc_id
```

The `!terraform.state` YAML function is:
- Faster (10-100x) as it reads from state file directly
- More reliable path handling
- Recommended best practice

---

## Testing

### Reproduce the Issue

```bash
# From a subdirectory (not project root)
cd /path/to/project/subdirectory
atmos --chdir /path/to/project describe stacks

# Or with template function
# Stack file using: {{ atmos.Component "vpc" .stack }}
atmos describe stacks -s dev-ue1
```

### Verify the Fix

```bash
# After fix, both should work:
cd /path/to/project && atmos describe stacks
atmos --chdir /path/to/project describe stacks

# Template functions should resolve correctly
atmos describe component vpc -s dev-ue1
```

---

## Related Issues

- [#1644](https://github.com/cloudposse/atmos/pull/1644) - Add global --chdir flag for changing working directory
- [#1639](https://github.com/cloudposse/atmos/pull/1639) - Atmos Performance Optimization

---

## Files Modified

| File | Change |
|------|--------|
| `pkg/terraform/output/executor.go` | ✅ Added path resolution step to resolve relative `ComponentPath` against `atmosConfig.BasePath` |

---

## Verification Commands

```bash
# Run tests after fix
go test ./pkg/terraform/output/... -v
go test ./internal/exec/... -v -run "Test.*Describe"

# Integration test
cd tests/fixtures/scenarios/atmos-describe-affected
atmos describe affected --verbose
atmos describe stacks
```
