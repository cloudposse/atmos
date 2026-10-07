# Script Steps in Workflows, Custom Commands, and Hooks

A script step runs embedded Starlark inside Atmos:

```yaml
steps:
  - name: summarize
    type: script
    interpreter: starlark
    working_directory: components/terraform/vpc
    env:
      REGION: us-east-2
    script: |
      print("region", env["REGION"])
```

`interpreter` and `script` are required; never put `command` on a script step. See the
[script step reference](https://atmos.tools/steps/type/script) for the full field list and
the [Language reference](https://atmos.tools/automation/language).

## Templates in script source

The script body is rendered as a Go template before Starlark runs. This applies to inline
`script: |` bodies and to files loaded with `script: !include scripts/x.star`. Consequences:

- `{{` and `}}` in Starlark source are template delimiters. A literal `{{` in a string fails
  with a template error (for example `function "x" not defined`).
  Tag the script with `!literal` to skip rendering: `script: !literal |` runs the body exactly
  as written, in sequential steps, `parallel`/`matrix` children, custom commands, workflows,
  and hooks. Prefer it for any script that contains braces, including `"{}".format(...)`
  next to `{{`. `!literal` also works on `command`, `interpreter`, `working_directory`, and
  individual `env` values, and only the tagged field skips rendering.
- Fallback when `!literal` is not an option, such as a script loaded with `!include`, which is
  still rendered: avoid the sequence by splitting it, for example `"{" + "{ value }}"`.
  Escape sequences such as `{{"{{"}}` do not survive, because the rendered result is rendered
  again (workflows and custom commands run more than one render pass).
- Sprig and Gomplate functions (`{{ upper "abc" }}`) work the same in sequential steps and in
  `parallel`/`matrix` child steps: children render with the renderer and pass count of the
  context that contains them.
- Only the values declared under the step's `env:` are rendered. The ambient process
  environment passes through to the script and its child processes verbatim, so an inherited
  `FOO='{{ bad'` neither fails the step nor gets evaluated. A declared `env` value written with
  `!literal` is not rendered either.
- A template error names the step and field (`step "fmt" field script`) and, for a script read
  through `!include`, the source file. A hint points at `!literal` when the body contains `{{`.
- Do not template user input into script source. Read inputs through `ctx.flags`,
  `ctx.arguments`, and `env`: they are plain values, so quotes and braces in them cannot break
  the script or inject code.
- Standalone scripts (`atmos ./x.star`) are not templated.

## Inputs by context

| Context | `ctx.flags` | `ctx.arguments` | `ctx.args` | `ctx.component` |
|---|---|---|---|---|
| Custom command | Typed: string, bool, and `type: int` flags as integers | Named command arguments (strings); an omitted optional argument without a default is `""` | Empty; trailing args after `--` are not exposed | The command's `component:` (lazy), else `None` |
| Workflow | String map that always has `stack` (for example `{"stack": ""}`) | Empty | Empty | `None`; use `components.get(name, stack, type)` |
| Hook | Not part of the hook contract | Empty | Empty | The hook's component, plus `ctx.hook` and `ctx.operation` |
| Git hook (`git.hooks.<name>.steps`) | Not part of the hook contract | Empty | The arguments Git passes to the hook, such as the commit message path for `commit-msg` | `None` |
| `parallel`/`matrix` child | Inherited from parent | Inherited | Empty | Inherited, with `components.get` |

`env` holds only the step's declared `env` (resolved), not the ambient process environment. In a
custom command it holds only the step's own `env:` entries, not command-level `env:` entries.
`ctx.script` is set for standalone scripts and for script steps included through a local `!include`; it is `None` for inline step scripts. Child
processes started with `exec.run`, `component.exec`, or `atmos.*` still inherit the effective
process environment, plus per-call `env={...}` overrides.

A workflow has no component in scope: `ctx.component.stack` fails because `ctx.component` is `None`.
Resolve one explicitly with `components.get("vpc", "dev", "terraform")`.

## Output and labels

- `print` writes data to stdout. `ui.info`, `ui.success`, and `ui.warning` write to stderr.
  `log.*` goes through the Atmos logger with `step` (and `task`) fields.
- Script steps default to raw output with no `[step]` or `✓ step completed` labels, the same as
  shell steps. `show: {labels: true}` on the step restores the labels.
- The step `output:` field selects the display mode: `raw`, `log`, `viewport`, or `none`. Any
  other value, such as `output: capture`, fails validation before the step runs and the error
  lists the valid modes. The same check applies to `shell`, `atmos`, and `container` steps and to
  the workflow-level `output:`. (Subprocess capture is `exec.run(..., output="capture")` inside
  the script.) Container steps now default to raw output without labels, like the other command
  steps.
- A top-level `output = value` in the script becomes the step value for later steps:
  strings as-is, other values JSON-encoded. Without it, the captured stdout is the step value.
  `{{ .steps.<name>.value }}` renders a dict or list as JSON text (`{"n":3}`), never as Go map
  syntax. JSON text contains double quotes, so put it in single quotes in a shell command
  (`echo '{{ .steps.first.value }}'`) or pass it through `env` and `json.decode` it.
  An `output` that cannot be encoded fails with ``output` must be a string or JSON-encodable
  value: <reason>``.

## Timeouts and containers

- A `timeout:` on the script step is enforced: the interpreter runs under a deadline, the
  Starlark thread and its subprocesses are canceled when it elapses, and the step fails with
  `step timed out`. An invalid value (not a positive duration) fails with `invalid step
  timeout`. `shell` and `atmos` steps enforce `timeout:` the same way in workflows and custom
  commands. Per-task limits inside the script still use `steps.task(name, fn, timeout="30s")`
  and `steps.parallel`.
- Embedded Starlark runs inside Atmos, so a step under an enabled workflow or step `container`
  fails validation before the workflow starts. Set `container: false` on that step.
- `atmos workflow --dry-run` parses top-level script steps (reporting syntax errors) and runs
  no code or processes. Script steps inside `parallel`/`matrix` run nothing either.

## Files and paths

Use `script: !include scripts/deploy.star` for external source, or include a YAML step
definition through the existing include mechanism. `load()` and `fs.read_file` accept relative
and absolute paths.

- `!include` in a workflow manifest or custom command: `./x` and `../x` resolve against the
  file that contains the tag, bare `x/y` against the project `base_path`, absolute paths as
  written; never against the directory Atmos runs from.
- `load()` in a script read from a file (`!include`, `!include.raw`): relative to that file's
  directory, bare or `./` or `../`, also inside loaded modules. Tracebacks name that file and
  line.
- In a stack-manifest hook (`kind: step`/`steps`), a local `!include`/`!include.raw` `script`
  (no YQ expression, never remote) next to an `interpreter` gets an Atmos-recorded
  `script_source` sibling key, so hook `load()` also resolves next to the included file.
  `script_source` is provenance, visible in `describe component` and `describe stacks` output;
  never write it by hand. A sibling `script_source_sha256` validates it: if a child stack
  overrides only `script` (for example with an inline body), the inherited `script_source` no
  longer matches and is ignored, so no manual cleanup is needed.
- `load()` in an inline `script: |` body: relative to `working_directory`.
- `fs.read_file` and `exec.run`: relative to `working_directory`, which must exist.

Imports are local, cached per invocation, and reject cycles. Imported functions work in tasks
just like inline functions.

## Retries and errors

A step-level `retry:` re-runs the whole script. Make side effects repeatable, or put retries
around a narrower function with `steps.task(..., retry={...})`.

Script errors are single-line messages followed by the Starlark traceback. A subprocess
failure includes the last lines of its stderr. Runaway recursion stops at a depth limit with
a "recursion depth exceeded" error and a collapsed traceback.
