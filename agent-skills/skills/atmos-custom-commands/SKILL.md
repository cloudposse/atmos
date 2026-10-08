---
name: atmos-custom-commands
description: "Custom CLI commands: command definition in atmos.yaml, arguments, flags, shared native step types, when: conditions, output/UI steps, env vars, custom component types"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
  category: ci-automation
references:
  - references/command-syntax.md
---

# Atmos Custom Commands

Use this skill when users define or modify project-specific Atmos CLI commands in the top-level
`commands` section of `atmos.yaml`.

Custom commands replace scattered scripts with discoverable CLI commands that can use arguments,
flags, environment variables, authentication identities, tool dependencies, component config, nested
subcommands, and typed execution steps.

When a task is mainly about `steps`, step `type`, `working_directory`, step `env`, `output`,
scripts, workdirs, or hook-compatible step payloads, also load `atmos-steps`.

## Quick Shape

```yaml
commands:
  - name: hello
    description: Say hello
    arguments:
      - name: name
        default: World
    steps:
      - type: say
        text: "Hello {{ .Arguments.name }}"
```

```shell
atmos hello
atmos hello Erik
```

For full field syntax, read [references/command-syntax.md](references/command-syntax.md).

## Agent Workflow

1. Inspect existing `commands` in `atmos.yaml` and imported config before adding a new command.
2. Prefer native typed steps over large shell blocks.
3. Add flags and arguments with clear names and defaults.
4. Use `dependencies.tools` for tools the command needs in its execution context.
5. Use `identity` when the command needs Atmos Auth credentials.
6. Verify with `atmos <command> --help` and a dry-run or read-only invocation when possible.

## Native Step Preference

Default to structured steps before shell:

| Need | Prefer |
|---|---|
| Run an Atmos command | `type: atmos` |
| Operator messages | `say`, `toast`, `markdown`, `table`, `pager` |
| Data shaping | `format`, `join`, `filter`, `write` |
| Status/progress | `spin`, `stage`, `log`, `linebreak` |
| Concurrency | `parallel`, `matrix`, `wait`, `wait-all` |
| Containers/emulators | `container`, `emulator` |
| HTTP calls | `http` |
| Preconditions | `require` / `assert` |
| External command glue | `shell` / `exec` |

Shell is still appropriate for short glue commands, terminal-native tools, checked-in scripts, or
commands that genuinely need shell semantics.

## Common Patterns

### Flags and Arguments

```yaml
commands:
  - name: deploy-one
    arguments:
      - name: component
        required: true
    flags:
      - name: stack
        shorthand: s
        required: true
    steps:
      - type: atmos
        command: terraform deploy {{ .Arguments.component }} -s {{ .Flags.stack }}
```

Flag `type` is `string` (default), `bool`, or `int`. An `int` flag registers as an integer flag
(`--count int` in help), its `default` must be a whole number, and it reaches templates and
`ctx.flags` as an integer. Any other type fails when Atmos loads the command, with an error that
lists the supported types. An `int` flag reads base-10 only (`010` is 10, `0x10` is rejected). A `default` that does
not match the flag type makes the command a stub with a warning that names the flag. An argument with neither `required:` nor `default:` is required; set `required: false` to make it optional, and an
omitted optional argument with no `default` is an empty string in `{{ .Arguments.<name> }}` and `ctx.arguments`. Argument values keep
commas, empty strings, and non-ASCII text intact.

### Tool Dependencies

```yaml
commands:
  - name: scan
    dependencies:
      tools:
        checkov: "latest"
    steps:
      - type: shell
        command: checkov --directory .
```

### Authentication

```yaml
commands:
  - name: prod-whoami
    dependencies:
      tools:
        aws-cli: "^2.0.0"
    identity: prod-readonly
    steps:
      - type: shell
        command: aws sts get-caller-identity
```

### Custom Component Types

Use `component.type` and semantic `provides` bindings when a custom command should
resolve a custom component and stack. `component_config` is legacy Terraform syntax.

```yaml
commands:
  - name: render-app
    arguments:
      - name: component
        provides: component
        required: true
    flags:
      - name: stack
        shorthand: s
        provides: stack
        required: true
    component:
      type: application
    steps:
      - type: shell
        command: ./scripts/render-app.sh
```

An embedded Starlark script step in such a command reads the selected component as
`ctx.component` (resolved on first access). Workflows have no component in scope, so
`ctx.component` is `None` there; see `atmos-starlark`.

## Starlark Script Steps

Use a `type: script` step with `interpreter: starlark` when a command needs loops,
conditionals, data shaping, or concurrency that native steps cannot express. Inputs
reach the script as values, not as text:

```yaml
commands:
  - name: deploy-all
    description: Deploy components in parallel
    arguments:
      - name: group
        required: true
    flags:
      - name: dry-run
        type: bool
      - name: stack
        shorthand: s
        required: true
    steps:
      - name: deploy
        type: script
        interpreter: starlark
        script: |
          if ctx.flags["dry-run"]:
              ui.info("dry run for " + ctx.arguments["group"])
          else:
              atmos.terraform("deploy", ctx.arguments["group"], ctx.flags["stack"])
```

- Read declared flags and arguments from `ctx.flags` (string, bool, and `int` types preserved)
  and `ctx.arguments` (an omitted optional argument is `""`). `ctx.args` is empty, and trailing
  arguments after `--` are not exposed to scripts.
- Do not template flag values into the script source (`{{ .Flags.x }}`): the script body is
  rendered as a Go template first, so a literal `{{` in Starlark source breaks the step.
  `ctx.flags` and `ctx.arguments` are safe for any value.
- `cli.command`, `cli.arg`, and `cli.flag` are only for standalone scripts
  (`atmos ./tool.star`); declare the command interface in YAML here.
- Script steps default to raw output with no step labels; `show: {labels: true}` restores them.
  `output:` must be `raw`, `log`, `viewport`, or `none`; any other value (such as `capture`) fails
  before the step runs (`capture`/`stream` errors point to `exec.run`, which takes `stream|capture|viewport`).
- A `timeout:` on a script, shell, or atmos step is enforced: the step is canceled, its whole process tree ends, and it
  fails with `step timed out` (`Step '<name>' (type <t>) did not finish within its timeout of <d>`). Per-task limits inside a script still use `steps.task(..., timeout="30s")`.
- Atmos renders only the values declared under the step's `env:` as templates. The ambient process
  environment reaches the script and its child processes verbatim, so an inherited
  `FOO='{{ bad'` is harmless. In `ctx`-side Starlark, `env` holds only the step's own `env:`
  entries, not command-level `env:` entries.
- Sprig and Gomplate functions work the same in sequential steps and in `parallel`/`matrix`
  children.
- A script step's dict or list `output` reaches later steps as JSON text
  (`{{ .steps.<name>.value }}` is `{"n":3}`), never Go map syntax. Single-quote it in shell
  commands (`echo '{{ .steps.x.value }}'`) because JSON contains double quotes.
- A template error names the step and field, and the source file for an `!include`d script, with
  a hint to tag the field `!literal` when the body contains `{{` (for an `!include`d script, the hint says to move the
  text into a `load()`ed module or use `!include.raw`, which is used as written). `!literal` also works on `command`,
  `interpreter`, `working_directory`, `timeout`, and single `env` values, including command-level `env`; any other step
  field fails config load. `retry.conditions` works for shell and atmos steps too.
- `--profile` and `--identity` reach nested `atmos.*` calls from script steps. Ctrl-C runs `defer` calls (30 s grace)
  and exits 130 silently.

## Routing

| Need | Skill |
|---|---|
| Complete command schema and examples | [references/command-syntax.md](references/command-syntax.md) |
| Reusable multi-step orchestration | `atmos-workflows` |
| Shared step fields and step types | `atmos-steps` |
| Embedded Starlark, parallel function calls, and standalone `atmos ./tool.star` CLI apps | `atmos-starlark` |
| Smoke tests, integration tests, and test groups | `atmos-tests` |
| Tool versions and PATH behavior | `atmos-toolchain` |
| Auth providers, identities, assume role/root, OIDC | `atmos-auth` |
| Components and component inheritance | `atmos-components` |
| Go templates and YAML functions | `atmos-templates`, `atmos-yaml-functions` |

## Guardrails

- Do not override built-in commands unless the user explicitly wants that behavior.
- Do not hide complex business logic inside inline YAML shell blocks. Move it to a checked-in script
  or use native step types.
- Do not use custom commands for long-lived reusable orchestration when an Atmos workflow is a
  better fit.
- Keep command names stable; they become user-facing CLI API.
