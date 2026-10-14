# Starlark automation and command testing

**Last Updated:** 2026-10-06

**Status:** Interpreter registration, standalone and stdin execution, script-step
integration, direct step-library calls, structured errors, decoded query results,
process execution controls, and Git-hook scripts are implemented in the current
PR stack. Direct service modules and test-file discovery remain proposed. This
status does not identify a released version.

**Related:** [Atmos SDK](atmos-sdk.md), [Git-hook steps](git-hook-steps.md),
[native stack definitions (exploratory)](starlark-stack-definitions.md).

## Problem Statement

Custom commands and workflows need reusable, testable orchestration without large Bash
programs. Automation authors otherwise repeat process handling, retries, argument
parsing, and assertions across scripts, making failures and lifecycle behavior
harder to reason about. Embedded Starlark addresses this with native Atmos execution
services and standalone tools; repository examples and field-test fixes provide
evidence of these problems, but aggregate user frequency and time savings have not
been measured.

YAML custom commands retain their YAML declarations and component bindings.
Their embedded script steps receive parsed inputs directly. Standalone scripts
can declare their own arguments, typed flags, validation, and help in Starlark.

## Goals

1. Let automation authors reuse functions and modules across commands, workflows,
    hooks, and standalone tools, verified through each supported host integration.
2. Preserve cancellation, retries, environment isolation, and deterministic result
    ordering under concurrent execution, verified with controlled runtime tests.
3. Preserve parsed input types and literal user data, with no template rewriting
    or cross-invocation mutation in the input regression cases.
4. Give CLI users generated help and actionable errors, separating usage mistakes
    from script failures and keeping data output usable by other programs.
5. Let maintainers test automation through injected process, clock, and component
    services without requiring live infrastructure for unit validation.

## Non-Goals

- Replace YAML stack definitions; native stack authoring is a separate exploration.
- Implement full Python compatibility or replace every existing Python/shell tool;
  Starlark is an embedded language with a deliberately limited standard library.
- Run the embedded interpreter inside workflow containers; scripts currently opt
  out with `container: false`, and external interpreters retain their own paths.
- Implement a new state/secret engine or give direct script calls ownership of
  workflow background jobs; shared steps retain their host-context requirements.
- Execute external Safire migrations or publish script packages as part of this
  feature's tests; those require separate scope and side-effect decisions.

## User Stories

1. As a custom-command author, I want scripts to receive parsed flags and arguments
    so that I do not need to interpolate user values into source code.
2. As a workflow author, I want reusable functions with bounded parallel execution
    so that related operations share consistent retries, timeouts, and cancellation.
3. As a hook author, I want component and lifecycle context so that checks operate
    against the same resolved configuration as the parent operation.
4. As a standalone-tool author, I want typed inputs, validation, and generated help
    so that my script behaves like a documented CLI.
5. As a CLI user, I want invalid arguments to identify the mistake and show usage
    so that I can correct input without interpreting a Starlark traceback.
6. As an automation maintainer, I want literal braces, commas, missing optional
    inputs, and empty results handled explicitly so that user data is not corrupted.
7. As a test author, I want injectable host services so that I can verify failure,
    retry, and cancellation behavior without deploying infrastructure.

## Requirements

Priorities distinguish the implemented core from proposed extensions. Future
items are not commitments to ship; their contracts require further design.

### Must-Have (P0): implemented core

| ID | Requirement and acceptance criterion | Dependencies and constraints |
|----|--------------------------------------|------------------------------|
| R1 | Execute `interpreter: starlark` in process across supported script-step hosts; dry runs parse entry source without executing it. | Embedded engine registry, step adapters, and `container: false`; AC-01. |
| R2 | Preserve typed command inputs, literal fields, environment data, and output values across sequential/control-step boundaries. | Native command parsers, literal-field metadata, and step output handling; AC-02/AC-03. |
| R3 | Execute parallel functions with bounded concurrency, input-order results, failed-only retries, and cancellable deadlines. | Process runner, injected retry clock, frozen shared values; AC-04. |
| R4 | Expose resolved component/hook context and registered Atmos command wrappers through existing services. | Stack resolver, command catalog, authentication and masking; AC-05. |
| R5 | Support standalone shebang/file execution, declared interfaces, help, leading global flags, and argument-preserving re-execution. | CLI host, version/profile re-execution, installed Atmos binary; AC-06. |
| R6 | Keep usage errors distinct from script errors, enforce recursion protection, and route data/diagnostics consistently. | Error rendering, output writers, logger and cancellation; AC-07. |

### Nice-to-Have (P1): proposed follow-ups

| Capability | Acceptance criterion before becoming supported | Dependency |
|------------|------------------------------------------------|------------|
| Direct secret, store, and Terraform state/output APIs | Preserve identity, scope, masking, caching, cancellation, and provider behavior under success and failure tests. | Existing service adapters; see service-access requirements below. |
| Native `components.list` | Return documented structured results through the existing resolver with explicit scope and cancellation. | Resolver/query API design; `atmos.list` already exists as a CLI wrapper. |
| Dedicated Starlark error-event frames | Include safe filename/line/function and task context without duplicate reporting or secret/raw-output tags. | Error-to-observability mapping. |

### Future Considerations (P2): not implemented

| Capability | Acceptance criterion for a future increment | Dependency |
|------------|--------------------------------------------|------------|
| Starlark test-file discovery and command assertions | Discover tests predictably, isolate process/HTTP/step mocks, and require explicit integration-test opt-in. | Test-runner design; injected services already support Go-hosted tests. |
| File writes, temporary workspaces, and broader time helpers | Define permissions, deterministic ordering, parallel writes, and cleanup on failure/cancellation. | Filesystem and lifecycle API design. |
| External automation migrations | Demonstrate required APIs, side effects, retry boundaries, and cleanup in representative fixtures. | Separate migration scope; no Safire commands run here. |

## Success Metrics

No adoption or time-savings baseline is available. The following are verification
targets, not newly measured results. Evaluate them when runtime or host behavior
changes; historical validation evidence is linked at the end of this document.

| Outcome | Measurement | Target |
|---------|-------------|--------|
| Host compatibility | Focused interpreter, command, workflow, hook, and CLI fixture checks. | All supported-host regression cases pass. |
| Input fidelity | Typed flags, literal braces, commas, optional inputs, ambient env, and parallel/matrix propagation cases. | Exact expected values and types; no unintended rewriting. |
| Execution isolation | Barrier-based concurrency tests, fake-clock retry tests, cancellation cases, and race-enabled runtime checks. | Correct ordering/retry behavior, bounded concurrency, and no reported data races. |
| CLI behavior | Help, invalid input, global flags, version/profile re-execution, and output fixtures. | Correct usage exit code, no callbacks for declared help/parse failure, and preserved script arguments. |
| Diagnostics | Runtime/recursion failures, secret-masking cases, and data-versus-UI output snapshots. | Actionable diagnostics without traceback floods or secret leakage in displayed output. |

Future usability evaluation should measure task completion and debugging effort
against existing scripts before setting adoption or productivity targets.

## Open Questions

These questions concern unimplemented extensions; they do not block use of the
documented core. Owners are roles, not assigned individuals.

| Question | Owner | Timing |
|----------|-------|--------|
| What are the scope, return types, and side-effect rules for direct service reads/writes and `components.list`? | Runtime and service maintainers | Blocking before those APIs are implemented. |
| Should scripts gain scoped background-process ownership beyond existing workflow contexts? | Workflow/runtime maintainers | Design before expanding the implemented synchronous step library. |
| What discovery convention and mock boundaries should command-level Starlark tests use? | Test-runner maintainers | Blocking before test-file discovery. |
| How should Starlark frames map into observability events without unsafe details or retry duplicates? | Runtime and observability maintainers | Blocking before richer event mapping. |
| Which file/UI/lifecycle helpers and external migrations provide sufficient benefit to prioritize? | Product owner and automation authors | Non-blocking prioritization; no committed release. |

## Timeline Considerations

The current codebase implements the core described below, including the fixes
recorded on 2026-10-04. This PRD does not infer a released version from repository
state. Future P1/P2 work has no supplied deadline, staffing assignment, or release
commitment; each increment requires scoped contracts and validation before being
advertised as available. Further service access and command testing depend on existing Atmos subsystems
rather than independent replacements. Typed-step dispatch already reuses the shared runner.

## Implemented Behavior

### Interpreter, parallel functions, and step integration

- `interpreter: starlark` runs in process through the `pkg/script` engine registry.
  Registration owns name, extension, and optional Atmos-shebang/stdin fallback
  selection. Starlark registers `.star`; TypeScript is not implemented.
- `steps.parallel(functions=[fn, ...], max_concurrency=4, fail_fast=False)` invokes
  zero-argument functions concurrently and joins every branch before returning.
- `steps.task(name, function, args=[], kwargs={}, retry=None, timeout="")` describes
  a deferred function invocation; pass descriptors as `steps.parallel(tasks=[...])`.
- Exactly one of `functions` or `tasks` is required. Lists and tuples are accepted;
  an empty collection returns an empty list. Named tasks must have unique names.
- Results follow input order. Failure raises an aggregate error with branch names
  and Starlark backtraces. Default failure handling finishes independent branches.
  Fail-fast cancels running work and skips queued work after retries are exhausted.
- Each attempt receives a fresh thread. A task timeout covers all attempts and
  backoff. Retry configuration uses Atmos retry fields and validation; output-regex
  `conditions` are rejected because tasks are functions, not subprocesses. As with
  Atmos retry, omitted `max_attempts` in an explicit policy means unlimited; authors
  should supply an attempt limit or timeout. With no retry policy, run once.
- Functions, closure cells, defaults, module globals, and task inputs are frozen
  before scheduling. Locals created inside a branch remain mutable. Results are
  frozen before publication. Branches cannot mutate shared inputs. Nested parallel
  groups have independent concurrency limits.
- `exec.run(argv, working_directory=..., env={...}, output="stream", check=True)` uses the Atmos process runner,
  inherits the effective process environment, and returns captured stdout, stderr,
  and exit code. Nonzero exit raises an error eligible for task retry. It never
  changes process-wide environment or cwd. `ui.info/success/warning` use Atmos UI.
  `check=False` returns ordinary nonzero process exits for explicit assertions;
  launch failures, signals, cancellation and transport errors still raise.
  `output="capture"` captures stdout/stderr without showing either stream; the default
  `output="stream"` shows subprocess output live and captures it. Start failures
  (missing command, bad directory) always raise. `exec.run` and `component.exec`
  accept per-call timeouts and retries. One timeout bounds attempts and backoff.
  Only ordinary nonzero exits are retried; `check=False` returns them immediately.
  Retry output conditions inspect the failed attempt; captured results contain
  only the final attempt.
- Script `env` contains explicit step inputs, not ambient process variables.
  `print` is captured as stdout; an optional top-level `output` becomes the step
  value (a string as-is, any other value JSON-encoded; a non-encodable value fails
  with a clear error). An unset or `None` output supplies no explicit result; an
  embedded step then retains captured stdout as its value. Standalone scripts
  print no additional result for `None`. Nested `None` values still encode as JSON
  `null`. Structured step values reach later templates as JSON text and need
  appropriate quoting when passed through a shell.
- Local `load()` evaluates and caches modules per invocation, detects cycles, and
  supports functions from imported files. Inline scripts anchor loads at
  `working_directory`; a script read from a file, and every module it loads, resolves
  loads relative to its own file.
- Existing `!include` can supply raw `.star` source or a YAML definition. In workflow
  manifests and custom command definitions, `./x` and `../x` resolve against the file
  that contains the tag, bare `x/y` against the project base path, and absolute paths
  as written, independent of the process working directory. A script read from a local
  file through `!include` or `!include.raw` (without a YQ expression) records that file
  as its source (`ScriptSource`): `load()` anchors there and tracebacks name the file and
  line. Stack-manifest hook scripts record it in-band: when a mapping with an `interpreter`
  key holds a `script` that is a local `!include`/`!include.raw` (no YQ expression, not
  remote), stack processing adds an Atmos-owned `script_source` sibling key, project-relative
  when the file is inside the project base path and absolute otherwise. It survives stack
  merging, is visible in `describe component`/`describe stacks` like other provenance
  (`component_info`, `sources`), and the hook runner resolves it against the base path
  into `WorkflowStep.ScriptSource` for every step (including children of group steps).
  An include-derived value replaces a hand-written one. Stack inheritance deep-merges maps,
  so provenance is self-validating: a sibling `script_source_sha256` (hex SHA-256 of the
  included content) is recorded with it, and the hook runner honors `script_source` only
  when the step's raw `script` (before hook template rendering) still hashes to that value.
  A child stack that replaces an included script with an inline one therefore keeps the
  inherited keys in describe output, but they are ignored at run time (debug-logged).
  Custom commands are unaffected: command definitions merge by name and a `steps` list is
  replaced wholesale, so a step never mixes two definitions' `script` and `script_source`.
  Workflows record the source per step from their own file.
- Workflow, custom-command, and test-group script handlers route to the engine.
  YAML parallel/matrix script children also use it. A script step under an enabled
  workflow or step container fails validation before the workflow starts; authors set
  `container: false` on that step. Dry run executes no code or processes; top-level
  script steps are still parsed so syntax errors surface.
- Go host options inject a process runner, module reader, retry clock, and task
  observer. Observer events identify branch starts, attempts, and final outcomes.
- Custom-command, workflow, and hook script steps expose `components.get(name, stack, type)`,
  and `parallel`/`matrix` children inherit it from their parent context. Results are cached
  per run. A custom command with `component.type` supplies `ctx.component`, resolved lazily on
  first access; a hook's `ctx.component` is its own component; in a workflow `ctx.component`
  is `None`, so scripts pass an explicit stack to `components.get`. Handles expose
  read-only resolved config, vars, settings, metadata, env, logical name, implementation,
  and provider-aware path. `component.exec(argv)` runs in that directory with component
  env, command overrides, step overrides, then per-call overrides. Stack resolution is
  serialized; child processes remain concurrent. Resolution uses the execution pipeline
  so inspection-only secret masking does not supply placeholder credentials.
- Successful component resolutions are cached; failures may be retried and waiting
  callers may cancel. Lazy `ctx.component` attribute access uses the invocation's
  context. To bound resolution by a parallel task's deadline, call `components.get`
  inside that task with explicit name, stack, and type.

### Shared step library and filesystem inspection

- `steps.run(type, **fields)` and named functions such as `steps.input`, `steps.http`,
  and `steps.container` dispatch registered handlers through `automation.StepLibrary`.
  Calls run immediately; `steps.task` defers function calls for `steps.parallel`.
- Results expose `value`, `values`, `metadata`, `outputs`, `skipped`, and `error`.
  Execution failures raise. Named results are available to subsequent templates.
  `result.data` lazily decodes JSON from `value`; metadata and named outputs retain
  their existing types. Invalid/empty JSON fails only on access.
- Each invocation owns step state. Parallel branches and retries get isolated
  snapshots. Step environment changes affect subsequent step calls, not the script's
  immutable `env` or `exec.run` environment. Pass explicit overrides to those APIs.
- Prompt steps already provide text, confirmation, selection, and other inputs.
  They follow existing non-TTY defaults/errors. Terminal-owning operations cannot
  run in parallel script tasks. YAML-style parallel groups use `steps.run("parallel", ...)`.
- Workflow scheduling, identity, background jobs, and freshness remain host-owned.
  Unsupported direct-call policies fail; wait/cancel handlers require job context.
  The `exec` step replaces the process on Unix; `exec.run` returns to the script.
- `fs.glob`, `fs.stat`, `fs.exists`, and `fs.readlink` use the shared Go filesystem
  interface. Matches are sorted; stat reports bytes; existence follows symlinks;
  missing paths are distinguished from other errors. Recursive globbing and writes
  are not provided. See [the filesystem contract](git-hook-steps.md#filesystem-api).
- Local Git hooks now accept ordered `steps` with inline Starlark, independently
  of component lifecycle hooks. Their scripts receive positional `ctx.args` and
  run inside the hook process. See [Git-hook steps](git-hook-steps.md).

### Parsed inputs and literal fields

- Embedded scripts receive immutable `ctx.flags` and `ctx.arguments` dictionaries.
  They preserve host-supplied types and raw strings rather than rendering those
  strings as templates. Custom commands preserve native flag types; workflow flag
  values remain strings. Hosts without named inputs provide empty dictionaries.
- Sequential, parallel, matrix, and test children retain the host inputs. Dictionary
  conversion sorts keys and freezes nested values for deterministic iteration and
  isolation from host or script mutation.
- YAML custom-command flags support string, bool, and int types; invalid types and
  invalid integer defaults fail registration. Custom-command arguments containing
  commas are preserved. Omitted optional YAML arguments without defaults become
  empty strings; omitted standalone `cli.arg` values become `None`.
- Step fields remain templated unless marked `!literal`. The tag protects `script`,
  `command`, `interpreter`, `working_directory`, and individual declared `env`
  values through custom commands, workflows, hooks, parallel, and matrix execution.
  Loader-owned `literal_fields` metadata carries this intent to the runner.
- Only declared environment values are rendered; ambient process environment values
  pass through verbatim. Protecting `script` does not implicitly protect adjacent
  fields. Template failures identify the step, field, and included source when
  available, and suggest `!literal` for source containing template braces.
- A script step's `timeout:` applies to the interpreter and subprocess context;
  expiration is reported through the step timeout contract. Unknown step `output:`
  modes fail validation. The step output modes and `exec.run(output="capture")`
  are different interfaces: `output: capture` is not a valid step mode.

### Dialect, output, and diagnostics

- Numeric `sum` and `round` helpers are implemented.
- Top-level `if`/`for`/`while`, `set()`, and recursion are enabled for entry scripts and
  loaded modules. Globals stay single-assignment: rebinding a global (including `n += 1`
  at the top level or assigning `output` in both branches of an if/else) fails with
  `cannot reassign global` and a hint toward `output = a if cond else b` or
  `def main(): ... return v` with `output = main()`.
- Script output streams line by line while the script runs. In a `steps.parallel` group
  with more than one task, each line carries a `[<task name>] ` prefix and lines never
  interleave mid-line; a lone task inherits its enclosing prefix.
- Script steps use command-step output defaults: raw output and no step labels
  unless step/workflow output settings or `show.labels` opt in. Task prefixes are
  independent of these outer step labels.
- `working_directory` must exist, checked before any code runs. `load()` and `fs.read_file`
  accept relative and absolute paths. `fs.read_file` and `exec.run` resolve against the
  step's `working_directory`; `load()` resolves against the script's own file when it has
  one and against `working_directory` for an inline script.
- Runtime errors are single-line messages with the Starlark traceback as an explanation. Task
  timeouts read `task "<name>" timed out after <duration>`; failed processes include the
  last stderr lines. Standalone CLI usage errors bypass Starlark traceback wrapping.
- Every interpreter thread samples call depth every 4096 execution steps and
  cancels when depth exceeds 10,000 frames. This sampled circuit breaker can stop
  slightly beyond the threshold; it is not a total execution-step budget. Recursive
  failures use `ErrStarlarkRecursionLimit`, an actionable hint, and collapsed repeated
  traceback frames. Long-running nonrecursive work still needs cancellation/timeouts.
- Hook script steps (`kind: step`, `kind: steps`, `type: test`) can call `components.get`
  for components other than their own.

Structured errors use `errors.build(message)` with snake_case builder methods:
`with_title`, `with_explanation`, `with_hint`, `with_example`, `with_context`,
`with_cause`, and `with_exit_code`. Builders are immutable; `.fail()` raises the
error. Unhandled errors use normal Atmos reporting, including configured Sentry.
Context marked safe by the builder must not contain secrets. Masking depends on
registered secrets, patterns, and configuration, not on the choice of language.

### Logging

- `log.trace/debug/info/warn/error(message, **fields)` write to the Atmos logger
  (`pkg/logger`), so they respect `--logs-level`, `ATMOS_LOGS_LEVEL`, and `logs.file`. Fields
  are structured key/value pairs; `step` and, inside `steps.parallel`, `task` are added
  automatically. Messages and field values are masked before logging. Output never reaches
  the script's stdout or step value. The logger is injectable through `WithLogger`.
- Convention: `print` is data (stdout), `ui.*` is human status (stderr), `log.*` is
  diagnostics shown only at the configured level.

### Lifecycle context and Atmos commands

- Hook script steps receive immutable `ctx.component`, `ctx.hook`, and
  `ctx.operation` snapshots. Component vars, settings, metadata, env, and the hook's
  effective component directory are available without resolving the stack again.
- Hook identity includes name and canonical event. Operation data includes canonical
  command, status, exit code, and error. Before-hook results and uncaptured parent
  stdout/stderr are `None`. Hook subprocess results remain separate.
- Context is available to loaded functions, Starlark parallel branches, hook step
  lists, and nested YAML control children. Aggregate/unbound hooks have no component.
- Built-in command groups are exposed as callable `atmos` members, including
  `atmos.scaffold`, `atmos.vendor`, `atmos.list`, `atmos.describe`, and `atmos.config`.
  Generic wrappers accept positional CLI strings, `flags={}`, `args=[]`, and the
  standard cwd/env/output/check options; commands with no arguments work as well.
  These calls return process results. Named list/describe wrappers and config-get
  wrappers default to capture and request JSON when the selected command exposes
  a format flag. This includes `atmos.config("get", ...)` and
  `atmos.stack("config", "get", ...)` (plus the `stack get` alias). Explicit format
  and output choices win. The generic `atmos.run` keeps literal CLI behavior.
- Process results preserve `.stdout`, `.stderr`, and `.exit_code`. Their lazy,
  cached `.data` view decodes JSON from stdout without altering it; collections
  are frozen. Empty or invalid JSON raises only when `.data` is accessed. This
  lets scripts iterate affected components for monorepo build/test/deploy selection.
  It does not add remote caching or a Bazel-compatible build graph.
- The CLI supplies an immutable snapshot of its registered commands, including
  custom commands and aliases, to the runtime. Named wrappers and flag spelling
  follow that catalog rather than a separate hard-coded CLI surface. Embedding
  hosts without a catalog retain explicit `run`, `terraform`, `helm`, and
  `toolchain` helpers. Wrappers execute child Atmos processes, not direct Go command
  handlers or native object queries.
- `atmos.run(argv)` calls any CLI/custom command using the current binary.
  `atmos.terraform(command, component, stack, flags={}, args=[])` and `atmos.helm(...)`
  add structured arguments. All use the injectable process runner, effective env,
  shared I/O, cancellation, capture/check options, and process result type.
- Atmos calls default to invocation cwd, while component.exec defaults to the
  component directory. Parent CLI-only flags are not replayed automatically.
- Typed Terraform plans accept detailed-exitcode 2 when requested; ordinary errors
  still raise. Task policies own retries/timeouts/parallelism. Calls execute normal
  hook lifecycles, so recursive hook dispatch must be avoided by configuration.

### Standalone executable scripts

The CLI recognizes a `.star` filename or an explicit file path with an Atmos
shebang after any leading Atmos global flags. `#!/usr/bin/env atmos` and an
absolute Atmos interpreter path work through the operating system's normal script
dispatch. Invoking Atmos without arguments preserves the existing root UI/help
behavior even when stdin is redirected; ordinary CLI commands retain their existing path.
Bare extensionless names remain commands. A new `atmos script run` command is not
part of this interface. The `.star` extension is optional for an explicit script
path with a recognized shebang.

Leading global flags such as `atmos --chdir=dir ./tool.star arg` apply normally.
`--chdir` (or `ATMOS_CHDIR`) is applied before resolving a relative script path.
Arguments after the filename belong to the script and are isolated before CLI/config
argument processing and exposed as immutable `ctx.args`. `ctx.script.path` and
`.directory` identify the physical script
file, resolving symlinks so local module loads find adjacent libraries. The caller's
working directory and normal configuration discovery are preserved unless changed
through Atmos's global controls. Version/profile re-execution retains the original
script and its arguments, without stripping script-owned flags with names such as
`--chdir` or `--use-version`.

Standalone metrics summaries are disabled when `settings.metrics.enabled` is unset;
an explicit setting is honored. Tracebacks display paths relative to the working
directory where possible, while load resolution retains physical paths. Log fields
and hints use the script filename.

#### Stdin execution

Use `atmos - [args...] < script.star`, or `atmos - -- [args...]` to separate
script arguments explicitly. Select a registered engine with
`atmos --interpreter=starlark -`. Bare `atmos < script.star` and
`atmos -- foo bar < script.star` do not select script execution.

The CLI reads source through EOF only after detecting the explicit stdin marker.
Stdin scripts have `ctx.script = None`, report `<stdin>` source locations, and
resolve imports against the invocation working directory. Stdin consumed as
source is not simultaneously available as a separate script input stream.

#### Declared command interfaces

`cli.command` declares a standalone interface using native positional and flag
definitions. It is available once per invocation on the standalone main thread,
not in embedded steps or parallel children.

```python
#!/usr/bin/env atmos

def validate(args, flags):
    if flags["replicas"] < 1:
        fail("replicas must be positive")

def main(args, flags):
    return {"service": args["service"], "replicas": flags["replicas"]}

output = cli.command(
    description = "Report a service's replica count",
    args = [cli.arg("service", description = "Service name")],
    flags = [cli.flag("replicas", type = "int", shorthand = "r", default = 2)],
    validate = validate,
    run = main,
)
```

- `cli.arg` declares required or optional positional strings; required arguments
  precede optional ones. `cli.flag` supports `string`, `int`, `bool`, and
  `string_list`, with supported defaults, shorthand, choices, required values,
  descriptions, and optional environment binding. Required flags cannot have
  defaults; boolean flags use defaults rather than required declarations.
- Parsed callback arguments are immutable dictionaries. Explicit CLI flag values
  take precedence over environment bindings and defaults. Integer values parse in
  base 10. String-list CLI and environment inputs use comma-separated CSV semantics;
  choices are checked per list element. Names must be unique ignoring case.
- Help documents required flags, choices, and environment bindings, and skips both
  `validate` and `run`. Parsing failures also skip these callbacks. Top-level script
  statements still execute, so side effects belong inside `run`; help is not a
  parse-only mode for the entire file.
- Validation runs before `run` and accepts `None` or `True`, rejects `False`, and
  propagates callback errors. These are script validation failures, distinct from
  native parser usage errors.
- Invalid CLI input returns `ErrScriptUsage`, usage and a script-specific help hint,
  exit code 2, and no Starlark traceback. A boolean flag followed by a `true`/`false`
  word is rejected with guidance to use `--flag=false` or a literal argument after
  `--`. Unknown flags, missing/extra arguments, invalid values, and failed choices
  use the same usage-error path.
- `cli.command` returns the `run` callback's value; assigning it to top-level
  `output` emits that value under the normal output contract. Help or a callback
  returning `None` emits no additional result. `ctx.args` retains raw script argv;
  parsed standalone inputs are passed to callbacks, not substituted into the
  embedded-host `ctx.flags`/`ctx.arguments` snapshots.

#### Tool dependencies and distribution

Scripts declare tools directly through the dependency module:

```python
#!/usr/bin/env atmos
dependencies.tools("jqlang/jq", "1.7.1")
exec.run(["jq", "--version"])
```

Each declaration uses the existing dependency installer to provision a missing tool
and add its directory to PATH for subsequent subprocess calls in that invocation.
Repeated identical pins are cached; conflicting versions fail. Declare dependencies
before starting parallel tasks; declarations inside branches are rejected. Subprocess executable lookup
uses the supplied environment, without mutating the process-wide PATH, including
inside parallel branches.

Explicit toolchain operations use `atmos.toolchain("install", "jqlang/jq@1.7.1")`,
consistent with the Terraform and Helm wrappers. This is a normal child command
with process results, output/error policies, and optional flags/args; it does not
change the calling script's PATH. Use `dependencies.tools` to provision and activate
a tool for the current script.

Distribution uses the existing toolchain registry's `type: http`, `format: raw`
support for a single executable script. Atmos must already be available on PATH for
an `env atmos` shebang. A recorded local HTTP experiment covered installing an extensionless
deployment script, executing it directly, auto-installing its missing helper tool,
parallel helper calls, and a second run using the cached helper. No infrastructure
or external registry publication is needed. Archive/module packaging remains outside
this proof of concept.

## Migration Inventory

The initial cast-authoring review recorded 102 Python script-step declarations in 80 files
across `demo/casts`, `examples`, and `.github` before converting the Starlark cast's
validator. Most cast validators import the shared `demo/casts/cast_checks.py`.
These are historical counts, not a current inventory or adoption metric. This
inventory is a migration guide, not a requirement to replace Python tools or
web-server fixtures with Starlark.

| Existing Python usage | Starlark surface | Status |
| --- | --- | --- |
| `Path.read_text()` in 26 files | `fs.read_file(path)` | Implemented; working-directory-relative, injectable and usable in parallel functions. |
| Regex operations in 17 files, including ANSI cleanup and progress assertions | `regex.search`, `regex.replace`, `regex.findall` | Implemented with Go/RE2 syntax, full-match results and literal replacement. Python lookbehind patterns must be rewritten. |
| JSON cast events, payloads and manifest checks | `json.encode/decode` | Already implemented. |
| `Path.write_text()` in 21 files, notably cast sanitizers | Atomic `fs.write_file(path, content)` | Proposed; define permissions, replacement and parallel-writer behavior before exposing. |
| File inspection in validators and repository checks | `fs.exists`, sorted `fs.glob`, `fs.stat`, `fs.readlink` | Implemented; byte sizes and explicit symlink behavior. No recursive globbing or write API. |
| `subprocess.run` in screengrab generation | `exec.run(argv, env=..., working_directory=..., check=False, output="capture")` | Implemented; explicit nonzero-result policy and capture without streaming. |
| Python server start/stop and readiness polling in HTTP/weather fixtures | Shared background, HTTP and retry step interfaces | HTTP and retry step APIs are exposed; background ownership still belongs to the workflow. No detached-process API is added. |
| Temporary directories and atomic replacement in screengrab generation | Scoped temporary workspace and atomic file APIs | Proposed; deterministic cleanup on success, failure and cancellation. |
| UTC timestamps and hook artifacts in `examples/hooks-custom-command/scripts/notify.py` | Injectable clock, explicit hook inputs, file writes and Markdown UI | Proposed; avoid ambient environment reads and make time deterministic in tests. |

The shared `demo/casts/cast_checks.star` module and `scripts/validate-starlark.star`
use file reads, JSON, regex and `fail()` to validate the real Starlark recording
without a Python subprocess. Keep cast-specific assertions in loaded Starlark
modules rather than adding cast-specific builtins to the interpreter.

## Acceptance and Validation

The following are regression criteria for the implemented core, not a record of
tests rerun during this PRD refresh:

- [ ] **AC-01:** Embedded scripts execute through supported hosts; entry-source dry
  runs execute no code/processes, and enabled containers fail with actionable guidance.
- [ ] **AC-02:** Typed/raw inputs survive sequential, parallel, matrix, and test
  boundaries; commas, optional arguments, literal fields, and ambient env remain
  faithful to the documented host-specific contracts.
- [ ] **AC-03:** Strings remain raw results; structured values become JSON text;
  top-level `None` produces no explicit result. Output defaults, labels, and invalid
  step-mode validation agree with command-step behavior.
- [ ] **AC-04:** Controlled concurrency/retry/deadline tests preserve order, retry
  only failed work, isolate environment/cwd, and allow cancellation without leaked
  shared mutations. Include recursion inside parallel tasks.
- [ ] **AC-05:** Component/hook context, resolver caching and cancellation, catalog
  wrappers, flag spelling, auth-aware resolution, and displayed secret masking retain
  their existing execution contracts.
- [ ] **AC-06:** Standalone help and parser failures skip callbacks; typed flags,
  environment precedence, leading globals, `--chdir`, and version/profile re-execution
  preserve script selection and arguments.
- [ ] **AC-07:** Parser usage errors exit 2 without script tracebacks, runtime errors
  retain useful source context, recursive traces are bounded in presentation, and
  stdout/UI/log output and standalone metrics follow their documented policies.

### Runtime verification

Prove bounded overlapping execution and input-order results using barriers, not
timing guesses. Verify failed-only retries with mocked processes and a fake clock;
wait-all, fail-fast, pending cancellation, deadlines, loaded functions, module cycles,
frozen globals/closures/arguments, mutable branch locals, nested groups, process
directory/environment isolation, captured output, invalid policies, and dry runs.
Run race detection on the engine and integration tests through the step handlers,
YAML control runner, and `type: test`. Ship the `atmos-starlark` skill in the existing
offline catalog, with executable examples of implemented APIs only.

The Helm lifecycle scenario now loads shared Starlark assertions for 23 checks:
resource absence, release records, CRDs, Job completion, hook behavior, expected
failures, rollback state, dependency readiness, deployed secrets and release
progress. Kubernetes JSON replaces JSONPath/shell comparisons. Checked
`--ignore-not-found` queries distinguish missing resources from API failures.
Mocked-process tests cover expected and unexpected failures without deployments.
Binary image streaming and temporary-file template/summary checks remain shell
until the corresponding stream and scoped-artifact APIs exist.

## First-class service access and observability requirements

Existing YAML functions and resolved component handles provide indirect reads.
The registered `steps.store` handler is available, but there are no dedicated
`secrets`, `stores`, or Terraform state/output Starlark modules
in this increment. The next service APIs must delegate to Atmos's existing
authentication, stack resolution, secret masking, caching and provider services:

- Secret reads with explicit component/stack scope and existing secret declarations.
- Named-store reads and writes preserving the store's key and value semantics;
  independent invocations must not change global auth or environment state.
- Terraform output/state reads preserving component/stack selection, workspace,
  backend and identity behavior. Do not reimplement backend access in scripts.
- Injectable service boundaries for tests, including failures and parallel calls;
  dry runs must skip reads/writes with side effects as defined by the runner.

Unhandled script errors already reach `main.go`'s normal `CaptureError` call and
the configured Sentry client. Expected failures handled through `check=False`,
forgiven failures, and retries that eventually succeed produce no final parent
error. Invoked Atmos subprocesses retain their own reporting behavior.
Dedicated Starlark filename/line/function frames and task-name/attempt context
still need explicit mapping into events, preserving the existing safe-detail
redaction policy and avoiding duplicate capture per failed attempt. Secret values,
raw component config and captured subprocess output must not become event tags.

## Broader product requirements, not implemented by this increment

The following retain the earlier design intent; they must not be advertised as
available Starlark APIs yet:

- Add native `components.list`; typed invocation inputs are already implemented.
  Custom providers' no-op `Execute`
  methods must never stand in for actual custom-command execution.
- Expand UI aliases, polling, cleanup, and state helpers through existing services.
- Add `type: test` discovery of Starlark test files and command-level assertions,
  isolated process/HTTP/step mocks, fake time, and explicit integration-test opt-in.
- Add Safire command migrations after the required APIs exist; document external
  side effects, retry boundaries, and cleanup using representative command fixtures.

No external Safire commands are executed by this implementation or its tests.

## Product boundaries

The embedded language supports application builds, tests, releases, deployments,
and monorepo orchestration as well as infrastructure operations. Teams can reuse
one script locally and in CI. Atmos ships the interpreter, step library, CLI input
conventions, and diagnostics in its binary; called tools, container engines,
credentials, registries, and remote services remain external requirements.

Starlark avoids requiring a Python or Node package environment for the embedded
language itself. Host functions intentionally add process execution and filesystem
I/O, so arbitrary Atmos scripts are neither deterministic evaluations nor security
sandboxes. The language's constrained syntax and shared APIs reduce orchestration
choices; they do not guarantee reliable deployments or eliminate external failures.

## References and Change History

Implementation sources: [engine](../../pkg/script/starlark/engine.go),
[script-step adapter](../../pkg/runner/step/script.go),
[standalone host](../../cmd/standalone_script.go),
[CLI declaration binding](../../pkg/script/starlark/stdlib/cli/module.go), and
[runtime architecture](../../pkg/script/starlark/README.md).

User documentation: [script step](../../website/docs/steps/type/script.mdx) and
[standalone CLI apps](../../website/docs/automation/standalone-cli-apps.mdx).

The following fix records contain historical validation evidence; they do not
replace checks against the current codebase:

- [Declared inputs](../fixes/2026-10-04-starlark-declared-command-inputs.md).
- [Output defaults and recursion protection](../fixes/2026-10-04-starlark-output-defaults-and-recursion-limit.md).

| Date | PRD change |
|------|------------|
| 2026-10-06 | Reconciled the shared SDK, extension registry, stdin execution, step library, structured errors, query defaults and decoded results, process controls, filesystem inspection, and local Git-hook integration. |
| 2026-10-05 | Reorganized into explicit PRD sections and reconciled implemented inputs, literal fields, output, recursion, command catalog, standalone declarations, global flags, usage errors, and re-execution. Kept unimplemented service and testing APIs separate. |

## Computed YAML values

See [Starlark YAML values](starlark-yaml-values.md) for `!starlark` function
bodies that return typed configuration from a read-only merged component
context. This host uses `return`; executable scripts and steps retain `output`.
