# Automation Go API

This package defines automation contracts and execution policies shared by Go
callers and embedded language adapters. It has no dependency on Starlark. `pkg/runner/step.NewAutomationLibrary`
implements the API using the existing step registry, schema, and executor.

```text
Go caller ───────────────────────────┐
                                    v
Starlark steps.* -> automation.StepLibrary -> step registry -> existing handlers
```

A future interpreter supplies argument/result conversion and calls this same
interface. It does not implement prompts, HTTP, containers, or step validation
again. Interpreter registration and the other shared host services remain in
`pkg/script`; this package establishes the step-facing SDK contract, not a
separate implementation of those services.

## Go usage

```go
library := step.NewAutomationLibrary(nil, nil)
request := &automation.StepCall{
    Type: "join",
    Configuration: map[string]any{
        "options": []string{"api", "worker"},
        "separator": ",",
    },
}
if err := library.Validate(request); err != nil {
    return err
}
result, err := library.Run(ctx, request)
// result.Value is "api,worker" when err is nil.
```

Import `github.com/cloudposse/atmos/pkg/automation` for the contract and
`github.com/cloudposse/atmos/pkg/runner/step` for its implementation. A CLI host
passes its existing `Variables` and optional `WorkflowDefinition` to the
constructor to supply Atmos configuration, component resolution, and template
context. The CLI also links the workflow package's control/test runners.

## Ownership and execution

- `Names` discovers canonical names and aliases from the registry.
- `Validate` checks configuration without invoking a handler. `Run` validates
  again before execution; callers are not required to call `Validate` first.
- `Configuration` contains native Go values using the documented YAML field
  names. The shared schema decodes polymorphic fields such as `with` and `output`.
  Configuration decoding does not mutate the request.
- `StepResult` carries the handler's value, selections, metadata, named outputs,
  skipped flag, and error text. Treat returned results as read-only.
- An instance owns its mutable state. Call `Fork` before using it in a concurrent
  branch; fork the parent before starting its children, and wait for them before
  mutating the parent again. AAL does this automatically for every task attempt.
- Calls preserve context cancellation and error chains. Handler-level terminal
  and workflow-context requirements still apply.
- Explicit writers route command output through the caller. Captures retain raw
  values while displayed output uses Atmos masking. Interactive forms and casts
  retain the existing handlers' terminal facilities.
- `Parallel` rejects operations that require exclusive terminal/process access.
  Nested scripts inherit that restriction. Nested automation calls have a depth
  limit so starting another embedded interpreter cannot bypass recursion limits.
- Workflow scheduling, identity preparation, background jobs, and freshness
  policies remain responsibilities of the enclosing YAML runner. Unsupported
  direct-call policies fail explicitly.

The Go API is exercised independently in `pkg/runner/step/automation_library_test.go`.
The Starlark adapter is tested against a generated mock of `StepLibrary`, with
additional handler integration tests in `pkg/runner/step/script_library_test.go`.

## Execution policy

The `ExecutionPolicy` type applies one timeout across an operation and its retry
waits. The operation receives a context; the caller supplies the predicate that
decides which errors may be retried. It reuses the shared retry implementation.
Script process calls use this policy for `exec.run` and `component.exec`.
The step registry adapter still owns handler-specific execution behavior.

## Filesystem inspection

The `FileSystem` interface supplies context-aware `Glob`, `Stat`, `Exists`, and
`Readlink` operations. `LocalFileSystem` implements them with the operating system.
`FileInfo` carries byte size and regular-file, directory, and symlink flags.
Interpreters resolve paths against their invocation directory and convert the
results to language values. The Starlark engine accepts an alternate implementation
through `WithFileSystem`.

For synchronous host-owned step lists, `AutomationLibrary.RunSteps` accepts
`schema.Tasks` plus invocation streams and working directory. It validates the
sequence before execution and shares outputs and environment changes between
steps. Workflow scheduler policies are rejected. Atmos Git hooks use this path.
