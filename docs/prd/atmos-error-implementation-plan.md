# AtmosError Implementation Plan - Complete

## Executive Summary

Integrate `cockroachdb/errors` library to provide:
- Rich error context with hints and structured data
- Exit code tracking
- TTY-aware color formatting with smart chain wrapping
- Optional Sentry error reporting (PII-safe) with full context
- Idiomatic Go error handling with no custom wrapper types

## Architecture

### Core Design Principles

1. **Use cockroachdb/errors directly** - no custom AtmosError wrapper type
2. **Sentinel errors unchanged** - keep simple `errors.New()`
3. **Builder pattern optional** - provides fluent API for complex errors
4. **Context via `WithSafeDetails()`** - PII-safe key-value pairs
5. **Hints via `WithHint()`** - user-facing actionable suggestions
6. **Exit codes via secondary errors** - extraction at boundaries
7. **Sentry opt-in** - configurable in `atmos.yaml` under top-level `errors`

### Why No Custom Wrapper?

Creating `AtmosError` struct would:
- ❌ Add unnecessary complexity
- ❌ Break idiomatic Go patterns
- ❌ Duplicate cockroachdb/errors functionality
- ❌ Require type assertions everywhere

### Why cockroachdb/errors?

- ✅ Battle-tested in CockroachDB (distributed systems)
- ✅ Automatic stack traces
- ✅ Built-in Sentry integration
- ✅ PII-safe reporting
- ✅ Network-portable errors
- ✅ Compatible with standard library
- ✅ `errors.Is()` and `errors.As()` work perfectly

## Visual Error Formatting Examples

### Simple Error (TTY)
```text
[RED BOLD]Error:[/] [RED]Component 'vpc' not found in stack 'prod/us-east-1'[/]

[CYAN]💡 Use 'atmos list components --stack prod/us-east-1' to see available components[/]
[CYAN]💡 Check that the component is defined in your stack configuration[/]

[DIM]Context:[/]
  [DIM]component: vpc, stack: prod/us-east-1, config_file: stacks/prod/us-east-1.yaml[/]
```

### Wrapped Error Chain - Short (Collapsed, Default)
```text
Error: component 'vpc' not found

💡 Use 'atmos list components'
```

### Wrapped Error Chain - Long (Collapsed, Smart Wrapping)
```text
Error: failed to initialize component vpc
  caused by: failed to connect to database
  caused by: connection refused
  caused by: dial tcp 10.0.1.5:5432: i/o timeout

💡 Check database connectivity
💡 Verify database credentials

Context:
  component: vpc, stack: prod/us-east-1

(use --verbose to see full error chain and stack trace)
```

### Wrapped Error Chain (Verbose Mode)
```text
Error: failed to initialize component vpc

Error Chain:
  1. failed to initialize component vpc
     at: cmd/terraform.go:45

  2. failed to connect to database
     at: database/connect.go:123

  3. connection refused
     at: net/dial.go:89

  4. dial tcp 10.0.1.5:5432: i/o timeout
     at: internal/poll/fd_unix.go:172

Stack Trace:
  cmd/terraform.go:45
    main.terraformApply()
  database/connect.go:123
    db.Connect()
  net/dial.go:89
    dial.TCP()

💡 Check database connectivity
💡 Verify database credentials

Context:
  component: vpc
  stack: prod/us-east-1
  terraform_workspace: prod-use1
```

### Fatal Error with Sentry
```text
[RED BOLD]Error:[/] [RED]Authentication failed[/]

[CYAN]💡 Check your AWS credentials[/]
[CYAN]💡 Ensure the correct AWS profile is set[/]

[DIM]Context:[/]
  [DIM]aws_profile: prod-admin, region: us-east-1[/]

[DIM]Error ID:[/] [YELLOW]a1b2c3d4-e5f6-7890[/]
[DIM]This error has been reported. Use this ID when contacting support.[/]
```

### Non-TTY Output (Plain Text)
```text
Error: Component 'vpc' not found in stack 'prod/us-east-1'

💡 Use 'atmos list components --stack prod/us-east-1' to see available components
💡 Check that the component is defined in your stack configuration

Context:
  component: vpc, stack: prod/us-east-1, config_file: stacks/prod/us-east-1.yaml

(use --verbose to see full error chain and stack trace)
```

## Configuration Structure

### Top-Level `errors` Config
```yaml
# atmos.yaml

errors:
  # Formatting options
  format:
    verbose: false          # Show full error chains by default
    color: auto            # auto | always | never

  # Sentry error reporting
  sentry:
    enabled: true
    dsn: "https://[email protected]/project"

    # Sentry "environment" = deployment environment (dev/staging/prod)
    # This is different from Atmos "environment" context field
    environment: production  # deployment environment
    release: "1.0.0"        # optional, defaults to atmos version
    sample_rate: 1.0        # 0.0-1.0
    debug: false

    # Custom tags sent to Sentry with all errors
    tags:
      team: platform
      service: atmos

    # Automatically capture Atmos stack context as Sentry tags
    # Adds tags like: atmos.stack, atmos.component, etc.
    capture_stack_context: true

logs:
  level: info
  file: /var/log/atmos.log
```text

### Schema Structure

```go
// pkg/schema/schema.go

type AtmosConfiguration struct {
    // ... existing fields ...

    Errors ErrorsConfig `yaml:"errors,omitempty" json:"errors,omitempty" mapstructure:"errors"`
    Logs   Logs         `yaml:"logs,omitempty" json:"logs,omitempty" mapstructure:"logs"`

    // ... rest of fields ...
}

type ErrorsConfig struct {
    // Formatting options
    Format ErrorFormatConfig `yaml:"format,omitempty" json:"format,omitempty" mapstructure:"format"`

    // Sentry reporting
    Sentry SentryConfig `yaml:"sentry,omitempty" json:"sentry,omitempty" mapstructure:"sentry"`
}

type ErrorFormatConfig struct {
    Verbose bool   `yaml:"verbose,omitempty" json:"verbose,omitempty" mapstructure:"verbose"`
    Color   string `yaml:"color,omitempty" json:"color,omitempty" mapstructure:"color"` // auto, always, never
}

type SentryConfig struct {
    Enabled              bool              `yaml:"enabled" json:"enabled" mapstructure:"enabled"`
    DSN                  string            `yaml:"dsn" json:"dsn" mapstructure:"dsn"`
    Environment          string            `yaml:"environment,omitempty" json:"environment,omitempty" mapstructure:"environment"`
    Release              string            `yaml:"release,omitempty" json:"release,omitempty" mapstructure:"release"`
    SampleRate           float64           `yaml:"sample_rate,omitempty" json:"sample_rate,omitempty" mapstructure:"sample_rate"`
    Debug                bool              `yaml:"debug,omitempty" json:"debug,omitempty" mapstructure:"debug"`
    Tags                 map[string]string `yaml:"tags,omitempty" json:"tags,omitempty" mapstructure:"tags"`
    CaptureStackContext  bool              `yaml:"capture_stack_context,omitempty" json:"capture_stack_context,omitempty" mapstructure:"capture_stack_context"`
}

type Logs struct {
    File  string `yaml:"file" json:"file" mapstructure:"file"`
    Level string `yaml:"level" json:"level" mapstructure:"level"`
    // Errors moved to top-level
}
```

## Sentry Context Integration

### What Gets Sent to Sentry

**1. Stack Context (Sentry Tags):**
```text
atmos.stack: prod/us-east-1
atmos.component: vpc
atmos.namespace: cp
atmos.environment: prod
atmos.stage: ue1
atmos.region: us-east-1
atmos.workspace: prod-use1
```

**2. Error Safe Details (Sentry Tags):**
```text
error.component: vpc
error.config_file: stacks/prod/us-east-1.yaml
error.operation: terraform plan
```

**3. Hints (Sentry Breadcrumbs):**
```text
[INFO] hint: Check database connectivity
[INFO] hint: Verify database credentials
```

**4. Custom Tags (from config):**
```text
team: platform
service: atmos
datacenter: us-east-1
```

### Implementation

```go
// errors/sentry.go

func ReportError(err error, stackContext *schema.Context) string {
    if !sentryInitialized || err == nil {
        return ""
    }

    sentry.ConfigureScope(func(scope *sentry.Scope) {
        // 1. Add stack context as tags
        if stackContext != nil && sentryConfig.CaptureStackContext {
            if stackContext.Stack != "" {
                scope.SetTag("atmos.stack", stackContext.Stack)
            }
            if stackContext.Component != "" {
                scope.SetTag("atmos.component", stackContext.Component)
            }
            if stackContext.Namespace != "" {
                scope.SetTag("atmos.namespace", stackContext.Namespace)
            }
            if stackContext.Environment != "" {
                scope.SetTag("atmos.environment", stackContext.Environment)
            }
            if stackContext.Stage != "" {
                scope.SetTag("atmos.stage", stackContext.Stage)
            }
            if stackContext.Region != "" {
                scope.SetTag("atmos.region", stackContext.Region)
            }
            if stackContext.Workspace != "" {
                scope.SetTag("atmos.workspace", stackContext.Workspace)
            }
        }

        // 2. Add safe details from error as tags
        details := errors.GetSafeDetails(err)
        for _, detail := range details.SafeDetails {
            // Parse "key=value" format
            parts := strings.SplitN(detail, "=", 2)
            if len(parts) == 2 {
                scope.SetTag("error."+parts[0], parts[1])
            } else {
                // Add as extra context if not key=value
                scope.SetExtra("detail", detail)
            }
        }

        // 3. Add hints as breadcrumbs
        hints := errors.GetAllHints(err)
        for _, hint := range hints {
            scope.AddBreadcrumb(&sentry.Breadcrumb{
                Category: "hint",
                Message:  hint,
                Level:    sentry.LevelInfo,
            }, nil)
        }
    })

    // Report using cockroachdb/errors integration
    eventID := errors.ReportError(err)

    if eventID != "" {
        log.Debug("Error reported to Sentry",
            "event_id", eventID,
            "stack", stackContext.Stack,
            "component", stackContext.Component,
        )
    }

    return eventID
}
```text

## Implementation Phases

### Phase 1: Foundation (This PR)

**Scope:**
1. Add cockroachdb/errors dependency
2. Add Sentry dependency
3. Update configuration schema (top-level `errors`)
4. Implement exit code support
5. Implement error builder
6. Implement smart formatting (with long chain handling)
7. Implement Sentry integration with full context
8. Update logger
9. Write comprehensive tests (100% coverage target)
10. Write complete documentation

**Deliverables:**
- All new error infrastructure
- 100% test coverage on new code
- Complete PRD
- Developer guide (`docs/errors.md`)
- User guide (added to `website/docs/troubleshoot/errors.mdx`)
- Updated CLAUDE.md
- Working examples

**Test Coverage:**
- Unit tests for all new code
- Golden tests for all formatting combinations
- Integration tests for logger + Sentry
- Mocked Sentry tests
- Table-driven tests throughout

### Phase 2: Hints Migration (Separate PR)

**Scope:**
1. Create `errors/hints.go` with `AddCommonHints()`
2. Migrate hints from error messages to WithHint
3. Simplify sentinel error messages
4. Update all error creation sites to use hints
5. Add comprehensive hint coverage

**Process:**
1. **Audit current errors** - Find all errors with inline hints
2. **Extract hints** - Move to `AddCommonHints()` or call sites
3. **Simplify sentinels** - Remove hint text from error messages
4. **Add missing hints** - Identify errors that need hints
5. **Test coverage** - Ensure 100% coverage of hint mappings

**Deliverables:**
- All 200+ sentinel errors migrated
- `hints.go` with comprehensive hint mapping
- 100% test coverage for hints
- Documentation of hint patterns

### Phase 3: Hint Addition (Separate PR or Multiple PRs)

**Scope:**
1. Review all error return sites in codebase
2. Add helpful hints where missing
3. Add context to errors
4. Set appropriate exit codes

**Target:**
- 80-90% of error sites have helpful hints
- All user-facing commands have good error messages
- All configuration errors have actionable hints
- All component errors have troubleshooting hints

## File Structure

```
errors/
  errors.go              # Sentinels (import changed to cockroachdb/errors)
  exit_code.go           # Exit code support (NEW)
  exit_code_test.go      # Tests (NEW)
  builder.go             # Error builder (NEW)
  builder_test.go        # Tests (NEW)
  format.go              # Smart formatting with chain handling (NEW)
  format_test.go         # Tests (NEW)
  sentry.go              # Sentry integration with full context (NEW)
  sentry_test.go         # Tests (NEW)
  hints.go               # Common hint mappings (Phase 2)
  hints_test.go          # Tests (Phase 2)
  error_funcs.go         # Updated functions
  error_funcs_test.go    # Tests
  examples_test.go       # Usage examples (NEW)
  integration_test.go    # End-to-end tests (NEW)

  testdata/
    golden/
      simple_tty.golden
      simple_nonttty.golden
      hints_tty.golden
      wrapped_short.golden
      wrapped_long.golden
      wrapped_verbose.golden
      full_error_tty.golden
      ... (comprehensive golden files)

pkg/schema/
  schema.go              # ErrorsConfig at top-level

pkg/datafetcher/schema/config/global/
  1.0.json               # Updated schema with errors config

pkg/logger/
  atmos_logger.go        # Updated with hints + Sentry
  atmos_logger_test.go   # Updated tests

cmd/
  root.go                # Sentry init, SetConfig for formatting
  root_test.go           # Integration tests

docs/
  errors.md              # Developer how-to guide (NEW)

  prd/
    atmos-error-handling.md              # Architecture PRD (NEW)
    atmos-error-implementation-plan.md   # This file
    error-handling-strategy.md           # Updated

website/docs/
  cli/
    errors.mdx           # User-facing error documentation

CLAUDE.md              # Updated with error handling patterns
```text

## Error Usage Patterns

### Simple Error with Hint
```go
err := errors.WithHint(ErrMissingStack, "Use --stack flag")
return err
```

### Error with Context (PII-safe)
```go
err := errors.WithSafeDetails(ErrInvalidComponent,
    "component=%s stack=%s",
    errors.Safe(component),
    errors.Safe(stack),
)
```text

### Complex Error (Builder)
```go
return Build(ErrInvalidComponent).
    WithHint("Use 'atmos list components'").
    WithHint("Check stack configuration").
    WithContext("component", componentName).
    WithContext("stack", stackName).
    WithExitCode(1).
    Err()
```

### Wrapping External Errors
```go
if err != nil {
    return errors.Wrapf(err, "failed to load %s", path)
}
```text

### Logger Integration
```go
err := Build(ErrMissingStack).
    WithHint("Use --stack flag").
    Err()

logger.Error(err)  // Shows error + hints
logger.Fatal(err)  // Reports to Sentry, exits with code
```

## Success Criteria

### Phase 1 (This PR)
1. ✅ All new infrastructure implemented
2. ✅ 100% test coverage on new code
3. ✅ Smart error chain formatting working
4. ✅ Long chains formatted readably
5. ✅ TTY-aware colors working
6. ✅ Sentry integration with full context working
7. ✅ Stack context → Sentry tags
8. ✅ Safe details → Sentry tags
9. ✅ Hints → Sentry breadcrumbs
10. ✅ Configuration schema updated
11. ✅ Complete documentation (3 levels)
12. ✅ All tests passing
13. ✅ No breaking changes

### Phase 2 (Hints Migration)
1. ✅ All 200+ sentinels migrated
2. ✅ Hints extracted to WithHint
3. ✅ `hints.go` with common mappings
4. ✅ 100% test coverage
5. ✅ Error messages simplified
6. ✅ No breaking changes (same hints, different mechanism)

### Phase 3 (Hint Addition)
1. ✅ 80-90% of errors have hints
2. ✅ All user commands covered
3. ✅ All config errors covered
4. ✅ All component errors covered
5. ✅ Tests for all new hints
6. ✅ Documentation updated

## Benefits

### For Users
- **Never confused** - Every error has actionable hints
- **Beautiful output** - Clean, readable formatting
- **Smart wrapping** - Long error chains formatted nicely
- **Quick scanning** - Collapsed by default, verbose on demand
- **Error tracking** - Get an ID to reference when asking for help

### For Developers
- **Easy debugging** - Full chains and stack traces available
- **Simple API** - Builder pattern for complex errors
- **Good tests** - 100% coverage, easy to maintain
- **Clear patterns** - Centralized hint management
- **Rich context** - Full error details sent to Sentry

### For Operations
- **Complete coverage** - All errors tracked in Sentry
- **Rich context** - Stack/component/environment tagged
- **PII-safe** - Automatic safe detail filtering
- **Actionable** - Every error has remediation hints
- **Searchable** - Find errors by tags in Sentry

## Environment Variables

```bash
# Override error formatting
ATMOS_ERRORS_FORMAT_VERBOSE=true
ATMOS_ERRORS_FORMAT_COLOR=always

# Override Sentry config
ATMOS_ERRORS_SENTRY_ENABLED=true
ATMOS_ERRORS_SENTRY_DSN="https://..."
ATMOS_ERRORS_SENTRY_ENVIRONMENT=staging
ATMOS_ERRORS_SENTRY_CAPTURE_STACK_CONTEXT=true
```

## References

- [cockroachdb/errors](https://github.com/cockroachdb/errors) - Underlying library
- [Sentry Go SDK](https://docs.sentry.io/platforms/go/) - Error reporting
- [Go Error Handling](https://go.dev/blog/go1.13-errors) - Go 1.13+ errors
