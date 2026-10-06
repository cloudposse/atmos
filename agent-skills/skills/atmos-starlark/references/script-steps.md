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
  Escape sequences such as `{{"{{"}}` do not survive, because the rendered
  result is rendered again (workflows and custom commands run more than one render pass).
  Avoid the sequence instead, for example by splitting it: `"{" + "{ value }}"`.
- Sprig functions (`{{ upper "abc" }}`) work in sequential steps but not in `parallel`/`matrix`
  child steps.
- Do not template user input into script source. Read inputs through `ctx.flags`,
  `ctx.arguments`, and `env`: they are plain values, so quotes and braces in them cannot break
  the script or inject code.
- Standalone scripts (`atmos ./x.star`) are not templated.

## Inputs by context

| Context | `ctx.flags` | `ctx.arguments` | `ctx.args` | `ctx.component` |
|---|---|---|---|---|
| Custom command | String and bool types preserved | Named command arguments | Empty; trailing args after `--` are not exposed | The command's `component:` (lazy), else `None` |
| Workflow | String map (for example `{"stack": ""}`) | Empty | Empty | `None`; use `components.get(name, stack, type)` |
| Hook | Not part of the hook contract | Empty | Empty | The hook's component, plus `ctx.hook` and `ctx.operation` |
| `parallel`/`matrix` child | Inherited from parent | Inherited | Empty | Inherited, with `components.get` |

`env` holds only the step's declared `env` (resolved), not the ambient process environment.
`ctx.script` is `None` in script steps; it exists only for standalone scripts. Child
processes started with `exec.run`, `component.exec`, or `atmos.*` still inherit the effective
process environment, plus per-call `env={...}` overrides.

A workflow has no component in scope: `ctx.component.stack` fails because `ctx.component` is `None`.
Resolve one explicitly with `components.get("vpc", "dev", "terraform")`.

## Output and labels

- `print` writes data to stdout. `ui.info`, `ui.success`, and `ui.warning` write to stderr.
  `log.*` goes through the Atmos logger with `step` (and `task`) fields.
- Script steps default to raw output with no `[step]` or `✓ step completed` labels, the same as
  shell steps. `show: {labels: true}` on the step restores the labels.
- The step `output:` field selects the display mode: `raw`, `log`, `viewport`, or `none`.
  `output: capture` is not a valid mode. (Subprocess capture is `exec.run(..., output="capture")`
  inside the script.)
- A top-level `output = value` in the script becomes the step value for later steps:
  strings as-is, other values JSON-encoded. Without it, the captured stdout is the step value.

## Timeouts and containers

- A `timeout:` key on the script step is not enforced today. Put time limits inside the script
  with `steps.task(name, fn, timeout="30s")` and `steps.parallel`.
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
