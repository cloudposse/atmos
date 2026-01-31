# Interactive Workflows Demo

This example demonstrates Atmos's powerful interactive workflow step types. These step types enable building sophisticated CLI wizards and interactive deployment pipelines.

## Prerequisites

- Atmos CLI installed
- Terminal with TTY support (interactive mode)

## Quick Start

```bash
cd examples/interactive-workflows

# Run the main deployment workflow
atmos workflow deploy -f interactive

# Run the component selection workflow
atmos workflow select-components -f interactive

# Run the full deployment pipeline
atmos workflow full-deploy -f interactive
```

## Available Workflows

### `deploy`
Basic deployment wizard that:
- Displays a markdown welcome message
- Prompts for environment selection
- Shows confirmation for production
- Collects deployment notes
- Displays a summary

### `select-components`
Multi-select workflow demonstrating:
- Filter step with multiple selection
- Variable passing to show selection count

### `select-config`
File picker workflow that:
- Scans for YAML/JSON files
- Displays file selection UI
- Shows the selected file path

### `credentials`
Secure input workflow with:
- Warning message
- Username input
- Password input (masked)

### `write-notes`
Text editor workflow:
- Multi-line text input
- Markdown rendering of notes

### `build`
Spinner workflow that:
- Shows progress during command execution
- Displays success message

### `show-environments`
Table display workflow:
- Renders structured data as table
- Custom columns

### `format-output`
Output formatting workflow:
- Join array values with separator
- Display formatted result

### `full-deploy`
Complete pipeline combining:
- Markdown intro
- Environment selection
- Multi-component selection
- Notes collection
- Confirmation
- Summary display

## Step Types Demonstrated

| Type | Description | Example Usage |
|------|-------------|---------------|
| `markdown` | Render markdown content | Welcome messages, documentation |
| `choose` | Single selection from list | Environment selection |
| `filter` | Filterable selection (single/multi) | Component selection |
| `input` | Single-line text input | Names, messages |
| `confirm` | Yes/No confirmation | Production safeguards |
| `write` | Multi-line text editor | Notes, descriptions |
| `file` | File picker | Config file selection |
| `toast` | Styled message with icon | Status updates, completion notices |
| `spin` | Spinner with command | Long-running tasks |
| `table` | Tabular data display | Environment lists |
| `join` | Join array to string | Format selections |
| `log` | Structured logging | Debugging, monitoring |
| `style` | Styled text with borders | Summary boxes |

## Variable Passing

Steps can access previous step results using Go templates:

```yaml
steps:
  - name: select_env
    type: choose
    prompt: "Select environment"
    options: [dev, staging, prod]

  - name: show_env
    type: toast
    level: info
    content: "You selected: {{ .steps.select_env.value }}"
```

### Available Template Variables

- `{{ .steps.<name>.value }}` - Primary value from step
- `{{ .steps.<name>.values }}` - Array of values (multi-select)
- `{{ .steps.<name>.metadata.<key> }}` - Metadata (exit_code, stdout, stderr)
- `{{ .steps.<name>.skipped }}` - Whether step was skipped
- `{{ .steps.<name>.error }}` - Error message if failed
- `{{ .env.<VAR> }}` - Environment variables

## Output Modes

Control how command output is displayed:

```yaml
workflows:
  my-workflow:
    output: log  # log | raw | viewport | none
    steps:
      - name: step1
        type: shell
        command: "echo hello"
        output: viewport  # Step-level override
```

## CI/CD Considerations

Interactive steps require a TTY. In CI environments:
- Use `--dry-run` to preview workflows
- Set default values in configuration
- Use environment variables instead of prompts.
