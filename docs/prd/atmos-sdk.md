# Atmos SDK

**Last Updated:** 2026-10-06

**Status:** Proposed SDK scope. Initial shared execution interfaces and the Starlark
adapter are implemented; a complete, stable Go SDK is not yet available.

**Related:** [Automation language](starlark-automation-and-command-testing.md),
[Git-hook steps](git-hook-steps.md), [workflow steps](workflow-step-types.md).

## Problem

Applications need to use Atmos capabilities directly: resolve a component's
configuration, find affected stacks, install a tool, assume an identity, or run a
deployment. Today, an integration must choose among invoking CLI commands and
parsing their output, coupling itself to implementation packages, or reproducing
Atmos behavior. Each approach adds integration and maintenance work. Subprocesses
also make it harder to pass cancellation, structured results, and errors between
the application and Atmos.

The same need exists inside Atmos. Commands, workflows, hooks, and embedded
languages need access to common capabilities with consistent behavior. Each entry
point should be able to call the same service and adapt its inputs and results to
its own interface.

## Goal

Provide a supported Go SDK that applications can use to embed Atmos capabilities.
Callers should supply configuration and execution context, invoke typed operations,
and receive structured results and errors. Atmos should own its configuration
resolution, execution rules, and integration with tools and identities.

The SDK serves applications such as deployment services, developer tools, CI
integrations, and Atmos itself. The CLI and language interpreters are consumers
of this API. Supporting another interpreter is one use case within this broader
scope.

## Scope

The target is an SDK for Atmos as a whole, introduced incrementally across these
capability areas:

- **Configuration and queries:** resolve stacks and components, inspect metadata,
  and identify dependencies and affected components.
- **Tools and identities:** resolve and install toolchain dependencies and prepare
  authenticated execution contexts.
- **Execution:** run commands and registered steps, invoke component operations,
  and execute workflows and hooks with their documented lifecycle behavior.
- **Project operations:** vendor dependencies, generate files, scaffold projects,
  and validate configuration through the same services used by Atmos commands.
- **Results and interaction:** expose structured results, named outputs, error
  chains, input prompts, and output streams through caller-supplied interfaces.

These areas define the intended scope, not a list of completed public APIs. Each
area needs an explicit contract, tests, and a compatibility policy before it is
promoted to a supported SDK surface. Callers should be able to use the capabilities
they need without initializing an interpreter or a CLI command tree.

## Initial implementation

The shared execution interfaces below are the first implemented part of this SDK
direction. They provide reusable contracts for step execution, process policies,
and filesystem inspection. Configuration queries and several other operations
still pass through CLI wrappers. Those wrappers do not yet provide the direct,
typed APIs described in the target scope.

The full SDK design must establish package organization, supported entry points,
and compatibility guarantees before committing to a public API. The package
names below describe the current implementation.

### Implemented boundaries

| Layer | Responsibility |
|---|---|
| `pkg/automation` | Native Go contracts for step calls/results, execution policies, and filesystem inspection. No Starlark dependency. |
| `pkg/script` | Interpreter registry, invocation/result types, standalone file detection, and shared host services. |
| `pkg/script/starlark` | Language evaluation, argument/result conversion, immutable values, module loading, and Starlark diagnostics. |
| `pkg/runner/step` | Registered step implementations and the automation adapter that validates and executes them. |
| CLI/workflow/hook hosts | Configuration, available component services, invocation streams/inputs, and their own lifecycle policies. |

The registry associates interpreter names with engines and case-sensitive file
extensions. One registration may own the Atmos-shebang and default-stdin fallback.
Conflicting registrations fail at registration time. Starlark currently registers
`.star`; no TypeScript or JavaScript extension is registered.

### Step contract

`StepLibrary` exposes discovery, validation, execution, and state forks. A `StepCall`
contains native Go configuration, cwd/environment, streams, and parallel context.
A `StepResult` contains the primary string value, selections, metadata, named
outputs, skipped status, and diagnostic text. Language adapters decide how to
expose those values without changing handler semantics.

Each invocation owns mutable step state. A parallel branch gets a snapshot;
branch mutations do not escape to siblings or the parent. Named results are
available to later calls in the same branch. A task retry starts from a fresh
snapshot. Caller-owned configuration and argument slices must remain unchanged.

Git hooks use the adapter's typed `RunSteps` entry point for synchronous lists.
Workflow scheduling, identity preparation, background jobs, and freshness remain
host responsibilities; unsupported direct-call policies fail explicitly.

### Processes and data

Shared execution policy bounds attempts and backoff with one timeout. Process
calls preserve stdout, stderr, and exit status. The Starlark result's lazy `data`
view decodes JSON without replacing stdout; parsing errors occur on access, not
when the process completes. Step results use the same idea over `value`.

Named list/describe wrappers and config-get wrappers default to capture and JSON
where the selected command supports a format flag. Explicit caller choices win.
The generic `atmos.run` remains a literal CLI invocation. These wrappers still
launch Atmos subprocesses; they are not direct native query APIs.

### Filesystem contract

`FileSystem` supplies context-aware glob, stat, existence, and symlink-target
operations. `LocalFileSystem` uses the OS. See [Git-hook steps](git-hook-steps.md#filesystem-api)
for errors, path rules, and symlink behavior. Hosts can inject another implementation
through the Starlark engine's options. Existing module/file-content injection remains
separate; the initial interfaces do not redesign module loading or add filesystem writes.

## Future interpreter requirements

A future TypeScript adapter would register an engine and extensions, convert
arguments/results at this boundary, and reuse these services. It must define
transpilation and type-checking separately, module loading, JS runtime compatibility,
asynchronous execution, cancellation, and error mapping before becoming supported.
Registering an extension alone does not supply a Node/npm environment.

Do not impose Starlark's frozen-value model on every language. Require equivalent
invocation and concurrent-state isolation through each runtime's supported model.
Likewise, cancellation and interpreter execution counters are not hard memory or
CPU isolation for exposed native operations.

## SDK acceptance

- An application can use supported Atmos capabilities through typed Go APIs without launching the CLI.
- Configuration, toolchain, identity, and execution contracts state their dependencies and lifecycle responsibilities.
- Public entry points document compatibility guarantees and distinguish supported APIs from internal implementation packages.
- CLI commands and language adapters reuse the same underlying services as external callers.

### Initial execution interfaces

- Go callers can use shared services without importing Starlark.
- Registered step behavior is implemented once and exercised independently of a language adapter.
- Host inputs, output streams, cancellation, and error chains survive conversion.
- Concurrent invocations cannot mutate each other's step state or input slices.
- Tests verify raw and decoded results, explicit output/format overrides, and failure paths.
- A future engine can register without changing standalone filename dispatch.
