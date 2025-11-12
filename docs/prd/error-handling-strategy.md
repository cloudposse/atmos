# Error Handling Strategy PRD

## Executive Summary

This document defines the error handling strategy for the Atmos codebase, addressing the evolution of Go's error handling capabilities from Go 1.13 onwards and establishing clear patterns for error creation, wrapping, and chaining.

## Problem Statement

### Background
Go 1.13 introduced significant improvements to error handling:
- The `%w` verb in `fmt.Errorf` for error wrapping
- The `errors.Is()` and `errors.As()` functions for error inspection
- The `errors.Join()` function (Go 1.20) for combining multiple errors

### Current Challenges
1. **Invalid error wrapping patterns**: The codebase previously used `fmt.Errorf` with multiple `%w` verbs (e.g., `"%w: %w"`), which is invalid in Go and breaks error chains
2. **Linter false positives**: The err113 linter flags legitimate uses of `fmt.Errorf` when adding context to errors
3. **Inconsistent patterns**: Mixed approaches to error handling make the codebase harder to maintain
4. **Loss of error context**: Improper error handling can lose valuable debugging information

## Design Goals

1. **Preserve error chains**: Ensure `errors.Is()` and `errors.As()` work correctly throughout the codebase
2. **Maintain context**: Add meaningful context to errors without breaking error chains
3. **Satisfy tooling**: Configure linters to support legitimate patterns without excessive exceptions
4. **Clear guidelines**: Establish unambiguous patterns for different error handling scenarios
5. **Pragmatic approach**: Balance correctness with readability and maintainability

## Technical Specification

### Error Handling Patterns

#### Pattern 0: ErrorBuilder with Sentinel Errors (MANDATORY for User-Facing Errors)

**This is THE primary pattern for all user-facing errors in Atmos.**

Atmos uses CockroachDB's `errors.Mark()` to attach sentinel errors to error chains, enabling `errors.Is()` checks to work through multiple layers of wrapping. The ErrorBuilder provides a fluent API that automatically handles sentinel marking.

**Two supported patterns:**

```go
// Pattern A: Sentinel as base error (auto-marked)
err := errUtils.Build(errUtils.ErrContainerRuntimeOperation).
    WithExplanation("Failed to start container").
    WithHint("Check Docker is running").
    WithContext("container", containerName).
    WithExitCode(3).
    Err()

// errors.Is(err, ErrContainerRuntimeOperation) ✅ true (auto-marked)

// Pattern B: Wrap actual error + explicit sentinel
err := errUtils.Build(actualError).
    WithSentinel(errUtils.ErrContainerRuntimeOperation).
    WithHint("Check Docker is running").
    Err()

// errors.Is(err, ErrContainerRuntimeOperation) ✅ true (explicitly marked)
// errors.Is(err, actualError) ✅ true (both preserved)
```

**Testing errors (MANDATORY):**
```go
// ✅ CORRECT: Always use errors.Is() in tests
assert.ErrorIs(t, err, errUtils.ErrContainerRuntimeOperation)

// ❌ WRONG: Never use string matching - breaks with wrapping
assert.Contains(t, err.Error(), "container runtime")
```

**Why ErrorBuilder + Sentinel Errors?**
1. **Type-safe error checking**: `errors.Is()` works across wrapped errors
2. **Prevents typos**: Compile-time checking vs runtime string matching
3. **Testable**: Clear, predictable error assertions with `assert.ErrorIs()`
4. **Maintainable**: Errors centralized in `errors/errors.go`
5. **User-friendly**: Rich context with explanations, hints, and examples

See [Error Handling Guide](../errors.md) for complete ErrorBuilder API documentation.

#### Pattern 1: Combining Multiple Errors
When you have two or more error values to combine, use `errors.Join`:

```go
// ✅ CORRECT: Combining two errors
return errors.Join(errUtils.ErrFailedToUpload, underlyingErr)

// ✅ CORRECT: Combining multiple errors
return errors.Join(errUtils.ErrInvalidConfig, errUtils.ErrMissingField, validationErr)

// ❌ WRONG: Using fmt.Errorf with multiple %w (invalid in Go)
return fmt.Errorf("%w: %w", errUtils.ErrFailedToUpload, underlyingErr)
```

#### Pattern 2: Adding String Context to Errors
When adding descriptive string context to an error, use `fmt.Errorf` with a single `%w`:

```go
// ✅ CORRECT: Adding formatted context
return fmt.Errorf("%w: component=%s stack=%s", errUtils.ErrInvalidComponent, component, stack)

// ✅ CORRECT: Adding path context
return fmt.Errorf("%w: %s", errUtils.ErrFileNotFound, filepath)

// ✅ CORRECT: Adding descriptive message
return fmt.Errorf("%w: failed to process configuration for %s", errUtils.ErrInvalidConfig, configName)
```

#### Pattern 3: Converting Strings to Errors
When you need to add string context to an error, prefer `fmt.Errorf` with `%w`:

```go
// ✅ PREFERRED: Use fmt.Errorf with %w for adding string context
return fmt.Errorf("%w: flag: %s", errUtils.ErrInvalidFlag, arg)
return fmt.Errorf("%w: invalid value: %s", errUtils.ErrValidation, value)

// ⚠️ AVOID: errors.Join with fmt.Errorf (triggers err113 linter)
return errors.Join(errUtils.ErrValidation, fmt.Errorf("%s", value))
```

#### Pattern 4: Static Error Definitions
All base errors must be defined as static package-level variables:

```go
// ✅ CORRECT: In errors/errors.go
var (
    ErrInvalidComponent = errors.New("invalid component")
    ErrFailedToProcess = errors.New("failed to process")
    ErrConfigNotFound = errors.New("configuration not found")
)

// ❌ WRONG: Dynamic error creation
return errors.New("something went wrong")
```

### Error Wrapping Decision Tree

```
Do you have multiple errors to combine?
├─ YES → Use errors.Join(err1, err2, ...)
└─ NO → Do you need to add string context?
    ├─ YES → Use fmt.Errorf("%w: context", err)
    └─ NO → Return the error directly
```

### Linter Configuration

Configure `.golangci.yml` to allow legitimate error patterns:

```yaml
issues:
  exclude-rules:
    # Allow fmt.Errorf when wrapping static errors from errUtils package
    - linters:
        - err113
      source: 'fmt\.Errorf\("\%w: .+", errUtils\.'

    # Allow fmt.Errorf when wrapping any static Err* variable
    - linters:
        - err113
      source: 'fmt\.Errorf\("\%w: .+", Err[A-Z]'

    # Allow errors.Join with fmt.Errorf for converting strings to errors
    - linters:
        - err113
      source: 'errors\.Join\(.*, fmt\.Errorf'
```

## Implementation Guidelines

### Do's
- ✅ **ALWAYS use ErrorBuilder for user-facing errors** (see Pattern 0)
- ✅ **ALWAYS use `errors.Is()` for error checking** - never string matching
- ✅ **ALWAYS use sentinel errors** defined in `errors/errors.go`
- ✅ **ALWAYS use `assert.ErrorIs()` in tests** - never `assert.Contains(err.Error(), ...)`
- ✅ Use `errors.Join` when combining multiple error values
- ✅ Use `fmt.Errorf` with `%w` when adding string context
- ✅ Preserve error chains for `errors.Is()` and `errors.As()`
- ✅ Add meaningful context that aids debugging
- ✅ Log squelched errors at Trace level (see Squelched Error Handling below)

### Don'ts
- ❌ **NEVER use string-based error checking** (`assert.Contains(err.Error(), ...)`)
- ❌ **NEVER use string matching** (`if err.Error() == "..."` or `strings.Contains(err.Error(), ...)`)
- ❌ **NEVER create dynamic errors** with `errors.New()` (except for static sentinel definitions)
- ❌ Never use multiple `%w` verbs in a single `fmt.Errorf`
- ❌ Don't use `fmt.Errorf` without `%w` unless converting a string to an error
- ❌ Don't lose error context by converting errors to strings unnecessarily
- ❌ Never squelch errors silently with `_ = ...` - always log at Trace level

## Migration Strategy

### Phase 1: Fix Invalid Patterns (Completed)
- Replace all `fmt.Errorf` with multiple `%w` verbs with `errors.Join`
- Ensure error chains are preserved

### Phase 2: Configure Tooling (Current)
- Update `.golangci.yml` to allow legitimate patterns
- Avoid excessive `//nolint` comments

### Phase 3: Refine Patterns (Completed)
- Refactored redundant error conversions
- Simplified `errors.Join(err, fmt.Errorf("%v", err2))` to `errors.Join(err, err2)`
- Refactored `errors.Join(staticErr, fmt.Errorf("text: %s", arg))` to `fmt.Errorf("%w: text: %s", staticErr, arg)`

### Phase 4: Documentation and Education
- Update CLAUDE.md with error handling guidelines
- Ensure all developers understand the patterns

## Squelched Error Handling

### Definition

Squelched errors are errors that are intentionally ignored because they don't affect the critical execution path. However, they **must always be logged at Trace level** to maintain complete error visibility.

### When to Squelch Errors

Errors should only be squelched when:
1. **Non-critical cleanup** - Removing temporary files, closing file handles
2. **Best-effort operations** - Optional configuration binding, UI rendering
3. **Defer statements** - Resource cleanup where error handling would be complex
4. **Recovery is impossible** - Already in error path or cleanup code

### The Golden Rule

**Never squelch errors silently.** Every squelched error must be logged at Trace level.

### Pattern: Squelched Error Logging

```go
// ❌ WRONG: Silent error squelching
_ = os.Remove(tempFile)
_ = file.Close()
_ = viper.BindEnv("VAR", "ENV_VAR")

// ✅ CORRECT: Log squelched errors at Trace level
if err := os.Remove(tempFile); err != nil && !os.IsNotExist(err) {
    log.Trace("Failed to remove temporary file during cleanup", "error", err, "file", tempFile)
}

if err := file.Close(); err != nil {
    log.Trace("Failed to close file", "error", err, "file", file.Name())
}

if err := viper.BindEnv("VAR", "ENV_VAR"); err != nil {
    log.Trace("Failed to bind environment variable", "error", err, "var", "VAR")
}
```

### Special Cases

#### Defer Statements
Capture errors in a closure for logging:

```go
defer func() {
    if err := lock.Unlock(); err != nil {
        log.Trace("Failed to unlock", "error", err, "path", lockPath)
    }
}()
```

#### File Removal
Check for `os.IsNotExist` to avoid logging expected conditions:

```go
if err := os.Remove(tempFile); err != nil && !os.IsNotExist(err) {
    log.Trace("Failed to remove file", "error", err, "file", tempFile)
}
```

#### Log File Cleanup
Use stderr to avoid logger recursion:

```go
func cleanupLogFile() {
    if logFileHandle != nil {
        if err := logFileHandle.Sync(); err != nil {
            // Don't use logger here as we're cleaning up the log file
            fmt.Fprintf(os.Stderr, "Warning: failed to sync log file: %v\n", err)
        }
    }
}
```

### Benefits

1. **Complete Error Visibility**: No errors are truly lost, even if intentionally ignored
2. **Debugging Support**: Trace logs provide full context when investigating issues
3. **Pattern Detection**: Aggregated trace logs can reveal systemic problems
4. **Auditing**: Complete error trail for compliance and security reviews

### Common Squelched Error Categories

| Operation Type | Examples | Trace Logging Pattern |
|---------------|----------|---------------------|
| File cleanup | `os.Remove()`, `os.RemoveAll()` | Check `os.IsNotExist()` |
| Resource closing | `file.Close()`, `client.Close()` | Log all errors |
| Lock operations | `lock.Unlock()` | Capture in defer closure |
| Config binding | `viper.BindEnv()`, `viper.BindPFlag()` | Log all errors |
| UI operations | `fmt.Fprint()`, `cmd.Help()` | Log all errors |

---

## Examples

### Example 1: API Error Handling
```go
// Before (invalid):
return fmt.Errorf("%w: %w", errUtils.ErrAPIFailed, errUtils.ErrInvalidResponse)

// After (correct):
return errors.Join(errUtils.ErrAPIFailed, errUtils.ErrInvalidResponse)
```

### Example 2: File Operations
```go
// Correct - adding path context:
return fmt.Errorf("%w: %s", errUtils.ErrFileNotFound, filepath)

// When you have multiple errors to chain:
return errors.Join(
    errUtils.ErrFileOperation,
    underlyingErr,
)

// When adding context with another error:
return fmt.Errorf("%w: failed to read %s", errUtils.ErrFileOperation, filepath)
```

### Example 3: Validation Errors
```go
// Combining validation results:
var errs []error
if component == "" {
    errs = append(errs, errUtils.ErrMissingComponent)
}
if stack == "" {
    errs = append(errs, errUtils.ErrMissingStack)
}
if len(errs) > 0 {
    return errors.Join(errs...)
}
```

### Example 4: Squelched Error Handling
```go
// File cleanup with proper trace logging:
func processComponent(component string) error {
    tempFile, err := os.CreateTemp("", "atmos-*")
    if err != nil {
        return fmt.Errorf("%w: %v", errUtils.ErrCreateTempFile, err)
    }
    defer func() {
        if err := os.Remove(tempFile.Name()); err != nil && !os.IsNotExist(err) {
            log.Trace("Failed to remove temporary file during cleanup", "error", err, "file", tempFile.Name())
        }
    }()
    defer func() {
        if err := tempFile.Close(); err != nil {
            log.Trace("Failed to close temporary file", "error", err, "file", tempFile.Name())
        }
    }()

    // Process component...
    return nil
}

// Configuration binding with proper trace logging:
func init() {
    if err := viper.BindEnv("component_path", "ATMOS_COMPONENT_PATH"); err != nil {
        log.Trace("Failed to bind component_path environment variable", "error", err)
    }
    if err := viper.BindPFlag("stack", cmd.Flags().Lookup("stack")); err != nil {
        log.Trace("Failed to bind stack flag", "error", err)
    }
}
```

## Testing Requirements

1. **Error Chain Preservation**: Tests must verify that `errors.Is()` works correctly
2. **Context Preservation**: Ensure error messages contain expected context
3. **Linter Compliance**: All code must pass `make lint` without warnings
4. **Cross-platform**: Error handling must work on Linux, macOS, and Windows

## Success Criteria

1. No invalid `fmt.Errorf` patterns with multiple `%w` verbs
2. Linter configured to allow legitimate patterns without excessive exceptions
3. Clear, documented patterns that developers can follow
4. Error chains preserved for proper error inspection
5. Meaningful error context throughout the codebase

## References

- [Go 1.13 Error Handling](https://go.dev/blog/go1.13-errors)
- [Go errors.Join Documentation](https://pkg.go.dev/errors#Join)
- [err113 Linter Documentation](https://github.com/Djarvur/go-err113)
- [Atmos Error Package](../errors/errors.go)

## Revision History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2025-09-28 | System | Initial PRD documenting error handling strategy |
| 1.1 | 2025-09-28 | System | Updated Pattern 3 to prefer fmt.Errorf over errors.Join for string context to satisfy err113 linter |
| 1.2 | 2025-10-11 | System | Added Squelched Error Handling section with patterns and examples for logging intentionally ignored errors at Trace level |
