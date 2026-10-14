---
name: atmos-starlark
description: "Author or debug Atmos Automation Language, the Python-like automation built on embedded Starlark, including parallel function calls, deferred tasks, retries, module loading, and migration of Bash orchestration. Use atmos-steps for the YAML step catalog and atmos-custom-commands for CLI declarations."
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
  category: ci-automation
---

# Atmos Automation Language

Atmos Automation Language uses the embedded Starlark runtime. The script step
configuration remains `interpreter: starlark`.

Keep command declarations, arguments, flags, and component bindings in YAML. Put
reusable orchestration in `type: script`, `interpreter: starlark`. Check the target
Atmos version supports embedded Starlark before converting an existing command.

## Available surface

- `env`: immutable explicit step inputs. Ambient process environment is not exposed.
- `print`: captured stdout, for data the step produces. Use `ui.info/success/warning`
  for status and progress messages (stdout is data, stderr is for people). `output`: optional top-level step value; a string is used
  as-is (`output = "abc"` gives `abc`), any other value is JSON-encoded, and a value
  that cannot be encoded (such as a function) fails the step. Without `output`,
  captured stdout is the step value.
- Dialect: top-level `if`/`for`/`while`, `set()`, and recursion are allowed. Globals are
  assigned once: `x = 1` then `x = 2`, `n += 1` at the top level, or assigning `output`
  in both branches of an if/else fails with `cannot reassign global`. Use
  `output = a if cond else b`, or `def main(): ... return v` then `output = main()`.
  Mutate a dict (`state["n"] += 1`) before `steps.parallel`, which freezes shared state.
- `json.encode/decode`: structured data without shell quoting or JSON command pipes.
- `fs.read_file(path)`: read a local file as a string, relative to the step working
  directory (also inside loaded functions); absolute paths are accepted. Reads use
  the same injectable file reader as module loading. No write API is available yet.
  `working_directory` must exist; Atmos checks it before the script runs.
- `regex.search(pattern, text)`: boolean match anywhere; `regex.findall(pattern, text)`:
  list of complete matches; `regex.replace(pattern, replacement, text)`: replace all
  matches with literal text. Uses Go/RE2 syntax, including inline flags; no lookaround,
  pattern backreferences, or replacement capture expansion. Useful for cast assertions
  and ANSI stripping. Use ordinary string methods for fixed text.
- `exec.run(argv, working_directory=..., env={...}, output="stream", check=True)`:
  argv-based child process; returns `.stdout`, `.stderr`, `.exit_code`, and raises
  on failure by default, with the last stderr lines in the error. Use `check=False`
  to assert an expected nonzero exit; start failures (command not found, bad
  directory), cancellation, signals and transport errors always raise. The default
  `output="stream"` shows subprocess output live and captures it; `output="capture"`
  captures both streams without showing them. It inherits the effective execution
  environment without changing global cwd or environment.
- `ui.info/success/warning(message)`: native Atmos UI output.
- `log.trace/debug/info/warn/error(message, **fields)`: diagnostics through the Atmos
  logger (honors `--logs-level`/`ATMOS_LOGS_LEVEL` and `logs.file`; secrets masked). Fields
  are structured key/value pairs (`log.debug("resolved", component="api", attempts=2)`);
  `step` and, inside `steps.parallel`, `task` are added automatically. Convention: `print` =
  data (stdout, step value), `ui.*` = human status (stderr), `log.*` = diagnostics shown only
  at the configured log level, never on stdout and never the step value.
- Built-in command wrappers such as `atmos.vendor("pull", flags={"stack": "dev"})`,
  `atmos.scaffold("generate", "template", "target")`, `atmos.list("components")`,
  `atmos.describe("component", "api", flags={"stack": "dev"})`, and
  `atmos.config("get", args=["base_path"])` accept literal positional CLI arguments,
  keyword-only `flags`/`args`, and the process options below. No-argument commands
  work too, for example `atmos.version()`. Other built-in names in CLI help use the
  same API; Terraform, Helm, and toolchain retain their dedicated signatures.
  Wrappers preserve native CLI behavior and return process results, not parsed data.
- `atmos.run(argv)`: invoke the current Atmos binary, including custom commands.
  `atmos.terraform(command, component=..., stack=...)` and `atmos.helm(...)` add
  structured component arguments, `flags={...}`, and `args=[...]`. All accept
  `working_directory`, `env`, `output`, and `check` like `exec.run`, and return the
  same process result fields. Bare flag keys become `--name`; explicit dash prefixes
  preserve native tool spelling. Values are string/int/bool or lists for repeated
  flags. True emits a bare flag, False emits `=false`. `detailed-exitcode` uses a
  single dash and allows Terraform plan exit 2. Generic `atmos.run` has ordinary
  exit handling. Default cwd is the Atmos invocation directory, not the component
  or script directory. Parent CLI-only flags are not replayed automatically.
- `steps.parallel(functions=[fn, ...], max_concurrency=4, fail_fast=False)`:
  concurrent zero-argument functions, joined results in input order.
- `steps.task(name, function, args=[], kwargs={}, retry=None, timeout="")`:
  deferred descriptor, used with `steps.parallel(tasks=[...])`.
- `components.get(name, stack, type)`: resolved component handle in custom command,
  workflow, and hook script steps, and in their `parallel`/`matrix` children. Results
  are cached per run. Handles expose `name`, `stack`, `type`, `implementation`,
  `path`, `vars`, `settings`, `metadata`, `env`, and `config`.
- `ctx.component` depends on the execution context. A custom command with `component:`
  (`component.type` plus semantic `provides` inputs) gets that component, resolved
  lazily on first access. A hook gets the hook's component, plus `ctx.hook` and
  `ctx.operation`. A workflow has no component in scope, so `ctx.component` is `None`
  and `ctx.component.stack` fails with "NoneType has no .stack field"; call
  `components.get(name, stack, type)` with an explicit stack. `parallel`/`matrix`
  children inherit `ctx.component` and `components.get` from their parent.
- `component.exec(argv, working_directory=..., env={...}, output=..., check=...)`:
  process execution in the component's physical directory (the default
  `working_directory`), using component env with command, step, and per-call
  overrides. Use the handle instead of inferring a path from a logical name.

Do not invent `components.list`, `commands.run`, `steps.run`, or Starlark test-file
discovery: those broader interfaces are not implemented yet.
Use explicit YAML `env` inputs when a value is not available through a component handle.

There are no direct secret, store, or Terraform state/output builtins yet. Resolved
YAML inputs and component configuration can carry values from those existing
services. Unhandled script errors propagate to Atmos's configured error reporting;
Starlark-specific Sentry frames and task tags are not implemented.

## Hook context

Starlark script steps work in `kind: step`, `kind: steps`, and `type: test` hooks:

```yaml
hooks:
  smoke-test:
    events: [after.terraform.apply]
    kind: step
    type: script
    on_failure: fail
    with:
      interpreter: starlark
      script: |
        if ctx.operation.status == "success":
            ctx.component.exec(ctx.component.settings.get("smoke_command", ["./smoke-test"]))
            ui.success("Smoke tests passed for " + ctx.component.name)
```

`ctx.hook` has `name` and canonical `event`. `ctx.operation` has `command`, `status`,
`exit_code`, `error`, `stdout`, and `stderr`. These describe the parent operation,
not a hook subprocess. Before hooks have None for unavailable results. After hooks
use the supplied lifecycle outcome; absent errors are None. Parent stdout/stderr
are not captured by ordinary lifecycle hooks and remain None. Outside hooks,
`ctx.hook` and `ctx.operation` are None. Aggregate/unbound hooks have no component.

Use `ctx.component.settings.get("post_apply", {})` for inherited behavior controls.
The config snapshot is read-only, including nested lists/maps, and works in loaded
functions and parallel tasks. `ctx.component.exec` uses the hook's component workdir;
hook env overrides component env, followed by command, step, and per-call overrides.
Do not assume Terraform outputs/state are attached or refreshed automatically.
Atmos commands called from a hook run their normal lifecycle; scope events or use
supported hook-skipping flags to avoid re-entering the same hook.

## Standalone script spike

Executable files can use `#!/usr/bin/env atmos`. Run `./deploy.star args...` or
`atmos ./deploy.star args...`; a leading `.star` file or explicit path with an Atmos
shebang selects standalone execution. No args preserve the default CLI screen.
Bare extensionless names remain CLI commands. The spike does not add `atmos script run`
or support global CLI options before the script filename.

`ctx.args` is an immutable list of script arguments, including script-owned flags.
`ctx.script.path` and `.directory` identify the physical file. Imports are relative
to that file, while process cwd/config discovery stay with the caller.

Declare tools needed by the script with `dependencies.tools(name, version)`:

```python
#!/usr/bin/env atmos
dependencies.tools("jq", "1.7.1")
exec.run(["jq", "--version"])
```

The existing dependency installer installs missing tools and scopes PATH to the
invocation. Single-file scripts can themselves use an HTTP/raw toolchain registry
entry and `atmos toolchain install owner/tool@version`; Atmos must already be on
PATH for the shebang. Declare dependencies before parallel tasks; declarations
inside branches are rejected. Repeated identical pins are cached and conflicting
versions fail. Packaging libraries is outside this spike.

Use `atmos.toolchain("install", "jq@1.7.1")` to invoke an explicit toolchain command.
It returns the usual process result and does not modify the calling script's PATH.
Use `dependencies.tools` when subsequent script operations need the tool on PATH.
There is no `atmos.toolchain.install` nested API.

## Parallel functions

```python
def describe(name):
    return {"service": name, "region": env["REGION"]}

output = steps.parallel(
    tasks = [
        steps.task(name=n, function=describe, args=[n])
        for n in ["api", "worker"]
    ],
    max_concurrency = 2,
)
```

Pass functions, not their evaluated results. Use either `functions` or `tasks`;
named tasks must be unique. Shared globals, closures, default arguments, task
arguments, and returned branch results become immutable. Create branch-local
collections and return them; do not append to a shared results list. Nested groups
each have their own concurrency limit, so avoid unbounded nested fan-out.

Default behavior finishes all branches and aggregates failures. `fail_fast=True`
cancels outstanding work only after a branch's retry policy is exhausted. The
caller always joins running work. Timeout spans all attempts and backoff.

Retry repeats the entire function. Make its side effects repeatable or place the
retry around a narrower function. Use existing retry keys such as `max_attempts`,
`initial_delay`, `backoff_strategy`, and `max_delay`; `delay` is not a valid key.
Provide `max_attempts` or a task timeout: an explicit policy without an attempt
limit retries until success, timeout, or cancellation. Output-regex `conditions` are
not supported here. A task timeout failure reads `task "<name>" timed out after <duration>`.

## Files and execution

Use `script: !include scripts/deploy.star` for external source, or include a YAML
step definition through the existing include mechanism. `load()` and `fs.read_file`
accept relative and absolute paths. Path rules:

- `!include` in a workflow manifest or custom command: `./x` and `../x` resolve against
  the file that contains the tag, bare `x/y` against the project `base_path`, absolute
  paths as written; never against the directory Atmos runs from.
- `load()` in a script read from a file (`!include`, `!include.raw`, standalone `.star`):
  relative to that file's directory, bare or `./` or `../`, also inside loaded modules.
  Tracebacks name that file and line. In a stack-manifest hook (`kind: step`/`steps`),
  a local `!include`/`!include.raw` `script` (no YQ expression, never remote) next to an
  `interpreter` gets an Atmos-recorded `script_source` sibling key (project-relative
  when inside the project, absolute otherwise), so hook `load()` also resolves next to
  the included file. `script_source` is provenance, visible in `describe component` and
  `describe stacks` output; never write it by hand. A sibling `script_source_sha256`
  (hex SHA-256 of the included content, checked against the script before hook templates
  render) validates it: if a child stack overrides only `script` (for example with an
  inline body), the inherited `script_source` no longer matches and is ignored, so no
  manual cleanup is needed and `load()` resolves against `working_directory`.
- `load()` in an inline `script: |` body: relative to `working_directory`.
- `fs.read_file` and `exec.run`: relative to `working_directory`.

Imports are local, cached per invocation, and reject cycles. Imported functions work in
tasks just like inline functions. `component.exec` defaults to the component directory.

Embedded Starlark runs inside Atmos, so a step under an enabled workflow or step
`container` fails validation before the workflow starts. Set `container: false` on that
step. `atmos workflow --dry-run` parses top-level script steps (reporting syntax
errors) and runs no code or processes; script steps inside `parallel`/`matrix` run nothing either.

Script output is live: lines appear as they are produced. Inside `steps.parallel` with
more than one task, each line is prefixed `[<task name>] ` and lines never interleave
mid-line. Errors are single-line messages with the Starlark traceback shown below.

Validate representative success and failure paths with `type: test` script steps.
When developing the runtime, inject the process runner and retry clock; use barriers
to prove concurrency and run race detection. Test retries below the function boundary
so they exercise real policy logic. Do not run a deployment to test orchestration.

For Kubernetes absence checks, use a checked `kubectl get --ignore-not-found -o json`
and assert empty stdout. Do not treat every nonzero exit as absence. For expected
Helm failures, use `check=False`, require a nonzero code, and check the specific
lifecycle diagnostic.

Read [the script reference](https://atmos.tools/workflows/steps/type/script) for the
public API, and the `atmos-tests` skill for existing YAML test-group reporting.
