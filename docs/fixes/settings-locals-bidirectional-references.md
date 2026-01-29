# Fix: Settings can't refer to locals anymore (1.205 regression)

**Date**: 2025-01-28

**GitHub Issue**: [#2032](https://github.com/cloudposse/atmos/issues/2032)

## Problem

After PR #1994, `locals` could reference `settings` but `settings` could no longer reference `locals`.
This was a regression from version 1.204 where `settings` could reference `locals` but `locals` couldn't reference `settings`.

### Example

```yaml
# With atmos 1.205.0, this configuration had issues:
locals:
  stage: dev
  stage_from_setting: "{{ .settings.context.stage }}"  # Works in 1.205

settings:
  context:
    stage: dev
    stage_from_local: "{{ .locals.stage }}"  # BROKEN in 1.205, worked in 1.204

vars:
  stage: dev
  setting_referring_to_local: "{{ .settings.context.stage_from_local }}"  # Got "{{ .locals.stage }}" instead of "dev"
  local_referring_to_setting: "{{ .locals.stage_from_setting }}"          # Works in 1.205
```

**Expected behavior**: Bidirectional references between settings and locals should work:
- Settings can refer to locals, vars, and env
- Locals can refer to settings, vars, and env
- Vars can refer to settings, locals, and env

**Actual behavior (before fix)**: Settings templates that referenced locals remained unresolved, causing vars that referenced those settings to get the raw template string instead of the resolved value.

## Root Cause

The template processing order was:
1. Parse raw YAML
2. Resolve locals (with access to raw settings/vars/env)
3. Add raw settings/vars/env to context
4. Process templates in full manifest

The problem was that settings were added to the context with their raw template values (like `{{ .locals.stage }}`), not resolved values. When vars later tried to access `{{ .settings.context.stage_from_local }}`, they got the raw template string instead of the resolved value.

Go templates don't recursively process template strings that exist in data values, so the nested template was never expanded.

## Solution

Modified `extractAndAddLocalsToContext()` in `internal/exec/stack_processor_utils.go` to process templates in settings, vars, and env sections AFTER locals are resolved:

1. First, resolve locals (which can reference raw settings/vars/env)
2. Add resolved locals to context
3. Process templates in settings using the resolved locals context
4. Process templates in vars using the resolved locals AND processed settings context
5. Process templates in env using the resolved locals, processed settings, AND processed vars context
6. Add all processed sections to the final context

This ensures that:
- Locals can reference settings (resolved during locals processing with raw settings)
- Settings can reference locals (resolved by new template processing step)
- Vars can reference both locals and processed settings
- Env can reference locals, processed settings, and processed vars

### New Helper Function

Added `processTemplatesInSection()` that:
- Converts a section to YAML
- Checks if it contains template markers (`{{`)
- Processes templates using the provided context
- Parses the result back to a map

## Files Changed

- `internal/exec/stack_processor_utils.go`: Modified `extractAndAddLocalsToContext()` and added `processTemplatesInSection()` helper
- `internal/exec/stack_processor_utils_test.go`: Added `TestExtractAndAddLocalsToContext_BidirectionalReferences`

## Testing

Added `TestExtractAndAddLocalsToContext_BidirectionalReferences` with test cases for:
- Settings referencing locals
- Vars referencing settings that reference locals
- Full bidirectional references (the exact scenario from the issue)

## Usage

After the fix, bidirectional references work correctly:

```yaml
locals:
  stage: dev
  stage_from_setting: "{{ .settings.context.stage }}"  # Works: "dev"

settings:
  context:
    stage: dev
    stage_from_local: "{{ .locals.stage }}"  # Now works: "dev"

vars:
  stage: dev
  setting_referring_to_local: "{{ .settings.context.stage_from_local }}"  # Now works: "dev"
  local_referring_to_setting: "{{ .locals.stage_from_setting }}"          # Works: "dev"
```

---

# Fix: Stack-Level vs Component-Level Locals Handling

**Date**: 2025-01-28

**Related to**: GitHub Issue [#2032](https://github.com/cloudposse/atmos/issues/2032)

## Problem

After implementing bidirectional references, a new issue emerged:

1. **Stack-level locals appearing in final output**: Locals defined at the stack level (for template resolution) were incorrectly appearing in the final component output, polluting the configuration.

2. **Component-level locals being removed**: Locals explicitly defined within a component section (user-intentional) were being removed from output.

### Example of Stack-Level vs Component-Level Locals

```yaml
# Stack file: stacks/deploy/dev.yaml

# STACK-LEVEL LOCALS - For template resolution only, should NOT appear in output
locals:
  stage: dev
  environment: development
  name_prefix: "myapp-{{ .locals.stage }}"

# Component sections
components:
  terraform:
    vpc:
      vars:
        vpc_name: "{{ .locals.name_prefix }}-vpc"  # Uses stack-level locals
      # NO component-level locals defined

    database:
      # COMPONENT-LEVEL LOCALS - User-intentional, SHOULD appear in output
      locals:
        db_engine: postgres
        db_version: "15"
        instance_class: "{{ .vars.environment_size }}"  # Template that needs resolution
      vars:
        environment_size: small
```

**Expected behavior**:
- `vpc` component output: No `locals` section (only used stack-level locals for resolution)
- `database` component output: Has `locals` section with `db_engine`, `db_version`, `instance_class` (resolved)

**Actual behavior (before fix)**:
- Both components had all locals (stack-level + component-level) in output
- Or: Neither component had any locals in output

## Root Causes

### Root Cause 1: No Distinction Between Locals Types

The original implementation treated all locals the same. Stack-level locals were merged with component-level locals for template processing, but after processing, all locals were either kept or removed - there was no way to distinguish which should be preserved.

### Root Cause 2: Multi-Pass Processing State Pollution

`ProcessComponentConfig` in `internal/exec/utils.go` is called multiple times for the same component:
1. First call: `componentSection` has NO locals → stack locals are merged in
2. Second call: `componentSection` NOW has locals (from first merge) → incorrectly treated as component-level

The problem was that `componentSection` directly references the map in `stacksMap`. Modifications to `componentSection` persisted in `stacksMap` and affected subsequent calls.

```
┌─────────────────────────────────────────────────────────────────────┐
│ First Call to ProcessComponentConfig                                │
│ ┌─────────────────────┐     ┌─────────────────────────────────────┐ │
│ │ stacksMap["vpc"]    │ ──► │ componentSection (no locals)        │ │
│ │ (no locals)         │     │ Merge stack locals → now has locals │ │
│ └─────────────────────┘     └─────────────────────────────────────┘ │
│                                        │                            │
│                                        ▼                            │
│                              MODIFIES stacksMap!                    │
└─────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────┐
│ Second Call to ProcessComponentConfig                               │
│ ┌─────────────────────┐     ┌─────────────────────────────────────┐ │
│ │ stacksMap["vpc"]    │ ──► │ componentSection (has locals from   │ │
│ │ (now HAS locals!)   │     │ first merge - incorrectly seen as   │ │
│ └─────────────────────┘     │ component-level locals)             │ │
│                             └─────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────┘
```

## Solution

### Part 1: Track Original Component-Level Local Keys

Before merging stack-level locals with component-level locals, save only the KEYS of component-level locals (not the values, since they may contain templates that need resolution):

```go
// Track original component-level locals before merging
var componentLocalKeys map[string]bool
if existingLocals, ok := componentSection[cfg.LocalsSectionName].(map[string]any); ok && len(existingLocals) > 0 {
    componentLocalKeys = make(map[string]bool, len(existingLocals))
    for k := range existingLocals {
        componentLocalKeys[k] = true
    }
}
```

### Part 2: Make Shallow Copy to Prevent State Pollution

Create a shallow copy of `componentSection` before modifying it. This prevents changes from persisting in `stacksMap`:

```go
// Make a shallow copy of componentSection to avoid modifying the original stacksMap
componentSectionCopy := make(map[string]any, len(componentSection))
for k, v := range componentSection {
    componentSectionCopy[k] = v
}
componentSection = componentSectionCopy
```

### Part 3: Guard Against Multiple Passes

Only perform the merging on the first pass by checking if `OriginalComponentLocals` has been set:

```go
if configAndStacksInfo.OriginalComponentLocals == nil {
    // First pass - do the merging
    // ...
}
```

### Part 4: Filter Locals After Template Processing

After templates are processed, filter the locals to keep only component-level ones (with their resolved values):

```go
// Filter locals to keep only component-level locals (with resolved values)
if len(configAndStacksInfo.OriginalComponentLocals) > 0 {
    if resolvedLocals, ok := configAndStacksInfo.ComponentSection[cfg.LocalsSectionName].(map[string]any); ok {
        filteredLocals := make(map[string]any)
        for k := range configAndStacksInfo.OriginalComponentLocals {
            if v, exists := resolvedLocals[k]; exists {
                filteredLocals[k] = v
            }
        }
        if len(filteredLocals) > 0 {
            configAndStacksInfo.ComponentSection[cfg.LocalsSectionName] = filteredLocals
        } else {
            delete(configAndStacksInfo.ComponentSection, cfg.LocalsSectionName)
        }
    }
} else {
    // No original component locals - remove the merged locals entirely
    delete(configAndStacksInfo.ComponentSection, cfg.LocalsSectionName)
}
```

## Files Changed

- **`internal/exec/utils.go`**:
  - Added shallow copy of `componentSection` before modification
  - Added tracking of original component-level local keys
  - Added filtering logic in `postProcessTemplatesAndYamlFunctions`
  - Added guard against multiple processing passes

- **`internal/exec/describe_stacks.go`**:
  - Added initialization of `OriginalComponentLocals` in `ConfigAndStacksInfo` for terraform, helmfile, and packer component sections
  - Removed deletion of locals that was causing component-level locals to be lost

- **`pkg/schema/schema.go`**:
  - Added `OriginalComponentLocals` field to `ConfigAndStacksInfo` struct

## Testing

### Test Cases Added

1. **`TestLocalsNotInFinalOutput`**: Verifies stack-level locals don't appear in final component output
2. **`TestComponentLevelLocals`**: Verifies component-level locals ARE preserved with resolved values:
   - `standalone_component_with_component-level_locals`
   - `component_inheriting_with_locals_override`
   - `component_inheriting_without_locals_override`
   - `component_with_component_attribute`

### Test Fixtures

- `tests/fixtures/scenarios/locals/stacks/deploy/dev.yaml` - Stack-level locals tests
- `tests/fixtures/scenarios/locals-component-level/stacks/deploy/dev.yaml` - Component-level locals tests

## Usage After Fix

```yaml
# Stack-level locals (for template resolution only)
locals:
  stage: dev
  name_prefix: "myapp-{{ .locals.stage }}"

components:
  terraform:
    # Component WITHOUT component-level locals
    vpc:
      vars:
        name: "{{ .locals.name_prefix }}-vpc"  # Resolves to "myapp-dev-vpc"
      # Output: No 'locals' section

    # Component WITH component-level locals
    database:
      locals:
        engine: postgres
        full_name: "{{ .locals.name_prefix }}-db"  # Resolves using stack-level locals
      vars:
        db_name: "{{ .locals.engine }}-primary"
      # Output: Has 'locals' section with {engine: "postgres", full_name: "myapp-dev-db"}
```

## Key Insights

1. **Save keys, not values**: Component-level locals may contain templates that need resolution. Save only the keys to identify which locals to preserve, then use the resolved values after processing.

2. **Go maps are references**: Modifying a map passed to a function modifies the original. Always make a copy if you need to modify without side effects.

3. **Multi-pass processing**: Be aware that functions may be called multiple times. Use state markers (like `OriginalComponentLocals == nil`) to ensure idempotent behavior.

4. **Template resolution order**: Stack-level locals are merged with component-level locals BEFORE template processing, ensuring templates in component-level locals can reference stack-level locals.

---

# Fix: Locals Merging on Every ProcessComponentConfig Call

**Date**: 2025-01-28

**Related to**: GitHub Issue [#2032](https://github.com/cloudposse/atmos/issues/2032)

## Problem

After implementing the shallow copy fix, a new issue emerged where locals were still not being resolved correctly in the `describe component` path. Template expressions like `{{ .locals.namespace }}` were returning `<no value>`.

### Root Cause

The `ProcessComponentConfig` function in `internal/exec/utils.go` is called multiple times during stack processing via `findComponentInStacks`. The original fix only performed locals merging on the FIRST pass (guarded by `if OriginalComponentLocals == nil`), but subsequent passes still needed the merged locals to be present for template resolution.

```
First call:  OriginalComponentLocals == nil → merges stack locals into component ✓
Second call: OriginalComponentLocals != nil → skips merging entirely ✗ (no locals available for templates!)
```

## Solution

The key insight is that there are TWO separate concerns that were incorrectly bundled together:

1. **Tracking original component-level local keys** (should happen ONCE on first pass)
2. **Merging stack locals with component locals** (must happen on EVERY pass)

### Code Change

Moved the locals merging code OUTSIDE the `if OriginalComponentLocals == nil` block:

```go
// Track original component-level locals only once (first pass).
if configAndStacksInfo.OriginalComponentLocals == nil {
    var componentLocalKeys map[string]bool
    if existingLocals, ok := componentSection[cfg.LocalsSectionName].(map[string]any); ok && len(existingLocals) > 0 {
        componentLocalKeys = make(map[string]bool, len(existingLocals))
        for k := range existingLocals {
            componentLocalKeys[k] = true
        }
    }
    configAndStacksInfo.OriginalComponentLocals = make(map[string]any)
    for k := range componentLocalKeys {
        configAndStacksInfo.OriginalComponentLocals[k] = true
    }
}

// Make a shallow copy of componentSection to avoid modifying the original stacksMap.
// This MUST happen on every call, not just the first pass.
componentSectionCopy := make(map[string]any, len(componentSection))
for k, v := range componentSection {
    componentSectionCopy[k] = v
}
componentSection = componentSectionCopy

// Merge stack-level locals with component-level locals.
// This MUST happen on every call to ensure locals are available for template processing.
if stackLocals, ok := stackSection[cfg.LocalsSectionName].(map[string]any); ok && len(stackLocals) > 0 {
    mergedLocals := make(map[string]any)
    for k, v := range stackLocals {
        mergedLocals[k] = v
    }
    if existingLocals, ok := componentSectionCopy[cfg.LocalsSectionName].(map[string]any); ok {
        for k, v := range existingLocals {
            mergedLocals[k] = v
        }
    }
    componentSection[cfg.LocalsSectionName] = mergedLocals
}
```

## Key Insight

The shallow copy serves two purposes:
1. Prevents pollution of the `findStacksMapCache` global cache
2. Allows each call to have its own independent copy for locals merging

---

# Fix: spacelift_stack and atlantis_project Template Resolution

**Date**: 2025-01-28

**Related to**: GitHub Issue [#2032](https://github.com/cloudposse/atmos/issues/2032)

## Problem

After fixing the locals merging, another issue was discovered: templates referencing `{{ .spacelift_stack }}` and `{{ .atlantis_project }}` were returning `<no value>`.

### Example

```yaml
# In stack defaults
vars:
  tags:
    spacelift_stack: "{{ .spacelift_stack }}"
    atlantis_project: "{{ .atlantis_project }}"
```

**Expected**: `spacelift_stack: tenant1-ue2-dev-top-level-component1`
**Actual**: `spacelift_stack: <no value>`

## Root Cause

In `ProcessComponentConfig`, the `spacelift_stack` and `atlantis_project` values were computed AFTER template processing:

```go
// Template processing happened here
settingsSection, err = ProcessTmplWithDatasources(...)

// But spacelift_stack was computed AFTER, so templates couldn't reference it
spaceliftStackName, err := BuildSpaceliftStackNameFromComponentConfig(...)
configAndStacksInfo.ComponentSection["spacelift_stack"] = spaceliftStackName
```

## Solution

Moved the computation of `spacelift_stack` and `atlantis_project` BEFORE template processing so they can be referenced in templates:

```go
configAndStacksInfo.TerraformWorkspace = workspace
configAndStacksInfo.ComponentSection["workspace"] = workspace

// Spacelift stack - compute BEFORE template processing so it can be referenced in templates.
spaceliftStackName, err := BuildSpaceliftStackNameFromComponentConfig(atmosConfig, configAndStacksInfo)
if err != nil {
    return configAndStacksInfo, err
}
if spaceliftStackName != "" {
    configAndStacksInfo.ComponentSection["spacelift_stack"] = spaceliftStackName
}

// Atlantis project - compute BEFORE template processing so it can be referenced in templates.
atlantisProjectName, err := BuildAtlantisProjectNameFromComponentConfig(atmosConfig, configAndStacksInfo)
if err != nil {
    return configAndStacksInfo, err
}
if atlantisProjectName != "" {
    configAndStacksInfo.ComponentSection["atlantis_project"] = atlantisProjectName
}

// NOW process templates - spacelift_stack and atlantis_project are available
settingsSection, err = ProcessTmplWithDatasources(...)
```

## Files Changed

- **`internal/exec/utils.go`**:
  - Moved `spacelift_stack` computation before template processing
  - Moved `atlantis_project` computation before template processing
  - Removed duplicate computation after template processing

## Testing

The following tests verify the fix:
- `TestCLICommands/describe_component_with_current_directory_(.)`
- `TestCLICommands/describe_component_with_relative_path`
- `TestCLICommands/describe_component_with_component_name_(backward_compatibility)`

All now correctly show:
```yaml
spacelift_stack: tenant1-ue2-dev-top-level-component1
atlantis_project: tenant1-ue2-dev-top-level-component1
```

## Key Insight

Template processing order matters. Any values that should be available for template resolution must be computed and added to the context BEFORE calling `ProcessTmplWithDatasources`.
