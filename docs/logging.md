# Logging Guidelines

This document defines the logging levels in Atmos and provides guidance on when to use each level. It covers the critical distinction between logging and user interface output.

## Core Principle: Logging is Not UI

**Logging** is for diagnostics and debugging. **UI output** is for user interaction.

**Key distinction:**
- **UI output is required** - Without it, the user can't use the command
- **Logging is optional metadata** - Adds diagnostic context but user doesn't need it

**Don't use `logger.Info()` for user messages** - users won't see them if logging is disabled. Use UI output instead (see [I/O and UI Output Guide](io-and-ui-output.md)).

**Quick test:** "What happens if I disable logging (`--logs-level=Off`)?"
- Breaks user experience → It's UI output, use `ui.Success()`, `ui.Error()`, `ui.Data()`
- User unaffected → It's logging, use `log.Debug()`, `log.Trace()`

**Examples:**

```go
// ❌ WRONG - using logger for user-facing output
log.Info("Configuration loaded successfully")

// ✅ CORRECT - UI for user, logging for diagnostics
ui.Success("Configuration loaded")
log.Debug("config.LoadAtmosYAML: loaded 42 stacks from atmos.yaml")
```

---

## Log Levels

### Trace

**Purpose**: Execution flow tracing, detailed debugging, and squelched error logging.

#### When to Use

- Function entry and exit points.
- Intermediate values during complex calculations.
- Branch decisions in control flow.
- Loop iterations in complex algorithms.
- Template expansion steps.
- Detailed state at each step of multi-stage operations.
- **Squelched errors** - errors that are intentionally ignored but should be recorded.

#### Characteristics

- Extremely verbose.
- Significant performance impact.
- Should never be enabled in production.
- Typically generates 10-100x more output than Debug.
- Useful for understanding the exact execution path through complex code.

#### Examples

- "Entering ProcessStackConfig with stack=dev, component=vpc"
- "Template evaluation: input={...}, context={...}, output={...}"
- "Cache lookup: key=stack-dev-vpc, found=true, age=2.3s"
- "Failed to close temporary file during cleanup" (squelched error)
- "Failed to remove temporary directory during cleanup" (squelched error)
- "Failed to bind environment variable" (squelched error)

---

### Debug

**Purpose**: Primary diagnostic information level. This is where most informational logging belongs.

#### When to Use

- Configuration values and settings.
- State transitions.
- External command execution.
- API calls and responses.
- File operations.
- Cache operations.
- Component and stack processing details.
- Validation outcomes.
- Most informational messages that developers need.

#### Characteristics

- This is the default level for diagnostic information.
- Safe for development and testing environments.
- Generally disabled in production.
- If unsure between Debug and Info, use Debug.

#### Examples

- "Stack configuration loaded from /stacks/dev.yaml"
- "Terraform workspace set to 'dev'"
- "Found 5 components in stack 'prod-us-east-1'"
- "Configuration merged: 15 keys updated"
- "Executing command: terraform plan"

---

### Info

**Purpose**: Sparse, high-level operational events. Almost never the right choice.

#### When to Use (Almost Never)

Info level is so rarely appropriate that it's difficult to justify. In a CLI tool like Atmos, there are almost no cases where Info is the right level. The user ran the command - they don't need a log telling them it started. They'll see the output or an error - they don't need a log telling them it finished.

#### Characteristics

- Should produce fewer than 10 log lines in a typical run.
- Not for routine operations.
- Not for progress indication.
- May be enabled in production for high-level visibility.
- If your Info logs are numerous, you're using it wrong.

#### When Info Might Be Appropriate

Info level is extremely rare. It might be appropriate for:
- Critical mode changes that fundamentally alter behavior (e.g., "Operating in offline mode due to network failure")
- Security-relevant events (e.g., "Authentication bypassed in development mode")
- Data loss warnings (e.g., "Running in ephemeral mode, changes will not be persisted")

Even these examples are borderline - they could arguably be Debug or Warning depending on context. When in doubt, use Debug.

---

### Warning

**Purpose**: Potentially problematic situations that don't prevent operation.

#### When to Use

- Deprecated feature usage.
- Retryable failures.
- Missing optional configuration (where defaults are applied).
- Performance degradation.
- Resource constraints approaching limits.
- Unusual but recoverable conditions.

#### Characteristics

- Always enabled in production.
- Indicates situations requiring attention but not immediate action.
- System continues functioning normally.

#### Examples

- "Retrying connection to database. Attempt 3 of 5."
- "Configuration value missing, using default."
- "Deprecated flag '--legacy' used, will be removed in v2.0"
- "Response time degraded: 5.2s (threshold: 2s)"

---

### Error

**Purpose**: Failure conditions that allow continued operation.

#### When to Use

- Failed operations that don't halt execution.
- Caught exceptions.
- Invalid input that can be skipped.
- Data integrity issues.
- Non-recoverable API or system call failures.

#### Characteristics

- Always enabled in production.
- Indicates failures requiring investigation.
- System continues but potentially in degraded state.

#### Examples

- "Failed to persist cache: disk full"
- "Invalid component configuration: missing required field 'vpc_id'"
- "API request failed after 3 retries"

---

### Fatal

**Purpose**: Unrecoverable errors requiring termination.

#### When to Use

- Missing required configuration.
- Unrecoverable initialization failures.
- Critical resource unavailability.
- Data corruption that prevents safe continuation.

#### Behavior

- Logs error message and immediately exits with non-zero code.
- Last message before process termination.

#### Examples

- "Critical failure: Unable to load required configuration. Exiting."
- "Fatal: Cannot establish database connection after 10 attempts"

---

## Squelched Errors

### What Are Squelched Errors?

Squelched errors are errors that are intentionally ignored because they don't affect the critical path of execution. Common examples include:
- Cleanup operations (removing temporary files, closing file handles)
- Non-critical configuration binding (environment variables, flags)
- Resource unlocking in defer statements
- UI rendering errors

### When to Squelch Errors

Errors should only be squelched when:
1. **The error is non-critical** - failure doesn't prevent the operation from succeeding
2. **Recovery is impossible** - we're in cleanup/defer code where we can't do anything about the error
3. **The operation is best-effort** - like optional environment variable binding

### The Golden Rule: Always Log Squelched Errors

Even when errors are intentionally ignored, they **must** be logged at Trace level. This ensures errors are never truly lost and can be investigated during debugging.

### Pattern: Squelched Error Logging

**Wrong**: Silent error squelching
```go
// ❌ WRONG: Error is completely lost
_ = os.Remove(tempFile)
_ = file.Close()
_ = viper.BindEnv("OPTIONAL_VAR", "VAR")
```

**Right**: Log squelched errors at Trace level
```go
// ✅ CORRECT: Cleanup with trace logging
if err := os.Remove(tempFile); err != nil && !os.IsNotExist(err) {
    log.Trace("Failed to remove temporary file during cleanup", "error", err, "file", tempFile)
}

// ✅ CORRECT: Resource closing with trace logging
if err := file.Close(); err != nil {
    log.Trace("Failed to close file", "error", err, "file", file.Name())
}

// ✅ CORRECT: Non-critical configuration with trace logging
if err := viper.BindEnv("OPTIONAL_VAR", "VAR"); err != nil {
    log.Trace("Failed to bind environment variable", "error", err, "var", "OPTIONAL_VAR")
}
```

### Special Cases

#### Defer Statements
When squelching errors in defer statements, capture them in a closure:

```go
defer func() {
    if err := lock.Unlock(); err != nil {
        log.Trace("Failed to unlock", "error", err, "path", lockPath)
    }
}()
```

#### File Existence Checks
When removing files, check for `os.IsNotExist` to avoid logging expected conditions:

```go
if err := os.Remove(tempFile); err != nil && !os.IsNotExist(err) {
    log.Trace("Failed to remove file", "error", err, "file", tempFile)
}
```

#### Log File Cleanup
When cleaning up log files, use stderr instead of the logger to avoid recursion:

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

### Why This Matters

1. **Debugging**: When investigating issues, trace logs reveal the full story, including non-critical failures
2. **Metrics**: Aggregate trace logs to identify patterns (e.g., frequent cleanup failures might indicate disk issues)
3. **Auditing**: Complete error trail for compliance and security reviews
4. **Transparency**: No hidden errors - everything is recorded somewhere

### Common Squelched Error Categories

| Category | Examples | Trace Logging Required |
|----------|----------|----------------------|
| File cleanup | `os.Remove()`, `os.RemoveAll()` | ✅ Yes |
| Resource closing | `file.Close()`, `client.Close()` | ✅ Yes |
| Lock operations | `lock.Unlock()` | ✅ Yes |
| Config binding | `viper.BindEnv()`, `viper.BindPFlag()` | ✅ Yes |
| UI rendering | `fmt.Fprint()` to UI buffers | ✅ Yes |
| Command help | `cmd.Help()` | ✅ Yes |

---

## Common Anti-Patterns

### Using Logging as UI

**Wrong**: Using Info level for user feedback
```go
log.Info("Deploying component 'vpc'...")
log.Info("✓ Component deployed successfully!")
log.Info("Starting validation...")
log.Info("Validation passed")
```

**Right**: Separate UI from logging
```go
// User feedback via TextUI
utils.PrintfMessageToTUI("Deploying component 'vpc'...\n")
utils.PrintfMessageToTUI("✓ Component deployed successfully!\n")

// Diagnostic logging at appropriate level
log.Debug("Component deployment started", "component", "vpc", "stack", stack)
log.Debug("Component deployment completed", "component", "vpc", "duration", duration)
```

This separation ensures:
- Users see progress and status (via TextUI).
- Developers can debug issues (via logs).
- Logs can be disabled without breaking the user experience.
- Log aggregation systems don't get polluted with UI elements.

### Misusing Info Level

**Wrong**: Using Info for routine operations
```go
log.Info("Loading configuration file")
log.Info("Processing stack 'dev'")
log.Info("Found 5 components")
log.Info("Running terraform plan")
```

**Right**: Use Debug for diagnostic information
```go
log.Debug("Loading configuration", "file", configPath)
log.Debug("Processing stack", "stack", "dev")
log.Debug("Component scan complete", "count", 5)
log.Debug("Executing terraform", "command", "plan")
```

Info level should be so sparse that seeing an Info log makes you pay attention. If Info logs are scrolling by constantly, they've lost their significance.

### Progress Indicators in Logs

**Wrong**: Using any log level for progress
```go
log.Info("Processing 1 of 10...")
log.Info("Processing 2 of 10...")
log.Info("Processing 3 of 10...")
```

**Right**: Progress belongs in UI, summary in logs
```go
// Show progress to user
utils.PrintfMessageToTUI("Processing components: 3/10\r")

// Log the summary once
log.Debug("Component processing completed", "total", 10, "duration", totalTime)
```

---

## Debug vs Info: The Critical Distinction

### The Litmus Test

Ask yourself: "Is this something the user needs to see to use the tool?"
- Yes → Use TextUI output
- No → It's logging

Then ask: "Would hiding this information impact operations or debugging?"
- Yes → Debug level
- No → Don't log it at all

Info level is almost never the answer. If you're considering Info, you probably want Debug or Warning.

### Real Examples

Info level is so rarely appropriate that it's hard to provide good examples. In practice, almost everything should be Debug level:

These are all Debug level (diagnostic details):
- "Atmos initialized"
- "Connected to Atmos Pro"
- "Graceful shutdown initiated"
- "Stack configuration loaded"
- "Terraform plan completed"
- "Component validation passed"
- "Cache refreshed"
- "Template rendered"

The user doesn't need to know any of these things - they just want the tool to work. If you're struggling to justify why something should be Info instead of Debug, it should be Debug.

These are NOT logging at all (user interface):
- "✓ Successfully deployed"
- "Press Enter to continue"
- "Deploying component..."
- Progress bars or percentages

---

## Why This Matters

### For Production Operations

When logs are properly leveled:
- Warning and Error levels provide signal without noise.
- Info level highlights significant events worth noting.
- Debug level can be enabled temporarily for troubleshooting.
- Trace level exists for deep debugging without code changes.

### For Development

Proper log levels mean:
- Debug logs provide useful diagnostics without terminal spam.
- Trace level captures detail when hunting complex bugs.
- Log output doesn't interfere with UI testing.
- Performance impact is predictable.

### For Users

Correct separation means:
- They see what they need to see (via TextUI).
- They don't see what they don't need (logs).
- `--logs-level=Off` doesn't break their experience.
- Error messages are actionable, not buried in noise.

---

## Severity Hierarchy

`Fatal > Error > Warning > Info > Debug > Trace`

Production systems typically run with Warning or Error as minimum level.
Development environments typically use Debug level.
Trace level is reserved for specific debugging sessions.

---

## Performance Considerations

### Trace Level Impact

- Can increase log volume by 10-100x compared to Debug.
- Significant performance overhead due to:
  - String formatting of complex objects.
  - File I/O for massive log volumes.
  - Memory usage for buffering.
- Should only be enabled temporarily for specific debugging.
- Consider using conditional logging for expensive operations.

### Enabling Trace Level

Trace should only be enabled:
- During active debugging sessions.
- For specific problematic operations.
- In development environments.
- With output redirected to files (not terminals).
- For limited time periods.

Example:
```bash
# Enable trace only for specific debugging session
ATMOS_LOGS_LEVEL=Trace atmos terraform plan component -s stack

# Or temporarily in config (remember to revert!)
logs:
  level: Trace
```

---

## Structured and Semantic Logging

Structured logs record events as key/value pairs so machines and humans can parse them. Semantic logging standardizes those keys so logs can be understood across tools and teams. Atmos uses the [Charm Logger](https://charm.sh/blog/the-charm-logger/) for this purpose.

See [structured-logging.md](structured-logging.md) for detailed guidance and examples.
