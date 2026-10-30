# Atmos Automation Language API Reference

Everything below is predeclared; there is nothing to import. The published
[function reference](https://atmos.tools/functions/automation) documents each function in
more depth. If a name is not listed here, check that page before concluding it does not exist.

## Context and inputs

- `ctx.args`: immutable list of raw script arguments. Standalone scripts get the tokens after the
  script name; Git hooks get the arguments Git passes to the hook. It is empty elsewhere.
- `ctx.flags`, `ctx.arguments`: immutable parsed inputs for script steps. Custom command
  string, bool, and `type: int` flags keep their types; workflow flags are strings and always
  include `stack`. In standalone scripts
  both are empty; use the `cli.command` callback instead.
- `ctx.script`: `.path` and `.directory` of the physical file. Set for standalone scripts and for
  script steps included through a local `!include`; `None` for inline step scripts.
- `ctx.component`, `ctx.hook`, `ctx.operation`: see the entry-point table in `SKILL.md`.
- `env`: immutable dict of the step's explicit `env` inputs.

## Standalone command line

- `cli.command(run, name=, description=, args=, flags=, validate=)`, `cli.arg(...)`,
  `cli.flag(...)`: see [standalone-scripts.md](standalone-scripts.md). Standalone main thread only.

## Environment

- `os.platform` and `os.arch`: read-only strings describing the running Atmos executable (Go names such as
  `linux`, `darwin`, `windows`, `amd64`, `arm64`). Under emulation, architecture refers to the executable.
- `os.getcwd()`: current absolute script execution directory, matching `fs.resolve(".")`.
- `os.environ()`: frozen snapshot of the effective environment, including `os.setenv` and pinned tool paths.
  Empty values are preserved; duplicate names use the last value. Windows snapshot keys are uppercase.
- `os.setenv(name, value)`: set a string variable for subsequent process and step calls in this invocation.
  Overrides component/step inputs; per-call `env=` takes precedence. Does not mutate the declared `env` dictionary.
- `os.chdir(path)`: validate an existing directory and change script-relative file operations and default
  `exec.run`, `atmos.*`, and `steps.*` directories. Component calls keep their component-directory default;
  file-backed imports stay anchored to their importing file.
- `setenv` and `chdir` are main-thread-only. Parallel tasks inherit prior changes and must use per-call
  overrides for differences. Neither function modifies host process state or other invocations.

- `os.getenv(name, default=None)`: read the effective subprocess environment (including tool `PATH`).
  Returns the string, including an explicitly empty string, or the default if absent. Does not change the
  existing `env` dictionary. Per-call subprocess overrides do not change subsequent lookups.

## Processes

- `exec.run(argv, working_directory=, env={...}, output="stream", check=True, timeout="", retry=None, stdin=None)`: argv-based
  child process; returns a result with `.stdout`, `.stderr`, `.exit_code`, and `.data` (stdout
  decoded as JSON on first access; reading it on non-JSON stdout fails). `timeout="30s"` bounds the
  whole call, and `retry={"max_attempts": 3, "initial_delay": "1s", "conditions": [regex, ...]}`
  retries ordinary nonzero exits (`conditions` match the failed attempt's output). It raises on
  failure by default, with the last stderr lines in the error. Use `check=False` to assert
  an expected nonzero exit. Start failures (command not found, bad directory), cancellation,
  signals, and transport errors always raise. `output="stream"` (default) shows output live
  and also captures it; `output="capture"` captures without showing; `output="viewport"` shows live output in a bounded
  viewport (streams without a TTY or in parallel tasks). A `check=True` failure that reaches the top exits Atmos with
  the child's exit code. It inherits the
  effective execution environment without changing the global working directory or
  environment. Relative `working_directory` resolves against the current script directory, including `os.chdir` changes.
  `stdin` accepts a string or `None`: exact bytes, no added newline, replayed fully for each retry;
  absent/`None` gives EOF. Input is not included in command diagnostics.
- `exec.which(name)`: absolute path of the executable `exec.run` would start for `name`, or `None`.
  Searches the same `PATH` as `exec.run` (including `dependencies.tools` directories); a name with a
  path separator resolves relative to the working directory. Use it for prerequisite checks instead of
  running a tool just to see whether it exists.
- `component.exec(argv, working_directory=, env={...}, output=, check=, timeout="", retry=None, stdin=None)`: same, but runs in the
  component's physical directory and uses the component env (then command, step, `os.setenv`, and per-call
  overrides). `os.chdir` does not change its default directory. Use the handle rather than inferring a path from a logical name.

## Atmos commands

- `atmos.<command>(*positional, flags={}, args=[], working_directory=, env=, output=, check=, stdin=None)`
  for every registered command, including custom commands and aliases (`print(dir(atmos))`).
  Examples: `atmos.version()`, `atmos.list("components")`, `atmos.vendor("pull", flags={"stack": "dev"})`,
  `atmos.describe("component", "api", flags={"stack": "dev", "format": "json"}, output="capture")`,
  `atmos.scaffold("generate", "template", "target")`, `atmos.config("get", args=["base_path"])`.
  The `atmos.list`, `atmos.describe`, `atmos.config("get")`, `atmos.stack("get")`, and
  `atmos.stack("config", "get")` wrappers default to JSON and captured output and expose `.data`.
- `atmos.run(argv, working_directory=, env=, output=, check=, stdin=None)`: invoke the current Atmos binary
  with a full argument list. Use it for hyphenated custom command names that cannot be
  attributes: `atmos.run(["my-command", "--flag=value"])`.
- `atmos.terraform(command, component, stack, flags={}, args=[], working_directory=, env=, output=, check=, stdin=None)`
  and `atmos.helm(...)`: structured component arguments. `atmos.tf` is `atmos.terraform` (same keyword `component=`/`stack=`
  signature and `detailed-exitcode` tolerance). Pass the stack as the `stack`
  argument, not as a `flags` entry. Prefer `deploy` over `apply` in non-interactive scripts.
- `atmos.toolchain(command, tool=, flags=, args=, ...)`: explicit toolchain commands, for
  example `atmos.toolchain("install", "jqlang/jq@1.7.1")`. It behaves like the CLI and pins the
  tool in the project's `.tool-versions`. Commands use installed pins; automatic installation depends
  on `toolchain.install` (`always` installs all pins on every run). It does not change the script's
  `PATH`; use `dependencies.tools` for a
  script-scoped pin.
- Flag mapping: bare keys become `--name`; an explicit dash prefix preserves native tool
  spelling (`"-detailed-exitcode"`). Values are strings, ints, bools, or lists (a list
  repeats the flag). `True` emits a bare flag; `False` emits `--name=false`. `detailed-exitcode`
  on `terraform plan` uses a single dash and accepts exit code 2 without raising.
- All wrappers return process results (`.stdout`, `.stderr`, `.exit_code`, `.data`).
  `.data` parses captured stdout as JSON. The query wrappers listed above default to JSON
  and `output="capture"`; other wrappers default to `output="stream"`. Default `check` is
  `True`. The initial default directory is where Atmos was invoked; `os.chdir` changes it for subsequent
  calls. All wrappers accept `stdin` as a string or `None`, with the same byte-preserving behavior as
  `exec.run`; required CLI flags such as `--stdin` must be supplied separately. Parent
  CLI-only flags are not replayed automatically.

## Stack configuration

- `components.get(name, stack, type)`: resolved component handle in standalone scripts (with an
  Atmos project), and in custom command, workflow, and hook script steps and their
  `parallel`/`matrix` children. Results are cached per run. The handle exposes `name`,
  `stack`, `type`, `implementation`, `path`, `vars`, `settings`, `metadata`, `env`, `config`
  (read-only), and `exec`.

## Libraries and records

- `module.load(path)` → frozen namespace containing a local file's public globals. Prefer
  `kube = module.load("lib/kube.star")` and `kube.get(...)`; the file needs no export declaration.
  Excludes `_` names and predeclared builtins. Ordinary `load()` imports require explicit assignment to re-export.
- Accepts a nonempty positional or keyword string, including computed paths, inside functions and parallel tasks.
  Relative paths resolve beside the calling source file, or against the inline script's working directory.
  Both import styles share successful evaluations per run. Concurrent callers wait for initialization;
  cycles fail, and failed initialization may be retried. Top-level effects use the first caller's thread.
- `struct(**fields)` → record with dot access. Keyword-only; fields cannot be reassigned. Nested collections
  remain mutable until frozen by module loading or parallel execution. JSON encoding requires data-only fields.
- Neither API is available in YAML `!starlark` or policy evaluators. `module(...)` is not a constructor.

## Typed configuration

- `config.load(path, schema=..., format="yaml"|"json")` reads one plain YAML/JSON document. Format is inferred
  from `.yaml`, `.yml`, or `.json` when omitted. `config.validate(value, schema=...)` validates existing data.
- Inline schemas use nested dictionaries and `str`, `int`, `float`, `bool` values (not strings).
  Fields are required and unknown fields fail. `schema.field(int, default=5432, min=1, max=65535)` supplies
  defaults/bounds; `schema.optional(str)` permits missing/null values (missing becomes None), `schema.list(str)`
  validates lists, and `schema.enum("dev", "prod")` accepts listed scalars. No implicit type coercion.
- A string `schema="schemas/app.schema.json"` loads a standard JSON Schema file (JSON or YAML).
  File schemas follow JSON Schema semantics: defaults are annotations, required/unknown-field rules must be
  explicit. Local `$ref` files resolve beside the schema. HTTPS schema URLs and relative HTTPS references work with
  30-second timeouts, 4 MiB response limits, cancellation, and no remote-to-local references.
- `cfg.merge(other, list_merge_strategy="replace")` uses Atmos deep merge and returns a new frozen record;
  `other` can be a config record or dictionary. Maps merge recursively and right-hand scalars replace, including
  null/false/zero. Lists replace by default; `"append"` concatenates, `"merge"` merges object pairs by index and
  keeps left-hand overlapping non-object values. Inputs stay unchanged; validate the result explicitly if needed.
  `cfg.merge` is reserved for the method; a data field of that name uses `cfg["merge"]`.
- Both initial file paths resolve beside the caller (inline scripts use working_directory). Each call reads
  afresh. Results are deeply frozen records with dot/index access and JSON encoding; inputs are copied.
  File errors include line/column and field paths; in-memory values have field paths only. Safe in parallel.
  No Atmos YAML tags/templates/imports. Automation-only; dry runs do not read files.

## Concurrency

- `steps.parallel(functions=[fn, ...] | tasks=[...], max_concurrency=4, fail_fast=False)`:
  runs zero-argument functions or tasks; returns results in input order.
- `steps.task(name, function, args=[], kwargs={}, retry=None, timeout="")`: deferred task
  descriptor for `steps.parallel(tasks=[...])`. `retry` keys: `max_attempts`, `initial_delay`,
  `backoff_strategy`, `max_delay`.

## Cleanup

- `defer(fn, *args, **kwargs)`: registers a call to run when the enclosing script or `steps.parallel`
  task finishes, last in first out, after a normal return, `fail()`, or cancellation (then with a
  30 s grace context so `exec.run`/`atmos.*` still work). Arguments are captured at registration. The
  body error stays primary; a failing deferred call is reported after it. This is the `finally` /
  `trap EXIT` equivalent; `when: always` is the YAML-step counterpart. Never write cleanup after a
  `fail()` and expect it to run.

## Step library

- `steps.run(type, **fields)` and `steps.<type>(**fields)` (for example `steps.input`, `steps.choose`,
  `steps.join`, `steps.http`) execute any registered step handler immediately and return its result.
  See [steps.run](https://atmos.tools/functions/automation/steps.run).

## Data helpers

- `json.encode(value)`, `json.decode(text)`: structured data without shell quoting.
  Use `json.indent(text, prefix=, indent=)` to pretty-print JSON text, and `json.encode_indent(value)`
  to encode and format in one call.
- `fs.read_file(path)`: read a local file as a string (relative to the step working
  directory, also inside loaded functions; absolute paths accepted; a leading `~` expands to the home
  directory in every `fs.*` call).
  Use `fs.glob(pattern)`, `fs.stat(path)`, `fs.exists(path)`, and `fs.readlink(path)` to inspect the
  filesystem. `fs.resolve(path)` returns the absolute path (tilde expanded, relative paths joined to the
  working directory, cleaned) without touching the filesystem, so use it instead of hand-rolled
  `~` and absolute-path checks.
- `fs.write_file(path, content, overwrite=False, mode=0o600)`: write exact string bytes and return `None`.
  Paths resolve like `fs.read_file`; parent directories must exist. Default creation fails if the destination
  exists. Explicit overwrite atomically replaces contents and preserves existing permissions, rejecting symlink and nonregular destinations. Mode applies to new files,
  subject to operating system permission rules and umask.
- `yaml.decode(content)`: one YAML data document as dictionaries, lists, and scalars;
  empty input returns `None`. Mapping keys must be strings. Duplicate keys, multiple
  documents, non-finite numbers, and recursive aliases fail. Tags do not execute
  Atmos functions; timestamps become RFC 3339 strings.
- `yaml.validate(content)`: syntax-only validation of every document, returning
  `None`. Accepts custom tags, duplicate keys, non-scalar keys, and empty input.
- `digest.sha256(data)`, `digest.sha512(data)`, `digest.sha1(data)`, `digest.md5(data)`: lowercase hex digest of a
  string. Strings are bytes, so `digest.sha256(fs.read_file(path))` fingerprints a binary file exactly.
  There is no incremental API; sha1 and md5 exist for interoperability (Git ids, S3 ETags), not security.
- `errors.build(message)`: error builder with `.with_title`, `.with_explanation`, `.with_hint`,
  `.with_example`, `.with_context`, `.with_exit_code`, and a final `.fail()`.
- `regex.search(pattern, text)`: boolean match anywhere. `regex.findall(pattern, text)`:
  list of full matches. `regex.replace(pattern, replacement, text)`: replace all matches
  with literal text. Go/RE2 syntax, including inline flags; no lookaround, backreferences,
  or replacement capture expansion. Use ordinary string methods for fixed text.

## Git sandbox

`git.sandbox(run, scope="staged", base_ref="origin/main", fixes="none",
fail_on_changes=True, env={}, path_prepend=[])` calls `run(box)` inside a private
temporary checkout using Atmos's existing Git runner. `staged` materializes the
Git index; `pr` selects changes against the merge base; `all` selects indexed files.
PR/all contents include tracked working-tree edits. Missing base refs fail without
fetching. This supports initial commits and linked worktrees without registering
another worktree or changing branches.

`box` exposes `root`, `path`, `scope`, `base`, `files`, and `added`.
Call `box.track(paths)` before a formatter writes extra outputs such as `go.sum`.
Use `box.copy(paths)` for prebuilt untracked support files; they must be regular
files with absent destinations and are never reconciled back.

Relative filesystem, subprocess, Atmos, and nested-step calls use the sandbox.
Component handles retain their component context. `path_prepend` resolves paths
before entering the sandbox and retains inherited PATH. `env` overrides child
process variables without changing the script's explicit `env` dictionary.

By default, modifications fail and leave working files unchanged. With
`fixes="unstaged"`, fixes merge with existing unstaged edits where conflict-free;
the index remains untouched. Overlap/concurrent edits preserve the original and
save a recovery patch named in the error. Set `fail_on_changes=False` for a format
command; operational errors still propagate. Callback failures can return useful
fixes, but cancellation applies none. Defers run before reconciliation and cleanup
always runs. Nested sandboxes are rejected. This is isolation for cooperating
checks, not a security boundary against scripts accessing external absolute paths.

For commit checks, use sequential `steps.test` children with `output="failures"`
and `fail={"mode": "wait_all"}`. Prepare tools outside the hook. See
[atmos-git](../../atmos-git/SKILL.md) for installation.

## Output

- `print(...)`: stdout (data).
- `ui.info(message)` (`▶`), `ui.success(message)` (`✓`), `ui.warning(message)` (`⚠`): stderr,
  with Markdown rendering (emphasis, code, links, and lists), including in no-color output.
  Ordered lists render with `1.`-style markers, unordered lists with `•`; bare emails and URLs
  render once. Named Markdown links retain their destination. To show a literal marker,
  use `ui.info("1\\. literal")`, which prints `▶ 1. literal`; two backslashes in the Starlark string
  pass one Markdown escape character to the renderer.
- `log.trace/debug/info/warn/error(message, **fields)`: Atmos logger. Field names must be
  identifiers. `step` and `task` fields are added automatically. Never the step value.

## CI

- `ci.context`: read-only struct, resolved on first access: `provider`, `local` (no CI provider
  detected; the generic provider renders locally), `event`, `sha`, `branch`, `repo`, `actor`,
  `run_id`, `run_url`, `elevated` (`pull_request_target`/`workflow_run`), and `pr` (`number`,
  `head`, `base`, `url`) or `None`. `ci.base()`: the affected-detection base
  (`ref`, `sha`, `head_sha`, `target_branch`, `source`) or `None`.
- Writes, one call each, with a local rendering on stderr when no provider is detected:
  `ci.summary(markdown)`, `ci.output(name, value)`, `ci.env(name, value)`, `ci.path(dir)`,
  `ci.mask(value)` (registers with Atmos masking at once; `::add-mask::` in CI),
  `ci.annotate(level, message, file="", line=0, end_line=0, title="")` (`error|warning|notice`),
  `ci.comment(body, key="", behavior="upsert", pr=0)` → struct `id`, `url`, `created` (a `key`
  makes later runs update the same comment; locally a preview, never posted),
  `ci.check(name, state="pending", description="", url="")` → handle with `name`, `state`, `id`,
  `url`, and `update(state, description="", url="")` (a commit status on GitHub; no body),
  `ci.group(title, fn)` → `fn()`'s result inside a collapsible log group, and
  `ci.sarif(path, category="")`.
- In CI each write honors the same switches as native reporting (`ci.enabled` plus
  `ci.summary.enabled`, `ci.output.enabled` (also env/path), `ci.annotations.enabled`,
  `ci.results.enabled`, `ci.checks.enabled`, `ci.comments.enabled`, `ci.groups.mode`). A
  gated-off call renders locally and warns with the disabled flag; local rendering errors can
  still fail the script. `ci.mask` registers the secret with Atmos masking without a gate warning
  when `ci.enabled` is disabled. Comments
  and checks are held on elevated events unless `ci.allow_unsafe_fork_execution` is set.
- Distinct from `atmos.ci(...)`, which runs the `atmos ci` CLI command group as a subprocess.
  Reference: [ci](https://atmos.tools/functions/automation/ci).

## Toolchain

- `dependencies.tools(name, version)`: install and pin a tool for the invocation. Main thread
  only, before `steps.parallel`.

## Language dialect

Top-level `if`, `for`, and `while`; `set()`; and recursion are enabled. Globals can be assigned
once. There are no classes, exceptions, or `import`; use `load("path.star", "name")` for local modules
and `fail("message")` to abort.
