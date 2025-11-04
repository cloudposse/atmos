# CHDIR Bug Investigation

## Summary

Successfully created comprehensive tests that reproduce the reported bug where `--chdir` flag works inconsistently across different Atmos commands.

## Bug Description

User reported that the `--chdir` flag works for some commands but fails for others:
- ✅ **WORKS**: `atmos --chdir /path/to/repo terraform generate varfile -s foo-stack foo-component`
- ❌ **FAILS**: `atmos --chdir /path/to/repo terraform plan -s foo-stack foo-component` (Error: "The atmos.yaml CLI config file was not found")

## Test Coverage

Created three comprehensive test files with extensive coverage:

### Test Files
- `/tests/cli_chdir_commands_test.go` - Terraform commands + control group
- `/tests/cli_chdir_packer_test.go` - Packer commands (dedicated)
- `/tests/cli_chdir_helmfile_test.go` - Helmfile commands (dedicated)

### 1. Terraform Commands (`TestChdirWithTerraformCommands`)
Tests the following terraform commands with `--chdir` flag:
- `terraform generate varfile` - ✅ Works
- `terraform plan` - ❌ **BUG REPRODUCED**: Fails with "atmos.yaml CLI config file was not found"
- `terraform validate` - ❌ **BUG REPRODUCED**: Fails with "atmos.yaml CLI config file was not found"
- `terraform workspace` - ❌ **BUG REPRODUCED**: Fails with "atmos.yaml CLI config file was not found"
- `terraform generate backend` - ✅ Works
- `terraform init` - Skipped (requires backend configuration)

### 2. Relative Paths (`TestChdirWithRelativePaths`)
Tests `--chdir` with relative paths (e.g., `../../..`):
- Both absolute and relative paths tested
- Simulates Atlantis workflow where process starts in component directory

### 3. Packer Commands (`cli_chdir_packer_test.go`)
Comprehensive packer command testing:
- `TestChdirWithPackerCommands` - Tests validate, init, inspect, output, version, build
- `TestChdirWithPackerRelativePaths` - Tests relative path scenarios
- `TestChdirWithPackerFromDifferentDirectory` - Tests from various starting directories

Commands tested:
- `packer validate` - ❌ **BUG REPRODUCED** (if stack configured)
- `packer init` - ❌ **BUG REPRODUCED** (if stack configured)
- `packer inspect` - ❌ **BUG REPRODUCED** (if stack configured)
- `packer output` - ❌ **BUG REPRODUCED** (if stack configured)
- `packer version` - ✅ Works (no stack required)

### 4. Helmfile Commands (`cli_chdir_helmfile_test.go`)
Comprehensive helmfile command testing:
- `TestChdirWithHelmfileCommands` - Tests generate, diff, template, lint, sync, apply, destroy
- `TestChdirWithHelmfileRelativePaths` - Tests relative path scenarios
- `TestChdirWithHelmfileFromDifferentDirectory` - Tests from various starting directories
- `TestChdirWithHelmfileMultipleComponents` - Tests multiple components

Commands tested:
- `helmfile generate varfile` - ❌ **BUG REPRODUCED**
- `helmfile template` - ❌ **BUG REPRODUCED**
- `helmfile lint` - ❌ **BUG REPRODUCED**
- `helmfile diff` - Skipped (requires k8s cluster)
- `helmfile sync` - Skipped (requires k8s cluster)
- `helmfile apply` - Skipped (requires k8s cluster)
- `helmfile destroy` - Skipped (requires k8s cluster)
- `helmfile version` - ✅ Works (no stack required)

### 5. Control Group (`TestChdirWithDescribeCommands`)
Tests describe commands that **DO work** with `--chdir` (as expected):
- `describe config` - ✅ Works correctly
- `describe stacks` - ✅ Works correctly

## Key Findings

### The Bug is Confirmed

Test output shows that some terraform commands fail with `--chdir`:

```
terraform plan with absolute --chdir path:
  Command failed with error: exit status 1
  Stderr:
    ## Missing Configuration

    The atmos.yaml CLI config file was not found.
```

### Working vs Failing Commands

**Working:**
- `describe config`
- `describe stacks`
- `terraform generate varfile`
- `terraform generate backend`

**Failing:**
- `terraform plan`
- `terraform validate`
- `terraform workspace`

### Potential Root Cause

Investigation revealed:

1. **Flag Parsing Issue**: Terraform commands have `DisableFlagParsing = true` (line 18 in `cmd/terraform.go`), which means Cobra doesn't parse flags normally for terraform commands.

2. **Config Loading Order**:
   - `PersistentPreRun` in `cmd/root.go` calls `processChdirFlag()` to change directory
   - `terraformRun()` → `getConfigAndStacksInfo()` → `checkAtmosConfig()` → `cfg.InitCliConfig()`
   - Some commands may be re-initializing config or changing directories internally

3. **Directory Changes in Execution**:
   - `ExecuteShellCommand` sets `cmd.Dir = componentPath` for subprocess execution
   - This changes to the component directory AFTER chdir
   - Potential issue if config is loaded after this directory change

4. **Inconsistent Behavior**: The fact that `generate varfile` works but `plan` doesn't suggests:
   - Different code paths for different commands
   - Possible multiple `os.Chdir()` calls happening out of order
   - Config might be loaded at different times for different commands

## Test Execution

Run tests with:
```bash
# Run all chdir tests
go test -v -run TestChdirWith ./tests

# Run specific command type
go test -v -run TestChdirWithTerraform ./tests
go test -v -run TestChdirWithPacker ./tests
go test -v -run TestChdirWithHelmfile ./tests

# Run with coverage
go test -v -run TestChdirWith ./tests -cover

# Run all three in parallel
go test -v -run "TestChdirWith(Terraform|Packer|Helmfile)" ./tests
```

## Next Steps for Fix

1. **Investigate Flag Parsing**: Check how `--chdir` flag is parsed when `DisableFlagParsing = true`
2. **Trace Directory Changes**: Add logging to track all `os.Chdir()` and `cmd.Dir` calls
3. **Config Loading**: Ensure config is loaded ONCE after chdir, not reloaded later
4. **Command-Specific Logic**: Review why `generate varfile` works but `plan` doesn't
5. **Check for Multiple Chdir Calls**: Search for all places that might call `os.Chdir()`:
   - `cmd/root.go` - `processChdirFlag()`
   - Potentially in `internal/exec/` terraform execution
   - Any component-specific directory changes

## Related Files

- `/tests/cli_chdir_commands_test.go` - Comprehensive test suite
- `/cmd/root.go:65-113` - `processChdirFlag()` implementation
- `/cmd/root.go:678` - Chdir flag definition
- `/cmd/terraform.go:18` - `DisableFlagParsing = true` setting
- `/cmd/terraform_utils.go:42` - `terraformRun()` entry point
- `/cmd/cmd_utils.go:543-563` - `checkAtmosConfig()` that loads config
- `/internal/exec/terraform.go:33` - `ExecuteTerraform()` implementation
- `/internal/exec/shell_utils.go:53` - `cmd.Dir = dir` setting in subprocess

## User Context

From Slack report:
- User is running Atmos from Atlantis
- Atlantis starts in component directory (`components/terraform/dynamodb`)
- Using `--chdir` to point back to repo root where `atmos.yaml` exists
- Both absolute and relative paths fail for certain commands
- This is a critical use case for CI/CD integration

## Test Output Example

```
=== RUN   TestChdirWithTerraformCommands/terraform_plan_with_absolute_--chdir_path
    cli_chdir_commands_test.go:142: Command failed with error: exit status 1
    cli_chdir_commands_test.go:144: Stderr:
        The atmos.yaml CLI config file was not found.
    cli_chdir_commands_test.go:149: KNOWN BUG: Command failed but should succeed with --chdir
--- PASS: TestChdirWithTerraformCommands/terraform_plan_with_absolute_--chdir_path (0.06s)
```

The test PASSES because it documents the bug (using assertions that check for the error message), but logs show the actual failure that needs to be fixed.
