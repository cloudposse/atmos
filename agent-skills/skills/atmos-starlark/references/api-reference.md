# Atmos Automation Language API Reference

Everything below is predeclared; there is nothing to import. See the
[language overview](https://atmos.tools/automation/language) and
[script step](https://atmos.tools/steps/type/script) for usage. If a name is not listed here, it does not exist.

## Context and inputs

- `ctx.args`: immutable list of raw script arguments (standalone scripts only).
- `ctx.flags`, `ctx.arguments`: immutable parsed inputs for script steps. Custom command
  string and bool flags keep their types; workflow flags are strings. In standalone scripts
  both are empty; use the `cli.command` callback instead.
- `ctx.script`: `.path` and `.directory` of the physical file for standalone scripts and
  file-backed included script steps; `None` for inline steps.
- `ctx.component`, `ctx.hook`, `ctx.operation`: see the entry-point table in `SKILL.md`.
- `env`: immutable dict of the step's explicit `env` inputs.

## Standalone command line

- `cli.command(run, name=, description=, args=, flags=, validate=)`, `cli.arg(...)`,
  `cli.flag(...)`: see [standalone-scripts.md](standalone-scripts.md). Standalone main thread only.

## Processes

- `exec.run(argv, working_directory=, env={...}, output="stream", check=True)`: argv-based
  child process; returns a result with `.stdout`, `.stderr`, and `.exit_code`. It raises on
  failure by default, with the last stderr lines in the error. Use `check=False` to assert
  an expected nonzero exit. Start failures (command not found, bad directory), cancellation,
  signals, and transport errors always raise. `output="stream"` (default) shows output live
  and also captures it; `output="capture"` captures without showing. It inherits the
  effective execution environment without changing the global working directory or
  environment. Relative `working_directory` resolves against the step working directory.
- `component.exec(argv, working_directory=, env={...}, output=, check=)`: same, but runs in the
  component's physical directory and uses the component env (then command, step, and per-call
  overrides). Use the handle rather than inferring a path from a logical name.

## Atmos commands

- `atmos.<command>(*positional, flags={}, args=[], working_directory=, env=, output=, check=)`
  for every registered command, including custom commands and aliases (`print(dir(atmos))`).
  Examples: `atmos.version()`, `atmos.list("components")`, `atmos.vendor("pull", flags={"stack": "dev"})`,
  `atmos.describe("component", "api", flags={"stack": "dev", "format": "json"}, output="capture")`,
  `atmos.scaffold("generate", "template", "target")`, `atmos.config("get", args=["base_path"])`.
- `atmos.run(argv, working_directory=, env=, output=, check=)`: invoke the current Atmos binary
  with a full argument list. Use it for hyphenated custom command names that cannot be
  attributes: `atmos.run(["my-command", "--flag=value"])`.
- `atmos.terraform(command, component, stack, flags={}, args=[], working_directory=, env=, output=, check=)`
  and `atmos.helm(...)`: structured component arguments. Pass the stack as the `stack`
  argument, not as a `flags` entry. Prefer `deploy` over `apply` in non-interactive scripts.
- `atmos.toolchain(command, tool=, flags=, args=, ...)`: explicit toolchain commands, for
  example `atmos.toolchain("install", "jq@1.7.1")`. Does not change the script's `PATH`.
- Flag mapping: bare keys become `--name`; an explicit dash prefix preserves native tool
  spelling (`"-detailed-exitcode"`). Values are strings, ints, bools, or lists (a list
  repeats the flag). `True` emits a bare flag; `False` emits `--name=false`. `detailed-exitcode`
  on `terraform plan` uses a single dash and accepts exit code 2 without raising.
- All wrappers return process results (`.stdout`, `.stderr`, `.exit_code`) and preserve native
  CLI behavior; they do not return parsed data. Default `output` is `"stream"`, default
  `check` is `True`, and the default working directory is where Atmos was invoked. Parent
  CLI-only flags are not replayed automatically.

## Stack configuration

- `components.get(name, stack, type)`: resolved component handle in standalone scripts (with an
  Atmos project), and in custom command, workflow, and hook script steps and their
  `parallel`/`matrix` children. Results are cached per run. The handle exposes `name`,
  `stack`, `type`, `implementation`, `path`, `vars`, `settings`, `metadata`, `env`, `config`
  (read-only), and `exec`.

## Concurrency

- `steps.parallel(functions=[fn, ...] | tasks=[...], max_concurrency=4, fail_fast=False)`:
  runs zero-argument functions or tasks; returns results in input order.
- `steps.task(name, function, args=[], kwargs={}, retry=None, timeout="")`: deferred task
  descriptor for `steps.parallel(tasks=[...])`. `retry` keys: `max_attempts`, `initial_delay`,
  `backoff_strategy`, `max_delay`.

## Data helpers

- `json.encode(value)`, `json.decode(text)`: structured data without shell quoting.
- `fs.read_file(path)`: read a local file as a string (relative to the step working
  directory, also inside loaded functions; absolute paths accepted). There is no write API.
- `regex.search(pattern, text)`: boolean match anywhere. `regex.findall(pattern, text)`:
  list of full matches. `regex.replace(pattern, replacement, text)`: replace all matches
  with literal text. Go/RE2 syntax, including inline flags; no lookaround, backreferences,
  or replacement capture expansion. Use ordinary string methods for fixed text.

## Output

- `print(...)`: stdout (data).
- `ui.info(message)` (`▶`), `ui.success(message)` (`✓`), `ui.warning(message)` (`⚠`): stderr.
- `log.trace/debug/info/warn/error(message, **fields)`: Atmos logger. Field names must be
  identifiers. `step` and `task` fields are added automatically. Never the step value.

## Toolchain

- `dependencies.tools(name, version)`: install and pin a tool for the invocation. Main thread
  only, before `steps.parallel`.

## Language dialect

Top-level `if`, `for`, and `while`; `set()`; and recursion are enabled. Globals can be assigned
once. There are no classes, exceptions, or `import`; use `load("path.star", "name")` for local modules
and `fail("message")` to abort.
