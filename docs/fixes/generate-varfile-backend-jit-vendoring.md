# Fix: generate varfile and generate backend don't work with JIT vendoring

**Date**: 2025-01-28

**GitHub Issue**: [#2019](https://github.com/cloudposse/atmos/issues/2019)

## Problem

The `atmos terraform generate varfile` and `atmos terraform generate backend` commands only work on directories inside `components/terraform`. They do not work with JIT (Just-in-Time) vendored components.

When a component is configured with a `source` attribute for JIT vendoring, these commands fail to find the component directory because they skip the JIT provisioning hooks that automatically download component sources.

### Example

A JIT-vendored component configuration:

```yaml
components:
  terraform:
    my-vpc:
      source: "git::https://github.com/cloudposse/terraform-aws-vpc.git//modules/vpc?ref=v1.0.0"
      vars:
        cidr_block: "10.0.0.0/16"
```

**Expected behavior**: `atmos terraform generate varfile my-vpc -s dev` should provision the component source if needed and then generate the varfile.

**Actual behavior (before fix)**: The command fails because the component directory doesn't exist and JIT provisioning is not triggered.

## Root Cause

The `generate varfile` and `generate backend` commands used `ProcessStacks()` directly without triggering the JIT provisioning hooks that automatically download component sources. Regular terraform commands (plan, apply, etc.) route through `ExecuteTerraform()` which includes JIT provisioning checks.

Additionally, `writeBackendConfigFile` in `terraform_generate_backend.go` hardcoded the path construction instead of using `constructTerraformComponentWorkingDir()` which properly handles the `WorkdirPathKey` set by JIT provisioning.

## Solution

Added JIT provisioning support to both commands following the pattern used in `ExecuteTerraform()`:

1. **Check if component path exists**
2. **If not, check if component has `source` configured** using `provSource.HasSource()`
3. **Run JIT provisioning** using `provSource.AutoProvisionSource()`
4. **Check if provisioner set a workdir path** via `provWorkdir.WorkdirPathKey`
5. **Use the workdir path** for subsequent file operations

Also fixed `writeBackendConfigFile` to use `constructTerraformComponentWorkingDir()` instead of hardcoding the path, ensuring it properly uses the workdir path set by JIT provisioning.

## Files Changed

- `internal/exec/terraform_generate_varfile.go`: Added JIT provisioning check before writing varfile
- `internal/exec/terraform_generate_backend.go`: Added JIT provisioning check; fixed `writeBackendConfigFile` to use `constructTerraformComponentWorkingDir()`
- `internal/exec/path_utils_test.go`: Added tests for JIT vendored component path handling

## Testing

Added tests in `path_utils_test.go`:
- `TestConstructTerraformComponentVarfilePath_WithWorkdirPath`: Tests varfile path with JIT vendored components
- `TestConstructTerraformComponentWorkingDir_JITVendoredComponent`: Tests working dir resolution for JIT vendored components

## Impact

Both `generate varfile` and `generate backend` commands now properly support:
- JIT-vendored components (components with `source` configuration)
- Components that use `workdir` provisioner
- Components outside the standard `components/terraform` directory

## Related Files

The JIT provisioning system consists of:
- `pkg/provisioner/source/provision_hook.go`: Hook that runs before terraform init
- `pkg/provisioner/source/extract.go`: `HasSource()` function to detect source configuration
- `pkg/provisioner/workdir/workdir.go`: `WorkdirPathKey` constant for workdir path storage
