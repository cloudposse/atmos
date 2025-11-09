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
import (
    "fmt"

    errUtils "github.com/cloudposse/atmos/errors"
)

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
- **Displayed in verbose mode** as a styled table (`--verbose` flag or `ATMOS_VERBOSE=1`)
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

The formatter provides smart error display with structured markdown sections. Color output is controlled by the global terminal settings (see Terminal Color Control below):

```go
import errUtils "github.com/cloudposse/atmos/errors"

config := errUtils.DefaultFormatterConfig()
config.Verbose = false  // Collapsed mode

formatted := errUtils.Format(err, config)
fmt.Fprint(os.Stderr, formatted)
```

### Structured Markdown Output

Errors are formatted as structured markdown with hierarchical sections:

```text
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
- **MaxLineLength**: `80` (default) wraps long error messages

### Terminal Color Control

Color output is controlled by the global terminal settings, not the error formatter config. The formatter automatically respects:

- **CLI Flags**: `--no-color`, `--color`, `--force-color`
- **Environment Variables**: `NO_COLOR`, `CLICOLOR`, `CLICOLOR_FORCE`
- **Configuration**: `settings.terminal.color` and `settings.terminal.no_color` in `atmos.yaml`

Example:
```bash
# Disable color for error output
atmos terraform plan --no-color

# Force color even when piped
atmos terraform plan --force-color | tee output.log
```

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

Atmos only sends **command failures** to Sentry - errors that prevent a command from completing successfully and cause Atmos to exit with an error code.

**Errors sent to Sentry:**
- Command failures (invalid arguments, missing configuration, component not found)
- Stack validation errors that prevent deployment
- Workflow execution failures
- Authentication/authorization errors
- File system errors (missing atmos.yaml, unreadable stack files)

**Errors NOT sent to Sentry:**
- Debug/trace log messages
- Warnings (e.g., deprecated configuration options)
- Non-fatal errors that Atmos recovers from
- Successful commands (exit code 0)

**Information included with each error:**
1. **Error message and type** - What went wrong
2. **Hints** → Sentry breadcrumbs to help debug the issue
3. **Context** → Tags like component name, stack name, region (PII-safe)
4. **Exit code** → Command exit code for automation
5. **Stack traces** → Full error chain with file/line information for debugging

This ensures Sentry focuses on actionable failures that affect users, without overwhelming it with internal logging or successful operations.

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

// Format it (color controlled by terminal settings)
formatted := errUtils.Format(err, errUtils.FormatterConfig{
    Verbose:       false,  // or true for stack traces
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

## Component-Level Sentry Configuration

### Overview

Atmos supports overriding Sentry configuration at the stack and component level, allowing fine-grained control over error tracking, sampling rates, and tagging for different components.

### Global Configuration

Configure Sentry globally in `atmos.yaml`:

```yaml
errors:
  sentry:
    enabled: true
    dsn: "https://your-dsn@sentry.io/project"
    environment: "production"
    release: "1.0.0"
    sample_rate: 0.1  # 10% sampling by default
    debug: false
    capture_stack_context: true
    tags:
      service: "atmos"
      team: "platform"
```

### Stack-Level Override

Define settings for all components in a stack:

```yaml
# stacks/prod/critical.yaml
settings:
  errors:
    sentry:
      sample_rate: 1.0  # 100% sampling for all components in this stack
      tags:
        team: "payments"
        criticality: "critical"
        sla: "99.99"

components:
  terraform:
    payment-processor:
      # Inherits settings.errors.sentry from stack level

    payment-gateway:
      # Can override specific settings
      settings:
        errors:
          sentry:
            tags:
              subteam: "gateway"  # Additional tag
```

### Component-Level Override

Override settings for a specific component:

```yaml
# stacks/prod/us-east-1.yaml
components:
  terraform:
    vpc:
      settings:
        errors:
          sentry:
            tags:
              team: "infrastructure"

    rds:
      settings:
        errors:
          sentry:
            sample_rate: 0.5  # 50% sampling for non-critical component
            environment: "prod-database"
            tags:
              team: "database"
              component_type: "rds"
```

### Configuration Merging

Component settings are deep merged with global settings in the following order:

1. **Global** (`atmos.yaml`) - Base configuration
2. **Stack** (`settings.errors.sentry`) - Stack-level overrides
3. **Component** (`components.terraform.X.settings.errors.sentry`) - Component-specific overrides

**Example:**

```yaml
# atmos.yaml (global)
errors:
  sentry:
    enabled: true
    dsn: "https://example@sentry.io/1"
    environment: "production"
    sample_rate: 0.1  # 10% default
    tags:
      service: "atmos"

# stacks/prod/critical.yaml (stack level)
settings:
  errors:
    sentry:
      sample_rate: 1.0  # Override to 100%
      tags:
        team: "payments"  # Add team tag

components:
  terraform:
    payment-processor:
      # Merged config:
      # - enabled: true (from global)
      # - dsn: "https://example@sentry.io/1" (from global)
      # - environment: "production" (from global)
      # - sample_rate: 1.0 (from stack)
      # - tags:
      #     service: "atmos" (from global)
      #     team: "payments" (from stack)

      settings:
        errors:
          sentry:
            tags:
              criticality: "critical"  # Add component-specific tag
      # Final merged config adds criticality: "critical" to tags
```

### Using Component-Specific Error Capture

```go
import (
    errUtils "github.com/cloudposse/atmos/errors"
    "github.com/cloudposse/atmos/pkg/schema"
)

// In a command that has access to ConfigAndStacksInfo
func executeComponentAction(info *schema.ConfigAndStacksInfo) error {
    // ... component logic ...

    if err != nil {
        // Capture with component-specific Sentry config
        errUtils.CaptureErrorWithComponentConfig(err, info, map[string]string{
            "action": "terraform-plan",
            "region": "us-east-1",
        })
        return err
    }

    return nil
}
```

### Use Cases

#### Critical Components with Full Sampling

```yaml
# stacks/prod/critical.yaml
settings:
  errors:
    sentry:
      sample_rate: 1.0  # Capture all errors
      tags:
        criticality: "critical"
        pci_compliant: "true"

components:
  terraform:
    payment-processor:
    fraud-detection:
    user-auth:
```

#### Team-Based Error Grouping

```yaml
# stacks/prod/database.yaml
settings:
  errors:
    sentry:
      tags:
        team: "database"
        oncall: "db-team@company.com"

components:
  terraform:
    rds-primary:
    rds-replica:
    redis-cache:
```

#### Environment-Specific Configuration

```yaml
# stacks/staging/us-west-2.yaml
settings:
  errors:
    sentry:
      environment: "staging"
      sample_rate: 0.5  # 50% sampling in staging
      tags:
        auto_deploy: "true"
```

#### Gradual Rollout with Sampling

```yaml
# Start with low sampling
components:
  terraform:
    new-experimental-feature:
      settings:
        errors:
          sentry:
            sample_rate: 0.01  # 1% sampling initially

# Increase as confidence grows
# sample_rate: 0.1  # 10%
# sample_rate: 0.5  # 50%
# sample_rate: 1.0  # 100%
```

### Client Management

Atmos automatically manages Sentry clients per configuration:

- **Client Reuse**: Identical configurations share the same Sentry client
- **Isolation**: Different configurations use separate clients
- **Automatic Cleanup**: Clients are flushed and closed on shutdown

### Multiple Sentry Projects

Use different DSNs for different components:

```yaml
# Global default
errors:
  sentry:
    dsn: "https://default@sentry.io/general"

# Component override
components:
  terraform:
    special-component:
      settings:
        errors:
          sentry:
            dsn: "https://special@sentry.io/special-project"
```

### Troubleshooting

**Sentry Not Receiving Errors:**
1. Check DSN configuration in `atmos.yaml`
2. Verify `enabled: true` at the appropriate level
3. Check sample rate isn't too low
4. Enable debug mode: `sentry.debug: true`
5. Check network connectivity to Sentry

**Errors Not Using Component Config:**
1. Ensure `settings.errors.sentry` is in the component section
2. Verify stack processing is working: `atmos describe component X -s Y`
3. Check that the command uses `CaptureErrorWithComponentConfig()`

## Reference

- [cockroachdb/errors Documentation](https://github.com/cockroachdb/errors)
- [Sentry Go SDK Documentation](https://docs.sentry.io/platforms/go/)
- [Error Handling Strategy PRD](prd/error-handling-strategy.md)
- [User Guide](../website/docs/core-concepts/errors.mdx)
