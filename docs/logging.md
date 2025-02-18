
# Go Log Levels

This rubric provides guidance on how to use log levels (`LogWarn`, `LogError`, `LogTrace`, `LogDebug`, `LogFatal`) consistently.

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
