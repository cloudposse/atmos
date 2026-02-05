# Plan-Diff Does Not Respect metadata.component

**Date:** 2026-02-05

## Summary

This document describes an issue where `atmos terraform plan-diff` does not respect `metadata.component` when resolving planfile paths. The command incorrectly looks for planfiles in the wrong directory when the component instance name differs from the actual component.

| Issue                                           | Status  | Description                                                                        |
|-------------------------------------------------|---------|------------------------------------------------------------------------------------|
| `plan-diff` ignores `metadata.component`        | Fixed   | Command uses `ComponentFromArg` instead of resolved `FinalComponent`               |
| Missing `ProcessStacks` call                    | Fixed   | `TerraformPlanDiff` doesn't call `ProcessStacks` to resolve component metadata     |
| Planfile path resolution incorrect              | Fixed   | Looks in `components/terraform/` instead of `components/terraform/<component>/`    |

---

## User-Reported Issue

### Error Message

```console
Error: original plan file '/home/runner/work/github-action-atmos-terraform-
apply/github-action-atmos-terraform-
apply/tests/opentofu/components/terraform/plat-
ue2-sandbox-foobar-atmos-pro-bfc35b762c09ef79e17d2bbfa1ef2551226a0410.planfile'
does not exist
```

### Setup

- Component instance `foobar-atmos-pro` has `metadata.component: foobar` in stack config
- Actual terraform code lives in `components/terraform/foobar/`
- `github-action-atmos-terraform-plan` correctly stores planfile to S3 from `components/terraform/foobar/*.planfile`
- `github-action-atmos-terraform-apply` correctly downloads planfile to `components/terraform/foobar/*.planfile`

### Observed Behavior

- `atmos terraform plan-diff foobar-atmos-pro --stack plat-ue2-sandbox --orig {filename}` looks for the file in `components/terraform/` (missing the `foobar/` subdirectory)
- The command does not resolve `metadata.component` to find the actual component path

### Expected Behavior

- `atmos terraform plan-diff` should resolve the component path using `metadata.component` (same as other `atmos terraform` commands)
- Should look in `components/terraform/foobar/` for the planfile

### Environment

- Atmos v1.205.0
- Affects: `github-action-atmos-terraform-apply` CI tests

---

## Root Cause Analysis

### The Bug: Missing ProcessStacks Call

The `TerraformPlanDiff` function directly uses `info.FinalComponent` without first calling `ProcessStacks()` to resolve the component metadata.

#### 1. How Other Commands Work (Correct)

`ExecuteTerraform` in `internal/exec/terraform.go` (line 193):

```go
if shouldProcess {
    info, err = ProcessStacks(&atmosConfig, info, shouldCheckStack, info.ProcessTemplates, info.ProcessFunctions, info.Skip, authManager)
    if err != nil {
        return err
    }
}

// Later uses the resolved info.FinalComponent
componentPath, err := u.GetComponentPath(&atmosConfig, "terraform", info.ComponentFolderPrefix, info.FinalComponent)
```

`ProcessStacks()` resolves:
- `info.FinalComponent` from `metadata.component`
- `info.ComponentFolderPrefix` from `metadata.component_folder_prefix`

#### 2. How plan-diff Works (Incorrect)

`TerraformPlanDiff` in `internal/exec/terraform_plan_diff.go` (line 52):

```go
func TerraformPlanDiff(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
    // ...
    // BUG: Uses info.FinalComponent directly without calling ProcessStacks!
    componentPath := filepath.Join(atmosConfig.TerraformDirAbsolutePath, info.ComponentFolderPrefix, info.FinalComponent)
    // ...
}
```

#### 3. The Command Setup (Missing Resolution)

`cmd/terraform/plan_diff.go` creates the info struct:

```go
info := &schema.ConfigAndStacksInfo{
    ComponentFromArg: component,  // e.g., "foobar-atmos-pro"
    Stack:            stack,
    SubCommand:       "plan-diff",
    ComponentType:    cfg.TerraformComponentType,
}
// BUG: FinalComponent is empty! Never resolved from metadata.component
return e.TerraformPlanDiff(&atmosConfig, info)
```

### Flow Diagram

```text
User runs: atmos terraform plan-diff foobar-atmos-pro -s plat-ue2-sandbox --orig=foo.planfile
        ↓
cmd/terraform/plan_diff.go:
    info.ComponentFromArg = "foobar-atmos-pro"
    info.FinalComponent = "" (empty!)
        ↓
e.TerraformPlanDiff():
    componentPath = filepath.Join(atmosConfig.TerraformDirAbsolutePath, "", "")
    // Results in: "components/terraform/"  (WRONG!)
        ↓
validateOriginalPlanFile():
    origPlanFile = filepath.Join("components/terraform/", "foo.planfile")
    // Looking for: "components/terraform/foo.planfile"
    // Should be: "components/terraform/foobar/foo.planfile"
        ↓
ERROR: original plan file does not exist
```

---

## Implemented Fix

Call `ProcessStacks()` in `TerraformPlanDiff` to properly resolve the component metadata before using the component path.

### Changes Made

**File: `internal/exec/terraform_plan_diff.go`**

```go
func TerraformPlanDiff(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
    defer perf.Track(atmosConfig, "exec.TerraformPlanDiff")()

    // Process stacks to resolve component metadata (metadata.component, component_folder_prefix)
    // This is required to get the correct FinalComponent and ComponentFolderPrefix
    processedInfo, err := ProcessStacks(atmosConfig, *info, true, true, true, nil, nil)
    if err != nil {
        return err
    }

    // Extract flags and setup paths
    origPlanFile, newPlanFile, err := parsePlanDiffFlags(processedInfo.AdditionalArgsAndFlags)
    if err != nil {
        return err
    }

    // ... rest of function uses processedInfo instead of info

    // Get the component path using the resolved FinalComponent
    componentPath := filepath.Join(atmosConfig.TerraformDirAbsolutePath, processedInfo.ComponentFolderPrefix, processedInfo.FinalComponent)
    // ...
}
```

---

## Testing

### Test Fixture

Created `tests/fixtures/scenarios/atmos-terraform-plan-diff/` with:
- Component instance `derived-component` with `metadata.component: base-component`
- Actual terraform code in `components/terraform/base-component/`

### Reproduce the Issue (Before Fix)

```bash
cd tests/fixtures/scenarios/atmos-terraform-plan-diff

# Create a dummy planfile in the CORRECT location (where it should be found)
touch components/terraform/base-component/test.planfile

# Run plan-diff with the derived component name
atmos terraform plan-diff derived-component -s test-stack --orig=test.planfile

# ERROR: plan-diff looks in components/terraform/ instead of components/terraform/base-component/
```

### Verify the Fix (After Fix)

```bash
cd tests/fixtures/scenarios/atmos-terraform-plan-diff

# Create a dummy planfile
touch components/terraform/base-component/test.planfile

# Run plan-diff - should now find the file in the correct location
atmos terraform plan-diff derived-component -s test-stack --orig=test.planfile

# Success: plan-diff correctly resolves metadata.component and finds the file
```

---

## Related Issues

- Reported via Slack by Daniel Miller (2026-02-05)
- Affects `github-action-atmos-terraform-apply` CI tests (`test-atmos_pro.yml`)

---

## Files Modified

| File                                         | Change                                                              |
|----------------------------------------------|---------------------------------------------------------------------|
| `internal/exec/terraform_plan_diff.go`       | Added `ProcessStacks` call to resolve component metadata            |
| `internal/exec/terraform_plan_diff_test.go`  | Updated tests to be proper unit tests of helper functions           |
| `internal/exec/terraform_plan_diff_main_test.go` | Updated flag parsing test to test `parsePlanDiffFlags` directly |

---

## Verification Commands

```bash
# Run unit tests
go test ./internal/exec/... -v -run "TestPlanDiff"

# Integration test with fixture
cd tests/fixtures/scenarios/atmos-terraform-plan-diff
atmos terraform plan-diff derived-component -s test-stack --orig=test.planfile
```
