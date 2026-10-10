# Starlark automation and command testing

## Purpose

Custom commands and workflows need reusable, testable orchestration without large Bash
programs. Embed Starlark in `type: script`, and use Atmos execution services for
concurrency, retries, processes, output, and cancellation. Command declarations,
arguments, flags, and custom-component bindings remain in YAML.

## Implemented increment: parallel functions

- `interpreter: starlark` runs in process through the `pkg/script` engine registry.
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
  (missing command, bad directory) always raise.
- Script `env` contains explicit step inputs, not ambient process variables.
  `print` is captured as stdout; an optional top-level `output` becomes the step
  value (a string as-is, any other value JSON-encoded; a non-encodable value fails
  with a clear error). Without `output`, the value remains stdout.
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

## Implemented increment: dialect, output, and diagnostics

- Top-level `if`/`for`/`while`, `set()`, and recursion are enabled for entry scripts and
  loaded modules. Globals stay single-assignment: rebinding a global (including `n += 1`
  at the top level or assigning `output` in both branches of an if/else) fails with
  `cannot reassign global` and a hint toward `output = a if cond else b` or
  `def main(): ... return v` with `output = main()`.
- Script output streams line by line while the script runs. In a `steps.parallel` group
  with more than one task, each line carries a `[<task name>] ` prefix and lines never
  interleave mid-line; a lone task inherits its enclosing prefix.
- `working_directory` must exist, checked before any code runs. `load()` and `fs.read_file`
  accept relative and absolute paths. `fs.read_file` and `exec.run` resolve against the
  step's `working_directory`; `load()` resolves against the script's own file when it has
  one and against `working_directory` for an inline script.
- Errors are single-line messages with the Starlark traceback as an explanation. Task
  timeouts read `task "<name>" timed out after <duration>`; failed processes include the
  last stderr lines.
- Hook script steps (`kind: step`, `kind: steps`, `type: test`) can call `components.get`
  for components other than their own.

## Implemented increment: logging

- `log.trace/debug/info/warn/error(message, **fields)` write to the Atmos logger
  (`pkg/logger`), so they respect `--logs-level`, `ATMOS_LOGS_LEVEL`, and `logs.file`. Fields
  are structured key/value pairs; `step` and, inside `steps.parallel`, `task` are added
  automatically. Messages and field values are masked before logging. Output never reaches
  the script's stdout or step value. The logger is injectable through `WithLogger`.
- Convention: `print` is data (stdout), `ui.*` is human status (stderr), `log.*` is
  diagnostics shown only at the configured level.

## Implemented increment: lifecycle context and Atmos commands

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
  These calls retain native CLI behavior and return process results.
- `atmos.run(argv)` calls any CLI/custom command using the current binary.
  `atmos.terraform(command, component, stack, flags={}, args=[])` and `atmos.helm(...)`
  add structured arguments. All use the injectable process runner, effective env,
  shared I/O, cancellation, capture/check options, and process result type.
- Atmos calls default to invocation cwd, while component.exec defaults to the
  component directory. Parent CLI-only flags are not replayed automatically.
- Typed Terraform plans accept detailed-exitcode 2 when requested; ordinary errors
  still raise. Task policies own retries/timeouts/parallelism. Calls execute normal
  hook lifecycles, so recursive hook dispatch must be avoided by configuration.

## Acceptance and validation

### Standalone shebang spike

The workspace spike recognizes a leading `.star` filename or an explicit file path
with an Atmos shebang. `#!/usr/bin/env atmos` and an absolute Atmos interpreter path
work through the operating system's normal script dispatch. No arguments preserve
the existing root UI/help behavior; ordinary CLI commands retain their existing path.
Bare extensionless names remain commands. A new `atmos script run` command is not
part of this spike, nor are global flags preceding a script filename.

Script arguments are isolated before CLI/config argument processing and exposed as
immutable `ctx.args`. `ctx.script.path` and `.directory` identify the physical script
file, resolving symlinks so local module loads find adjacent libraries. The caller's
working directory and Atmos configuration discovery remain unchanged. Script errors
return through the normal error-reporting path.

Scripts declare tools directly through the dependency module:

```python
#!/usr/bin/env atmos
dependencies.tools("jq", "1.7.1")
exec.run(["jq", "--version"])
```

Each declaration uses the existing dependency installer to provision a missing tool
and add its directory to PATH for subsequent subprocess calls in that invocation.
Repeated identical pins are cached; conflicting versions fail. Declare dependencies
before starting parallel tasks; declarations inside branches are rejected. Subprocess executable lookup
uses the supplied environment, without mutating the process-wide PATH, including
inside parallel branches.

Explicit toolchain operations use `atmos.toolchain("install", "jq@1.7.1")`,
consistent with the Terraform and Helm wrappers. This is a normal child command
with process results, output/error policies, and optional flags/args; it does not
change the calling script's PATH. Use `dependencies.tools` to provision and activate
a tool for the current script.

Distribution uses the existing toolchain registry's `type: http`, `format: raw`
support for a single executable script. Atmos must already be available on PATH for
an `env atmos` shebang. A local HTTP spike verifies installing an extensionless
deployment script, executing it directly, auto-installing its missing helper tool,
parallel helper calls, and a second run using the cached helper. No infrastructure
or external registry publication is needed. Archive/module packaging remains outside
this proof of concept.

### Python automation inventory

The cast-authoring review found 102 Python script-step declarations in 80 files
across `demo/casts`, `examples`, and `.github` before converting the Starlark cast's
validator. Most cast validators import the shared `demo/casts/cast_checks.py`.
This inventory is a migration guide, not a requirement to replace Python tools
or web-server fixtures with Starlark.

| Existing Python usage | Starlark surface | Status |
| --- | --- | --- |
| `Path.read_text()` in 26 files | `fs.read_file(path)` | Implemented; working-directory-relative, injectable and usable in parallel functions. |
| Regex operations in 17 files, including ANSI cleanup and progress assertions | `regex.search`, `regex.replace`, `regex.findall` | Implemented with Go/RE2 syntax, full-match results and literal replacement. Python lookbehind patterns must be rewritten. |
| JSON cast events, payloads and manifest checks | `json.encode/decode` | Already implemented. |
| `Path.write_text()` in 21 files, notably cast sanitizers | Atomic `fs.write_file(path, content)` | Proposed; define permissions, replacement and parallel-writer behavior before exposing. |
| File existence and globbing in `screengrabs/cli.yaml` and `cast_checks.py` | `fs.exists`, sorted `fs.glob`, and path helpers | Proposed; distinguish missing paths from permission errors and define recursive/symlink behavior. |
| `subprocess.run` in screengrab generation | `exec.run(argv, env=..., working_directory=..., check=False, output="capture")` | Implemented; explicit nonzero-result policy and capture without streaming. |
| Python server start/stop and readiness polling in HTTP/weather fixtures | Shared background, HTTP and retry step interfaces | Reuse Atmos lifecycle services when typed-step dispatch is exposed; do not add detached processes or global environment mutation. |
| Temporary directories and atomic replacement in screengrab generation | Scoped temporary workspace and atomic file APIs | Proposed; deterministic cleanup on success, failure and cancellation. |
| UTC timestamps and hook artifacts in `examples/hooks-custom-command/scripts/notify.py` | Injectable clock, explicit hook inputs, file writes and Markdown UI | Proposed; avoid ambient environment reads and make time deterministic in tests. |

The new `demo/casts/cast_checks.star` module and `scripts/validate-starlark.star`
use file reads, JSON, regex and `fail()` to validate the real Starlark recording
without a Python subprocess. Keep cast-specific assertions in loaded Starlark
modules rather than adding cast-specific builtins to the interpreter.

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
There are no direct `secrets`, `stores`, or Terraform state/output Starlark modules
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

- Expose registered Atmos steps through one shared policy-aware runner, including
  context restrictions for interactive, background, and terminal-control steps.
- Add typed invocation inputs and `components.list`. Custom providers' no-op `Execute`
  methods must never stand in for actual custom-command execution.
- Expand UI aliases, polling, cleanup, and state helpers through existing services.
- Add `type: test` discovery of Starlark test files and command-level assertions,
  isolated process/HTTP/step mocks, fake time, and explicit integration-test opt-in.
- Add Safire command migrations after the required APIs exist; document external
  side effects, retry boundaries, and cleanup using representative command fixtures.

No external Safire commands are executed by this implementation or its tests.
