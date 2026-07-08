---
name: atmos-custom-commands
description: "Custom CLI commands: command definition in atmos.yaml, arguments, flags, native step types, when: conditions, output/UI steps, env vars, custom component types"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Custom Commands

Atmos custom commands extend the CLI with project-specific commands defined in `atmos.yaml`. They
appear in `atmos help` output alongside built-in commands, providing a unified interface for all
operational tooling in a project. Custom commands replace scattered bash scripts with a consistent,
discoverable CLI.

## What Custom Commands Are

Custom commands are user-defined CLI commands configured in the `commands` section of `atmos.yaml`.
Each command can have:

- A name and description
- Positional arguments and flags (with shorthand, required/optional, defaults)
- Boolean flags
- Environment variables (supporting Go templates)
- One or more execution steps using the same native step types as workflows
- Nested subcommands
- Access to resolved component configuration via `component_config`
- Authentication via `identity`
- Tool dependencies
- Working directory control

Custom commands can call Atmos built-in commands, shell scripts, workflows, or any other CLI tools.
They are fully interoperable with Atmos workflows.

Default rule: use structured native step types before inline shell scripts. A custom command with
many `echo` lines, a large multiline shell block, ad hoc prompts, hand-formatted tables, or shell
loops over components is probably doing too much in shell. Prefer `atmos`, `toast`, `table`, `pager`,
`format`, `spin`, `parallel`, `matrix`, `container`, `emulator`, `http`, and other typed steps when
they express the intent directly. Shell is still appropriate for short glue commands, external CLIs,
terminal-native sessions, or invoking a real script checked into the repo.

## Defining Commands in atmos.yaml

Commands are defined under the top-level `commands` key in `atmos.yaml`:

```yaml
# atmos.yaml
commands:
  - name: hello
    description: This command says Hello world
    steps:
      - "echo Hello world!"
```

Run it with:

```shell
atmos hello
```

## Command Structure

### Basic Command

```yaml
commands:
  - name: greet
    description: Greet a user by name
    arguments:
      - name: name
        description: Name to greet
        required: true
        default: "World"
    steps:
      - "echo Hello {{ .Arguments.name }}!"
```

```shell
atmos greet Alice        # Hello Alice!
atmos greet              # Hello World! (uses default)
```

### Command with Flags

```yaml
commands:
  - name: hello
    description: Say hello with flags
    flags:
      - name: name
        shorthand: n
        description: Name to greet
        required: true
    steps:
      - "echo Hello {{ .Flags.name }}!"
```

```shell
atmos hello --name world
atmos hello -n world
```

### Boolean Flags

```yaml
commands:
  - name: deploy
    description: Deploy with options
    flags:
      - name: dry-run
        shorthand: d
        description: Perform a dry run
        type: bool
      - name: verbose
        shorthand: v
        description: Enable verbose output
        type: bool
        default: false
      - name: auto-approve
        description: Auto-approve without prompting
        type: bool
        default: true
    steps:
      - |
        {{ if .Flags.dry-run }}
        echo "DRY RUN MODE"
        {{ end }}
        {{ if .Flags.verbose }}
        echo "Verbose output enabled"
        {{ end }}
        {{ if .Flags.auto-approve }}
        terraform apply -auto-approve
        {{ else }}
        terraform apply
        {{ end }}
```

```shell
atmos deploy --dry-run
atmos deploy -d
atmos deploy --auto-approve=false
```

Boolean flags render as `true` or `false` (lowercase strings) in templates.

### Flag Defaults

Both string and boolean flags support default values:

```yaml
flags:
  - name: environment
    description: Target environment
    default: "development"
  - name: force
    type: bool
    description: Force the operation
    default: false
```

When a flag has a default, users can omit it from the command line.

### Trailing Arguments

Arguments after `--` are accessible via `{{ .TrailingArgs }}`:

```yaml
commands:
  - name: ansible run
    description: Run an Ansible playbook
    arguments:
      - name: playbook
        description: Playbook to run
        default: site.yml
        required: true
    steps:
      - "ansible-playbook {{ .Arguments.playbook }} {{ .TrailingArgs }}"
```

```shell
atmos ansible run -- --limit web
# Runs: ansible-playbook site.yml --limit web
```

## Nested Subcommands

Commands can contain nested subcommands for hierarchical command structures:

```yaml
commands:
  - name: terraform
    description: Execute terraform commands
    commands:
      - name: provision
        description: Provision terraform components
        arguments:
          - name: component
            description: Component name
        flags:
          - name: stack
            shorthand: s
            description: Stack name
            required: true
        env:
          - key: ATMOS_COMPONENT
            value: "{{ .Arguments.component }}"
          - key: ATMOS_STACK
            value: "{{ .Flags.stack }}"
        steps:
          - atmos terraform plan $ATMOS_COMPONENT -s $ATMOS_STACK
          - atmos terraform apply $ATMOS_COMPONENT -s $ATMOS_STACK
```

```shell
atmos terraform provision vpc -s plat-ue2-dev
```

### Overriding Existing Commands

You can override built-in commands by matching their name:

```yaml
commands:
  - name: terraform
    description: Execute terraform commands
    commands:
      - name: apply
        description: Apply with auto-approve
        arguments:
          - name: component
            description: Component name
        flags:
          - name: stack
            shorthand: s
            description: Stack name
            required: true
        steps:
          - atmos terraform apply {{ .Arguments.component }} -s {{ .Flags.stack }} -auto-approve
```

## Environment Variables

The `env` section sets environment variables accessible in steps. Values support Go templates:

```yaml
commands:
  - name: deploy
    env:
      - key: ATMOS_COMPONENT
        value: "{{ .Arguments.component }}"
      - key: ATMOS_STACK
        value: "{{ .Flags.stack }}"
    steps:
      - atmos terraform plan $ATMOS_COMPONENT -s $ATMOS_STACK
```

## Component Configuration Access

The `component_config` section resolves the full configuration for a component in a stack,
making it available via `{{ .ComponentConfig.xxx }}` in templates:

```yaml
commands:
  - name: show-backend
    component_config:
      component: "{{ .Arguments.component }}"
      stack: "{{ .Flags.stack }}"
    steps:
      - 'echo "Backend: {{ .ComponentConfig.backend.bucket }}"'
      - 'echo "Workspace: {{ .ComponentConfig.workspace }}"'
```

Available fields: `.ComponentConfig.component`, `.ComponentConfig.backend`, `.ComponentConfig.workspace`,
`.ComponentConfig.vars`, `.ComponentConfig.settings`, `.ComponentConfig.env`, `.ComponentConfig.deps`,
`.ComponentConfig.metadata`. For the complete field reference, see
[references/command-syntax.md](references/command-syntax.md).

## Custom Component Types

The top-level `component` key is a distinct, narrower, more advanced mechanism from
`component_config` above: instead of resolving an already-known component/stack pair,
`component: {type, base_path}` registers a brand-new Atmos component *type* on the fly, backed by
a generic provider, and exposes the resolved config as `{{ .Component.* }}`:

```yaml
commands:
  - name: run-script
    description: Execute a custom "script"-type component
    component:
      type: script
      base_path: components/script    # optional; defaults to "components/<type>"
    arguments:
      - name: component
        description: Script component name
        type: component                # semantic type: resolves the component argument
        required: true
    flags:
      - name: stack
        shorthand: s
        description: Stack name
        semantic_type: stack           # semantic type: resolves the stack flag
        required: true
    steps:
      - "echo Running {{ .Component.component }} in stack {{ .Flags.stack }}"
```

When this command runs, Atmos:

1. Finds the argument tagged `type: component` (or flag tagged `semantic_type: component`) and the
   argument tagged `type: stack` (or flag tagged `semantic_type: stack`) to get the component and
   stack names (`pkg/schema/command.go`'s `CommandArgument.Type` / `CommandFlag.SemanticType`).
2. Registers a `component.ComponentProvider` for the declared `type` on demand -- idempotent, safe
   across repeated invocations -- via `pkg/component/custom.EnsureRegistered`, using `base_path`
   (or the `components/<type>` default) as the type's base directory.
3. Resolves the component's full configuration in that stack and exposes it as `{{ .Component.* }}`
   in templates -- note this is `.Component`, not `.ComponentConfig` (the separate,
   pre-existing mechanism documented above).

Because step 2 registers into Atmos's shared component provider registry -- the same registry
built-in `terraform`/`helmfile`/`packer` types use -- this unlocks generic tooling for the new
type for free: `atmos list components --component-types=script` and `atmos describe component`
recognize `script` components declared under `components.script.<name>` in stacks, the same way
they recognize built-in types.

**`component:` vs `component_config:`**: `component_config` only resolves a *known* existing
component/stack pair into `{{ .ComponentConfig.* }}` -- it never creates a new component type.
`component: {type, base_path}` defines an entirely new component type, dynamically, from within a
single command definition. This is a narrower, more advanced feature than `component_config`, and
there is no dedicated website docs page for it yet -- this skill is currently the most complete
reference for it.

## Go Templates in Steps

Steps support Go template syntax. Access arguments with `{{ .Arguments.name }}`, flags with
`{{ .Flags.stack }}`, and component config with `{{ .ComponentConfig.backend.bucket }}`.

```yaml
steps:
  - "echo Hello {{ .Arguments.name }}"
  - >
    {{ if .Flags.stack }}
    atmos describe stacks --stack {{ .Flags.stack }} --format json
    {{ else }}
    atmos describe stacks --format json
    {{ end }}
```

Supports `if`/`else`, `not`, `eq`, and boolean-to-shell conversion. For complete template
examples, see [references/command-syntax.md](references/command-syntax.md).

## Conditional Steps

Custom command steps support `when:` conditions too, through the same step engine and syntax as
workflow steps. Omitting `when` always runs the step; a false condition skips it without failing
the command:

```yaml
commands:
  - name: release
    description: Run release checks
    steps:
      - type: shell
        command: ./scripts/ci-release-checks.sh
        when: ci
      - type: shell
        command: ./scripts/prod-only.sh
        when: !cel 'stack == "prod" && ci'
```

See [atmos-workflows](../atmos-workflows/SKILL.md#conditional-execution-with-when) for the full
`when:`/CEL syntax reference.

## Authentication

Custom commands can specify an `identity` for authentication:

```yaml
commands:
  - name: deploy-infra
    description: Deploy infrastructure with admin privileges
    identity: superadmin
    arguments:
      - name: component
        description: Component to deploy
        required: true
    flags:
      - name: stack
        shorthand: s
        description: Stack to deploy to
        required: true
    steps:
      - atmos terraform plan {{ .Arguments.component }} -s {{ .Flags.stack }}
      - atmos terraform apply {{ .Arguments.component }} -s {{ .Flags.stack }} -auto-approve
```

The `identity` applies to all steps. Override at runtime:

```shell
# Use command-defined identity
atmos deploy-infra vpc -s plat-ue2-prod

# Override with different identity
atmos deploy-infra vpc -s plat-ue2-prod --identity developer

# Skip authentication
atmos deploy-infra vpc -s plat-ue2-prod --identity ""
```

The `--identity` flag is automatically added to all custom commands.

## Verbose Output

Control whether step commands are printed before execution:

```yaml
commands:
  - name: set-eks-cluster
    description: Set EKS cluster context
    verbose: false          # Don't print commands (default is true)
    steps:
      - aws eks update-kubeconfig ...
```

## Working Directory

Control where steps execute:

```yaml
commands:
  - name: build
    description: Build from repository root
    working_directory: !repo-root .
    steps:
      - make build
      - make test
```

Path resolution:
- Absolute paths used as-is
- Relative paths resolved against `base_path`
- `!repo-root .` resolves to git repository root

## Tool Dependencies

Declare tools that must be available before execution:

```yaml
commands:
  - name: lint
    description: Run tflint on components
    dependencies:
      tools:
        tflint: "0.54.0"
    arguments:
      - name: component
        description: Component to lint
        required: true
    flags:
      - name: stack
        shorthand: s
        description: Stack name
        required: true
    steps:
      - atmos terraform generate varfile {{ .Arguments.component }} -s {{ .Flags.stack }}
      - tflint --chdir=components/terraform/{{ .Arguments.component }}
```

When you run the command, Atmos:
1. Checks if the tool is installed at the required version
2. Installs it from the toolchain registry if missing
3. Updates PATH to include the tool
4. Executes the steps

Do not add a separate workflow or CI step that runs `atmos toolchain install <tool>` for tools the
custom command itself invokes. Put utilities such as `bat`, `tree`, `jq`, `tflint`, or `checkov`
under the command's `dependencies.tools` so the command is self-contained.

Multiple tools and SemVer constraints are supported:

```yaml
dependencies:
  tools:
    tflint: "0.54.0"          # Exact version
    checkov: "3.0.0"          # Exact version
    kubectl: "latest"         # Latest available
    terraform: "^1.10.0"      # Compatible range
```

## Common Patterns

Common custom command patterns include: listing stacks/components, setting EKS cluster context,
security scanning, cost estimation, and documentation generation. Prefer typed output/UI steps for
operator-facing messages instead of chains of `echo` commands. For complete examples, see
[references/command-syntax.md](references/command-syntax.md).

### Quick Example: List Stacks

```yaml
commands:
  - name: list
    description: List stacks and components
    commands:
      - name: stacks
        description: List all Atmos stacks
        steps:
          - >
            atmos describe stacks --process-templates=false --sections none | grep -e "^\S" | sed s/://g
```

### Quick Example: Security Scan with Dependencies

```yaml
commands:
  - name: security-scan
    description: Run security scans on infrastructure code
    dependencies:
      tools:
        tflint: "0.54.0"
        checkov: "3.0.0"
    steps:
      - tflint --chdir=components/terraform
      - checkov -d components/terraform
```

## Best Practices

1. **Provide descriptive names and descriptions.** Commands show in `atmos help`, so clear
   descriptions help team members discover and understand available tooling.

2. **Use Go templates for conditional logic.** Rather than writing separate commands, use
   template conditionals to handle optional flags.

3. **Set sensible defaults.** Use the `default` attribute on arguments and flags so common
   cases require minimal input.

4. **Use component_config for context-aware commands.** When a command needs to know about
   a component's resolved configuration (backend, vars, workspace), use `component_config`
   instead of hardcoding values.

5. **Use `verbose: false` for noisy commands.** Suppress command echo for commands that produce
   a lot of output or contain sensitive information.

6. **Leverage tool dependencies.** Instead of documenting prerequisites or adding preinstall
   steps, declare every command-owned CLI in `dependencies.tools` so it is auto-installed.

7. **Prefer native step types over shell-heavy commands.** Use structured steps for Atmos commands,
   output, prompts, tables, containers, matrices, and CI-aware behavior. Keep shell for short glue or
   external CLIs.

8. **Organize with nested subcommands.** Use nested `commands` for related operations
   (e.g., `atmos list stacks`, `atmos list components`).

9. **Combine with workflows.** Use custom commands for atomic operations and workflows for
   multi-step orchestration. They can call each other.

10. **Use `!repo-root .` for working_directory** when commands need to run from the repository
   root regardless of where `atmos` is invoked.

11. **Use environment variables for shared values.** Define values once in `env` and reference
    them in multiple steps via shell variables like `$ATMOS_COMPONENT`.

## Additional Resources

- For the complete custom command YAML syntax reference, see [references/command-syntax.md](references/command-syntax.md)
