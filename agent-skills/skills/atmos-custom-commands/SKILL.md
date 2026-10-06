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

- Read declared flags and arguments from `ctx.flags` (string and bool types preserved) and
  `ctx.arguments`. `ctx.args` is empty, and trailing arguments after `--` are not exposed to
  scripts.
- Do not template flag values into the script source (`{{ .Flags.x }}`): the script body is
  rendered as a Go template first, so a literal `{{` in Starlark source breaks the step.
  `ctx.flags` and `ctx.arguments` are safe for any value.
- `cli.command`, `cli.arg`, and `cli.flag` are only for standalone scripts
  (`atmos ./tool.star`); declare the command interface in YAML here.
- Script steps default to raw output with no step labels; `show: {labels: true}` restores them.
  A `timeout:` on the script step is not enforced; use `steps.task(..., timeout="30s")`.

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
