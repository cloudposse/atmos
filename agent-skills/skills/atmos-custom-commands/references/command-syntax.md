# Atmos Custom Command YAML Syntax Reference

Complete reference for all fields in the `commands` section of `atmos.yaml`.

## Top-Level Structure

Custom commands are defined as a list under the `commands` key in `atmos.yaml`:

```yaml
# atmos.yaml
commands:
  - name: command-1
    description: "First command"
    steps:
      - "echo hello"

  - name: command-2
    description: "Second command"
    steps:
      - "echo world"
```

## Command Definition Fields

```yaml
commands:
  - name: string                     # Required: Command name
    description: string              # Optional: Help text description
    verbose: boolean                 # Optional: Print commands before execution (default: true)
    identity: string                 # Optional: Authentication identity name
    working_directory: string        # Optional: Working directory for steps
    arguments: []                    # Optional: Positional arguments
    flags: []                        # Optional: Named flags
    env: []                          # Optional: Environment variables
    component_config:                # Optional: Component configuration access
      component: string
      stack: string
    dependencies:                    # Optional: Tool dependencies
      tools: {}
    commands: []                     # Optional: Nested subcommands
    steps: []                        # Required (unless has subcommands): Execution steps
```

### name (Required)

The command name as used on the command line. Must be unique among sibling commands.

```yaml
- name: hello
# Usage: atmos hello

- name: set-eks-cluster
# Usage: atmos set-eks-cluster
```

For nested subcommands, the name forms the command path:

```yaml
- name: terraform
  commands:
    - name: provision
# Usage: atmos terraform provision
```

Multi-word names are also supported:

```yaml
- name: ansible run
# Usage: atmos ansible run
```

### description (Optional)

Human-readable description shown in `atmos help` and `atmos <command> --help`.
Supports multiline YAML strings.

```yaml
- name: deploy
  description: |
    Deploy infrastructure components.

    Example usage:
      atmos deploy vpc -s plat-ue2-dev
      atmos deploy eks/cluster -s plat-ue2-prod
```

### verbose (Optional)

Controls whether step commands are printed before execution. Default: `true`.

```yaml
- name: set-eks-cluster
  verbose: false    # Suppress command echo
```

### identity (Optional)

Authentication identity to use before executing steps. Atmos authenticates and sets up
credential environment variables for all steps.

```yaml
- name: deploy-infra
  identity: superadmin
```

Override at runtime with `--identity <name>` or skip with `--identity ""`.
The `--identity` flag is automatically added to all custom commands.

### working_directory (Optional)

Working directory where steps execute.

```yaml
- name: build
  working_directory: !repo-root .    # Git repository root
  # Or:
  working_directory: /tmp            # Absolute path
  working_directory: scripts/build   # Relative to base_path
```

Path resolution rules:
- Absolute paths are used as-is
- Relative paths resolve against the Atmos `base_path`
- `!repo-root` or `!repo-root .` resolves to the git repository root

## Arguments

Positional arguments are defined as a list under `arguments`:

```yaml
arguments:
  - name: string           # Required: Argument name
    description: string    # Optional: Help text
    required: boolean      # Optional: Whether argument is required (default: false)
    default: string        # Optional: Default value if not provided
```

### Access in Templates

Arguments are accessed via `{{ .Arguments.<name> }}`:

```yaml
arguments:
  - name: component
    description: Component name
    required: true
steps:
  - "echo Component: {{ .Arguments.component }}"
```

### Required vs Optional

If `required: true` and no value is provided, the command fails unless a `default` is specified.

```yaml
arguments:
  - name: name
    description: Name to greet
    required: true
    default: "John Doe"    # Used when argument is omitted
```

### Multiple Arguments

Arguments are positional and matched in order:

```yaml
arguments:
  - name: component
    description: Component name
    required: true
  - name: environment
    description: Target environment
    default: dev
```

```shell
atmos mycommand vpc staging    # component=vpc, environment=staging
atmos mycommand vpc            # component=vpc, environment=dev (default)
```

## Flags

Named flags are defined as a list under `flags`:

```yaml
flags:
  - name: string           # Required: Flag name (used as --name)
    shorthand: string      # Optional: Single-character shorthand (used as -x)
    description: string    # Optional: Help text
    required: boolean      # Optional: Whether flag is required (default: false)
    type: string           # Optional: "bool" for boolean flags (default: string)
    default: string|bool   # Optional: Default value
```

### Access in Templates

Flags are accessed via `{{ .Flags.<name> }}`:

```yaml
flags:
  - name: stack
    shorthand: s
    description: Stack name
    required: true
  - name: verbose
    type: bool
    default: false
steps:
  - "echo Stack: {{ .Flags.stack }}"
  - |
    {{ if .Flags.verbose }}
    echo "Verbose mode"
    {{ end }}
```

### String Flags

```yaml
flags:
  - name: environment
    shorthand: e
    description: Target environment
    required: true
  - name: region
    shorthand: r
    description: AWS region
    default: us-east-1
```

```shell
atmos mycommand --environment prod --region us-west-2
atmos mycommand -e prod -r us-west-2
```

### Boolean Flags

Boolean flags are declared with `type: bool`. When present on the command line without a value,
they are set to `true`. They render as lowercase `true` or `false` in templates.

```yaml
flags:
  - name: dry-run
    shorthand: d
    type: bool
    description: Dry run mode
  - name: force
    type: bool
    default: false
    description: Force operation
  - name: auto-approve
    type: bool
    default: true
    description: Auto-approve changes
```

```shell
atmos mycommand --dry-run              # dry-run=true
atmos mycommand -d                     # dry-run=true
atmos mycommand --auto-approve=false   # auto-approve=false (explicit override)
```

### Flag Defaults

Both string and boolean flags support defaults:

```yaml
flags:
  - name: environment
    default: "development"           # String default
  - name: force
    type: bool
    default: false                   # Boolean default
  - name: auto-approve
    type: bool
    default: true                    # Boolean default (true)
```

## Trailing Arguments

Arguments after `--` on the command line are accessible via `{{ .TrailingArgs }}`:

```yaml
- name: ansible run
  arguments:
    - name: playbook
      default: site.yml
      required: true
  steps:
    - "ansible-playbook {{ .Arguments.playbook }} {{ .TrailingArgs }}"
```

```shell
atmos ansible run -- --limit web --check
# Runs: ansible-playbook site.yml --limit web --check
```

## Environment Variables

The `env` section defines environment variables available to all steps. Values support
Go template syntax.

```yaml
env:
  - key: string            # Required: Environment variable name
    value: string          # Required: Value (supports Go templates)
```

### Examples

```yaml
env:
  - key: ATMOS_COMPONENT
    value: "{{ .Arguments.component }}"
  - key: ATMOS_STACK
    value: "{{ .Flags.stack }}"
  - key: AWS_REGION
    value: "{{ .ComponentConfig.vars.region }}"
  - key: KUBECONFIG
    value: "/dev/shm/kubecfg.{{ .Flags.stack }}-{{ .Flags.role }}"
```

Environment variables are accessible in steps as standard shell variables (`$ATMOS_COMPONENT`).

## Component Config

The `component_config` section instructs Atmos to resolve the full configuration for a component
in a stack. This makes all component sections available via `{{ .ComponentConfig }}`.

```yaml
component_config:
  component: string        # Required: Component name (supports Go templates)
  stack: string            # Required: Stack name (supports Go templates)
```

### Usage

```yaml
component_config:
  component: "{{ .Arguments.component }}"
  stack: "{{ .Flags.stack }}"
```

### Available Fields

After resolution, the following are available:

| Template Variable | Description |
|-------------------|-------------|
| `{{ .ComponentConfig.component }}` | Terraform component path |
| `{{ .ComponentConfig.backend }}` | Backend configuration map |
| `{{ .ComponentConfig.backend.bucket }}` | Backend bucket name |
| `{{ .ComponentConfig.backend.region }}` | Backend region |
| `{{ .ComponentConfig.workspace }}` | Computed workspace name |
| `{{ .ComponentConfig.vars }}` | All component variables |
| `{{ .ComponentConfig.vars.namespace }}` | Namespace variable |
| `{{ .ComponentConfig.vars.tenant }}` | Tenant variable |
| `{{ .ComponentConfig.vars.environment }}` | Environment variable |
| `{{ .ComponentConfig.vars.stage }}` | Stage variable |
| `{{ .ComponentConfig.vars.region }}` | Region variable |
| `{{ .ComponentConfig.settings }}` | Component settings |
| `{{ .ComponentConfig.env }}` | Environment variable config |
| `{{ .ComponentConfig.metadata }}` | Component metadata |
| `{{ .ComponentConfig.deps }}` | Component dependencies |

## Dependencies

Declare tool dependencies auto-installed before execution:

```yaml
dependencies:
  tools:
    tool-name: "version"
```

### Version Formats

| Format | Meaning | Example |
|--------|---------|---------|
| `"1.10.3"` | Exact version | Only 1.10.3 |
| `"~> 1.10.0"` | Pessimistic (patch) | 1.10.x but not 1.11.0 |
| `"^1.10.0"` | Compatible (minor) | 1.x.x but not 2.0.0 |
| `"latest"` | Latest available | Most recent version |

### Example

```yaml
dependencies:
  tools:
    tflint: "0.54.0"
    checkov: "3.0.0"
    tfsec: "1.28.0"
```

Tools are installed to the configured `toolchain.install_path` and resolved via configured registries.

## Nested Subcommands

Commands can contain nested `commands` for hierarchical structures:

```yaml
commands:
  - name: list
    description: List resources
    commands:
      - name: stacks
        description: List all stacks
        steps:
          - atmos describe stacks --sections none | grep -e "^\S" | sed s/://g
      - name: components
        description: List components
        flags:
          - name: stack
            shorthand: s
            description: Stack name
        steps:
          - >
            {{ if .Flags.stack }}
            atmos describe stacks --stack {{ .Flags.stack }} --format json --sections none |
              jq ".[].components.terraform" | jq -s add | jq -r "keys[]"
            {{ else }}
            atmos describe stacks --format json --sections none |
              jq ".[].components.terraform" | jq -s add | jq -r "keys[]"
            {{ end }}
```

```shell
atmos list stacks
atmos list components -s plat-ue2-dev
```

Nesting can go multiple levels deep.

## Steps (Required)

Steps are the commands to execute, defined as a list of strings. Each step is a shell command
that supports Go template syntax.

```yaml
steps:
  - "echo Hello {{ .Arguments.name }}"
  - "atmos terraform plan {{ .Arguments.component }} -s {{ .Flags.stack }}"
  - |
    {{ if .Flags.verbose }}
    echo "Verbose mode enabled"
    {{ end }}
```

Steps execute sequentially. If a step fails, subsequent steps are not executed.

### Go Template Functions Available

| Expression | Description |
|------------|-------------|
| `{{ .Arguments.<name> }}` | Positional argument value |
| `{{ .Flags.<name> }}` | Flag value |
| `{{ .TrailingArgs }}` | Arguments after `--` |
| `{{ .ComponentConfig.<path> }}` | Resolved component config |
| `{{ if .Flags.name }}...{{ end }}` | Conditional block |
| `{{ if not .Flags.name }}...{{ end }}` | Negated conditional |
| `{{ if eq .Flags.type "value" }}...{{ end }}` | Equality check |
| `{{ else if eq .Flags.type "other" }}` | Else-if branch |
| `{{ else }}` | Else branch |

## Complete Example

```yaml
# atmos.yaml
commands:
  - name: deploy-stack
    description: |
      Deploy all components in a stack in the correct order.

      Example:
        atmos deploy-stack -s plat-ue2-dev
        atmos deploy-stack -s plat-ue2-prod --dry-run
    identity: infrastructure-admin
    flags:
      - name: stack
        shorthand: s
        description: Target stack
        required: true
      - name: dry-run
        shorthand: d
        type: bool
        description: Preview without applying
        default: false
      - name: verbose
        shorthand: v
        type: bool
        description: Verbose output
        default: false
    dependencies:
      tools:
        terraform: "^1.10.0"
    env:
      - key: DEPLOY_STACK
        value: "{{ .Flags.stack }}"
    steps:
      - |
        {{ if .Flags.verbose }}
        echo "Deploying stack: {{ .Flags.stack }}"
        echo "Dry run: {{ .Flags.dry-run }}"
        {{ end }}
      - |
        {{ if .Flags.dry-run }}
        echo "DRY RUN: Would deploy vpc to {{ .Flags.stack }}"
        echo "DRY RUN: Would deploy eks/cluster to {{ .Flags.stack }}"
        echo "DRY RUN: Would deploy app to {{ .Flags.stack }}"
        {{ else }}
        atmos terraform deploy vpc -s {{ .Flags.stack }}
        atmos terraform deploy eks/cluster -s {{ .Flags.stack }}
        atmos terraform deploy app -s {{ .Flags.stack }}
        {{ end }}
```

```shell
atmos deploy-stack -s plat-ue2-dev
atmos deploy-stack -s plat-ue2-prod --dry-run
atmos deploy-stack -s plat-ue2-dev --verbose
```
