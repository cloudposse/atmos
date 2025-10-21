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

```text
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

**Decision**: Optional builder for errors with multiple enrichments including explanations and examples.

**Rationale**:
- Fluent API for readability
- No performance impact for simple errors
- Composable enrichments
- Nil-safe error handling
- Support for rich error context (explanations, examples)

**Example**:
```go
import (
    errUtils "github.com/cloudposse/atmos/errors"
)

//go:embed examples/database_connection.md
var databaseConnectionExample string

err := errUtils.Build(baseErr).
    WithExplanation("Failed to establish connection to the database server.").
    WithExampleFile(databaseConnectionExample).
    WithHint("Check credentials").
    WithHintf("Verify connectivity to %s", host).
    WithContext("component", "vpc").
    WithExitCode(2).
    Err()
```

**Builder Methods**:
- `WithExplanation(string)` - Detailed description of what went wrong
- `WithExplanationf(format, args...)` - Formatted explanation
- `WithExample(string)` - Inline code/config example
- `WithExampleFile(string)` - Embedded markdown example (preferred)
- `WithHint(string)` - User-facing actionable suggestion
- `WithHintf(format, args...)` - Formatted hint
- `WithContext(key, value)` - Structured debugging context
- `WithExitCode(int)` - Custom exit code
- `Err()` - Returns the enriched error

### 5. Structured Markdown Error Presentation

**Decision**: Format errors as structured markdown with hierarchical sections rendered through Glamour.

**Rationale**:
- Visual hierarchy improves readability
- Sections organize different types of information
- Markdown provides formatting flexibility
- Conditional rendering keeps output clean
- Terminal rendering with Glamour adds color and style

**Section Structure**:
1. **# Error** - Main error title and message
2. **## Explanation** - Detailed description of what went wrong and why
3. **## Example** - Code/config examples showing correct usage
4. **## Hints** - Actionable suggestions for resolving the error
5. **## Context** - Structured debugging info as markdown table
6. **## Stack Trace** - Full error chain (verbose mode only)

**Features**:
- Auto-detect TTY for color
- Sections only render when data is available
- Display hints with 💡 emoji
- Context as clean markdown table
- Examples from embedded markdown files
- Collapsed vs verbose modes

**Example Output**:
```text
# Error

workflow file not found

## Explanation

The workflow manifest file `stacks/workflows/dne.yaml` does not exist.

## Example

```
# Verify the workflow file exists
ls -la stacks/workflows/

# Check your atmos.yaml configuration
cat atmos.yaml | grep -A5 workflows
```

## Hints

💡 Use `atmos list workflows` to see available workflows
💡 Verify the workflow file exists at: stacks/workflows/dne.yaml

## Context

| Key       | Value                     |
|-----------|---------------------------|
| file      | stacks/workflows/dne.yaml |
| base_path | stacks/workflows          |

```

### 6. Sentry Integration with Full Context

**Decision**: Automatic extraction of hints, context, and stack traces for Sentry.

**Rationale**:
- Centralized error monitoring
- Structured error data
- Hints as breadcrumbs
- Context as tags
- PII-safe reporting

**What Gets Sent to Sentry**:

Atmos only sends **command failures** to Sentry - errors that prevent a command from completing successfully and cause Atmos to exit with an error code.

- **Sent**: Command failures, validation errors that prevent deployment, workflow execution failures, authentication errors, file system errors
- **NOT sent**: Debug/trace logs, warnings, non-fatal errors that Atmos recovers from, successful commands (exit code 0)

**Mapping**:
- Error hints → Sentry breadcrumbs (category: "hint")
- Safe details → Sentry tags (prefix: "error.")
- Atmos context → Sentry tags (prefix: "atmos.")
- Exit codes → Sentry tag "atmos.exit_code"
- Stack traces → Full error chain

This ensures Sentry focuses on actionable failures that affect users, without overwhelming it with internal logging or successful operations.

## Data Flow

### Error Creation

```text
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

```text
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

```text
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

Use builder for rich errors with explanations, examples, hints, and context:
```go
//go:embed examples/component_not_found.md
var componentNotFoundExample string

Build(err).
    WithExplanation("Component could not be found in the stack configuration.").
    WithExampleFile(componentNotFoundExample).
    WithHint("Run 'atmos list components --stack prod' to see available components").
    WithHintf("Check the component name and stack: %s/%s", component, stack).
    WithContext("component", "vpc").
    WithContext("stack", "prod").
    WithExitCode(2).
    Err()
```

Use for:
- User-facing errors requiring detailed explanations
- Errors that benefit from code/config examples
- Structured, programmatic context
- Errors with actionable hints
- Errors requiring custom exit codes
- Rich error presentation with multiple sections

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
```text
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

### Phase 6: Structured Markdown Error Formatting ✅ Complete

- ✅ Implement `WithExplanation()` and `WithExplanationf()` methods
- ✅ Implement `WithExample()` and `WithExampleFile()` methods
- ✅ Add Go embed pattern for example markdown files
- ✅ Refactor formatter to build structured markdown sections:
  - # Error header
  - ## Explanation section
  - ## Example section
  - ## Hints section
  - ## Context section (markdown table)
  - ## Stack Trace section (verbose only)
- ✅ Conditional section rendering (only show sections with data)
- ✅ Convert workflow errors to use new builder pattern:
  - ErrWorkflowFileNotFound with explanation and example
  - ErrInvalidWorkflowManifest with explanation and example
  - ErrWorkflowNoWorkflow with explanation and example
- ✅ Update exit code handling in workflow commands
- ✅ Add 21 comprehensive tests (7 builder + 14 formatter)
- ✅ Regenerate golden test snapshots
- ✅ Documentation updates (developer guide and PRD)

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
