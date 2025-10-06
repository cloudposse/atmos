# Atmos Error Handling - Developer Guide

This document explains how to use the Atmos error handling system for creating, enriching, and reporting errors.

## Overview

Atmos uses [cockroachdb/errors](https://github.com/cockroachdb/errors) as the foundation for error handling, providing:
- Automatic stack traces
- User-facing hints
- PII-safe context for error reporting
- Network-portable errors
- Sentry integration for error tracking
- Custom exit codes

## Quick Start

### Basic Error Creation

Use static errors from `errors/errors.go`:

```go
import errUtils "github.com/cloudposse/atmos/errors"

// Simple error
return errUtils.ErrInvalidComponent

// Error with context
return fmt.Errorf("%w: component=%s stack=%s",
    errUtils.ErrInvalidComponent, component, stack)
```

### Error Builder

For rich errors with explanations, examples, hints, context, and exit codes:

```go
import (
    _ "embed"
    "github.com/cockroachdb/errors"
    errUtils "github.com/cloudposse/atmos/errors"
)

//go:embed examples/database_connection.md
var databaseConnectionExample string

err := errUtils.Build(errors.New("database connection failed")).
    WithExplanation("Failed to establish connection to the database server.").
    WithExampleFile(databaseConnectionExample).
    WithHint("Check database credentials in atmos.yaml").
    WithHintf("Verify network connectivity to %s", dbHost).
    WithContext("component", "vpc").
    WithContext("stack", "prod").
    WithContext("host", dbHost).
    WithExitCode(2).
    Err()
```

## Error Builder API

The builder provides a fluent API for constructing enriched errors:

### Build(err error) *ErrorBuilder

Creates a new ErrorBuilder from a base error.

```go
builder := errUtils.Build(errors.New("base error"))
```

### WithHint(hint string) *ErrorBuilder

Adds a user-facing hint that will be displayed with a 💡 emoji:

```go
err := errUtils.Build(baseErr).
    WithHint("Run 'atmos validate stacks' to check configuration").
    Err()
```

### WithHintf(format string, args ...interface{}) *ErrorBuilder

Adds a formatted hint:

```go
err := errUtils.Build(baseErr).
    WithHintf("Check the file at %s", filepath).
    Err()
```

### WithExplanation(explanation string) *ErrorBuilder

Adds a detailed explanation of what went wrong and why. Explanations are displayed in a dedicated "## Explanation" section in formatted errors.

```go
err := errUtils.Build(baseErr).
    WithExplanation("The workflow manifest must contain a top-level workflows: key.").
    Err()
```

### WithExplanationf(format string, args ...interface{}) *ErrorBuilder

Adds a formatted explanation:

```go
err := errUtils.Build(baseErr).
    WithExplanationf("The workflow manifest file `%s` does not exist.", filepath).
    Err()
```

### WithExample(example string) *ErrorBuilder

Adds an inline code or configuration example to help users understand the correct usage. Examples are displayed in a dedicated "## Example" section.

```go
err := errUtils.Build(baseErr).
    WithExample("```yaml\nworkflows:\n  deploy:\n    steps:\n      - command: terraform apply\n```").
    Err()
```

### WithExampleFile(content string) *ErrorBuilder

Adds a code/config example from an embedded markdown file. This is the preferred method for examples as it keeps them maintainable and separate from code.

```go
//go:embed examples/workflow_invalid_manifest.md
var workflowInvalidManifestExample string

err := errUtils.Build(baseErr).
    WithExampleFile(workflowInvalidManifestExample).
    Err()
```

**Example file** (`examples/workflow_invalid_manifest.md`):
````markdown
```yaml
workflows:
  deploy-vpc:
    description: Deploy VPC infrastructure
    steps:
      - command: terraform apply vpc -s prod
```
````

### WithContext(key string, value interface{}) *ErrorBuilder

Adds PII-safe structured context for programmatic access and error reporting.

Context is:
- **Displayed in verbose mode** as a styled table (`--verbose` flag or `ATMOS_LOGS_LEVEL=Debug`)
- **Sent to Sentry** automatically via `BuildSentryReport()`
- **Programmatically accessible** via `errors.GetSafeDetails(err)`
- **Included in verbose output** via `%+v` formatting

```go
err := errUtils.Build(baseErr).
    WithContext("component", "vpc").
    WithContext("stack", "prod").
    WithContext("region", "us-east-1").
    Err()
```

**Verbose Mode Output:**
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

### WithExitCode(code int) *ErrorBuilder

Attaches a custom exit code:

```go
err := errUtils.Build(baseErr).
    WithExitCode(2).  // Usage error
    Err()
```

### Err() error

Finalizes and returns the enriched error:

```go
err := builder.Err()
if err != nil {
    return err
}
```

## Error Formatting

The formatter provides smart error display with TTY detection, color support, and structured markdown sections:

```go
import errUtils "github.com/cloudposse/atmos/errors"

config := errUtils.DefaultFormatterConfig()
config.Verbose = false  // Collapsed mode
config.Color = "auto"   // auto, always, never

formatted := errUtils.Format(err, config)
fmt.Fprint(os.Stderr, formatted)
```

### Structured Markdown Output

Errors are formatted as structured markdown with hierarchical sections:

```
# Error

workflow file not found

## Explanation

The workflow manifest file `stacks/workflows/dne.yaml` does not exist.

## Example

```bash
# Verify the workflow file exists
ls -la stacks/workflows/

# Check your atmos.yaml for workflow paths configuration
cat atmos.yaml | grep -A5 workflows
```

## Hints

💡 Use `atmos list workflows` to see available workflows
💡 Verify the workflow file exists at: stacks/workflows/dne.yaml
💡 Check `workflows.base_path` in `atmos.yaml`: stacks/workflows

## Context

| Key       | Value              |
|-----------|-------------------|
| file      | stacks/workflows/dne.yaml |
| base_path | stacks/workflows   |

## Stack Trace

(shown in verbose mode only)
```

**Section Order:**
1. **# Error** - Title and error message
2. **## Explanation** - Detailed description (from `WithExplanation()`)
3. **## Example** - Code/config examples (from `WithExample()` or `WithExampleFile()`)
4. **## Hints** - Actionable suggestions (from `WithHint()`)
5. **## Context** - Key-value debugging info (from `WithContext()`)
6. **## Stack Trace** - Full stack trace (verbose mode only)

Sections are conditionally rendered - they only appear if data is available.

### Configuration Options

- **Verbose**: `false` (default) shows compact errors with context table, `true` shows full stack traces
- **Color**: `"auto"` (default) uses TTY detection, `"always"` forces color, `"never"` disables color
- **MaxLineLength**: `80` (default) wraps long error messages

## Exit Codes

### Attaching Exit Codes

```go
err := errUtils.WithExitCode(baseErr, 2)
```

Or use the builder:

```go
err := errUtils.Build(baseErr).
    WithExitCode(2).
    Err()
```

### Extracting Exit Codes

```go
exitCode := errUtils.GetExitCode(err)
// Returns:
// - 0 if err is nil
// - Custom exit code from WithExitCode
// - exec.ExitError exit code from command execution
// - 1 (default)
```

### Standard Exit Codes

- `0`: Success
- `1`: General error
- `2`: Usage error (incorrect arguments, invalid configuration)
- Other codes: Application-specific

## Sentry Integration

### Configuration

In `atmos.yaml`:

```yaml
errors:
  format:
    verbose: false
    color: auto
  sentry:
    enabled: true
    dsn: "https://examplePublicKey@o0.ingest.sentry.io/0"
    environment: "production"
    release: "1.0.0"
    sample_rate: 1.0
    debug: false
    capture_stack_context: true
    tags:
      team: "platform"
      service: "atmos"
```

### Initialize Sentry

```go
import errUtils "github.com/cloudposse/atmos/errors"

// From Atmos configuration
err := errUtils.InitializeSentry(&atmosConfig.Errors.Sentry)
if err != nil {
    log.Warn("Failed to initialize Sentry", "error", err)
}
defer errUtils.CloseSentry()
```

### Capture Errors

```go
// Simple error capture
errUtils.CaptureError(err)

// With Atmos context
context := map[string]string{
    "component": "vpc",
    "stack":     "prod",
    "region":    "us-east-1",
}
errUtils.CaptureErrorWithContext(err, context)
```

### What Gets Sent to Sentry

1. **Hints** → Sentry breadcrumbs (category: "hint")
2. **Safe details** → Sentry tags with `error.` prefix
3. **Atmos context** → Sentry tags with `atmos.` prefix
4. **Exit codes** → Sentry tag `atmos.exit_code`
5. **Stack traces** → Full error chain with file/line information

## Best Practices

### 1. Use Static Errors

Define all base errors in `errors/errors.go`:

```go
var (
    ErrInvalidComponent = errors.New("invalid component")
    ErrMissingStack     = errors.New("stack is required")
)
```

### 2. Add Structured Context

Use `.WithContext()` for programmatic, structured context:

```go
// ❌ BAD: No context
return errUtils.ErrInvalidComponent

// ✅ GOOD: Structured context (accessible programmatically, shown in verbose mode)
return errUtils.Build(errUtils.ErrInvalidComponent).
    WithContext("component", component).
    WithContext("stack", stack).
    Err()

// ⚠️ ACCEPTABLE: String context (for simple error messages only)
// Use this only when you don't need programmatic access to the values
return fmt.Errorf("%w: component=%s stack=%s",
    errUtils.ErrInvalidComponent, component, stack)
```

**Why use `.WithContext()`?**
- Programmatically accessible via `errors.GetSafeDetails(err)`
- Displayed as clean table in verbose mode
- Automatically sent to Sentry as structured data
- PII-safe by design

### 3. Provide Helpful Hints

```go
err := errUtils.Build(errors.New("failed to validate stack")).
    WithHint("Run 'atmos validate stacks' to see detailed errors").
    WithHintf("Check the stack file: %s", stackPath).
    Err()
```

### 4. Use Appropriate Exit Codes

```go
// Usage errors
err := errUtils.Build(errUtils.ErrMissingStack).
    WithExitCode(2).
    Err()

// Application errors
err := errUtils.Build(errUtils.ErrProcessingFailed).
    WithExitCode(1).
    Err()
```

### 5. Check Error Types

Use `errors.Is()` for error checking:

```go
if errors.Is(err, errUtils.ErrInvalidComponent) {
    // Handle invalid component
}
```

### 6. Don't Include PII in Hints

```go
// ❌ BAD: Contains user credentials
.WithHint("Failed to connect with password: secret123")

// ✅ GOOD: Generic hint
.WithHint("Check database credentials in atmos.yaml")
```

## Error Wrapping Patterns

### Combining Multiple Errors

```go
import "github.com/cockroachdb/errors"

// Multiple error values
return errors.Join(errUtils.ErrFailedToProcess, underlyingErr)
```

### Adding String Context

```go
// Single error with formatted context
return fmt.Errorf("%w: failed to process %s", errUtils.ErrInvalidConfig, configName)
```

### Preserving Error Chains

Always use `%w` verb when wrapping errors:

```go
// ✅ CORRECT: Preserves error chain
return fmt.Errorf("%w: additional context", originalErr)

// ❌ WRONG: Breaks error chain
return fmt.Errorf("%v: additional context", originalErr)
```

## Testing Errors

### Test Drive Error Formatting Locally

To see the error formatting in action, run the examples test:

```bash
# See all error formatting examples
go test -v ./errors -run TestExampleErrorFormatting

# This will show:
# - Simple errors
# - Errors with hints
# - Error chains (collapsed and verbose)
# - Builder pattern examples
# - Color modes (auto, always, never)
# - Long message wrapping
```

You can also test error formatting in your code:

```go
import (
    "github.com/cockroachdb/errors"
    errUtils "github.com/cloudposse/atmos/errors"
)

// Create an error
err := errUtils.Build(errors.New("test error")).
    WithHint("This is a helpful hint").
    Err()

// Format it
formatted := errUtils.Format(err, errUtils.FormatterConfig{
    Verbose:       false,  // or true for stack traces
    Color:         "auto", // or "always", "never"
    MaxLineLength: 80,
})

fmt.Fprintf(os.Stderr, "%s\n", formatted)
```

### Check Error Messages

```go
func TestErrorMessage(t *testing.T) {
    err := errUtils.Build(errors.New("test error")).
        WithHint("hint 1").
        Err()

    assert.Contains(t, err.Error(), "test error")

    hints := errors.GetAllHints(err)
    assert.Len(t, hints, 1)
    assert.Equal(t, "hint 1", hints[0])
}
```

### Check Exit Codes

```go
func TestExitCode(t *testing.T) {
    err := errUtils.Build(errors.New("test")).
        WithExitCode(42).
        Err()

    code := errUtils.GetExitCode(err)
    assert.Equal(t, 42, code)
}
```

### Check Error Types

```go
func TestErrorType(t *testing.T) {
    err := fmt.Errorf("%w: component=vpc", errUtils.ErrInvalidComponent)

    assert.True(t, errors.Is(err, errUtils.ErrInvalidComponent))
}
```

## Migration Guide

### From Old Error Handling

```go
// Old style
return errors.New("invalid component: " + component)

// New style
return fmt.Errorf("%w: component=%s", errUtils.ErrInvalidComponent, component)
```

### Adding Hints to Existing Errors

```go
// Before
return errUtils.ErrMissingStack

// After
return errUtils.Build(errUtils.ErrMissingStack).
    WithHint("Specify stack with --stack flag or -s shorthand").
    Err()
```

## Reference

- [cockroachdb/errors Documentation](https://github.com/cockroachdb/errors)
- [Sentry Go SDK Documentation](https://docs.sentry.io/platforms/go/)
- [Error Handling Strategy PRD](prd/error-handling-strategy.md)
- [User Guide](../website/docs/core-concepts/errors.mdx)
