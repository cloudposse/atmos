# PRD: Error Types and Sentinel Errors in Atmos

## Overview

This document defines the error architecture patterns used in Atmos, explaining when to use sentinel errors versus error types, and the benefits of each approach.

## Problem Statement

Proper error handling requires different approaches for different use cases:
- Simple error matching needs lightweight sentinel errors
- Complex error context requires structured error types
- Command execution failures need to preserve exit codes and context
- Workflow orchestration failures need workflow-specific information
- Error formatting must distinguish between different error categories

Without clear patterns, developers might:
- Create dynamic errors that break `errors.Is()` checking
- Use string matching instead of type-safe error checking
- Lose important context like exit codes or command details
- Conflate different error categories in formatting

## Naming Conventions

### Sentinel Errors: `Err` Prefix
Sentinel errors use the `Err` prefix and are simple static error values:

```go
var (
    ErrNotFound         = errors.New("not found")
    ErrInvalidArgument  = errors.New("invalid argument")
    ErrWorkflowStepFailed = errors.New("workflow step execution failed")
)
```

### Error Types: `Error` Suffix
Error types use the `Error` suffix and are structured types implementing the `error` interface:

```go
type PathError struct {
    Op   string
    Path string
    Err  error
}

type ExecError struct {
    Cmd      string
    Args     []string
    ExitCode int
    cause    error
}
```

This follows Go's standard library conventions (e.g., `os.PathError`, `exec.ExitError`).

## Sentinel Errors

### When to Use

Use sentinel errors for:
1. **Simple error conditions** - Single failure mode without additional context
2. **Error matching** - When callers need to check for specific errors with `errors.Is()`
3. **Error categorization** - As base errors that get wrapped with additional context
4. **Validation failures** - Simple validation errors that don't need structured data

### Definition Pattern

```go
package errors

var (
    // Error categorization - high-level error types
    ErrInvalidConfig    = errors.New("invalid configuration")
    ErrNotFound         = errors.New("not found")

    // Validation errors - simple checks
    ErrMissingComponent = errors.New("component name is required")
    ErrInvalidStack     = errors.New("invalid stack name")

    // Operation failures - simple failure modes
    ErrLoadFailed       = errors.New("failed to load")
    ErrParseFailed      = errors.New("failed to parse")
)
```

### Usage Pattern

**Returning sentinel errors:**
```go
func LoadComponent(name string) (*Component, error) {
    if name == "" {
        return nil, ErrMissingComponent
    }

    component, err := findComponent(name)
    if err != nil {
        // Wrap with context using fmt.Errorf
        return nil, fmt.Errorf("%w: component=%s", ErrNotFound, name)
    }

    return component, nil
}
```

**Checking sentinel errors:**
```go
component, err := LoadComponent(name)
if err != nil {
    if errors.Is(err, ErrMissingComponent) {
        // Handle missing component
        return defaultComponent()
    }
    if errors.Is(err, ErrNotFound) {
        // Handle not found
        log.Warn("Component not found", "name", name)
    }
    return err
}
```

### Benefits

1. **Lightweight** - No additional types or fields needed
2. **Easy to define** - Simple `errors.New()` call
3. **Works with `errors.Is()`** - Type-safe error checking
4. **Composable** - Can be wrapped with `fmt.Errorf("%w: context", sentinel)`
5. **Backward compatible** - Adding context doesn't break existing checks

## Error Types

### When to Use

Use error types for:
1. **Rich context** - Multiple fields of structured data (exit codes, commands, paths)
2. **Exit code preservation** - Command execution errors that need to propagate exit codes
3. **Type-specific behavior** - Errors that need custom formatting or methods
4. **Error hierarchies** - When errors need to be checked with `errors.As()` for type-specific handling

### Definition Pattern

```go
package errors

// ExecError represents a failure from executing an external command.
type ExecError struct {
    Cmd      string   // Command name (e.g., "terraform", "helm")
    Args     []string // Command arguments
    ExitCode int      // Non-zero exit code
    Stderr   string   // Optional stderr output
    cause    error    // Wrapped underlying error
}

func (e *ExecError) Error() string {
    if e.cause != nil {
        return e.cause.Error()
    }
    cmdStr := e.Cmd
    if len(e.Args) > 0 {
        cmdStr = cmdStr + " " + strings.Join(e.Args, " ")
    }
    return fmt.Sprintf("command %s exited with code %d", cmdStr, e.ExitCode)
}

func (e *ExecError) Unwrap() error {
    return e.cause
}

func NewExecError(cmd string, args []string, exitCode int, cause error) *ExecError {
    return &ExecError{
        Cmd:      cmd,
        Args:     args,
        ExitCode: exitCode,
        cause:    cause,
    }
}
```

### Usage Pattern

**Returning error types:**
```go
func ExecuteCommand(cmd string, args []string) error {
    execCmd := exec.Command(cmd, args...)
    err := execCmd.Run()

    if err != nil {
        if exitError, ok := err.(*exec.ExitError); ok {
            exitCode := exitError.ExitCode()
            // Return structured error with exit code
            return NewExecError(cmd, args, exitCode, err)
        }
        return err
    }

    return nil
}
```

**Checking error types:**
```go
err := ExecuteCommand("terraform", []string{"apply"})
if err != nil {
    // Type-safe extraction of structured data
    var execErr *ExecError
    if errors.As(err, &execErr) {
        log.Error("Command failed",
            "cmd", execErr.Cmd,
            "exit_code", execErr.ExitCode)

        // Access type-specific fields
        if execErr.ExitCode == 1 {
            // Handle specific exit code
        }

        // Propagate exit code to process exit
        os.Exit(execErr.ExitCode)
    }

    return err
}
```

### Benefits

1. **Structured data** - Multiple fields accessible via type assertion
2. **Exit code preservation** - Critical for CI/CD workflows
3. **Custom formatting** - Type-specific `Error()` methods
4. **Type-safe access** - Compiler-checked field access
5. **Rich context** - Command details, stderr output, etc.

## Error Type Hierarchy in Atmos

### Base Error Types

#### 1. ExecError - Command Execution Failures

**Purpose:** Represents failures from executing external commands (terraform, helmfile, packer, shell commands).

**When to use:**
- Any subprocess execution that exits with non-zero code
- Need to preserve command exit code for CI/CD workflows
- Want to capture stderr output for debugging

**Example:**
```go
// In shell_utils.go
err := cmd.Run()
if err != nil {
    if exitError, ok := err.(*exec.ExitError); ok {
        exitCode := exitError.ExitCode()
        return NewExecError("terraform", args, exitCode, err)
    }
    return err
}
```

**Fields:**
- `Cmd` - Command name (terraform, helm, etc.)
- `Args` - Command arguments
- `ExitCode` - Process exit code (for propagation to main)
- `Stderr` - Captured stderr (optional, for command output section)

#### 2. WorkflowStepError - Workflow Orchestration Failures

**Purpose:** Represents failures from workflow step execution, wrapping underlying command errors with workflow-specific context.

**When to use:**
- Workflow step fails (command or shell execution within workflow)
- Need workflow context (workflow name, step name, resume instructions)
- Higher-level orchestration failure vs. raw command failure

**Example:**
```go
// In workflow_utils.go
baseErr := Build(ErrWorkflowStepFailed).
    WithTitle("Workflow Error").
    WithExplanationf("The following command failed to execute:\n\n%s", failedCmd).
    WithHintf("To resume the workflow from this step, run:\n\n%s", resumeCommand).
    Err()

stepErr := NewWorkflowStepError(workflow, step.Name, failedCmd, exitCode, baseErr)
```

**Fields:**
- `Workflow` - Workflow name
- `Step` - Step name or index
- `Command` - Command that failed
- `ExitCode` - Exit code from failed command
- `cause` - Wrapped error with hints/explanations

**Why separate from ExecError?**
- Workflows are orchestration - higher abstraction level than raw command execution
- Need workflow-specific formatting ("workflow step execution failed with exit code X")
- Provides workflow context (which step, resume instructions)
- Prevents confusion between "command failed" and "workflow step failed"

#### 3. ExitCodeError (Legacy)

**Purpose:** Legacy error type for preserving exit codes before ExecError was introduced.

**Status:** Being phased out in favor of ExecError and WorkflowStepError.

**Why deprecating?**
- Less descriptive than ExecError (no command context)
- Doesn't distinguish between command execution and workflow failures
- Generic error message doesn't help with debugging

### Error Type Selection Matrix

| Scenario | Error Type | Reason |
|----------|-----------|--------|
| Terraform command exits with code 1 | `ExecError` | External command execution |
| Workflow step fails | `WorkflowStepError` | Orchestration failure with workflow context |
| Shell script exits non-zero | `ExecError` | External command execution |
| Packer command fails | `ExecError` | External command execution |
| Helmfile command fails | `ExecError` | External command execution |
| Custom command fails | `ExecError` | External command execution |
| Workflow recursion limit exceeded | Sentinel + context | Not a command failure, validation error |
| Configuration validation fails | Sentinel + context | Simple validation, no exit code needed |

## Combining Sentinels and Error Types

### Pattern: Sentinel as Base + Error Type as Wrapper

Use sentinels as the base error, wrapped in error types for structured context:

```go
// Define sentinel
var ErrWorkflowStepFailed = errors.New("workflow step execution failed")

// Build rich error with sentinel as base
baseErr := Build(ErrWorkflowStepFailed).
    WithTitle("Workflow Error").
    WithExplanationf("Command failed: %s", cmd).
    WithHint("Check the command output above").
    Err()

// Wrap in error type for structured data
stepErr := NewWorkflowStepError(workflow, step, cmd, exitCode, baseErr)

// Later: Check both ways
if errors.Is(err, ErrWorkflowStepFailed) {
    // Detected via sentinel
}

var workflowErr *WorkflowStepError
if errors.As(err, &workflowErr) {
    // Access structured fields
    log.Error("Workflow failed",
        "workflow", workflowErr.Workflow,
        "step", workflowErr.Step,
        "exit_code", workflowErr.ExitCode)
}
```

### Pattern: Multiple Error Wrapping

Use `errors.Join()` to combine independent errors (flat list), or `fmt.Errorf("%w", err)` to create error chains:

```go
// Flat list of independent errors (for parallel validations)
err1 := validateName(name)
err2 := validatePath(path)
err3 := validateConfig(config)
return errors.Join(err1, err2, err3)

// Error chain (for sequential context building)
if err := loadConfig(); err != nil {
    return fmt.Errorf("%w: failed to load config for component %s",
        ErrLoadFailed, componentName)
}
```

**Important distinction:**
- `errors.Join(err1, err2)` - Creates flat list, `Unwrap()` returns `nil`, must use `Unwrap() []error` interface
- `fmt.Errorf("%w", err)` - Creates chain, `Unwrap()` returns next error
- `fmt.Errorf("%w: %w", err1, err2)` - Like `errors.Join` (Go 1.20+), but prefer `errors.Join`

## Error Formatting

### Formatter Priority

The error formatter checks error types in priority order:

```go
// From errors/formatter.go
var workflowErr *WorkflowStepError
var execErr *ExecError

switch {
case errors.As(err, &workflowErr):
    // Highest priority: Workflow orchestration failures
    md.WriteString(fmt.Sprintf("**Error:** %s", workflowErr.WorkflowStepMessage()))

case errors.As(err, &execErr):
    // Second priority: External command execution failures
    md.WriteString(fmt.Sprintf("**Error:** %s with exit code %d", sentinelMsg, execErr.ExitCode))

default:
    // Lowest priority: All other errors (no exit code appending)
    md.WriteString("**Error:** " + sentinelMsg)
}
```

### Why Priority Matters

Without priority, workflow errors would be formatted as generic command errors:

**Before WorkflowStepError (incorrect):**
```
Error: command exited with code 1
```

**After WorkflowStepError (correct):**
```
Error: workflow step execution failed with exit code 1

## Explanation

The following command failed to execute:

atmos terraform apply vpc -s prod

## Hints

To resume the workflow from this step, run:

atmos workflow deploy-infra -f workflows --from-step apply-vpc
```

## Exit Code Propagation

### GetExitCode Priority

Exit codes are extracted in priority order to ensure the most specific exit code is used:

```go
// From errors/exit_code.go
func GetExitCode(err error) int {
    if err == nil {
        return 0
    }

    // Check for WorkflowStepError (workflow orchestration)
    var workflowErr *WorkflowStepError
    if errors.As(err, &workflowErr) {
        return workflowErr.ExitCode
    }

    // Check for ExecError (external command execution)
    var execErr *ExecError
    if errors.As(err, &execErr) {
        return execErr.ExitCode
    }

    // Check for ExitCodeError (legacy)
    var exitCodeErr ExitCodeError
    if errors.As(err, &exitCodeErr) {
        return exitCodeErr.Code
    }

    // Check for exitCoder (attached via WithExitCode)
    var ec *exitCoder
    if errors.As(err, &ec) {
        return ec.ExitCode()
    }

    // Check for exec.ExitError (from os/exec package)
    var exitErr *exec.ExitError
    if errors.As(err, &exitErr) {
        return exitErr.ExitCode()
    }

    return 1 // Default exit code for generic errors
}
```

### Main Exit Code Handling

```go
// From main.go
err := cmd.Execute()
if err != nil {
    // Check for WorkflowStepError first to preserve workflow step exit codes
    var workflowErr *errUtils.WorkflowStepError
    if errors.As(err, &workflowErr) {
        errUtils.Exit(workflowErr.ExitCode)
    }

    // Check for ExecError to preserve command exit codes
    var execErr *errUtils.ExecError
    if errors.As(err, &execErr) {
        errUtils.Exit(execErr.ExitCode)
    }

    // ... other exit code checks

    // Default: Print error and exit with code 1
    errUtils.CheckErrorPrintAndExit(err, "", "")
}
```

**Why this matters for CI/CD:**
- Terraform exit code 1 = plan failed → CI/CD should fail the build
- Terraform exit code 2 = plan succeeded but has changes → CI/CD might trigger approval
- Wrong exit code propagation breaks GitHub Actions workflows and CI/CD pipelines

## Best Practices

### DO: Use Sentinel Errors for Simple Checks

✅ **Good:**
```go
var ErrMissingComponent = errors.New("component name is required")

if name == "" {
    return ErrMissingComponent
}

// Later:
if errors.Is(err, ErrMissingComponent) {
    // Handle missing component
}
```

❌ **Bad:**
```go
// Dynamic error - breaks errors.Is()
if name == "" {
    return errors.New("component name is required")
}

// Later: String matching - fragile, not type-safe
if err.Error() == "component name is required" {
    // Handle missing component
}
```

### DO: Use Error Types for Rich Context

✅ **Good:**
```go
type ExecError struct {
    Cmd      string
    Args     []string
    ExitCode int
    cause    error
}

return NewExecError("terraform", args, exitCode, err)

// Later:
var execErr *ExecError
if errors.As(err, &execErr) {
    log.Error("Command failed",
        "cmd", execErr.Cmd,
        "exit_code", execErr.ExitCode)
}
```

❌ **Bad:**
```go
// Losing exit code and command context
return fmt.Errorf("command failed with exit code %d", exitCode)

// Later: String parsing - fragile
if strings.Contains(err.Error(), "exit code") {
    // Can't extract actual exit code reliably
}
```

### DO: Combine Sentinels with Error Types

✅ **Good:**
```go
var ErrWorkflowStepFailed = errors.New("workflow step execution failed")

baseErr := Build(ErrWorkflowStepFailed).
    WithHint("Check the command output").
    Err()

workflowErr := NewWorkflowStepError(workflow, step, cmd, exitCode, baseErr)

// Check both ways:
if errors.Is(err, ErrWorkflowStepFailed) { /* sentinel check */ }
var wfErr *WorkflowStepError
if errors.As(err, &wfErr) { /* type-specific access */ }
```

❌ **Bad:**
```go
// Either sentinel OR type, losing benefits of both
return errors.New("workflow step failed")  // No structured data
// OR
return &WorkflowStepError{...}  // Can't check with errors.Is()
```

### DO: Preserve Error Chains

✅ **Good:**
```go
if err := loadConfig(); err != nil {
    // Wrap with %w to preserve error chain
    return fmt.Errorf("%w: failed to load config", ErrLoadFailed)
}

// Later:
if errors.Is(err, ErrLoadFailed) {
    // Can still detect sentinel through wrapping
}
```

❌ **Bad:**
```go
if err := loadConfig(); err != nil {
    // Lost original error - can't unwrap
    return fmt.Errorf("failed to load config: %v", err)
}
```

### DON'T: Use String Matching

❌ **Bad:**
```go
if err.Error() == "component not found" {
    // Fragile - breaks if error message changes
}

if strings.Contains(err.Error(), "exit code") {
    // Can't extract structured data
}
```

✅ **Good:**
```go
if errors.Is(err, ErrNotFound) {
    // Type-safe, survives message changes
}

var execErr *ExecError
if errors.As(err, &execErr) {
    exitCode := execErr.ExitCode  // Type-safe field access
}
```

### DON'T: Create Dynamic Errors

❌ **Bad:**
```go
return errors.New(fmt.Sprintf("invalid value: %s", value))
```

✅ **Good:**
```go
return fmt.Errorf("%w: invalid value %s", ErrInvalidValue, value)
```

## Migration Guide

### From String Matching to Sentinel Errors

**Before:**
```go
// Creating
return errors.New("component not found")

// Checking
if err != nil && err.Error() == "component not found" {
    // ...
}
```

**After:**
```go
// Define sentinel
var ErrComponentNotFound = errors.New("component not found")

// Creating
return ErrComponentNotFound
// Or with context:
return fmt.Errorf("%w: component=%s", ErrComponentNotFound, name)

// Checking
if errors.Is(err, ErrComponentNotFound) {
    // ...
}
```

### From ExitCodeError to ExecError

**Before:**
```go
return ExitCodeError{Code: exitCode}

// Checking
var exitCodeErr ExitCodeError
if errors.As(err, &exitCodeErr) {
    os.Exit(exitCodeErr.Code)
}
```

**After:**
```go
return NewExecError(cmd, args, exitCode, err)

// Checking
var execErr *ExecError
if errors.As(err, &execErr) {
    log.Error("Command failed",
        "cmd", execErr.Cmd,
        "exit_code", execErr.ExitCode)
    os.Exit(execErr.ExitCode)
}
```

### From Generic Errors to WorkflowStepError

**Before:**
```go
return fmt.Errorf("workflow step failed: %s", cmd)
```

**After:**
```go
baseErr := Build(ErrWorkflowStepFailed).
    WithExplanationf("Command failed: %s", cmd).
    WithHint("Check the command output").
    Err()

return NewWorkflowStepError(workflow, step, cmd, exitCode, baseErr)
```

## Testing Error Types

### Testing Sentinel Errors

```go
func TestLoadComponent_MissingName(t *testing.T) {
    _, err := LoadComponent("")

    // Use errors.Is for sentinel checks
    assert.ErrorIs(t, err, ErrMissingComponent)
}
```

### Testing Error Types

```go
func TestExecuteCommand_ExitCode(t *testing.T) {
    err := ExecuteCommand("false", []string{})

    require.Error(t, err)

    // Use errors.As for type-specific checks
    var execErr *ExecError
    require.True(t, errors.As(err, &execErr), "error should be ExecError")

    assert.Equal(t, "false", execErr.Cmd)
    assert.Equal(t, 1, execErr.ExitCode)
}
```

### Testing Error Chains

```go
func TestWorkflowError_Chain(t *testing.T) {
    baseErr := Build(ErrWorkflowStepFailed).
        WithHint("test hint").
        Err()

    workflowErr := NewWorkflowStepError("test-workflow", "step1", "cmd", 1, baseErr)

    // Check sentinel through chain
    assert.ErrorIs(t, workflowErr, ErrWorkflowStepFailed)

    // Check error type
    var wfErr *WorkflowStepError
    require.True(t, errors.As(workflowErr, &wfErr))
    assert.Equal(t, "test-workflow", wfErr.Workflow)
    assert.Equal(t, 1, wfErr.ExitCode)
}
```

## References

### Go Error Handling Resources
- [Go Blog: Working with Errors in Go 1.13](https://go.dev/blog/go1.13-errors)
- [Go Blog: Error handling and Go](https://go.dev/blog/error-handling-and-go)
- [errors package documentation](https://pkg.go.dev/errors)

### Atmos Error Documentation
- [Error Handling Strategy](./error-handling-strategy.md)
- [Error Builder Pattern](../errors.md)
- [Sentry Integration](../errors.md#sentry-integration)

### Related PRDs
- [Command Registry Pattern](./command-registry-pattern.md) - Error handling in commands
- [Testing Strategy](./testing-strategy.md) - Testing error handling

## Conclusion

The combination of sentinel errors and error types provides:
- **Type safety** via `errors.Is()` and `errors.As()`
- **Rich context** via structured error types
- **Exit code preservation** critical for CI/CD
- **Error categorization** via the type hierarchy
- **Flexible formatting** based on error type priority

Follow these patterns consistently to maintain robust, debuggable error handling throughout Atmos.
