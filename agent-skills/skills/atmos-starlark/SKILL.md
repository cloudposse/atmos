---
name: atmos-starlark
description: "Write Atmos Automation Language scripts (Python-like, embedded Starlark): standalone executable tools (`atmos ./deploy.star`, `#!/usr/bin/env atmos`, typed flags via `cli.command`), `type: script` steps in workflows, custom commands, and hooks, parallel tasks, retries, and migration of Bash/Python glue. Reach for it when a task needs Atmos capabilities (deploy components, build and push containers, pin tools with dependencies.tools, use atmos.yaml auth identities, read resolved stack config with components.get) instead of Bash or Python. Use atmos-steps for the YAML step catalog and atmos-custom-commands for YAML command declarations."
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
  category: ci-automation
---

# Atmos Automation Language

Atmos Automation Language is a Python-like scripting language built on embedded
Starlark and run by the Atmos binary itself. There is no Python or Bash to install.
A script can call Atmos directly, so it sees the same stacks, components, identities,
and toolchain that the rest of Atmos uses.

Reach for it, instead of Bash or Python glue, when a task needs Atmos capabilities:

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
The script step configuration is `type: script` with `interpreter: starlark`.

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
| Git hook step | `git.hooks.<name>.steps` run by the installed Git shim | `ctx.args` holds the hook's arguments from Git, such as the commit message path for `commit-msg`. |

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
  inherit the effective process environment; pass `env={...}` to add per-call overrides.
- `ctx.flags` and `ctx.arguments`: immutable parsed command inputs, preserving host types
  in custom commands. Read these directly; do not map flags through `env` or template
  values into script source.
- Atmos renders step script bodies as Go templates before Starlark runs, so `{{` in source
  fails. Write `script: !literal |` for any script that contains braces; it runs the body exactly
  as written. Split the delimiter (`"{" + "{"`) only for included scripts, which are still rendered.
- Output channels: `print` writes data to stdout. `ui.info` (`▶`), `ui.success` (`✓`), and
  `ui.warning` (`⚠`) write human status to stderr. `log.trace/debug/info/warn/error(message, **fields)`
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
  and `steps.input` and `steps.choose` prompt. Also available: `fs.read_file`, `fs.glob`,
  `fs.stat`, `fs.exists`, `fs.readlink`, `fs.resolve` (absolute path; every `fs.*` call accepts a leading `~`), `exec.which` (path or `None`), `defer(fn, *args)` (cleanup that runs when the script or task finishes, even after `fail()`; there is no `try`/`finally`),
  `digest.sha256`/`digest.sha512`/`digest.sha1`/`digest.md5` (hex digest of a string or file contents), `errors.build(...)` (a builder ending in `.fail()`),
  `json.indent`, `json.encode_indent`, and the `ci` module (`ci.context`, `ci.summary`,
  `ci.comment`, `ci.annotate`, `ci.output`, `ci.env`, `ci.path`, `ci.mask`, `ci.check`,
  `ci.group`, `ci.sarif`, `ci.base`; distinct from the `atmos.ci(...)` command wrapper).
  Never invent `components.list`, `commands.run`,
  a file-write API, or direct secret, store, or Terraform state/output builtins. They do
  not exist. Resolved YAML inputs and component configuration can carry values from those
  services.
- Starlark-specific failures include the traceback; unhandled script errors propagate to
  Atmos's configured error reporting.

## Calling Atmos from a script

- `atmos.<command>(...)` exists for every registered Atmos command, including custom
  commands and aliases. Run `print(dir(atmos))` to list them. Positional arguments are
  literal CLI arguments; keyword-only options are `flags`, `args`, `working_directory`,
  `env`, `output`, and `check`. Example: `atmos.describe("component", "api", flags={"stack": "dev"})`.
- Names that are not valid identifiers (hyphenated command names) are called through
  `atmos.run(["my-command", "arg"])`.
- `atmos.terraform(command, component, stack, flags=, args=, working_directory=, env=, output=, check=)`
  and `atmos.helm(...)` take structured component arguments. Prefer `deploy` over `apply`
  in non-interactive scripts.
- Wrappers run the current Atmos binary as a subprocess and return a process result
  (`.stdout`, `.stderr`, `.exit_code`, and `.data`, the lazily decoded JSON of stdout).
  The `atmos.list`, `atmos.describe`, `atmos.config("get")`, `atmos.stack("get")`, and
  `atmos.stack("config", "get")` wrappers default to JSON and captured output, so read `.data`
  directly. For other wrappers, pass `--format=json` flags and read `.data` or
  `json.decode(result.stdout)`.
- Default working directory is the directory Atmos was invoked from, not the script or
  component directory.

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
not a hook subprocess. Before hooks have `None` for unavailable results. After hooks
use the supplied lifecycle outcome; absent errors are `None`. Parent stdout/stderr
are not captured by ordinary lifecycle hooks and remain `None`. Outside hooks,
`ctx.hook` and `ctx.operation` are `None`. Aggregate/unbound hooks have no component.

Use `ctx.component.settings.get("post_apply", {})` for inherited behavior controls.
The config snapshot is read-only, including nested lists/maps, and works in loaded
functions and parallel tasks. `ctx.component.exec` uses the hook's component workdir;
hook env overrides component env, followed by command, step, and per-call overrides.
Do not assume Terraform outputs/state are attached or refreshed automatically.
Atmos commands called from a hook run their normal lifecycle; scope events or use
supported hook-skipping flags to avoid re-entering the same hook.

## Parallel functions

```python
def describe(name):
    return {"service": name, "region": env["REGION"]}

output = steps.parallel(
    tasks = [
        steps.task(name=n, function=describe, args=[n], timeout="30s")
        for n in ["api", "worker"]
    ],
    max_concurrency = 2,
)
```

Pass functions, not their evaluated results. Use either `functions` or `tasks`;
named tasks must be unique. `max_concurrency` defaults to 4. Shared globals, closures,
default arguments, task arguments, and returned branch results become immutable. Create
branch-local collections and return them; do not append to a shared results list. Nested
groups each have their own concurrency limit, so avoid unbounded nested fan-out.

Default behavior finishes all branches and aggregates failures. `fail_fast=True`
cancels outstanding work only after a branch's retry policy is exhausted. The
caller always joins running work. A task timeout spans all attempts and backoff.

Retry repeats the entire function. Make its side effects repeatable or place the
retry around a narrower function. Use existing retry keys such as `max_attempts`,
`initial_delay`, `backoff_strategy`, and `max_delay`; `delay` is not a valid key.
Provide `max_attempts` or a task timeout: an explicit policy without an attempt
limit retries until success, timeout, or cancellation. Output-regex `conditions` do not
apply to function tasks; they work on `exec.run(..., retry={...})`, where they match the
failed attempt's output. A task timeout failure reads `task "<name>" timed out after <duration>`.

Use `steps.task(..., timeout="30s")` for per-task limits. A `timeout:` on the YAML script step
is enforced: the interpreter and its subprocesses are canceled and the step fails with
`step timed out`.

Script output is live: lines appear as they are produced. Inside `steps.parallel` with
more than one task, each line is prefixed `[<task name>] ` and lines never interleave
mid-line. Errors are single-line messages with the Starlark traceback shown below.

## Toolchain dependencies

Declare tools the script needs with `dependencies.tools(name, version)`:

```python
#!/usr/bin/env atmos
dependencies.tools("jqlang/jq", "1.7.1")
exec.run(["jq", "--version"])
```

The existing dependency installer installs missing tools and scopes `PATH` to the
invocation. Declare dependencies in the main thread before parallel tasks; declarations
inside branches are rejected. Repeated identical pins are cached and conflicting
versions fail. `atmos.toolchain("install", "jqlang/jq@1.7.1")` runs an explicit toolchain
command like the CLI: it pins the tool in the project's `.tool-versions`, after which every
Atmos command in that project installs it automatically. It returns the usual process result
and does not change the calling script's `PATH`. Use `dependencies.tools` for a script-scoped
pin that puts the tool on `PATH`. Spell tools `owner/name`; a short name such as `jq` fails
when it matches more than one package. There
is no `atmos.toolchain.install` nested API. See `atmos-toolchain` for registries.

## Files, modules, and testing

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

## Related skills

- `atmos-steps` for the YAML step catalog and shared step fields.
- `atmos-custom-commands` for YAML command declarations, `arguments`, and `flags`.
- `atmos-workflows` and `atmos-hooks` for where script steps run.
- `atmos-container`, `atmos-terraform`, `atmos-toolchain`, `atmos-auth` for the
  capabilities scripts call into.
