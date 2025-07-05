# Structured and Semantic Logging

**Structured logging** captures log entries as **key/value pairs** instead of raw strings. This makes logs easier for machines to parse and for humans to search.

**Semantic logging** goes a step further by **standardizing the keys**. Using predictable fields like `component`, `operation`, `request_id`, and `error` makes logs interpretable across different systems and teams.

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
log.Error("Failed to validate request", "component", "authn", "user", userID, "error", err)
```

This approach ensures:

- The message captures **what** happened.
- The key/value pairs capture **who**, **where**, and **why**.
- You can add whichever keys are meaningful for that event.

## Charm Logger in Atmos

Atmos uses the [Charm Logger](https://charm.sh/blog/the-charm-logger/), which offers:

- Simple APIs such as `log.Info`, `log.Warn`, and `log.Error`
- Automatic line wrapping and colorized output for terminals
- Friendly formatting for console output
- Optional raw JSON output for machine ingestion

Example:

```go
log.Warn("OCI image has no layers", "image", imageName, "component", "oci-parser")
```

This renders nicely in the terminal and can be configured to output JSON like:

```json
{
  "level": "warn",
  "message": "OCI image has no layers",
  "image": "nginx:latest",
  "component": "oci-parser"
}
```

## Linting and Conventions

Atmos runs `golangci-lint`, which includes Staticcheck rule `ST1019`.
That rule warns when logging keys are non-constant strings. To avoid an
explosion of rarely reused constants, some common logging keys are
already excluded from `ST1019` in our configuration. You can define your
own constants for frequently used keys or suppress the warning when it
makes the code clearer.

Follow the existing conventions in Atmos and keep suppression comments
to a minimum.

## Further Reading

- [Charm Logger](https://charm.sh/blog/the-charm-logger/)
- [Loggly: What is Structured Logging](https://www.loggly.com/use-cases/what-is-structured-logging-and-how-to-use-it/)
- [Otelic: What is Semantic Logging](https://otelic.com/en/reference-guide/what-is-semantic-logging-and-why-is-it-important)
