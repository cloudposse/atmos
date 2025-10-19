# Merge main into error handling branch and fix exit code preservation

## what

- Merged `origin/main` into `osterman/merge-main-errors` branch
- Fixed exit code preservation for workflow step failures
- Fixed error messages to show actual exit codes
- Fixed gomonkey test panics on ARM64 macOS
- Removed unrelated `auth-list.mdx` documentation that belongs to another PR

## why

### Merge Main
The error handling branch needed to be updated with latest changes from main to ensure compatibility and incorporate recent improvements.

### Exit Code Preservation
Workflow step failures were losing exit code information:
- All failures returned exit code 1 regardless of actual command exit code (2, 42, 127, etc.)
- Error messages didn't show the actual exit code
- This made it impossible to distinguish between different types of failures

### gomonkey ARM64 Incompatibility
Tests using the gomonkey mocking library were causing fatal SIGBUS panics on ARM64 macOS due to memory protection restrictions that prevent runtime code modification.

## changes

### Merge Resolution
Resolved 5 merge conflicts in:
- `cmd/workflow.go` - Used `GetExitCode()` for consistent exit code handling
- `errors/error_funcs.go` - Combined both approaches: kept error builder pattern with `OsExit` variable
- `internal/exec/terraform.go` - Combined both constant definitions
- `internal/exec/workflow_utils.go` - Used error builder pattern, removed cockroachdb/errors dependency
- `tests/snapshots/TestCLICommands_atmos_circuit-breaker.stderr.golden` - Used improved error messages from main

### Exit Code Fixes

**Root Causes:**
1. `ExitCodeError.Error()` didn't show the exit code in the message
2. `GetExitCode()` didn't check for `ExitCodeError` type
3. `workflow_utils.go` returned formatted error instead of preserving original error chain

**Solutions:**
1. Updated `ExitCodeError.Error()` to return: `"workflow step execution failed with exit code N"`
2. Added `ExitCodeError` type check in `GetExitCode()` function (checked before `exitCoder` and `exec.ExitError`)
3. Changed workflow error handling to use `errors.Join(stepErr, err)` to preserve exit code in error chain

**Updated Files:**
- `errors/errors.go` - Include exit code in error message
- `errors/exit_code.go` - Check for `ExitCodeError` type
- `internal/exec/workflow_utils.go` - Preserve original error with `errors.Join()`
- `pkg/utils/shell_utils_test.go` - Updated test expectations
- `tests/test-cases/complete.yaml` - Updated test pattern
- `tests/snapshots/*.golden` - Regenerated with new error messages

### gomonkey ARM64 Fix

Added `tests.SkipOnDarwinARM64()` calls to all tests using gomonkey in `pkg/list/utils/check_component_test.go` with detailed comments linking to upstream issues:

- [agiledragon/gomonkey#146](https://github.com/agiledragon/gomonkey/issues/146) - SIGBUS panics
- [agiledragon/gomonkey#122](https://github.com/agiledragon/gomonkey/issues/122) - ARM64 permission denied
- [agiledragon/gomonkey#169](https://github.com/agiledragon/gomonkey/issues/169) - Mac M3 failures

**Why This Approach:**
- gomonkey patches function code at runtime, which ARM64 macOS explicitly blocks via memory protection
- No current fix exists upstream for ARM64 macOS
- Tests still run on all CI/CD environments (typically Linux)
- Minimal impact on test coverage
- Alternative workarounds (Docker, xgo library) add unnecessary complexity

## testing

All tests pass:
- ✅ `workflow_shell_step_preserves_exit_code_2` - Exit code 2 properly preserved
- ✅ `workflow_shell_step_preserves_exit_code_42` - Exit code 42 properly preserved
- ✅ `atmos_workflow_shell_command_not_found` - Exit code 127 properly preserved
- ✅ `atmos_circuit-breaker` - Circuit breaker with exit code 1
- ✅ All shell_utils tests pass with new error message format
- ✅ All gomonkey tests skip gracefully on ARM64 macOS
- ✅ Build successful
- ✅ All CLI command tests passing

## references

- Gomonkey ARM64 issues:
  - https://github.com/agiledragon/gomonkey/issues/146
  - https://github.com/agiledragon/gomonkey/issues/122
  - https://github.com/agiledragon/gomonkey/issues/169
