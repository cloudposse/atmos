# Performance Tracking Annotations Summary

This document lists all the functions that have been annotated with `perf.Track()` for performance heatmap tracking in the `atmos describe stacks` execution path.

## Overview

All annotations follow the pattern:
```go
func FunctionName(...) (...) {
    defer perf.Track("FunctionName")()
    // ... function body
}
```

## Annotated Functions by File

### 1. cmd/describe_stacks.go

**Function**: `ExecuteDescribeStacks`
- **Line**: 120
- **Purpose**: Main entry point for `atmos describe stacks` command
- **What it tracks**: Overall command execution time from CLI entry to final output

```go
func ExecuteDescribeStacks(
    atmosConfig *schema.AtmosConfiguration,
    filterByStack string,
    components []string,
    componentTypes []string,
    sections []string,
    ignoreMissingFiles bool,
    processTemplates bool,
    processYamlFunctions bool,
    includeEmptyStacks bool,
    skip []string,
) (map[string]any, error) {
    defer perf.Track("ExecuteDescribeStacks")()
    // ...
}
```

---

### 2. internal/exec/utils.go

**Function**: `FindStacksMap`
- **Line**: 161
- **Purpose**: Discovers and processes all stack configuration files
- **What it tracks**: Time to find and load all stack manifests

```go
func FindStacksMap(
    atmosConfig *schema.AtmosConfiguration,
    ignoreMissingFiles bool,
) (map[string]any, map[string]map[string]any, error) {
    defer perf.Track("FindStacksMap")()
    // ...
}
```

**Function**: `ProcessStacks`
- **Line**: 191
- **Purpose**: Processes stack configuration for a specific component and stack
- **What it tracks**: Time to validate inputs and process component configuration

```go
func ProcessStacks(
    atmosConfig *schema.AtmosConfiguration,
    configAndStacksInfo schema.ConfigAndStacksInfo,
    checkStack bool,
    processTemplates bool,
    processYamlFunctions bool,
    skip []string,
) (schema.ConfigAndStacksInfo, error) {
    defer perf.Track("ProcessStacks")()
    // ...
}
```

---

### 3. internal/exec/stack_processor_utils.go

**Function**: `ProcessYAMLConfigFiles`
- **Line**: 54
- **Purpose**: Processes all YAML stack configuration files in parallel
- **What it tracks**: Time to load and parse all YAML files, including imports

```go
func ProcessYAMLConfigFiles(
    atmosConfig *schema.AtmosConfiguration,
    stacksBasePath string,
    terraformComponentsBasePath string,
    helmfileComponentsBasePath string,
    packerComponentsBasePath string,
    filePaths []string,
    processStackDeps bool,
    processComponentDeps bool,
    ignoreMissingFiles bool,
) (
    []string,
    map[string]any,
    map[string]map[string]any,
    error,
) {
    defer perf.Track("ProcessYAMLConfigFiles")()
    // ...
}
```

**Function**: `ProcessStackConfig`
- **Line**: 668
- **Purpose**: Processes a single stack manifest with deep-merging
- **What it tracks**: Time to process one stack file including all imports, inheritance, and component processing

```go
func ProcessStackConfig(
    atmosConfig *schema.AtmosConfiguration,
    stacksBasePath string,
    terraformComponentsBasePath string,
    helmfileComponentsBasePath string,
    packerComponentsBasePath string,
    stack string,
    config map[string]any,
    processStackDeps bool,
    processComponentDeps bool,
    componentTypeFilter string,
    componentStackMap map[string]map[string][]string,
    importsConfig map[string]map[string]any,
    checkBaseComponentExists bool,
) (map[string]any, error) {
    defer perf.Track("ProcessStackConfig")()
    // ...
}
```

**Function**: `FindComponentsDerivedFromBaseComponents`
- **Line**: 2638
- **Purpose**: Finds all components that inherit from base components
- **What it tracks**: Time to traverse component hierarchy and build inheritance tree

```go
func FindComponentsDerivedFromBaseComponents(
    stack string,
    allComponents map[string]any,
    baseComponents []string,
) ([]string, error) {
    defer perf.Track("FindComponentsDerivedFromBaseComponents")()
    // ...
}
```

---

### 4. internal/exec/template_utils.go

**Function**: `ProcessTmplWithDatasources`
- **Line**: 78
- **Purpose**: Processes Go templates with datasources (files, HTTP endpoints)
- **What it tracks**: Time to load datasources and render templates with context

```go
func ProcessTmplWithDatasources(
    atmosConfig *schema.AtmosConfiguration,
    configAndStacksInfo *schema.ConfigAndStacksInfo,
    settingsSection schema.Settings,
    tmplName string,
    tmplValue string,
    tmplData any,
    ignoreMissingTemplateValues bool,
) (string, error) {
    defer perf.Track("ProcessTmplWithDatasources")()
    // ...
}
```

**Function**: `ProcessTmplWithDatasourcesGomplate`
- **Line**: 352
- **Purpose**: Processes templates using Gomplate engine
- **What it tracks**: Time to execute Gomplate-specific template processing

```go
func ProcessTmplWithDatasourcesGomplate(
    tmplName string,
    tmplValue string,
    mergedData map[string]interface{},
    ignoreMissingTemplateValues bool,
) (string, error) {
    defer perf.Track("ProcessTmplWithDatasourcesGomplate")()
    // ...
}
```

---

### 5. internal/exec/yaml_func_utils.go

**Function**: `ProcessCustomYamlTags`
- **Line**: 19
- **Purpose**: Processes Atmos YAML functions like `atmos.Component()`, `atmos.Stack()`
- **What it tracks**: Time to evaluate all YAML functions in stack configurations

```go
func ProcessCustomYamlTags(
    atmosConfig *schema.AtmosConfiguration,
    input schema.AtmosSectionMapType,
    currentStack string,
    skip []string,
) (schema.AtmosSectionMapType, error) {
    defer perf.Track("ProcessCustomYamlTags")()
    // ...
}
```

---

### 6. pkg/merge/merge.go

**Function**: `MergeWithOptions`
- **Line**: 27
- **Purpose**: Deep-merges configuration maps with specified merge strategy
- **What it tracks**: Time spent in low-level merge operations

```go
func MergeWithOptions(
    inputs []map[string]any,
    appendSlice bool,
    sliceDeepCopy bool,
) (map[string]any, error) {
    defer perf.Track("MergeWithOptions")()
    // ...
}
```

**Function**: `Merge`
- **Line**: 81
- **Purpose**: Merges configurations using list merge strategy from atmos.yaml
- **What it tracks**: Time to merge with strategy determination

```go
func Merge(
    atmosConfig *schema.AtmosConfiguration,
    inputs []map[string]any,
) (map[string]any, error) {
    defer perf.Track("Merge")()
    // ...
}
```

**Function**: `MergeWithContext`
- **Line**: 122
- **Purpose**: Merges with file context tracking for error messages
- **What it tracks**: Time to merge with enhanced error context

```go
func MergeWithContext(
    atmosConfig *schema.AtmosConfiguration,
    inputs []map[string]any,
    context *MergeContext,
) (map[string]any, error) {
    defer perf.Track("MergeWithContext")()
    // ...
}
```

**Function**: `MergeWithOptionsAndContext`
- **Line**: 173
- **Purpose**: Combines options and context for comprehensive merge tracking
- **What it tracks**: Time to merge with both options and context

```go
func MergeWithOptionsAndContext(
    inputs []map[string]any,
    appendSlice bool,
    sliceDeepCopy bool,
    context *MergeContext,
) (map[string]any, error) {
    defer perf.Track("MergeWithOptionsAndContext")()
    // ...
}
```

---

## Execution Flow

When running `atmos describe stacks`, the functions are typically called in this order:

1. **ExecuteDescribeStacks** - Entry point from CLI
2. **FindStacksMap** - Discovers all stack files
3. **ProcessYAMLConfigFiles** - Loads and parses YAML files (parallel)
   - For each file:
     - **ProcessStackConfig** - Processes one stack file
       - **ProcessTmplWithDatasources** / **ProcessTmplWithDatasourcesGomplate** - Template rendering
       - **ProcessCustomYamlTags** - YAML function evaluation
       - **Merge** / **MergeWithContext** / **MergeWithOptions** / **MergeWithOptionsAndContext** - Deep merging at all scopes
       - **FindComponentsDerivedFromBaseComponents** - Component inheritance
4. **ProcessStacks** - Final component validation

## Performance Metrics Collected

For each annotated function, the following metrics are tracked with **microsecond precision**:

- **Count**: Number of times the function was called
- **Total**: Total execution time across all calls (displayed with microsecond precision)
- **Avg**: Average execution time per call (Total ÷ Count, displayed with microsecond precision)
- **Max**: Maximum execution time for a single call (displayed with microsecond precision)
- **P95**: 95th percentile latency (when `--heatmap-hdr` flag is used, displayed with microsecond precision)

All durations are displayed in Go's duration format (e.g., `123.456µs`, `12.345ms`, `1.234s`) with microsecond-level granularity to capture fast function executions accurately.

## Usage

View the performance heatmap after running any command:

```bash
# Basic heatmap
atmos describe stacks --heatmap

# With P95 percentile tracking
atmos describe stacks --heatmap --heatmap-hdr

# With specific visualization mode
atmos describe stacks --heatmap --heatmap-mode=table
```

## Expected Hotspots

Based on the annotation coverage, you should expect to see these functions as potential hotspots:

1. **ProcessYAMLConfigFiles** - Will show high total time due to I/O operations
2. **ProcessStackConfig** - Called for each stack file, will show high count
3. **Merge functions** - Called extensively for hierarchical configuration, will show high count
4. **ProcessTmplWithDatasources** - May show high avg/max time if templates load external data
5. **ProcessCustomYamlTags** - Time depends on number of YAML functions used

## Files Modified

Summary of files that required import additions:

- `internal/exec/describe_stacks.go` - Added `perf` import
- `internal/exec/utils.go` - Added `perf` import
- `internal/exec/stack_processor_utils.go` - Added `perf` import
- `internal/exec/template_utils.go` - Added `perf` import
- `internal/exec/yaml_func_utils.go` - Added `perf` import
- `pkg/merge/merge.go` - Added `perf` import

Total annotations: **13 functions** across **6 files**
