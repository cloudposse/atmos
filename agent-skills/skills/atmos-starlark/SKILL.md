---
name: atmos-starlark
description: "Write Atmos Automation Language scripts (Python-like, embedded Starlark): standalone executable tools (`atmos ./deploy.star`, `#!/usr/bin/env atmos`, typed flags via `cli.command`), `type: script` steps in workflows, custom commands, and hooks, parallel tasks, retries, and migration of Bash/Python glue. Reach for it when a task needs Atmos capabilities (deploy components, build and push containers, pin tools with dependencies.tools, use atmos.yaml auth identities, read resolved stack config with components.get) instead of Bash or Python. Use atmos-steps for the YAML step catalog and atmos-custom-commands for YAML command declarations."
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.1.0"
  category: ci-automation
---

# Atmos Automation Language

Atmos Automation Language embeds Python-like Starlark in Atmos, with no Python or
Bash installation. Scripts call Atmos directly, sharing its stacks, components,
identities, and toolchain.

Use it for tasks that need Atmos capabilities:

- Deploy components and run any Atmos command: `atmos.terraform("deploy", "vpc", "dev")`,
  or `atmos.container("build", "web", flags={"stack": "dev"})` (then `"push"`) to build and
  push container images.
- Read resolved stack configuration: `components.get("vpc", "dev", "terraform").vars`.
- Pin tools without a manual install: `dependencies.tools("jqlang/jq", "1.7.1")`.
- Use identities and credentials configured under `auth` in `atmos.yaml`; the scripts
  inherit them like any other Atmos command.
- Fan out work safely with `steps.parallel`, retries, and per-task timeouts.

Keep simple one-command automation in YAML (`atmos-workflows`, `atmos-custom-commands`,
`atmos-steps`). Use a script when the logic needs loops, conditionals, data shaping,
or concurrency. Declarations of project commands and component bindings stay in YAML.
The script step configuration is `type: script` with `interpreter: starlark`. To run a script file,
set `source: scripts/check.star` instead of `script:`; Atmos infers `starlark` from the `.star`
extension, and `load()` resolves relative to that file.

Documentation: [Language reference](https://atmos.tools/automation/language),
[Custom CLI apps](https://atmos.tools/automation/standalone-cli-apps),
[function reference](references/api-reference.md), and the
[script step](https://atmos.tools/steps/type/script).

## Choose the entry point

| Entry point | Run with | Inputs the script reads |
|---|---|---|
| Standalone script | `atmos ./deploy.star args...`, `atmos deploy.star`, or `./deploy` with an Atmos shebang | `cli.command` callback `args` and `flags` dicts; raw `ctx.args`. `ctx.flags`, `ctx.arguments`, and `env` are empty. |
| Custom command step | `atmos <name>` (command declared in `atmos.yaml`) | `ctx.flags` (string, bool, and `int` types preserved), `ctx.arguments` (an omitted optional argument is `""`). `ctx.args` is empty; arguments after `--` are not exposed to scripts. |
| Workflow step | `atmos workflow <name>` | `ctx.flags` (string map). `env` holds only the step's declared `env`. |
| Hook step | lifecycle event (`kind: step`, `kind: steps`, `type: test`) | `ctx.component`, `ctx.hook`, `ctx.operation`. |
| Git hook step | `git.hooks.<name>.steps` run by the installed Git shim | `ctx.args` holds the hook's arguments from Git, such as the commit message path for `commit-msg`. Shell steps get `"$@"` and `ATMOS_GIT_HOOK_ARGS`; `ATMOS_GIT_HOOK_STDIN` is a temp-file path for hooks that receive stdin. |

Details for each entry point:

- Standalone scripts and `cli.command`: [references/standalone-scripts.md](references/standalone-scripts.md).
- Script steps in YAML (templating, output, labels, timeouts, includes): [references/script-steps.md](references/script-steps.md).
- Every predeclared module and function: [references/api-reference.md](references/api-reference.md).

## Minimal examples

Standalone tool with typed inputs:

```python
#!/usr/bin/env atmos

def main(args, flags):
    dependencies.tools("jqlang/jq", "1.7.1")
    for stack in flags["stack"]:
        atmos.terraform("deploy", args["component"], stack)
        ui.success("deployed " + args["component"] + " to " + stack)

cli.command(
    run = main,
    description = "Deploy a component to one or more stacks",
    args = [cli.arg("component", description="Component name")],
    flags = [cli.flag("stack", type="string_list", shorthand="s", required=True)],
)
```

Run it with `./deploy.star vpc --stack=dev --stack=prod`.

Workflow step reading its flags, with live subprocess output:

```yaml
workflows:
  smoke:
    steps:
      - name: check
        type: script
        interpreter: starlark
        script: |
          comp = components.get("api", ctx.flags["stack"], "terraform")
          exec.run(["./smoke-test", comp.vars["url"]], working_directory=comp.path)
```

## Language rules

- Dialect: top-level `if`/`for`/`while`, `set()`, and recursion are allowed. A recursion
  depth limit (10,000 frames) stops runaway recursion with a clean "recursion depth
  exceeded" error and a collapsed traceback.
- Globals are assigned once: `x = 1` then `x = 2`, `n += 1` at the top level, or assigning
  `output` in both branches of an if/else fails with `cannot reassign global`. Use
  `output = a if cond else b`, or `def main(): ... return v` then `output = main()`.
  Mutate a dict (`state["n"] += 1`) before `steps.parallel`, which freezes shared state.
- `env`: immutable explicit step inputs (the step's declared `env`). The ambient process
  environment is not copied into it. Child processes from `exec.run` and `atmos.*` still
  inherit the effective process environment; read it with `os.getenv(name, default=None)`
  and pass `env={...}` to add per-call overrides. Missing variables return the default; empty values stay empty.
- `os.platform` and `os.arch` are read-only executable platform/architecture strings. `os.getcwd()` reads
  the script directory; `os.environ()` returns a frozen environment snapshot. `os.setenv(name, value)` and
  `os.chdir(path)` change invocation-local defaults only from the main thread. Parallel tasks inherit those
  defaults and use per-call overrides; host process state stays unchanged.
- `ctx.flags` and `ctx.arguments`: immutable parsed command inputs, preserving host types
  in custom commands. Read these directly; do not map flags through `env` or template
  values into script source.
- Atmos renders step script bodies as Go templates before Starlark runs. Valid `{{ ... }}` actions
  are rendered; malformed or unresolved actions can fail. Write `script: !literal |` when braces
  must remain literal. A script loaded with `!include.raw` is also used as written; one loaded with `!include` is still rendered (move
  the braces into a `load()`ed module, or use `!include.raw`). `!literal` also works on `command`, `interpreter`,
  `working_directory`, `timeout`, and single `env` values; on any other step field it fails config load.
- Output channels: `print` writes data to stdout. `ui.info` (`▶`), `ui.success` (`✓`), and
  `ui.warning` (`⚠`) write Markdown-formatted human status to stderr. Emphasis, code, links, and
  lists are interpreted even without color: ordered lists retain numbering, bullets render as `•`,
  and bare emails/URLs render once. Explicit Markdown links retain their destination.
  Escape Markdown punctuation for literal markers:
  `ui.info("1\\. literal")` prints `▶ 1. literal` (two Starlark backslashes pass one to Markdown).
  `log.trace/debug/info/warn/error(message, **fields)`
  writes diagnostics through the Atmos logger with `step` (and, in parallel tasks, `task`)
  fields; it honors `ATMOS_LOGS_LEVEL`, `--logs-level`, and `logs.file`, and masks secrets.
  `ci.summary`, `ci.comment`, `ci.annotate`, `ci.output`, `ci.env`, `ci.path`, `ci.mask`,
  `ci.check`, `ci.group`, and `ci.sarif` report into the CI provider running the script and
  render locally on stderr when none is detected; `ci.context` describes the run. In CI they
  honor the `ci.*.enabled` switches and warn (naming the flag) instead of failing when one is
  off. Use `ci.*`, never `exec.run(["gh", ...])` or hand-written `$GITHUB_ENV` appends; see
  [references/api-reference.md](references/api-reference.md#ci).
- Top-level `output`: a string is emitted raw, any other value is JSON-encoded, and a value
  that cannot be encoded (such as a function) fails the script. Without `output`, captured
  stdout is the step value.
- `steps.run(type, **fields)` and the `steps.<type>(...)` library call any registered step
  handler (see [steps.run](https://atmos.tools/functions/automation/steps.run)); they exist,
  and `steps.input` and `steps.choose` prompt. Also available: `fs.read_file`, `fs.write_file(path, content, overwrite=False, mode=0o600)`, `fs.glob`,
  `fs.stat`, `fs.exists`, `fs.readlink`, `fs.resolve` (absolute path; every `fs.*` call accepts a leading `~`), `exec.which` (path or `None`), `defer(fn, *args)` (cleanup that runs when the script or task finishes, even after `fail()`; there is no `try`/`finally`),
  `digest.sha256`/`digest.sha512`/`digest.sha1`/`digest.md5` (hex digest of a string or file contents), `errors.build(...)` (a builder ending in `.fail()`),
  `json.indent`, `json.encode_indent`, and the `ci` module (`ci.context`, `ci.summary`,
  `ci.comment`, `ci.annotate`, `ci.output`, `ci.env`, `ci.path`, `ci.mask`, `ci.check`,
  `ci.group`, `ci.sarif`, `ci.base`; distinct from the `atmos.ci(...)` command wrapper).
  `yaml.decode` reads one YAML data document; `yaml.validate` checks syntax across
  all documents without evaluating tags. `git.sandbox` provides isolated Git checkouts
  and safe unstaged fix reconciliation; see [the API reference](references/api-reference.md).
  Never invent `components.list`, `commands.run`,
  direct secret, store, or Terraform state/output builtins. They do
  not exist. Resolved YAML inputs and component configuration can carry values from those
  services.
- Starlark-specific failures include the traceback; unhandled script errors propagate to
  Atmos's configured error reporting.

## Calling Atmos from a script

- `atmos.<command>(...)` exists for every registered Atmos command, including custom
  commands and aliases. Run `print(dir(atmos))` to list them. Positional arguments are
  literal CLI arguments; keyword-only options are `flags`, `args`, `working_directory`,
  `env`, `output`, `check`, and `stdin`. `output` is `"stream"` (default), `"capture"`, or `"viewport"` (falls back to streaming
  without a TTY or in parallel tasks); `exec.run` and `component.exec` take the same modes. A failed `check=True` call
  that reaches the top of the script exits Atmos with the child's exit code. `--profile` and `--identity` reach nested
  `atmos.*` calls in every script context. Example: `atmos.describe("component", "api", flags={"stack": "dev"})`.
- Names that are not valid identifiers (hyphenated command names) are called through
  `atmos.run(["my-command", "arg"])`.
- `atmos.terraform(command, component, stack, flags=, args=, working_directory=, env=, output=, check=, stdin=None)`
  and `atmos.helm(...)` take structured component arguments; `atmos.tf` is `atmos.terraform` with the same signature
  (keyword `component=`, `stack=`, `detailed-exitcode` tolerance). Prefer `deploy` over `apply`
  in non-interactive scripts.
- Wrappers run the current Atmos binary as a subprocess and return a process result
  (`.stdout`, `.stderr`, `.exit_code`, and `.data`, the lazily decoded JSON of stdout).
  The `atmos.list`, `atmos.describe`, `atmos.config("get")`, `atmos.stack("get")`, and
  `atmos.stack("config", "get")` wrappers default to JSON and captured output, so read `.data`
  directly. For other wrappers, pass `--format=json` flags and read `.data` or
  `json.decode(result.stdout)`.
- The initial default working directory is the directory Atmos was invoked from. `os.chdir` changes
  that default for subsequent Atmos calls; relative per-call directories resolve against the current default.

## Hook context

Component lifecycle hooks expose `ctx.component`, `ctx.hook`, and `ctx.operation`.
Aggregate or unbound hooks have `ctx.component == None`: guard it before reading
`ctx.component.settings` or calling `ctx.component.exec` in shared handlers. Component
settings are read-only; `exec` runs in the component directory. Outside hooks,
`ctx.hook` and `ctx.operation` are `None`; guard them in scripts shared with other callers.
For examples, event outcomes, environment precedence, recursion limits, and stdout
precautions, read [Hook context](references/hook-context.md).

## Parallel functions

Use `steps.parallel` with named `steps.task` calls for concurrent work. Pass functions,
not their evaluated results; return branch-local values instead of mutating shared state.
See [parallel functions](references/parallel-functions.md) for the complete example,
retry and timeout rules, cancellation, and output behavior.

## Toolchain dependencies

Declare tools the script needs with `dependencies.tools(name, version)`:

```python
#!/usr/bin/env atmos
dependencies.tools("jqlang/jq", "1.7.1")
exec.run(["jq", "--version"])
```

The dependency installer installs missing tools and scopes `PATH` to this invocation.
Declare dependencies in the main thread before parallel tasks; branch declarations are
rejected. Identical pins are cached; conflicting versions fail.
`atmos.toolchain("install", "jqlang/jq@1.7.1")` pins the tool in `.tool-versions`.
Subsequent commands honor `toolchain.install`. It returns a process result without
changing the script's `PATH`; use `dependencies.tools` for a script-scoped pin on `PATH`.
Spell tools `owner/name`; ambiguous short names fail. There is no
`atmos.toolchain.install` nested API. See `atmos-toolchain` for registries.

## Process input and file output

Pass `stdin=value` to `exec.run`, `component.exec`, or any `atmos.*` wrapper to send exact bytes without
putting them in command arguments; omit it or pass `None` for EOF. Atmos excludes the input from command
diagnostics, while subprocess output still follows the configured output policy. Supply CLI flags such as
`--stdin` separately; the wrapper does not add them.

Use `fs.write_file(path, content)` to create a file with mode `0o600`; existing destinations fail unless
`overwrite=True`. Retry, permission, and path details are in
[references/api-reference.md](references/api-reference.md).

## Files, modules, and testing

Prefer `kube = module.load("lib/kube.star")` followed by `kube.get("pods")` for shared libraries.
The file defines `def get(...): ...` directly; no export object or repeated function list is needed.
Public globals are exported automatically; names starting with `_` remain private. The namespace and all
library globals are frozen after initialization. `module.load(path)` accepts computed paths and works
inside functions and parallel tasks. Relative paths resolve beside the calling file, or against
`working_directory` for inline scripts. Successful loads share a per-run cache with ordinary `load()`;
concurrent callers initialize a file once, and cycles fail. Ordinary `load()` imports are file-local;
assign one to a global to re-export it.

Use `struct(name="api", port=8080)` for ad hoc records with dot access. Fields cannot be reassigned;
nested collections remain mutable until frozen. Both APIs are automation-only, not YAML `!starlark`
or policy APIs. `module` is a namespace with `.load`, not a callable constructor.

Use `config.load("app.yaml", schema={"host": str, "port": schema.field(int, default=5432)})`
for typed YAML/JSON with dot access, or pass a JSON Schema file or HTTPS URL as `schema=`.
`config.validate(value, schema=...)` checks data already in memory, and `config1.merge(config2)`
deep-merges records with right-hand precedence. Results are frozen. Field rules, defaults, error
positions, and `list_merge_strategy` are in the "Typed configuration" section of
[references/api-reference.md](references/api-reference.md).

Use `script: !include scripts/deploy.star` for external source. Path rules, templating
of included files, and hook provenance keys are in
[references/script-steps.md](references/script-steps.md).

Validate representative success and failure paths with `type: test` script steps (see the
`atmos-tests` skill for YAML test groups). Test retries below the function boundary so
they exercise real policy logic, and never run a deployment to test orchestration.

For Kubernetes absence checks, use a checked `kubectl get --ignore-not-found -o json`
and assert empty stdout. Do not treat every nonzero exit as absence. For expected
Helm failures, use `check=False`, require a nonzero code, and check the specific
lifecycle diagnostic.

## Terminal recordings

Use `cast.recorder(...)` and `rec.terminal` to declare recordings. `rec.generate()`
executes them; `rec.validate()` checks saved evidence. See `atmos-cast` for the
API, lifecycle, validation, and presentation conventions.

## Related skills

- `atmos-steps` for the YAML step catalog and shared step fields.
- `atmos-custom-commands` for YAML command declarations, `arguments`, and `flags`.
- `atmos-workflows` and `atmos-hooks` for where script steps run.
- `atmos-container`, `atmos-terraform`, `atmos-toolchain`, `atmos-auth` for the
  capabilities scripts call into.
