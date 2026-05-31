# Custom Commands

This example demonstrates how to extend Atmos with custom CLI commands, making it easier for teams to use your toolchain through a single, consistent interface.

## Quick Start

```shell
# View available custom commands
atmos --help

# Run basic commands
atmos hello
atmos ip
atmos github status
atmos greet Alice
atmos weather --location LAX
```

## Example Structure

```
custom-commands/
├── atmos.yaml              # Basic custom commands
├── .atmos.d/
│   ├── interactive.yaml    # Interactive step types (choose, confirm, input, etc.)
│   └── advanced.yaml       # Boolean flags, component config, working directory
└── README.md
```

## Basic Commands (atmos.yaml)

### Hello World
The simplest custom command:
```shell
atmos hello
```

### Get Your IP
Returns your current public IP address:
```shell
atmos ip
```

### Nested Commands (GitHub)
Commands can be nested under parent commands:
```shell
# Check GitHub status
atmos github status

# Get stargazer count for a repository
atmos github stargazers cloudposse/atmos
```

### Positional Arguments
Pass arguments to commands:
```shell
# Uses default "John Doe"
atmos greet

# Pass a custom name
atmos greet Alice
```

### Flags
Use long or short flags:
```shell
atmos weather --location LAX
atmos weather -l NYC
```

## Interactive Commands (.atmos.d/interactive.yaml)

Interactive commands create CLI wizards using various step types.

**Note:** Interactive commands require a TTY (terminal) and won't work in CI/CD pipelines.

### Deploy Wizard
Interactive deployment with environment selection and confirmation:
```shell
atmos deploy-wizard
```

### Component Selection
Multi-select components with filtering:
```shell
atmos select-components
```

### Create Ticket
Collect information with various input types:
```shell
atmos create-ticket
```

### Collect Credentials
Secure credential collection with password masking:
```shell
atmos collect-credentials
```

### File Selection
Interactive file picker:
```shell
atmos select-config
```

### Show Environments
Display data in a formatted table:
```shell
atmos show-environments
```

### Build with Progress
Show progress spinner during command execution:
```shell
atmos build-with-progress
```

## Advanced Commands (.atmos.d/advanced.yaml)

### Boolean Flags
Commands with boolean flags and conditional execution:
```shell
# Default behavior (clean=true, verbose=false)
atmos build

# Enable verbose output
atmos build --verbose
# or
atmos build -v

# Disable clean step
atmos build --clean=false

# Dry run mode
atmos build --dry-run
```

### Environment Variables
Commands that set environment variables:
```shell
atmos deploy-component vpc -s dev
atmos deploy-component eks -s prod --auto-approve
```

### Component Configuration
Access component configuration in command steps:
```shell
# Requires stacks to be configured
atmos show-component vpc -s dev
```

### Working Directory
Run commands from specific directories:
```shell
# Run from repository root
atmos run-from-root
```

### Nested Subcommands
Multiple levels of command nesting:
```shell
atmos ops status
atmos ops health --verbose
atmos ops restart my-service --force
```

### Trailing Arguments
Pass additional arguments after `--`:
```shell
atmos run myscript.sh -- --arg1 value1 --arg2 value2
```

## Configuration Structure

Custom commands are defined in the `commands` section of `atmos.yaml` or any file in `.atmos.d/`:

```yaml
commands:
  - name: my-command
    description: Description shown in help
    arguments:
      - name: arg-name
        description: Argument description
        required: true
        default: default-value
    flags:
      - name: flag-name
        shorthand: f
        description: Flag description
        type: bool  # or string (default)
        default: false
    env:
      - key: MY_VAR
        value: "{{ .Arguments.arg-name }}"
    steps:
      - echo "Running command"
      - "{{ if .Flags.flag-name }}echo 'Flag enabled'{{ end }}"
```

## Available Step Types

Custom commands support various step types for building interactive CLIs:

| Type | Description |
|------|-------------|
| Shell command (string) | Execute a shell command |
| `markdown` | Display formatted markdown |
| `choose` | Single-select dropdown |
| `filter` | Filterable multi-select |
| `input` | Single-line text input |
| `write` | Multi-line text editor |
| `confirm` | Yes/No confirmation |
| `file` | File picker |
| `toast` | Styled status message |
| `spin` | Progress spinner |
| `table` | Tabular data display |
| `style` | Styled bordered content |

## Template Variables

Steps can access various template variables:

- `{{ .Arguments.name }}` - Positional argument value
- `{{ .Flags.name }}` - Flag value
- `{{ .TrailingArgs }}` - Arguments after `--`
- `{{ .steps.step_name.value }}` - Value from a previous step
- `{{ .steps.step_name.values }}` - Array of values (multi-select)
- `{{ .ComponentConfig.vars.xxx }}` - Component configuration (requires `component_config`)
- `{{ .env.VAR }}` - Environment variable

## Learn More

- [Custom Commands Documentation](https://atmos.tools/cli/configuration/commands)
- [Workflows Documentation](https://atmos.tools/workflows)
- [Interactive Workflows Example](../interactive-workflows)
