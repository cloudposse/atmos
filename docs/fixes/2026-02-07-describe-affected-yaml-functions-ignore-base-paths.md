# Issue: `describe affected` YAML functions ignore modified BASE paths

**Date**: 2026-02-07

## Problem Summary

When running `atmos describe affected`, YAML functions like `!terraform.state` and `!terraform.output` fail to find
components that exist in BASE but not in HEAD. This happens because these functions create a new `AtmosConfiguration`
from the current working directory instead of using the modified configuration that points to the BASE repository.

Error example:

```text
Error: failed to describe component prometheus in stack plat-usw1-staging
in YAML function: !terraform.state prometheus workspace_endpoint
Could not find the component prometheus in the stack plat-usw1-staging. Check
that all the context variables are correctly defined in the stack manifests.
Are the component and stack names correct? Did you forget an import?
```

## Scenario

1. **PR1** introduces a new component (e.g., `prometheus`) and merges into main
2. **PR2** was created from main **before** PR1 was merged
3. When PR2 runs `describe affected` against main, it fails because:
  - BASE (main) has a stack that references `prometheus` via `!terraform.state`
  - When processing BASE stacks, the YAML function tries to resolve the component
  - The function creates a new config pointing to HEAD (current working directory)
  - `prometheus` doesn't exist in HEAD, so the lookup fails

## Root Cause

The issue is in `internal/exec/describe_component.go` in `ExecuteDescribeComponent`:

```go
// ExecuteDescribeComponent calls ExecuteDescribeComponentWithContext with nil AtmosConfig
result, err := ExecuteDescribeComponentWithContext(DescribeComponentContextParams{
    AtmosConfig: nil, // <-- PROBLEM: passes nil instead of current config
    Component:   params.Component,
    Stack:       params.Stack,
    ...
})
```

Then in `ExecuteDescribeComponentWithContext`:

```go
atmosConfig := params.AtmosConfig
// Use provided atmosConfig or initialize a new one
if atmosConfig == nil {
    var config schema.AtmosConfiguration
    config, err = cfg.InitCliConfig(configAndStacksInfo, true) // Creates NEW config from CWD!
    ...
    atmosConfig = &config
}
```

**The call chain:**

1. `describe affected` modifies `atmosConfig` paths to point to BASE (worktree/temp/repo-path)
2. `ExecuteDescribeStacks` processes BASE stacks with modified paths
3. `!terraform.state prometheus ...` YAML function is evaluated
4. `GetTerraformState` → `ExecuteDescribeComponent` is called with **`nil` for AtmosConfig**
5. `ExecuteDescribeComponentWithContext` creates a **NEW** `AtmosConfig` from the current working directory (HEAD)
6. Component lookup uses HEAD paths, not the modified BASE paths
7. Component doesn't exist in HEAD → error

**This issue affects ALL execution modes** (default worktree checkout, `--clone-target-ref`, and `--repo-path`).

## Implemented Fix

The fix passes the current `atmosConfig` (with modified paths pointing to BASE) through the YAML function resolution
chain to `ExecuteDescribeComponent`, instead of passing `nil`.

### Changes Made

1. **`internal/exec/describe_component.go`**:
   - Added `AtmosConfig *schema.AtmosConfiguration` field to `ExecuteDescribeComponentParams`
   - Modified `ExecuteDescribeComponent` to pass `params.AtmosConfig` instead of `nil`

2. **`internal/exec/terraform_state_utils.go`**:
   - Updated `GetTerraformState` to pass `atmosConfig` to `ExecuteDescribeComponent`

3. **`pkg/terraform/output/executor.go`**:
   - Added `AtmosConfig *schema.AtmosConfiguration` field to `DescribeComponentParams`
   - Updated `GetOutput` and `fetchAndCacheOutputs` to pass `atmosConfig`

4. **`internal/exec/component_describer_adapter.go`**:
   - Updated to pass `AtmosConfig` through the adapter

### Code Example (After Fix)

```go
// GetTerraformState now passes atmosConfig
func GetTerraformState(
    atmosConfig *schema.AtmosConfiguration,
    ...
) (any, error) {
    ...
    componentSections, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
        AtmosConfig:          atmosConfig, // Now passed instead of nil
        Component:            component,
        Stack:                stack,
        ProcessTemplates:     true,
        ProcessYamlFunctions: true,
        Skip:                 nil,
        AuthManager:          resolvedAuthMgr,
    })
}
```

## Test Fixtures

Test fixtures have been added to reproduce this issue:

```text
tests/fixtures/scenarios/atmos-describe-affected-new-component-in-base/
├── atmos.yaml
├── stacks/
│   └── deploy/
│       └── staging.yaml              # HEAD state (without prometheus)
├── stacks-with-new-component/
│   └── deploy/
│       └── staging.yaml              # BASE state (with prometheus, no reference)
└── stacks-with-new-component-and-reference/
    └── deploy/
        └── staging.yaml              # BASE state (with prometheus + !terraform.state reference)
```

Tests:

- `TestDescribeAffectedNewComponentInBase` - Tests basic scenario (no YAML functions)
- `TestDescribeAffectedNewComponentInBaseWithYamlFunctions` - Tests the exact error scenario

## Implementation Status

- [x] Issue documented
- [x] Root cause identified
- [x] Test fixtures created
- [x] Tests added to reproduce the issue
- [x] Fix implemented
