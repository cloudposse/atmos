
# Logging Guidelines

This document explains how we log events in Atmos. It covers the
difference between writing to the terminal (TextUI) and writing logs,
outlines our log levels, and introduces structured and semantic
logging.

## TextUI vs. Logging

The terminal window is effectively a text-based user interface (TextUI)
for our CLI. Anything intended for user interaction—menus, prompts,
animations—should be rendered to the terminal as UI output.

Logging is different: logs are structured records of what the program is
doing. They help developers debug issues and feed into telemetry or
monitoring systems. Treat logging like sending messages to the developer
console in a browser, not to the UI seen by the user.

Keeping TextUI output and logging separate ensures we don't mix user
interface concerns with operational telemetry.

Atmos provides `utils.PrintfMessageToTUI` for writing TextUI messages.
It always writes to `stderr` so that `stdout` can remain a clean data
stream (for example, JSON output that may be piped to another command).
Avoid using `fmt.Println` or `fmt.Printf` for UI output because they
default to `stdout` and can corrupt piped data. Log functions such as
`log.Info` or `log.Error` are not meant for TextUI and may be silenced
via log level configuration. See
[structured-logging.md](structured-logging.md) for guidance on composing
well-structured log messages.

---

## Go Log Levels

This rubric provides guidance on how to use log levels (`LogWarn`,
`LogError`, `LogTrace`, `LogDebug`, `LogFatal`) consistently.

---

## LogWarn

**Purpose**: To highlight potentially harmful situations that require attention but do not stop the program from functioning.

### When to Use

- A deprecated API or feature is used.
- A retryable failure occurs (e.g., transient network issues, service timeouts).
- An unusual but recoverable condition arises.
- Missing optional configuration or resources (e.g., a default is applied).
- Significant performance degradation is detected.

### Examples

- ```console
  "Retrying connection to database. Attempt 3 of 5."
  ```

- ```console
  "Configuration value missing, using default."
  ```

---

## LogError

**Purpose**: To log serious issues that indicate a failure in the application but allow it to continue running (possibly in a degraded state).

### When to Use

- A critical operation failed (e.g., failed to write to a persistent store).
- Unexpected exceptions or panics caught in non-critical areas.
- Data integrity issues are detected (e.g., corrupted input).
- Non-recoverable API or system calls fail.

### Examples

- ```console
  "Failed to persist user data: %v"
  ```

- ```console
  "Unexpected nil pointer dereference at line 42."
  ```

---

## LogTrace

**Purpose**: To log detailed information useful for understanding program flow, particularly for debugging or performance optimization.

### When to Use

- Fine-grained details of function execution.
- Tracing API call flows or middleware behavior.
- Observing request/response lifecycles or context propagation.
- Profiling individual steps in complex operations.

### Examples

- ```console
  "Entering function 'ProcessRequest' with args: %v"
  ```

- ```console
  "HTTP request headers: %v"
  ```

---

## LogDebug

**Purpose**: To log information that is useful during development or debugging but not typically needed in production environments.

### When to Use

- Diagnosing application logic (e.g., unexpected state transitions).
- Verifying the correctness of logic or assumptions.
- Debugging integration issues (e.g., inspecting external service responses).
- Validating values of configurations, variables, or states.

### Examples

- ```console
  "Database query executed: %s"
  ```

- ```console
  "Feature toggle '%s' enabled."
  ```

---

## LogFatal

- **Purpose**: To log a critical error that prevents the application from continuing, and then terminate the application with a non-zero exit code.

### When to Use

- A failure occurs that cannot be recovered and requires immediate shutdown.
- Critical initialization steps fail (e.g., missing configuration, database connection failure).
- Irreparable corruption or state inconsistency is detected.

### Behavior

- Logs an error message at the `LogError` level.
- Terminates the application using `os.Exit(1)` or an equivalent method.

### Examples

- ```console
  "Critical failure: Unable to load required configuration. Exiting."
  ```

- ```console
  "Fatal error: Database connection failed. Application shutting down."
  ```

---

## Structured and Semantic Logging

Structured logs record events as key/value pairs so machines and humans
can parse them. Semantic logging standardizes those keys so logs can be
understood across tools and teams. Atmos uses the
[Charm Logger](https://charm.sh/blog/the-charm-logger/) for this
purpose.

See [structured-logging.md](structured-logging.md) for detailed guidance
and examples.

---

## General Guidance

### Severity Hierarchy**

Log levels should reflect severity and importance:

- `LogFatal > LogError > LogWarn > LogDebug > LogTrace`.

### Production Use (e.g. CI)**

- `LogError` and `LogWarn` should always be enabled in production to capture issues.
- `LogDebug` and `LogTrace` are often disabled in production to reduce verbosity.

### Consistency

- Use structured logging whenever possible (e.g., include fields like `requestID`, `userID`, or `operation`).
- Adopt a centralized logging format (e.g., JSON) for better parsing and analysis.

This rubric ensures clarity in log messaging, making it easier to diagnose and address issues effectively in any Go application.
