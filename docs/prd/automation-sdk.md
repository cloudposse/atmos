# Shared automation services and interpreter adapters

**Last Updated:** 2026-10-06

**Status:** Initial interfaces and Starlark adapter implemented in the current PR stack.
TypeScript remains a design possibility, not an implemented interpreter or committed release.

**Related:** [Automation language](starlark-automation-and-command-testing.md),
[Git-hook steps](git-hook-steps.md), [workflow steps](workflow-step-types.md).

## Problem and goal

The Starlark implementation identified services that any embedded language needs:
process execution, step dispatch, filesystem inspection, configuration context,
output streams, and cancellation. Language-specific values should not become the
public Go contract for those services. A second interpreter should reuse the same
Atmos behavior instead of implementing another set of steps or deployment policies.

This is an incremental SDK boundary, not a claim that every Atmos subsystem is
already exposed as a stable, versioned Go SDK.

## Implemented boundaries

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

## Step contract

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

## Processes and data

Shared execution policy bounds attempts and backoff with one timeout. Process
calls preserve stdout, stderr, and exit status. The Starlark result's lazy `data`
view decodes JSON without replacing stdout; parsing errors occur on access, not
when the process completes. Step results use the same idea over `value`.

Named list/describe wrappers and config-get wrappers default to capture and JSON
where the selected command supports a format flag. Explicit caller choices win.
The generic `atmos.run` remains a literal CLI invocation. These wrappers still
launch Atmos subprocesses; they are not direct native query APIs.

## Filesystem contract

`FileSystem` supplies context-aware glob, stat, existence, and symlink-target
operations. `LocalFileSystem` uses the OS. See [Git-hook steps](git-hook-steps.md#filesystem-api)
for errors, path rules, and symlink behavior. Hosts can inject another implementation
through the Starlark engine's options. Existing module/file-content injection remains
separate; this PR does not redesign module loading or add filesystem writes.

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

## Acceptance

- Go callers can use shared services without importing Starlark.
- Registered step behavior is implemented once and exercised independently of a language adapter.
- Host inputs, output streams, cancellation, and error chains survive conversion.
- Concurrent invocations cannot mutate each other's step state or input slices.
- Tests verify raw and decoded results, explicit output/format overrides, and failure paths.
- A future engine can register without changing standalone filename dispatch.
