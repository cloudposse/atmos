# Error Handling Guide

Complete guide to error handling in Atmos using the ErrorBuilder pattern with sentinel errors.

## Quick Start

### Creating Errors

Use the ErrorBuilder for all user-facing errors:

```go
import errUtils "github.com/cloudposse/atmos/errors"

// PREFERRED: Sentinel with underlying cause (stdlib compatible)
err := runtime.Start(ctx, containerID) // returns "container already running"
return errUtils.Build(errUtils.ErrContainerRuntimeOperation).
    WithCause(err).
    WithExplanation("Failed to start container").
    WithHint("Check Docker is running").
    WithContext("container", containerName).
    WithExitCode(3).
    Err()

// ALSO VALID: Sentinel as base (when no underlying error)
err := errUtils.Build(errUtils.ErrContainerRuntimeOperation).
    WithExplanation("Failed to start container").
    WithHint("Check Docker is running").
    Err()
```

### Testing Errors

**ALWAYS use `errors.Is()` in tests, NEVER string matching:**

```go
// ✅ CORRECT
assert.ErrorIs(t, err, errUtils.ErrContainerRuntimeOperation)

// ❌ WRONG - breaks with error wrapping
assert.Contains(t, err.Error(), "container runtime")
```

## Core Concepts

### Sentinel Errors

Sentinel errors are package-level error constants that act as error types. They enable type-safe error checking with `errors.Is()`.

**All sentinels are defined in `errors/errors.go`:**

```go
var (
    ErrComponentNotFound = errors.New("component not found")
    ErrInvalidStack      = errors.New("invalid stack")
    ErrConfigNotFound    = errors.New("configuration not found")
)
```

### Sentinel Marking with `errors.Mark()`

Atmos uses CockroachDB's `errors.Mark()` to attach sentinel errors to error chains. This allows `errors.Is()` checks to work even through multiple layers of wrapping.

**How it works:**
```go
// Create base error
baseErr := errors.New("connection refused")

// Mark it with a sentinel
err := errors.Mark(baseErr, ErrContainerRuntimeOperation)

// errors.Is() works for both!
errors.Is(err, ErrContainerRuntimeOperation) // ✅ true (marked)
errors.Is(err, baseErr)                      // ✅ true (original)
```

**ErrorBuilder does this automatically:**
```go
// When you use a sentinel as base, it's auto-marked
err := errUtils.Build(ErrContainerRuntimeOperation).
    WithHint("Check Docker").
    Err()

// errors.Is(err, ErrContainerRuntimeOperation) ✅ true (auto-marked)

// When you wrap another error, use WithCause()
err := errUtils.Build(ErrContainerRuntimeOperation).
    WithCause(actualError).
    Err()

// errors.Is(err, ErrContainerRuntimeOperation) ✅ true (sentinel)
// errors.Is(err, actualError) ✅ true (cause preserved)
```

## ErrorBuilder API

### Build(err error) *ErrorBuilder

Creates a new ErrorBuilder from a base error.

**Auto-detection:** If the error is a sentinel (leaf error with no wrapping), it's automatically marked.

```go
// Auto-marked as sentinel
builder := errUtils.Build(errUtils.ErrComponentNotFound)

// NOT auto-marked (already wrapped)
wrapped := errors.Wrap(someErr, "context")
builder := errUtils.Build(wrapped)
```

### WithExplanation(explanation string) *ErrorBuilder

**Explains WHAT WENT WRONG** - the technical cause or reason for the error.

```go
builder.WithExplanation("Failed to connect to the container runtime daemon")
builder.WithExplanationf("Connection error: %v", err)
```

**When to use multiple explanations:**
- First explanation: High-level what went wrong
- Subsequent explanations: Technical details, error messages from underlying systems

### WithHint(hint string) *ErrorBuilder

**Tells users WHAT TO DO** - actionable steps to resolve the error (displayed with 💡 emoji).

```go
builder.WithHint("Ensure Docker or Podman is installed and running")
builder.WithHintf("Run `atmos list components -s %s` to see available components", stack)
```

**Critical: Use single multi-line hint for sequential steps:**

```go
// ❌ WRONG - Creates visual clutter with too many 💡 icons
builder.
    WithHint("Add devcontainer configuration in atmos.yaml:").
    WithHint("  components:").
    WithHint("    devcontainer:").
    WithHint("      <name>:").
    WithHint("        spec:")

// ✅ CORRECT - Use single multi-line hint
builder.WithHint(`Add devcontainer configuration in atmos.yaml:
  components:
    devcontainer:
      <name>:
        spec:
          image: <docker-image>`)

// ✅ CORRECT - Use multiple hints for DIFFERENT actions
builder.
    WithHint("Verify the image name is correct").
    WithHint("Check your internet connection").
    WithHint("Authenticate with the registry if private: `docker login`")
```

### WithExample(example string) *ErrorBuilder

**Adds configuration or code examples** - displayed in dedicated "Example" section.

```go
builder.WithExample(`components:
  devcontainer:
    default:
      spec:
        image: "ubuntu:22.04"`)
```

**When to use examples:**
- Configuration file syntax (YAML, JSON, HCL)
- Code snippets showing correct usage
- Multi-line examples that would clutter hints

### WithContext(key string, value interface{}) *ErrorBuilder

Adds structured context (displayed as table in verbose mode).

```go
builder.
    WithContext("component", component).
    WithContext("stack", stack).
    WithContext("region", "us-east-1")
```

### WithCause(cause error) *ErrorBuilder

**PREFERRED METHOD** - Wraps the sentinel with an underlying cause error, preserving the original error message while allowing `errors.Is()` to match the sentinel.

This is the recommended pattern for wrapping errors from external libraries (Docker, Podman, AWS SDK, etc.) because it:
- ✅ Works with stdlib `errors.Is()` (no need for cockroachdb/errors in tests)
- ✅ Preserves the actual error message from the underlying system
- ✅ Allows `errors.Is()` checks for both the sentinel and the cause
- ✅ Clean, fluent API

```go
// Runtime returns: "Error: container already started"
err := runtime.Start(ctx, containerID)
if err != nil {
    return errUtils.Build(errUtils.ErrContainerRuntimeOperation).
        WithCause(err).  // Preserves "container already started"
        WithExplanation("Failed to start container").
        WithHint("Use --replace flag to recreate the container").
        Err()
}

// Both checks work:
errors.Is(result, errUtils.ErrContainerRuntimeOperation)  // ✅ true
errors.Is(result, err)                                     // ✅ true
// Message includes: "container already started"
```

**When to use WithCause:**
- Wrapping errors from Docker/Podman runtime operations
- Wrapping errors from AWS SDK, Terraform, external commands
- Any case where you want to preserve the underlying error message
- When your tests use stdlib `errors.Is()` (most of our codebase)

### WithExitCode(code int) *ErrorBuilder

Sets a custom exit code (default is 1).

```go
builder.WithExitCode(2)  // Usage/configuration errors
```

**Standard exit codes:**
- `0`: Success
- `1`: General runtime error
- `2`: Usage/configuration error
- `3`: Infrastructure error (missing dependencies, environment issues)

### Err() error

Finalizes the error builder and returns the constructed error.

```go
return builder.Err()
```

## Testing Patterns

### Always Use `errors.Is()`

**MANDATORY: All error checking in tests must use `errors.Is()`:**

```go
// ✅ CORRECT
assert.ErrorIs(t, err, errUtils.ErrComponentNotFound)

// ❌ WRONG - breaks with wrapping, not portable
assert.Contains(t, err.Error(), "component not found")
assert.Equal(t, "component not found", err.Error())
if strings.Contains(err.Error(), "component") { ... }
```

### Testing Error Chains with WithCause

```go
causeErr := errors.New("connection refused")
err := errUtils.Build(errUtils.ErrContainerRuntimeOperation).
    WithCause(causeErr).
    Err()

// Both sentinel and cause work
assert.ErrorIs(t, err, errUtils.ErrContainerRuntimeOperation)
assert.ErrorIs(t, err, causeErr)
```

## Common Patterns

### Component Not Found

```go
err := errUtils.Build(errUtils.ErrComponentNotFound).
    WithExplanationf("Component `%s` not found in stack `%s`", component, stack).
    WithExplanationf("Searched in: %s", searchPath).
    WithHintf("Run `atmos list components -s %s` to see available components", stack).
    WithHint("Verify the component path in your `atmos.yaml` configuration").
    WithContext("component", component).
    WithContext("stack", stack).
    WithContext("path", searchPath).
    WithExitCode(2).
    Err()
```

### Configuration Error

```go
err := errUtils.Build(errUtils.ErrInvalidConfig).
    WithExplanationf("Invalid configuration in `%s`", configFile).
    WithExplanationf("Parse error at line %d: %v", lineNumber, parseErr).
    WithHint("Check the syntax and structure of your configuration file").
    WithHint("Run `atmos validate config` to verify your configuration").
    WithContext("file", configFile).
    WithContext("line", lineNumber).
    WithExitCode(2).
    Err()
```

### Container Runtime Error

```go
err := errUtils.Build(errUtils.ErrContainerRuntimeOperation).
    WithExplanation("Failed to connect to container runtime daemon").
    WithExplanationf("Connection error: %v", err).
    WithHint("Ensure Docker or Podman is installed and running").
    WithHint("Check if Docker daemon is accessible: `docker ps`").
    WithHint(`Start the container runtime:
• macOS/Windows: Launch Docker Desktop application
• Linux: sudo systemctl start docker`).
    WithContext("runtime", "docker or podman").
    WithExitCode(3).
    Err()
```

### Wrapping External Errors

```go
// Wrap sentinel with third-party error as cause
err := errUtils.Build(errUtils.ErrContainerRuntimeOperation).
    WithCause(externalErr).
    WithExplanationf("Docker pull failed: %v", externalErr).
    WithHint("Check your internet connection").
    WithHintf("Try pulling manually: `docker pull %s`", image).
    Err()

// Both work
errors.Is(err, errUtils.ErrContainerRuntimeOperation)  // ✅ true
errors.Is(err, externalErr)                           // ✅ true
```

## Anti-Patterns

### ❌ String-Based Error Checking

```go
// NEVER do this - breaks with wrapping
if err.Error() == "component not found" { ... }
if strings.Contains(err.Error(), "component") { ... }
assert.Contains(t, err.Error(), "component")
```

### ❌ Dynamic Errors

```go
// WRONG: Creates dynamic error, breaks errors.Is()
return errors.New(fmt.Sprintf("component %s not found", component))

// CORRECT: Use sentinel + context
return errUtils.Build(errUtils.ErrComponentNotFound).
    WithExplanationf("Component `%s` not found", component).
    Err()
```

### ❌ Missing Explanation and Hints

```go
// WRONG: No explanation or guidance
return errUtils.ErrComponentNotFound

// CORRECT: Add explanation and hints
return errUtils.Build(errUtils.ErrComponentNotFound).
    WithExplanationf("Component `%s` not found", component).
    WithHint("Run `atmos list components` to see available components").
    WithContext("component", component).
    Err()
```

### ❌ Config Examples in Hints

```go
// WRONG: Creates visual clutter with multiple 💡 icons
return errUtils.Build(errUtils.ErrDevcontainerNotFound).
    WithHint("Add devcontainer configuration in atmos.yaml:").
    WithHint("  components:").
    WithHint("    devcontainer:").
    WithHint("      <name>:").
    Err()

// CORRECT: Use WithExample() for config syntax
return errUtils.Build(errUtils.ErrDevcontainerNotFound).
    WithExplanation("No devcontainers configured in atmos.yaml").
    WithExample(`components:
  devcontainer:
    <name>:
      spec:
        image: <docker-image>`).
    WithHint("Add devcontainer configuration to your atmos.yaml").
    Err()
```

## Why Sentinel Errors?

1. **Type-safe error checking**: `errors.Is()` works across wrapped errors
2. **Prevents typos**: Compile-time checking vs runtime string matching
3. **Testable**: Clear, predictable error assertions
4. **Maintainable**: Errors are centralized in one place
5. **Portable**: Works across different error messages and formats

## Migration Checklist

When converting code to use ErrorBuilder:

- [ ] Replace string-based error creation with sentinel errors
- [ ] Use `Build()` + `WithCause()` for wrapping external/library errors
- [ ] Add `WithExplanation()` for technical details
- [ ] Add `WithHint()` for actionable guidance
- [ ] Add `WithContext()` for debugging information
- [ ] Set appropriate exit code with `WithExitCode()`
- [ ] Update tests to use `errors.Is()` instead of string matching
- [ ] Remove any `assert.Contains(err.Error(), ...)` patterns

## See Also

- [Error Handling Strategy PRD](prd/error-handling-strategy.md) - Architecture decisions
- [Error Types and Sentinels](prd/error-types-and-sentinels.md) - Complete error catalog
- [Exit Codes](prd/exit-codes.md) - Exit code standards
- [Implementation Plan](prd/atmos-error-implementation-plan.md) - Migration phases

## Summary

- ✅ **ALWAYS** use sentinel errors from `errors/errors.go`
- ✅ **ALWAYS** use `errors.Is()` for error checking
- ✅ **ALWAYS** use ErrorBuilder for user-facing errors
- ✅ **ALWAYS** add explanations and hints
- ❌ **NEVER** use string-based error checking
- ❌ **NEVER** create dynamic errors
- ❌ **NEVER** use `assert.Contains(err.Error(), ...)`
