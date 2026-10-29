# PRD: Plugin-defined Starlark functions

**Status:** Proposed follow-on. The native pilot has auth and command services, not a generic function service or automatic Starlark module registration. All new metadata and protocol operations below are proposed. [All extension types](README.md).

## User experience

A selected OCI plugin exposes its declared functions as a Starlark module named after the consumer's package alias:

```yaml
plugins:
  packages:
    quality:
      source: oci://ghcr.io/acme/repo-tools
      version: "^1.0"
```

```python
result = quality.check(paths=["main.go"], fix=False)
if not result["ok"]:
    fail("Repository checks failed")
```

Functions are available automatically in supported Atmos automation contexts after normal plugin resolution and admission. Consumers do not write `load()` statements or reference package cache paths. The syntax is `alias.function(...)`; Starlark has no `::` namespace-call operator.

The implementation may be a Go SDK handler reached over RPC or an exported function in a packaged Starlark module. The call shape and validation rules are the same. A single package may provide both implementations alongside native auth/command services and [automatically loaded Atmos configuration](../package-configuration.md).

## Scope and naming

The first adapter serves Atmos's existing Starlark automation engine: script-backed custom commands, workflow steps, and Git hook steps. Function availability is scoped to the current project's enabled, admitted package selection and current execution. User-installed packages cannot fill missing exports in a project-selected package.

Each exporting alias and function name must be a valid non-keyword Starlark identifier. Aliases cannot shadow predeclared names such as `atmos`, `fs`, or `json`; exports beginning with `_` are private and cannot be published. Invalid names and duplicate exports fail registration without silently rewriting names. An alias containing a hyphen can remain valid for other capabilities but must be renamed to expose a Starlark function module.

The module exposes only manifest-declared functions. Unknown attributes report the package and available public names. Imported implementation modules and arbitrary native methods do not become public exports.

This contract does not automatically add HCL or YAML function bindings. Those require separate decisions about evaluation phases, unknown and sensitive values, dependencies, caching, and cycles. It also does not expose plugin functions during package discovery, early configuration loading, or admission policy evaluation; those operations must not depend on code they are deciding whether to admit.

## Discovery and signatures

Introduce a versioned `function` capability descriptor with manifest exports under `spec.capabilities.functions`. Each export declares:

| Field | Requirement |
|---|---|
| Name and description | Stable public identifier and static help. |
| Parameters | Ordered names, types, required/optional status, and literal defaults. |
| Result | One explicit return type, including container element types. |
| Effects | `pure` or `effectful`; no implicit default. |
| Implementation | Native handler name, or package-relative Starlark file and symbol. |

Use the existing compatibility mechanism to negotiate the function contract. Static signature metadata supports help and argument validation without starting a process or evaluating a module. Native `Info.Describe` advertises the service and export set; the new function service supplies live definitions that must agree with the manifest before invocation. Any disagreement fails activation of that service rather than changing the signature at runtime.

Version one supports fixed parameter lists with positional and keyword arguments. Reject duplicate arguments, unknown keywords, missing required parameters, and type mismatches before calling the implementation. Optional defaults must match their declared types and be copied per invocation. Variadic parameters, callbacks, generators, and higher-order functions are outside this first contract.

Changing a signature follows package compatibility/versioning rules. The selected digest pins both the declarations and their implementation; the host never discovers a different function version outside that package selection.

## Value model

Define a language-neutral tagged value encoding shared by native RPC and the Starlark adapter. Do not use Protobuf `Struct` or JSON numbers as the transport for all values: they cannot preserve arbitrary Starlark integers.

| Starlark value | Wire representation and constraints |
|---|---|
| `None` | Explicit null tag; distinct from omitted parameters. |
| `bool` | Boolean. |
| `int` | Canonical signed decimal string, preserving arbitrary precision. |
| `float` | IEEE 754 double; reject non-finite values in both directions. |
| `str` | A distinct string tag preserving the original bytes, including non-UTF-8 Starlark strings. |
| `bytes` | Byte sequence, distinct from text. |
| `list` / `tuple` | Separate ordered sequence tags; recursively typed elements. |
| `dict` | Ordered string-key/value entries; preserve insertion order and reject duplicate keys. |

The signature type algebra covers these primitives, typed sequences/maps, and explicit nullable types. Containers use one declared element/value type; arbitrary unions and opaque records are deferred. Both arguments and results are validated recursively, with no implicit number/string conversion or list/tuple coercion. Unsupported values, including functions, arbitrary structs, file handles, and non-string dictionary keys, fail with an argument/result path.

Arguments are copied snapshots and returned containers are fresh caller-owned values. Mutation cannot cross the RPC boundary or leak between callers. Hosts enforce finite message-size, container-size, integer-length, and nesting limits before allocating unbounded data; exact defaults belong to the versioned protocol specification and must have conformance tests before implementation ships.

## Execution and effects

The Starlark engine receives an execution-scoped module provider through dependency injection. A module attribute wraps the corresponding function as a Starlark builtin. It resolves a pinned package export, converts values, invokes the adapter, and converts the validated result back into Starlark. It must not register project-specific functions in a process-global namespace.

Native functions run in the managed Go SDK plugin process through a new versioned `Functions` service with definition discovery and invocation operations. `go-plugin` provides process/RPC transport; the Atmos protocol owns signatures, values, and invocation semantics. Reuse an admitted process within its existing execution/session lifetime rather than starting a new process for every call.

Packaged Starlark functions execute in the host's embedded engine. Load and evaluate their implementation module lazily on first invocation, with cancellation and the same execution context as the call. Freeze module globals and scope module caches to the execution and pinned release. No module evaluation occurs during package installation, configuration loading, inspection, or help.

An effectful function may run checks, invoke tools, or modify permitted project files through the existing scoped host APIs. A pure function declares that it has no external side effects. Purity is an author contract, not a claim that a native process is OS-sandboxed. Both kinds have **no automatic retries or result caching** in version one; a crashed formatter must never be replayed implicitly.

The host supplies explicit execution services and options separately from `context.Context`. Credentials and privileged APIs are not granted merely because another capability in the package uses them. Existing admission and execution policy governs exposure. Package resources remain read-only, while filesystem/process operations use the caller's project or Git staged sandbox context. Package-relative resources must not change the invocation's working directory.

Package implementations share code through Go libraries or package-relative Starlark imports. They must not hard-code the consumer's alias to find their own implementation; callers choose that alias. Adding a function export does not require a package's YAML command to invoke its own public module.

## Failure, cancellation, and concurrency

Propagate the caller's cancellation and deadline through the RPC call and into embedded Starlark execution. Bound interpreter steps and recursive cross-function calls; detect module-load cycles. Cancellation, timeouts, native crashes, policy denial, and malformed returns become Starlark evaluation errors and retain the original cause for diagnostics.

Errors identify the alias, selected package version, function, script call site, and invalid parameter/result path where applicable. They do not print argument values or credentials by default. Function diagnostics use Atmos's logging/UI services; return values are not mixed with process stdout.

Concurrent invocations must isolate arguments, results, module state, and execution scopes. Native handlers receive explicit per-call context; the SDK documents their concurrency requirements. An in-flight call retains its selected release until it completes. Updating a lockfile cannot replace its schemas or implementation midway through execution.

## SDK and platform integration

Extend the Go SDK with typed function registration, signature validation, dispatch, and value helpers. Scaffold contract tests for every declared function, including invalid arguments and results. Authors implement handlers without importing Atmos internal packages or transporting Go-specific types over RPC.

Register the capability through the extension registry and compose the adapter through bootstrap. Add manifest validation, static/live export agreement, gRPC bindings, and the Starlark module provider. Keep installation, OCI selection, trust, lockfiles, and rollback in the existing package lifecycle.

A hybrid native package retains native OS/architecture/variant selection while shipping the same shared configuration and Starlark resources across payloads. Starlark-only packages depend on the planned platform-independent package support. Neither consumers nor function callers compile Go plugins locally.

Deliver native function/value contracts and the Starlark caller adapter first, then packaged Starlark implementations and hybrid configuration integration. Each delivered subset must state its supported contracts explicitly; the current native auth/command pilot does not claim any of these capabilities.

## Acceptance criteria

- One OCI declaration makes `quality.check(...)` callable without a manual import; changing the alias changes the public module.
- A Go SDK handler and a packaged Starlark handler pass the same signature/value conformance suite, including a package that also contributes `atmos.yaml` and native services.
- Tests cover keyword/positional binding, copied mutable defaults, invalid names, built-in collisions, duplicate exports, unknown functions, missing arguments, and static/live signature disagreement.
- Round trips preserve integers above `2**53` and large negative integers, `None`, bytes, tuples, list order, and dictionary insertion order. Invalid values, non-finite floats, duplicate wire keys, and oversized/deep payloads fail predictably.
- Result validation catches a buggy implementation; argument/result mutation cannot affect another invocation or its source values.
- Help and configuration loading do not start native processes or evaluate Starlark modules. Admission denial prevents either implementation from executing.
- Cancellation, deadlines, step limits, recursion, process crashes, and malformed responses produce useful errors without retries, leaked processes, or secret values.
- Simultaneous projects and releases do not share module globals, mutable defaults, credentials, or function registrations.
- Effectful check/format functions use the same project or staged-sandbox context as ordinary Atmos automation and preserve existing file/index guarantees.
- Authored implementation patch coverage is at least 90%; generated protocol bindings are measured and reported separately.

## References

- [Package configuration](../package-configuration.md), [package composition](../package-composition.md), and [protocol design](../platform/protocol.md).
- [Starlark dot expressions](https://github.com/google/starlark-go/blob/master/doc/spec.md#dot-expressions) and [Go builtin bindings](https://pkg.go.dev/go.starlark.net/starlark#NewBuiltin).
- [Terraform provider-defined function implementation](https://developer.hashicorp.com/terraform/plugin/framework/functions/implementation) illustrates discovery and typed invocation; Atmos owns its own contract rather than adopting the Terraform provider protocol.
