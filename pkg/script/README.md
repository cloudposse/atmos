# Embedded script host

`script.Engine` is the boundary between Atmos execution hosts and embedded
languages. Workflow steps, custom commands, hooks, and standalone files pass a
`Spec` to an engine and receive a `Result`. Starlark is currently the only
registered language.

## Registration and file dispatch

An interpreter registers its name, engine, and optional standalone file metadata:

```go
script.Register("starlark", engine,
    script.WithExtensions(".star"),
    script.WithAtmosShebang(),
)
```

Workflow `interpreter:` values use the name registry. Standalone files use the
extension registry. Extensions are case-sensitive and include the leading dot.
A registered extension takes precedence over a file's shebang. Selection uses the
invoked filename, before resolving symlinks; the resolved path is used to read the
source and locate imports.

Explicit paths with an Atmos shebang use the registered shebang fallback when
there is no recognized extension. Starlark owns that fallback. Bare extensionless
names remain Atmos commands. Unknown extensions without an Atmos shebang do not
select an embedded interpreter.

Registration rejects invalid metadata and conflicting extension or shebang
owners before changing the registry. Replacing an engine under the same name
preserves its associations. This allows CLI initialization to inject the completed
command catalog without losing standalone dispatch.

## Stdin execution

`atmos - [args...]` reads a standalone script from stdin. The optional `--`
immediately following `-` is consumed; later arguments belong to the script.
`atmos --interpreter=starlark - [args...]` selects an engine by its registered
name. Without that option, the registered Atmos shebang fallback is used.
A bare `--` does not select stdin execution.

Detection does not read stdin. The CLI reads it through EOF when the standalone
command runs, after normal initialization.
`File.Stdin` distinguishes stdin from a physical file; `File.Path` is then `-`.
Starlark reports `<stdin>` in tracebacks, exposes `ctx.script = None`, and resolves
relative imports from the working directory. The standalone command parser still
provides typed arguments, flags, validation, and help.

## Shared host services

- `PrepareSpec` normalizes paths and provides discard streams for absent writers.
  Callers pass their own invocation copy; maps and slices are treated as read-only.
- `RunProcess` owns capture, optional streaming, cancellation, and exit-status
  policy. `check=false` tolerates ordinary nonzero exits, but still reports launch,
  signal, cancellation, and I/O failures. Terraform's plan exit-code exception is
  explicit. The interpreter supplies writers with masking and task attribution.
  Optional execution policies come from `pkg/automation`: one timeout bounds all
  attempts and backoff. Only ordinary nonzero exits with `Check=true` retry;
  output conditions inspect each attempt, and returned capture is from the last attempt.
- `ProcessEnvironment` copies inherited values and applies explicit overrides.
  Empty process environments remain empty rather than inheriting host secrets.
- `Tools` owns pins and installed directories per invocation. A successful pin is
  reused; conflicting versions fail. Failed or canceled installation does not
  publish state. Interpreters decide when declarations are permitted.
- `Diagnostic` carries immutable authored error metadata through wrappers.
  `EnrichDiagnostics` applies it at the reporting boundary, innermost first, so
  outer metadata wins and shared causes are not applied twice. The native error
  builder and reporting path continue to own rendering and telemetry.

Host configuration errors use `ErrScript`. Starlark evaluation failures retain
`ErrStarlark`; its binding also preserves the existing Starlark process and
argument classifications when translating shared service errors.

## Execution contract

Each invocation owns its mutable state. Engines must honor cancellation in both
language evaluation and host operations. Dry runs validate source without
executing scripts or host services; recursive validation of imported modules is
not implied.

Stdout and stderr are separate from the explicit result. `HasOutput=false` means
no result was produced. When present, strings pass through unchanged, including
empty strings; other supported language values are JSON-encoded by the engine.
The Starlark binding treats top-level `None` as no output; null values inside
containers are JSON-encoded. Workflow named outputs remain the runner's responsibility.

Language bindings own argument conversion, module resolution, language values,
callbacks, thread rules, and tracebacks. Parallel scheduling and retries reuse
Atmos services, while callable isolation remains interpreter-specific. The host
contract does not prescribe an event loop, cross-language values, or a universal
module loader.

## Step library

`Spec.Steps` supplies the language-independent `automation.StepLibrary` Go API.
Hosts construct it from their existing step variables and workflow context.
Starlark only maps function names, converts arguments/results, and forks the API
for each parallel task attempt. The step registry owns validation and execution.
See [the automation API](../automation/README.md) for the Go contract and ownership
rules. A standalone program and an embedded script use the same interface.
