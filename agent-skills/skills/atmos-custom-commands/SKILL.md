---
name: atmos-custom-commands
description: "Custom CLI commands: command definition in atmos.yaml, arguments, flags, native step types, when: conditions, output/UI steps, env vars, custom component types"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
references:
  - references/command-syntax.md
---

# Atmos Custom Commands

Use this skill when users define or modify project-specific Atmos CLI commands in the top-level
`commands` section of `atmos.yaml`.

Custom commands replace scattered scripts with discoverable CLI commands that can use arguments,
flags, environment variables, authentication identities, tool dependencies, component config, nested
subcommands, and typed execution steps.

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
    identity: prod-readonly
    steps:
      - type: shell
        command: aws sts get-caller-identity
```

### Custom Component Types

Use `component_config` when a custom command should resolve a component and stack before running:

```yaml
commands:
  - name: render-app
    arguments:
      - name: component
        required: true
    flags:
      - name: stack
        shorthand: s
        required: true
    component_config:
      component: "{{ .Arguments.component }}"
      stack: "{{ .Flags.stack }}"
    steps:
      - type: shell
        command: ./scripts/render-app.sh
```

## Routing

| Need | Skill |
|---|---|
| Complete command schema and examples | [references/command-syntax.md](references/command-syntax.md) |
| Reusable multi-step orchestration | `atmos-workflows` |
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
