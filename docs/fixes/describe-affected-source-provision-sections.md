# Issue: `describe affected` does not check `source` and `provision` sections

**Date**: 2026-02-07

## Problem Summary

When running `atmos describe affected`, changes to the `source` and `provision` sections of a component are NOT
detected. This means:

- If `source.version` changes (e.g., upgrading from `0.25.0` to `0.26.0`), the component is NOT marked as affected
- If `provision.workdir.enabled` changes, the component is NOT marked as affected
- Any other changes to `source` or `provision` configuration are silently ignored

## Scenario

1. **PR1** updates a component's `source.version` from `1.0.0` to `1.1.0` and merges into main
2. **PR2** was created from main **before** PR1 was merged
3. When PR2 runs `describe affected` against main:
   - Expected: The component should be marked as affected due to `source.version` change
   - Actual: The component is NOT marked as affected

## Root Cause

The `describe affected` command only checks these sections for changes:

- `metadata` - checked via `isEqual`
- `vars` - checked via `isEqual`
- `env` - checked via `isEqual`
- `settings` - checked via `isEqual`
- Component folder files - checked via `isComponentFolderChangedIndexed`

The `source` and `provision` sections are NOT included in the comparison.

**Code location:** `internal/exec/describe_affected_components.go`

```go
func processTerraformComponentsIndexed(...) ([]schema.Affected, error) {
    // ...

    // Check metadata section (OK)
    if !isEqual(remoteStacks, ..., metadataSection, sectionNameMetadata) { ... }

    // Check component folder changes (OK)
    changed, err := isComponentFolderChangedIndexed(component, ...)

    // Check vars section (OK)
    if !isEqual(remoteStacks, ..., varSection, sectionNameVars) { ... }

    // Check env section (OK)
    if !isEqual(remoteStacks, ..., envSection, sectionNameEnv) { ... }

    // Check settings section (OK)
    if !isEqual(remoteStacks, ..., settingsSection, cfg.SettingsSectionName) { ... }

    // MISSING: source section check
    // MISSING: provision section check
}
```

## Impact

This issue affects:

1. **Source Vendoring**: When upgrading module versions via `source.version`, the component won't be detected as
   affected, potentially causing stale deployments.

2. **Workdir Configuration**: Changes to `provision.workdir.enabled` or other workdir settings won't trigger the
   component to be marked as affected.

3. **CI/CD Pipelines**: Automated workflows relying on `describe affected` will miss components that need updates due
   to source or provision configuration changes.

## Test Fixtures

Test fixtures have been added to reproduce this issue:

```text
tests/fixtures/scenarios/atmos-describe-affected-source-vendoring/
├── atmos.yaml
├── components/terraform/mock/
│   └── main.tf
├── stacks/deploy/
│   └── staging.yaml                        # HEAD state (source.version = "1.0.0")
└── stacks-with-source-version-change/deploy/
    └── staging.yaml                        # BASE state (source.version = "1.1.0")
```

Tests:

- `TestDescribeAffectedSourceVersionChange` - Tests that source.version changes are detected (currently skipped due to
  bug)

## Proposed Fix

Add checks for `source` and `provision` sections in `processTerraformComponentsIndexed`,
`processHelmfileComponentsIndexed`, and `processPackerComponentsIndexed`.

### Option A: Add section checks (minimal change)

```go
// In processTerraformComponentsIndexed:

// Check source section for changes (NEW)
if sourceSection, ok := componentSection["source"].(map[string]any); ok {
    if !isEqual(remoteStacks, stackName, cfg.TerraformComponentType, componentName, sourceSection, "source") {
        err := addAffectedComponent(&affected, atmosConfig, componentName, stackName, cfg.TerraformComponentType,
            &componentSection, "stack.source", includeSpaceliftAdminStacks, currentStacks, includeSettings)
        if err != nil {
            return nil, err
        }
    }
}

// Check provision section for changes (NEW)
if provisionSection, ok := componentSection["provision"].(map[string]any); ok {
    if !isEqual(remoteStacks, stackName, cfg.TerraformComponentType, componentName, provisionSection, "provision") {
        err := addAffectedComponent(&affected, atmosConfig, componentName, stackName, cfg.TerraformComponentType,
            &componentSection, "stack.provision", includeSpaceliftAdminStacks, currentStacks, includeSettings)
        if err != nil {
            return nil, err
        }
    }
}
```

### Option B: Generic section comparison (more maintainable)

Create a list of sections to check and iterate over them:

```go
sectionsToCheck := []struct {
    name       string
    reasonFmt  string
}{
    {"metadata", affectedReasonStackMetadata},
    {"vars", affectedReasonStackVars},
    {"env", affectedReasonStackEnv},
    {"settings", affectedReasonStackSettings},
    {"source", "stack.source"},      // NEW
    {"provision", "stack.provision"}, // NEW
}

for _, section := range sectionsToCheck {
    if sectionData, ok := componentSection[section.name].(map[string]any); ok {
        if !isEqual(remoteStacks, stackName, componentType, componentName, sectionData, section.name) {
            // Add affected...
        }
    }
}
```

## Implementation Status

- [x] Issue documented
- [x] Root cause identified
- [x] Test fixtures created
- [x] Tests added to reproduce the issue
- [x] Fix implemented

## Implemented Fix

Added checks for `source` and `provision` sections in all three component processing functions:
- `processTerraformComponentsIndexed`
- `processHelmfileComponentsIndexed`
- `processPackerComponentsIndexed`

### Changes Made

**File: `internal/exec/describe_affected_components.go`**

1. Added new affected reason constants:
   - `affectedReasonStackSource = "stack.source"`
   - `affectedReasonStackProvision = "stack.provision"`

2. Added new section name constants:
   - `sectionNameSource = "source"`
   - `sectionNameProvision = "provision"`

3. Added checks in all three component processing functions to detect changes in `source` and `provision` sections.

### Test Results

```text
=== RUN   TestDescribeAffectedSourceVersionChange
    describe_affected_test.go:1388: Found 3 affected components
    describe_affected_test.go:1390:   - vpc-source in ue1-staging (affected by: stack.source)
    describe_affected_test.go:1390:   - vpc-source-workdir in ue1-staging (affected by: stack.source)
    describe_affected_test.go:1390:   - component-workdir-only in ue1-staging (affected by: stack.provision)
--- PASS: TestDescribeAffectedSourceVersionChange (6.84s)
```
