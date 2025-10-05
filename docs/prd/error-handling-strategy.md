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
- ✅ Always use static errors defined in `errors/errors.go`
- ✅ Use `errors.Join` when combining multiple error values
- ✅ Use `fmt.Errorf` with `%w` when adding string context
- ✅ Preserve error chains for `errors.Is()` and `errors.As()`
- ✅ Add meaningful context that aids debugging

### Don'ts
- ❌ Never use multiple `%w` verbs in a single `fmt.Errorf`
- ❌ Avoid creating dynamic errors with `errors.New()` (except for static definitions)
- ❌ Don't use `fmt.Errorf` without `%w` unless converting a string to an error
- ❌ Don't lose error context by converting errors to strings unnecessarily

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
