# Atmos Error Handling Architecture

## Executive Summary

This document describes the architecture of Atmos's error handling system, which provides comprehensive error management with user-facing hints, structured context for debugging, and optional error reporting via Sentry.

## Goals

1. **Developer Experience**: Clear, actionable error messages with helpful hints
2. **Debuggability**: Rich error context and stack traces for troubleshooting
3. **Observability**: Optional error reporting to Sentry for production monitoring
4. **Idiomatic Go**: Leverage standard Go error patterns with enhancements
5. **Privacy**: Ensure no PII or sensitive data in error reports

## Architecture Overview

### Core Components

```
errors/
├── exit_code.go          # Exit code wrapper and extraction
├── builder.go            # Fluent API for rich errors
├── formatter.go          # TTY-aware error display
└── sentry.go            # Sentry integration
```

### Dependencies

- **cockroachdb/errors**: Foundation for error handling
  - Automatic stack traces
  - Network-portable errors
  - PII-safe context (`WithSafeDetails`)
  - User-facing hints (`WithHint`)
  - Built-in Sentry integration

- **getsentry/sentry-go**: Error reporting SDK
  - Event capture with context
  - Breadcrumbs for hints
  - Tags for structured data

- **charmbracelet/lipgloss**: Styled terminal output
  - Color support
  - TTY detection

## Design Decisions

### 1. Use cockroachdb/errors Directly (No Custom Wrapper)

**Decision**: Use `cockroachdb/errors` as a drop-in replacement for standard Go errors without creating a custom `AtmosError` type.

**Rationale**:
- Idiomatic Go - works with standard `error` interface
- No additional abstraction layer
- Full compatibility with `errors.Is()`, `errors.As()`, `errors.Unwrap()`
- Builder pattern provides convenience when needed

**Example**:
```go
// Direct usage
err := errors.New("base error")
err = errors.WithHint(err, "Check configuration")

// Builder for complex cases
err := errUtils.Build(errors.New("base error")).
    WithHint("Check configuration").
    WithContext("component", "vpc").
    WithExitCode(2).
    Err()
```

### 2. Static Errors as Sentinels

**Decision**: Define all base errors as static values in `errors/errors.go`.

**Rationale**:
- Enables `errors.Is()` checks
- Prevents dynamic error creation
- Centralizes error definitions
- Supports linting for error usage

**Example**:
```go
// errors/errors.go
var (
    ErrInvalidComponent = errors.New("invalid component")
    ErrInvalidStack     = errors.New("invalid stack")
)

// Usage
return fmt.Errorf("%w: component=%s", errUtils.ErrInvalidComponent, component)
```

### 3. Exit Code Wrapper Pattern

**Decision**: Use wrapper type with `Unwrap()` method for exit codes.

**Rationale**:
- Preserves error chain for `errors.Is()` and `errors.As()`
- Allows extraction with `errors.As()`
- Compatible with `exec.ExitError`
- No impact on error messages

**Implementation**:
```go
type exitCoder struct {
    cause error
    code  int
}

func (e *exitCoder) Unwrap() error { return e.cause }
func (e *exitCoder) ExitCode() int { return e.code }
```

### 4. Builder Pattern for Complex Errors

**Decision**: Optional builder for errors with multiple enrichments.

**Rationale**:
- Fluent API for readability
- No performance impact for simple errors
- Composable enrichments
- Nil-safe error handling

**Example**:
```go
err := Build(baseErr).
    WithHint("Check credentials").
    WithHintf("Verify connectivity to %s", host).
    WithContext("component", "vpc").
    WithExitCode(2).
    Err()
```

### 5. Smart Error Formatting

**Decision**: Automatic error formatting with TTY detection and wrapping.

**Rationale**:
- Adapts to terminal capabilities
- Wraps long error chains
- Color output when appropriate
- Verbose mode for debugging

**Features**:
- Auto-detect TTY for color
- Wrap messages at 80 chars
- Display hints with 💡 emoji
- Collapsed vs verbose modes

### 6. Sentry Integration with Full Context

**Decision**: Automatic extraction of hints, context, and stack traces for Sentry.

**Rationale**:
- Centralized error monitoring
- Structured error data
- Hints as breadcrumbs
- Context as tags
- PII-safe reporting

**Mapping**:
- Error hints → Sentry breadcrumbs (category: "hint")
- Safe details → Sentry tags (prefix: "error.")
- Atmos context → Sentry tags (prefix: "atmos.")
- Exit codes → Sentry tag "atmos.exit_code"
- Stack traces → Full error chain

## Data Flow

### Error Creation

```
1. Create base error (static or dynamic)
   ↓
2. Optionally enrich with builder
   - Add hints
   - Add safe context
   - Add exit code
   ↓
3. Return enriched error
```

### Error Display

```
1. Receive error in CLI command
   ↓
2. Check configuration
   - Verbose mode?
   - Color mode?
   ↓
3. Format error
   - Extract hints
   - Wrap long messages
   - Apply colors (if enabled)
   ↓
4. Display to stderr
```

### Error Reporting

```
1. Check Sentry enabled
   ↓
2. Extract error data
   - Get hints → breadcrumbs
   - Get safe details → tags
   - Get Atmos context → tags
   - Get exit code → tag
   ↓
3. Create Sentry event
   ↓
4. Send to Sentry (async)
```

## Configuration Schema

```yaml
errors:
  format:
    verbose: bool        # Show full stack traces
    color: string       # "auto", "always", "never"
  sentry:
    enabled: bool
    dsn: string
    environment: string
    release: string
    sample_rate: float64
    debug: bool
    tags: map[string]string
    capture_stack_context: bool
```

## Error Categories

### 1. Static Errors (Sentinel)

Defined in `errors/errors.go`:
```go
var ErrInvalidComponent = errors.New("invalid component")
```

Use for:
- Domain errors
- Validation errors
- Expected error conditions

### 2. Wrapped Errors

Add context to static errors:
```go
fmt.Errorf("%w: component=%s stack=%s", ErrInvalidComponent, comp, stack)
```

Use for:
- Adding runtime context
- Preserving error type
- Building error chains

### 3. Builder-Enhanced Errors

Complex errors with multiple enrichments:
```go
Build(err).WithHint(...).WithContext(...).WithExitCode(...).Err()
```

Use for:
- User-facing errors
- Errors needing hints
- Errors with structured context
- Errors requiring custom exit codes

## Exit Code Strategy

### Standard Codes

- `0`: Success
- `1`: General error (default)
- `2`: Usage error (invalid args, config)
- `N`: Application-specific

### Extraction Priority

1. Custom exit code (via `WithExitCode`)
2. `exec.ExitError` exit code
3. Default (1)

### Implementation

```go
func GetExitCode(err error) int {
    if err == nil {
        return 0
    }

    // Check custom exit code
    var ec *exitCoder
    if errors.As(err, &ec) {
        return ec.ExitCode()
    }

    // Check exec.ExitError
    var exitErr *exec.ExitError
    if errors.As(err, &exitErr) {
        return exitErr.ExitCode()
    }

    return 1  // Default
}
```

## Privacy & Security

### PII-Safe Context

Use `WithSafeDetails()` or builder's `WithContext()` for error reporting:
```go
// ✅ Safe
.WithContext("component", "vpc")
.WithContext("stack", "prod")

// ❌ Unsafe - contains credentials
.WithContext("password", userPassword)
```

### Hint Guidelines

Hints should be:
- Generic and actionable
- Free of sensitive data
- Helpful for troubleshooting

```go
// ✅ Good
.WithHint("Check database credentials in atmos.yaml")

// ❌ Bad
.WithHint("Failed with password: " + password)
```

## Testing Strategy

### Unit Tests

Test each component independently:
- Exit code wrapper
- Error builder
- Formatter
- Sentry integration

### Coverage

Target: 100% for error handling code

Required tests:
- Nil error handling
- Error chaining
- Hint extraction
- Context extraction
- Exit code extraction
- Formatting modes

### Mocking

Use disabled Sentry for tests:
```go
config := &schema.SentryConfig{
    Enabled: false,
}
```

## Migration Path

### Phase 1: Foundation (Current PR)

- ✅ Add dependencies
- ✅ Implement exit codes
- ✅ Implement builder
- ✅ Implement formatter
- ✅ Implement Sentry integration
- ✅ Add configuration schema
- ✅ Create documentation

### Phase 2: Hint Migration (Future PR)

- Update existing errors to use static errors
- Add hints to common error cases
- Target: 80-90% hint coverage

### Phase 3: Hint Addition (Future PR)

- Identify missing hints
- Add comprehensive hints
- Update documentation

## Performance Considerations

### Minimal Overhead

- Static errors: No allocation
- Builder: Allocates only when used
- Formatting: Lazy evaluation
- Sentry: Async sending

### Memory Usage

- Pass large configs by pointer
- Reuse formatter config
- Cache static errors

### Benchmarks

Target:
- Error creation: < 100ns (static)
- Builder enrichment: < 1μs
- Formatting: < 10μs
- Sentry capture: < 100μs (async)

## Future Enhancements

1. **Error Metrics**: Track error frequency and types
2. **Error Patterns**: Common error pattern detection
3. **Auto-Recovery**: Suggest automatic fixes for common errors
4. **Error Search**: CLI command to search error documentation
5. **Error Analytics**: Dashboard for error trends

## References

- [cockroachdb/errors](https://github.com/cockroachdb/errors)
- [Sentry Go SDK](https://docs.sentry.io/platforms/go/)
- [Error Handling in Go](https://go.dev/blog/error-handling-and-go)
- [Error Handling Strategy PRD](error-handling-strategy.md)
- [Developer Guide](../errors.md)
- [User Guide](../../website/docs/core-concepts/errors.mdx)
