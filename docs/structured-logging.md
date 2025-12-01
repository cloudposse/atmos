# Structured and Semantic Logging

**Structured logging** captures log entries as **key/value pairs** instead of raw strings. This makes logs easier for machines to parse and for humans to search.

**Semantic logging** goes a step further by **standardizing the keys**. Using predictable fields like `component`, `operation`, `request_id`, and `error` makes logs interpretable across different systems and teams.

## Logging vs I/O Output

**Don't confuse logging with I/O & UI output:**
- **Logging** (this document) - For developers: diagnostics, debugging
- **I/O & UI output** - For users: status messages, data results

**See:** [I/O and UI Output Guide](io-and-ui-output.md) for user-facing output.

## Why We Use It

Structured and semantic logging provide:

- **Machine readability** – tools such as Datadog, Splunk, Honeycomb, and OpenTelemetry can process logs without regex or guesswork.
- **Contextual clarity** – each entry describes what happened and includes metadata about where and to whom.
- **Searchability and filtering** – query logs by fields like `user_id` or `service` to diagnose issues quickly.
- **Consistency** – separating the message from context removes the need for string interpolation.
- **Customizability** – emit colorized terminal output or structured JSON for ingestion.

## How to Write Logs

Describe the core event in the log **message**:

> Failed to validate request

Then add **contextual fields**:

```go
import "github.com/cloudposse/atmos/pkg/logger"

logger.Error("Failed to validate request", "component", "authn", "user", userID, "error", err)
```

This approach ensures:

- The message captures **what** happened.
- The key/value pairs capture **who**, **where**, and **why**.
- You can add whichever keys are meaningful for that event.

## Standard Semantic Keys

Use these keys consistently throughout the Atmos codebase:

- `component` - Atmos component name
- `stack` - Stack name
- `error` - Error value
- `duration` - Operation duration
- `path` - File system path
- `operation` - Operation name
- `status` - Operation status
- `count` - Numeric count
- `size` - Size in bytes
- `func` - Function name (primarily for trace level)
- `cmd` - Command being executed
- `key` - Cache or lookup key
- `version` - Version identifier

## Level-Appropriate Context

Different log levels warrant different amounts of context. Higher severity levels need more actionable information, while lower levels can include debugging details.

### Trace Level Context

Include maximum context for execution flow analysis:

```go
import "github.com/cloudposse/atmos/pkg/logger"

logger.Trace("Entering function",
    "func", "ProcessTemplate",
    "input_size", len(input),
    "template", templateName,
    "context_keys", contextKeys,
    "caller", callerFunc)

logger.Trace("Branch taken",
    "condition", conditionName,
    "value", evaluatedValue,
    "branch", "true")
```

Trace logs should provide enough information to reconstruct the exact execution path.

### Debug Level Context

Include diagnostic context for understanding behavior:

```go
import "github.com/cloudposse/atmos/pkg/logger"

logger.Debug("Component processed",
    "component", name,
    "stack", stack,
    "duration", duration)

logger.Debug("Cache operation",
    "operation", "get",
    "key", cacheKey,
    "hit", true,
    "age", cacheAge)
```

Debug logs should help diagnose issues without overwhelming detail.

### Info Level Context

Include minimal context for major events only:

```go
import "github.com/cloudposse/atmos/pkg/logger"

// Use sparingly - only for major lifecycle events
logger.Info("Service initialized",
    "version", version,
    "mode", operationMode)
```

Info logs should be self-explanatory with minimal context.

### Warning/Error Level Context

Include problem context for troubleshooting:

```go
import "github.com/cloudposse/atmos/pkg/logger"

logger.Warn("Retry attempt",
    "operation", "api_call",
    "attempt", retryCount,
    "max_attempts", maxRetries,
    "error", err)

logger.Error("Operation failed",
    "component", componentName,
    "stack", stackName,
    "operation", "deploy",
    "error", err,
    "duration", elapsed)
```

Error and warning logs need enough context to diagnose and fix issues.

## Charm Logger in Atmos

Atmos uses the [Charm Logger](https://charm.sh/blog/the-charm-logger/), which offers:

- Simple APIs through `pkg/logger` such as `logger.Trace`, `logger.Debug`, `logger.Info`, `logger.Warn`, and `logger.Error`
- Automatic line wrapping and colorized output for terminals
- Friendly formatting for console output
- Optional raw JSON output for machine ingestion

Example output in terminal:
```console
DEBU  Component processed  component=vpc stack=dev duration=1.2s
```

Example JSON output:
```json
{
  "level": "debug",
  "message": "Component processed",
  "component": "vpc",
  "stack": "dev",
  "duration": "1.2s"
}
```

## Performance Considerations

### Lazy Evaluation

For expensive operations in logs, especially at trace level:

```go
import "github.com/cloudposse/atmos/pkg/logger"

// Avoid: Serializes even if trace is disabled
logger.Trace("Config state", "json", toJSON(config))

// Better: Only computes if trace is enabled
if logger.IsLevelEnabled(logger.TraceLevel) {
    logger.Trace("Config state", "json", toJSON(config))
}
```

### Consistent Patterns

Always use structured logging with key-value pairs:

```go
import "github.com/cloudposse/atmos/pkg/logger"

// Wrong: String interpolation
logger.Debug(fmt.Sprintf("Processing component %s in stack %s", name, stack))

// Right: Structured logging
logger.Debug("Processing component", "component", name, "stack", stack)
```

## Linting and Conventions

Atmos runs `golangci-lint`, which includes Staticcheck rule `ST1019`. That rule warns when logging keys are non-constant strings. To avoid an explosion of rarely reused constants, some common logging keys are already excluded from `ST1019` in our configuration. You can define your own constants for frequently used keys or suppress the warning when it makes the code clearer.

Follow the existing conventions in Atmos and keep suppression comments to a minimum.

## Key Principles

1. **No String Interpolation**: Always use key-value pairs, never formatted strings with embedded values.

2. **Consistent Keys**: Use the same key names throughout the codebase for the same concepts.

3. **Appropriate Context**: Include enough context for the log level - maximum for trace, minimal for info.

4. **Performance Awareness**: Consider the cost of generating log data, especially for trace and debug levels.

5. **Machine First**: Structure logs for machine parsing first, human readability second (the logger handles formatting).

## Further Reading

- [Charm Logger](https://charm.sh/blog/the-charm-logger/)
- [Loggly: What is Structured Logging](https://www.loggly.com/use-cases/what-is-structured-logging-and-how-to-use-it/)
- [Otelic: What is Semantic Logging](https://otelic.com/en/reference-guide/what-is-semantic-logging-and-why-is-it-important)
