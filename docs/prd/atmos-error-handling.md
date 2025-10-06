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

### 2. Builder-Enhanced Errors

Use builder for structured context and enrichments:
```go
Build(err).
    WithContext("component", "vpc").
    WithContext("stack", "prod").
    WithHint("Check configuration").
    WithExitCode(2).
    Err()
```

Use for:
- Structured, programmatic context
- User-facing errors with hints
- Errors requiring custom exit codes
- Context displayed in verbose mode

### 3. Simple Wrapped Errors

Add descriptive text to errors (when structured context not needed):
```go
fmt.Errorf("%w: failed to process configuration", ErrInvalidComponent)
```

Use for:
- Simple error descriptions
- Preserving error type
- When programmatic context access not needed

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

Use builder's `WithContext()` for structured, programmatic context:
```go
// ✅ Safe - component/stack names
.WithContext("component", "vpc")
.WithContext("stack", "prod")
.WithContext("region", "us-east-1")

// ❌ Unsafe - contains credentials or PII
.WithContext("password", userPassword)
.WithContext("api_key", apiKey)
.WithContext("email", userEmail)
```

**Context Usage:**
- **Verbose Mode**: Displayed as styled 2-column table
- **Programmatic Access**: Via `errors.GetSafeDetails(err)`
- **Sentry Integration**: Automatically sent as structured tags
- **Debug Output**: Included in `%+v` formatting

**Example Verbose Output:**
```
component not found

┏━━━━━━━━━━━┳━━━━━━━━━━━┓
┃ Context   ┃ Value     ┃
┣━━━━━━━━━━━╋━━━━━━━━━━━┫
┃ component ┃ vpc       ┃
┃ region    ┃ us-east-1 ┃
┃ stack     ┃ prod      ┃
┗━━━━━━━━━━━┻━━━━━━━━━━━┛
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

### Phase 1: Foundation ✅ Complete

- ✅ Add dependencies (cockroachdb/errors)
- ✅ Implement exit codes
- ✅ Implement builder with hints and context
- ✅ Implement formatter with color support
- ✅ Implement Sentry integration
- ✅ Add configuration schema
- ✅ Create documentation

### Phase 2: Component & Stack Hints ✅ Complete

- ✅ Component discovery errors (5 scenarios)
  - `ErrComponentNotFound` - suggests checking component name and paths
  - `ErrComponentTypeNotValid` - suggests valid types (terraform, helmfile)
  - `ErrInvalidComponent` - provides validation details
  - `ErrStackNotFound` - suggests checking stack name
  - `ErrInvalidStack` - provides validation details

### Phase 3: Workflow & Vendor Hints ✅ Complete

- ✅ Workflow errors (2 scenarios)
  - Workflow file not found - suggests checking path and existence
  - Workflow syntax errors - suggests validation
- ✅ Vendor errors (2 scenarios)
  - Package errors - suggests checking vendor.yaml
  - Missing sources - provides configuration examples

### Phase 4: Validation Hints ✅ Complete

- ✅ Schema validation errors (3 scenarios)
  - OPA policy failures - shows policy violations
  - JSON schema failures - shows validation errors
  - Stack validation - provides specific error details
- ✅ Advanced error scenarios (2 scenarios)
  - Template rendering errors - suggests checking template syntax
  - Backend configuration - suggests checking backend settings

### Phase 5: Markdown Integration ✅ Complete

- ✅ Use configured Atmos markdown renderer from `atmos.yaml`
- ✅ Apply 4-space indentation (consistent with `LevelIndent: 4`)
- ✅ Support custom markdown styles for hints
- ✅ Graceful fallback to plain text when config unavailable
- ✅ Emoji and markdown formatting in hints

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
- [User Guide](../../website/docs/cli/errors.mdx)
