# PRD: Workflow Step Types

## Overview

This PRD defines extended step types for Atmos workflows, enabling interactive prompts, output formatting, and UI messaging directly within workflow definitions. The implementation follows the **Charmbracelet Gum** command model, bringing terminal UI capabilities natively into Atmos workflows while integrating with the existing theme system.

This addresses two Linear issues:
- **DEV-263**: Add Input Type to Atmos Workflows
- **DEV-2969**: Atmos viewports for workflows and subcommands

### Current State

Atmos workflows currently support only two step types:
- `atmos` - Execute Atmos CLI commands
- `shell` - Execute shell commands

### After Implementation

Workflows will support **25+ step types** across five categories:
1. **Interactive** - User input prompts (input, choose, confirm, filter, file, write)
2. **Output** - Display formatting (spin, table, pager, format, join, style, linebreak, log)
3. **UI Messages** - Status messages (toast, markdown)
4. **Terminal** - Terminal control and workflow management (alert, title, clear, env, exit)
5. **Command** - Existing types (atmos, shell)

Plus **per-step output modes** for controlling how command output is displayed.

## Goals

1. **Gum-Compatible Step Types**: Implement workflow step types that mirror Charmbracelet Gum commands for familiarity
2. **Variable System**: Enable capturing step outputs for use in subsequent steps via Go templates
3. **Theme Integration**: All interactive components use the Atmos theme system for consistent styling
4. **Registry Pattern**: Use extensible registry allowing custom step type registration
5. **Output Modes**: Support per-step output display control (viewport, raw, log, none)
6. **UI Messages**: Provide native Atmos UI message types (success, info, warn, error, markdown)
7. **TTY Awareness**: Interactive steps require TTY; fail clearly in CI environments
8. **Testability**: Interface-based design with dependency injection for comprehensive testing

## Non-Goals

1. **Full Gum CLI Parity**: We implement the most useful subset, not every Gum option
2. **External Process Execution**: Step types are native Go, not shelling out to `gum`
3. **Custom Themes per Step**: Steps use the global Atmos theme, not per-step theming
4. **Parallel Step Execution**: Steps execute sequentially (existing behavior)
5. **Conditional Logic**: No if/else branching in workflows (use shell for this)

## Proposed Configuration

### Complete Example

```yaml
# stacks/workflows/deploy.yaml
workflows:
  interactive-deploy:
    description: "Interactive deployment workflow with prompts and status messages"
    output: log  # Default output mode for command steps
    steps:
      # Show welcome message with markdown
      - name: welcome
        type: markdown
        content: |
          # Deployment Workflow

          This workflow will guide you through deploying infrastructure.

      # Interactive: Select environment
      - name: select_env
        type: choose
        prompt: "Select target environment"
        options:
          - development
          - staging
          - production
        default: development

      # Interactive: Select component with fuzzy filter
      - name: select_component
        type: filter
        prompt: "Select component to deploy"
        options:
          - vpc
          - eks
          - rds
          - s3-bucket
          - cloudfront

      # UI Message: Show summary
      - name: summary
        type: info
        content: "Deploying {{ .steps.select_component.value }} to {{ .steps.select_env.value }}"

      # Interactive: Confirm before proceeding
      - name: confirm_deploy
        type: confirm
        prompt: "Proceed with deployment?"

      # Output: Spinner while running command
      - name: plan
        type: spin
        title: "Running terraform plan..."
        command: atmos terraform plan {{ .steps.select_component.value }} -s {{ .steps.select_env.value }}

      # Command: Apply with viewport output
      - name: apply
        type: atmos
        command: terraform apply {{ .steps.select_component.value }} -s {{ .steps.select_env.value }} -auto-approve
        output: viewport

      # UI Message: Success
      - name: complete
        type: success
        content: "Deployment of {{ .steps.select_component.value }} completed!"

      # Shell command with environment variables from previous steps
      - name: notify
        type: shell
        command: |
          curl -X POST "$WEBHOOK_URL" \
            -d "component=$COMPONENT&env=$DEPLOY_ENV"
        env:
          COMPONENT: "{{ .steps.select_component.value }}"
          DEPLOY_ENV: "{{ .steps.select_env.value }}"
          WEBHOOK_URL: "https://hooks.example.com/deploy"
```

### Variable System

Step outputs are automatically captured and accessible via Go templates. The step's `name` is the key:

```yaml
steps:
  - name: select_env
    type: choose
    prompt: "Select environment"
    options: [staging, production]

  # Reference previous step output in templates
  - name: confirm
    type: confirm
    prompt: "Deploy to {{ .steps.select_env.value }}?"

  # Use in command arguments
  - name: deploy
    type: atmos
    command: terraform apply vpc -s {{ .steps.select_env.value }}

  # Export to environment variables for shell commands
  - name: notify
    type: shell
    command: echo "Deployed to $DEPLOY_ENV"
    env:
      DEPLOY_ENV: "{{ .steps.select_env.value }}"
```

### Output Modes

Per-step output control for `atmos` and `shell` steps:

```yaml
workflows:
  deploy:
    output: log  # Default for all steps
    steps:
      - name: plan
        type: atmos
        command: terraform plan vpc
        output: viewport  # Interactive pager

      - name: apply
        type: atmos
        command: terraform apply vpc -auto-approve
        output: log  # Grouped with boundaries

      - name: cleanup
        type: shell
        command: rm -rf .terraform
        output: none  # Silent
```

| Mode | Behavior |
|------|----------|
| `viewport` | Interactive TUI pager with scrolling |
| `raw` | Direct passthrough to stdout/stderr |
| `log` | Grouped output with step boundaries |
| `none` | Silent (exit code only) |

### Viewport Configuration

Configure viewport size for `viewport` output mode and `pager` step type:

```yaml
workflows:
  deploy:
    # Workflow-level viewport defaults
    viewport:
      height: 20    # Lines (default: terminal height - 5)
      width: 120    # Columns (default: terminal width)

    steps:
      - name: plan
        type: atmos
        command: terraform plan vpc
        output: viewport
        # Step-level override (nested under viewport for consistency)
        viewport:
          height: 30
          width: 100

      - name: show_logs
        type: pager
        content: "{{ .steps.build.value }}"
        viewport:
          height: 40  # Taller pager for log review
```

**Viewport behavior:**
- `height` - Number of lines (default: auto-detect terminal height minus status bar)
- `width` - Number of columns (default: auto-detect terminal width)
- Falls back to `log` mode if terminal is too small or not a TTY
- Respects `--force-tty` and `ATMOS_FORCE_TTY` for screenshot generation

### Show Configuration

Configure automatic display features for workflows. Supports workflow-level defaults with step-level overrides using deep merge.

```yaml
workflows:
  deploy:
    description: "Deploy infrastructure"
    show:
      header: true      # Display workflow description as styled header
      flags: true       # Show flag values under header (e.g., "stack: dev")
      command: true     # Show step command before execution ($ prefix)
      count: true       # Show step count prefix (e.g., "[1/3]")
      progress: true    # Show progress bar (Docker-build style)
    steps:
      - name: validate
        command: terraform validate
        type: shell
        show:
          command: false  # Override: don't show command for this step
```

**Show Features:**

| Feature | Description | Default |
|---------|-------------|---------|
| `header` | Display workflow description as styled header before first step | `false` |
| `flags` | Show workflow-level flag values (stack, identity, etc.) under header | `false` |
| `command` | Show step command before execution with `$` prefix | `false` |
| `count` | Show step count prefix `[1/3]` before step name | `false` |
| `progress` | Show right-aligned progress bar (Docker-build style, TTY only) | `false` |

**Show behavior:**
- All features default to `false` (opt-in for backward compatibility)
- Uses `*bool` fields for tri-state logic: `nil` (inherit), `true`, `false`
- Step-level `show` settings override workflow-level settings via deep merge
- Progress bar only renders in TTY mode (graceful degradation to no-op in CI)
- Header renders once before the first step executes
- Flags display workflow-level parameters: `--stack`, `--identity`, `--dry-run`

**Implementation notes:**
- `ShowConfig` struct in `pkg/schema/workflow.go`
- `GetShowConfig()` in `pkg/workflow/step/show_config.go` for resolution
- `ShowRenderer` in `pkg/workflow/show_renderer.go` for header/flags
- `ProgressRenderer` in `pkg/workflow/progress.go` for progress bar
- Follows existing patterns: `GetOutputMode()`, `GetViewportConfig()`

### Step Type Summary

| Category | Types | Description |
|----------|-------|-------------|
| **Interactive** | `input`, `confirm`, `choose`, `filter`, `file`, `write` | User prompts (require TTY) |
| **Output** | `spin`, `table`, `pager`, `format`, `join`, `style`, `linebreak`, `log` | Display formatting |
| **UI Messages** | `toast`, `markdown`, `sleep`, `stage` | Status messages, timing, and progress |
| **Terminal** | `alert`, `title`, `clear`, `env`, `exit` | Terminal control and workflow management |
| **Command** | `atmos`, `shell` | Execute commands (existing) |

---

## Architecture

### Step Type Registry

Following the Command Registry pattern (`cmd/internal/registry.go`), step types use a registry model for extensibility:

```go
// pkg/workflow/step/registry.go

// StepCategory groups step types for documentation and validation.
type StepCategory string

const (
    CategoryInteractive StepCategory = "interactive" // Requires user input
    CategoryOutput      StepCategory = "output"      // Displays formatted output
    CategoryUI          StepCategory = "ui"          // Status messages
    CategoryCommand     StepCategory = "command"     // Execute commands
)

// StepHandler defines the interface for workflow step type handlers.
type StepHandler interface {
    // GetName returns the step type name (e.g., "input", "choose", "success").
    GetName() string

    // GetCategory returns the step category for grouping.
    GetCategory() StepCategory

    // Execute runs the step and returns the result.
    Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error)

    // Validate checks step configuration before execution.
    Validate(step *schema.WorkflowStep) error

    // RequiresTTY returns true if the step requires an interactive terminal.
    RequiresTTY() bool
}

// StepResult captures the output of a step execution.
type StepResult struct {
    Value    string            // Primary output value (for variable capture)
    Values   []string          // For multiselect
    Metadata map[string]any    // Additional data
    Skipped  bool              // If step was skipped
}

// Variables holds step outputs accessible via Go templates.
type Variables struct {
    Steps map[string]*StepResult // step name -> result
    Env   map[string]string      // Environment variables
}

// Registry manages step type handlers.
type Registry struct {
    mu       sync.RWMutex
    handlers map[string]StepHandler
}

// Global registry instance.
var registry = &Registry{handlers: make(map[string]StepHandler)}

// Register adds a step handler to the registry.
// Called from init() in each handler file.
func Register(handler StepHandler) {
    registry.mu.Lock()
    defer registry.mu.Unlock()
    registry.handlers[handler.GetName()] = handler
}

// Get returns a handler by type name.
func Get(typeName string) (StepHandler, bool) {
    registry.mu.RLock()
    defer registry.mu.RUnlock()
    h, ok := registry.handlers[typeName]
    return h, ok
}

// List returns all registered handlers.
func List() map[string]StepHandler {
    registry.mu.RLock()
    defer registry.mu.RUnlock()
    result := make(map[string]StepHandler, len(registry.handlers))
    for k, v := range registry.handlers {
        result[k] = v
    }
    return result
}

// ListByCategory returns handlers grouped by category.
func ListByCategory() map[StepCategory][]StepHandler {
    registry.mu.RLock()
    defer registry.mu.RUnlock()
    result := make(map[StepCategory][]StepHandler)
    for _, h := range registry.handlers {
        cat := h.GetCategory()
        result[cat] = append(result[cat], h)
    }
    return result
}
```

### Schema Changes

Extend `WorkflowDefinition` and `WorkflowStep` in `pkg/schema/workflow.go`:

```go
// ViewportConfig configures viewport display settings.
type ViewportConfig struct {
    Height int `yaml:"height,omitempty" json:"height,omitempty" mapstructure:"height"` // Lines
    Width  int `yaml:"width,omitempty" json:"width,omitempty" mapstructure:"width"`    // Columns
}

type WorkflowDefinition struct {
    // Existing fields
    Description      string         `yaml:"description,omitempty" json:"description,omitempty" mapstructure:"description"`
    WorkingDirectory string         `yaml:"working_directory,omitempty" json:"working_directory,omitempty" mapstructure:"working_directory"`
    Steps            []WorkflowStep `yaml:"steps" json:"steps" mapstructure:"steps"`
    Stack            string         `yaml:"stack,omitempty" json:"stack,omitempty" mapstructure:"stack"`

    // New fields
    Output   string          `yaml:"output,omitempty" json:"output,omitempty" mapstructure:"output"`     // Default output mode for steps
    Viewport *ViewportConfig `yaml:"viewport,omitempty" json:"viewport,omitempty" mapstructure:"viewport"` // Default viewport settings
}

type WorkflowStep struct {
    // Existing fields
    Name             string       `yaml:"name,omitempty" json:"name,omitempty" mapstructure:"name"`
    Command          string       `yaml:"command,omitempty" json:"command,omitempty" mapstructure:"command"`
    Stack            string       `yaml:"stack,omitempty" json:"stack,omitempty" mapstructure:"stack"`
    Type             string       `yaml:"type,omitempty" json:"type,omitempty" mapstructure:"type"`
    WorkingDirectory string       `yaml:"working_directory,omitempty" json:"working_directory,omitempty" mapstructure:"working_directory"`
    Retry            *RetryConfig `yaml:"retry,omitempty" json:"retry,omitempty" mapstructure:"retry"`
    Identity         string       `yaml:"identity,omitempty" json:"identity,omitempty" mapstructure:"identity"`

    // New fields for extended step types
    Prompt      string            `yaml:"prompt,omitempty" json:"prompt,omitempty" mapstructure:"prompt"`           // Prompt text for interactive types
    Options     []string          `yaml:"options,omitempty" json:"options,omitempty" mapstructure:"options"`        // Options for choose/filter
    Default     string            `yaml:"default,omitempty" json:"default,omitempty" mapstructure:"default"`        // Default value
    Placeholder string            `yaml:"placeholder,omitempty" json:"placeholder,omitempty" mapstructure:"placeholder"` // Input placeholder
    Password    bool              `yaml:"password,omitempty" json:"password,omitempty" mapstructure:"password"`     // Mask input
    Content     string            `yaml:"content,omitempty" json:"content,omitempty" mapstructure:"content"`        // Content for output types
    Env         map[string]string `yaml:"env,omitempty" json:"env,omitempty" mapstructure:"env"`                    // Environment variables (supports templates)
    Title       string            `yaml:"title,omitempty" json:"title,omitempty" mapstructure:"title"`              // Title for spin/pager
    Data        []map[string]any  `yaml:"data,omitempty" json:"data,omitempty" mapstructure:"data"`                 // Data for table type
    Columns     []string          `yaml:"columns,omitempty" json:"columns,omitempty" mapstructure:"columns"`        // Columns for table
    Output      string            `yaml:"output,omitempty" json:"output,omitempty" mapstructure:"output"`           // Output mode: viewport, raw, log, none
    Path        string            `yaml:"path,omitempty" json:"path,omitempty" mapstructure:"path"`                 // Path for file picker
    Extensions  []string          `yaml:"extensions,omitempty" json:"extensions,omitempty" mapstructure:"extensions"` // File extensions filter
    Multiple    bool              `yaml:"multiple,omitempty" json:"multiple,omitempty" mapstructure:"multiple"`     // Allow multiple selection
    Limit       int               `yaml:"limit,omitempty" json:"limit,omitempty" mapstructure:"limit"`              // Selection limit
    Height      int               `yaml:"height,omitempty" json:"height,omitempty" mapstructure:"height"`           // Height for write type (editor lines)
    Viewport    *ViewportConfig   `yaml:"viewport,omitempty" json:"viewport,omitempty" mapstructure:"viewport"`     // Viewport settings for output mode
    Timeout     string            `yaml:"timeout,omitempty" json:"timeout,omitempty" mapstructure:"timeout"`        // Timeout duration
}
```

## Step Type Reference

### Interactive Step Types

#### `input` - Single-line Text Input

Prompts user for single-line text input. Maps to `gum input`.

```yaml
- name: username
  type: input
  prompt: "Enter username"
  placeholder: "john.doe"
  default: ""
  password: false  # Set true to mask input
```

Access result: `{{ .steps.username.value }}`

**Implementation:** `huh.NewInput()` with `NewAtmosHuhTheme()`

**Fields:**
- `prompt` (required) - Prompt text
- `placeholder` - Placeholder text in input field
- `default` - Default value
- `password` - Mask input characters

---

#### `confirm` - Yes/No Confirmation

Prompts user for yes/no confirmation. Maps to `gum confirm`.

```yaml
- name: proceed
  type: confirm
  prompt: "Are you sure you want to continue?"
  default: "no"  # "yes" or "no"
```

Access result: `{{ .steps.proceed.value }}` (returns "true" or "false")

**Implementation:** `huh.NewConfirm()` with `NewAtmosHuhTheme()`

**Fields:**
- `prompt` (required) - Confirmation question
- `default` - Default selection ("yes" or "no")

---

#### `choose` - Single Selection

Select one option from a list. Maps to `gum choose`.

```yaml
- name: environment
  type: choose
  prompt: "Select environment"
  options:
    - staging
    - production
    - development
  default: staging
```

Access result: `{{ .steps.environment.value }}`

**Implementation:** `huh.NewSelect[string]()` with `NewAtmosHuhTheme()`

**Fields:**
- `prompt` (required) - Selection prompt
- `options` (required) - List of options
- `default` - Pre-selected option

---

#### `filter` - Fuzzy Filter Selection

Filter and select from a list with fuzzy matching. Maps to `gum filter`.

```yaml
- name: component
  type: filter
  prompt: "Select component"
  options:
    - vpc
    - eks
    - rds
    - s3-bucket
    - cloudfront
  limit: 1  # Number of selections (1 = single, >1 = multiple)
```

Access result: `{{ .steps.component.value }}` (or `{{ .steps.component.values }}` for multiple)

**Implementation:** `huh.NewSelect[string]()` with filtering enabled, or custom bubbletea model

**Fields:**
- `prompt` (required) - Filter prompt
- `options` (required) - List of options to filter
- `limit` - Selection limit (default: 1)

---

#### `file` - File Picker

Browse and select a file. Maps to `gum file`.

```yaml
- name: config_file
  type: file
  prompt: "Select configuration file"
  path: "./configs"  # Starting directory
  extensions:
    - .yaml
    - .yml
```

Access result: `{{ .steps.config_file.value }}`

**Implementation:** Custom bubbletea file browser with `NewAtmosHuhTheme()` styling

**Fields:**
- `prompt` (required) - File picker prompt
- `path` - Starting directory (default: current directory)
- `extensions` - Filter by file extensions

---

#### `write` - Multi-line Text Input

Prompts for multi-line text input. Maps to `gum write`.

```yaml
- name: description
  type: write
  prompt: "Enter deployment notes"
  placeholder: "Describe the changes..."
  height: 10
```

Access result: `{{ .steps.description.value }}`

**Implementation:** `huh.NewText()` with `NewAtmosHuhTheme()`

**Fields:**
- `prompt` (required) - Prompt text
- `placeholder` - Placeholder text
- `height` - Editor height in lines (default: 5)

---

### Output Step Types

#### `spin` - Spinner with Command

Display a spinner while executing a command. Maps to `gum spin`.

```yaml
- name: build
  type: spin
  title: "Building project..."
  command: make build
```

**Implementation:** `bubbles/spinner` with `theme.Styles.Spinner`, command execution

**Fields:**
- `title` (required) - Spinner message
- `command` (required) - Command to execute while spinning
- `timeout` - Maximum duration (e.g., "5m")

---

#### `table` - Data Table

Render data as a formatted table. Maps to `gum table`.

```yaml
- name: show_components
  type: table
  title: "Components"
  columns:
    - Name
    - Stack
    - Status
  data:
    - {Name: vpc, Stack: ue2-dev, Status: deployed}
    - {Name: eks, Stack: ue2-dev, Status: pending}
```

**Implementation:** Reuse `pkg/list/format/table.go` and renderer pipeline

**Fields:**
- `title` - Table title
- `columns` - Column headers (optional, inferred from data)
- `data` (required) - Array of row objects
- `content` - Alternative: render from Go template result

---

#### `pager` - Scrollable Content

Display content in a scrollable pager. Maps to `gum pager`.

```yaml
- name: show_plan
  type: pager
  title: "Terraform Plan"
  content: "{{ .steps.terraform_plan.value }}"
```

**Implementation:** Reuse `pkg/pager/` (existing)

**Fields:**
- `title` - Pager title
- `content` (required) - Content to display (supports templates)

---

#### `format` - Template Formatting

Format and display text using Go templates. Maps to `gum format`.

```yaml
- name: summary
  type: format
  content: |
    Environment: {{ .steps.select_env.value }}
    Component: {{ .steps.select_component.value }}
    Status: Ready to deploy
```

**Implementation:** Go template rendering with existing FuncMap

**Fields:**
- `content` (required) - Go template string

---

#### `join` - Join Text

Join multiple values. Maps to `gum join`.

```yaml
- name: combined
  type: join
  separator: "\n"
  options:
    - "{{ .steps.header.value }}"
    - "{{ .steps.body.value }}"
    - "{{ .steps.footer.value }}"
```

Access result: `{{ .steps.combined.value }}`

**Implementation:** Simple string joining

**Fields:**
- `options` (required) - Array of strings/templates to join
- `separator` - String to join values with (default: newline)
- `content` - Alternative: single template string to resolve and return as-is

---

#### `style` - Apply Styling

Apply terminal styling to text. Maps to `gum style`.

```yaml
- name: header
  type: style
  content: "Deployment Summary"
  # Styling uses theme colors automatically
```

**Implementation:** lipgloss styling with theme colors

**Fields:**
- `content` (required) - Text to style

---

### UI Message Step Types

These map directly to existing Atmos UI functions.

#### `success` - Success Message

Display a success message with checkmark.

```yaml
- name: done
  type: success
  content: "Deployment completed successfully!"
```

**Implementation:** `ui.Success()` from `pkg/ui/`

**Fields:**
- `content` (required) - Message text (supports templates)

---

#### `info` - Info Message

Display an informational message.

```yaml
- name: status
  type: info
  content: "Processing {{ .steps.count.value }} components..."
```

**Implementation:** `ui.Info()` from `pkg/ui/`

**Fields:**
- `content` (required) - Message text (supports templates)

---

#### `warn` - Warning Message

Display a warning message.

```yaml
- name: caution
  type: warn
  content: "This will modify production resources!"
```

**Implementation:** `ui.Warning()` from `pkg/ui/`

**Fields:**
- `content` (required) - Message text (supports templates)

---

#### `error` - Error Message

Display an error message.

```yaml
- name: failed
  type: error
  content: "Deployment failed: {{ .steps.deploy.error }}"
```

**Implementation:** `ui.Error()` from `pkg/ui/`

**Fields:**
- `content` (required) - Message text (supports templates)

---

#### `markdown` - Rendered Markdown

Render and display markdown content.

```yaml
- name: help
  type: markdown
  content: |
    # Deployment Guide

    This workflow will:
    1. Select target environment
    2. Run terraform plan
    3. Apply changes after confirmation

    **Warning:** Production deployments require approval.
```

**Implementation:** `ui.Markdown()` / `ui.MarkdownMessage()` with Glamour

**Fields:**
- `content` (required) - Markdown text (supports templates)

---

#### `sleep` - Pause Execution

Pauses workflow execution for a specified duration. Useful for demos, rate limiting, or allowing time for resources to stabilize.

```yaml
- name: wait
  type: sleep
  timeout: 2s
```

**Implementation:** `time.After()` with context cancellation support

**Fields:**
- `timeout` (optional) - Duration to sleep (default: 1s). Supports Go duration format: `500ms`, `2s`, `1m30s`, etc.

**Notes:**
- Respects context cancellation (Ctrl+C will interrupt the sleep)
- Supports Go template resolution in timeout field

---

#### `stage` - Workflow Stage Indicator

Displays a high-level stage position within a workflow. Unlike `show.count` which shows position among all steps, `stage` shows position only among steps of type `stage`.

```yaml
- name: setup
  type: stage
  title: "Setup"

- name: setup-work
  type: toast
  content: "Performing setup tasks..."

- name: configure
  type: stage
  title: "Configuration"

- name: deploy
  type: stage
  title: "Deployment"
```

Output:
```
[Stage 1/3] Setup
ℹ Performing setup tasks...
[Stage 2/3] Configuration
[Stage 3/3] Deployment
```

**Implementation:** `StageHandler` in `pkg/workflow/step/stage.go`

**Fields:**
- `title` (optional) - Stage title to display (defaults to step name)

**Notes:**
- Total stages counted before execution starts
- Stage index auto-increments as each stage step executes
- Useful for showing high-level progress in complex workflows
- Can be combined with `show.progress` for detailed + high-level tracking

---

### Terminal Step Types

These step types control the terminal environment and workflow execution flow.

#### `alert` - Terminal Bell

Plays a terminal bell sound to notify the user.

```yaml
- name: notify
  type: alert
  content: "Workflow complete!"  # Optional message
```

**Implementation:** `terminal.Alert()` from `pkg/terminal/`

**Fields:**
- `content` (optional) - Message to display after the bell (supports templates)

**Notes:**
- Respects `settings.terminal.alerts` configuration
- Use at the end of long-running workflows to notify users

---

#### `title` - Terminal Window Title

Sets or restores the terminal window title.

```yaml
- name: set_title
  type: title
  content: "Deploying to {{ .steps.env.value }}..."
```

```yaml
- name: restore_title
  type: title  # Empty content restores original title
```

**Implementation:** `terminal.SetTitle()` / `terminal.RestoreTitle()` from `pkg/terminal/`

**Fields:**
- `content` (optional) - Title text (supports templates). If empty, restores original title.

**Notes:**
- Respects `settings.terminal.title` configuration
- Some terminals may not support title changes

---

#### `clear` - Clear Terminal Line

Clears the current terminal line.

```yaml
- name: clear_line
  type: clear
```

**Implementation:** `ui.ClearLine()` from `pkg/ui/`

**Fields:**
- None required

---

#### `env` - Set Environment Variables

Sets environment variables for subsequent steps in the workflow.

```yaml
- name: set_env
  type: env
  vars:
    DEPLOY_ENV: "{{ .steps.select_env.value }}"
    AWS_REGION: us-east-1
    TF_VAR_environment: "{{ .steps.select_env.value }}"
```

**Implementation:** `Variables.SetEnv()` from `pkg/workflow/step/`

**Fields:**
- `vars` (required) - Map of environment variable names to values (supports templates)

**Notes:**
- Variables are scoped to the workflow execution
- Use to pass user input or computed values to subsequent shell/atmos commands

---

#### `exit` - Exit Workflow

Exits the workflow immediately with a specific exit code.

```yaml
- name: abort
  type: exit
  code: 1
  content: "Deployment cancelled by user"
```

**Implementation:** `os.Exit()` with optional message

**Fields:**
- `code` (optional) - Exit code (default: 0). Use non-zero for error conditions.
- `content` (optional) - Message to display before exiting (supports templates)

**Notes:**
- The workflow terminates immediately; no subsequent steps run
- Use for early exit on user cancellation or error conditions

---

### Command Step Types

#### `atmos` - Atmos Command (existing)

Execute an Atmos CLI command.

```yaml
- name: plan
  type: atmos
  command: terraform plan vpc
  stack: "{{ .steps.select_env.value }}"
  output: viewport  # New: output mode
```

**Fields:**
- `command` (required) - Atmos command to execute
- `stack` - Stack override
- `output` - Output mode (viewport, raw, log, none)

---

#### `shell` - Shell Command (existing)

Execute a shell command.

```yaml
- name: build
  type: shell
  command: make build
  working_directory: ./app
  output: log
```

**Fields:**
- `command` (required) - Shell command to execute
- `working_directory` - Execution directory
- `output` - Output mode (viewport, raw, log, none)

---

## Output Modes

Per-step output mode control for `atmos` and `shell` step types:

| Mode | Behavior | Use Case |
|------|----------|----------|
| `viewport` | Interactive TUI pager with scrolling | Default for TTY when human is watching |
| `raw` | Direct passthrough to stdout/stderr | Debugging, live-tailing |
| `log` | Grouped output with step boundaries | CI environments, log files |
| `none` | Silent (exit code only) | Automation, suppressing verbose output |

```yaml
workflows:
  deploy:
    output: log  # Default for all steps in workflow
    steps:
      - name: plan
        type: atmos
        command: terraform plan vpc
        output: viewport  # Override for this step

      - name: apply
        type: atmos
        command: terraform apply vpc -auto-approve
        output: log

      - name: cleanup
        type: shell
        command: rm -rf .terraform
        output: none
```

**Mode Details:**

### `viewport` Mode
- Uses bubbletea TUI (reuses `pkg/pager/`)
- Shows compact summary at top (step name, progress)
- Scrollable viewport for command output
- Navigate with arrow keys, j/k, page up/down
- Gracefully falls back to `log` if no TTY

### `log` Mode (Taskfile `group` equivalent)
- Collects all output from step
- Prints with clear begin/end boundaries
- Format: `[step-name]` header, separator at end
- Color-coded: green=success, red=failure, yellow=running
- Best for CI logs and piping to files

### `raw` Mode (Taskfile `interleaved` equivalent)
- Direct passthrough to stdout/stderr
- Every line optionally prefixed with `[step-name]`
- TTY colors preserved
- Automatic masking via `pkg/io/`

### `none` Mode
- Suppresses all output
- Exit code still indicates success/failure
- Useful for cleanup steps or verbose operations

---

## TTY/Non-TTY Behavior

### Interactive Steps in Non-TTY

When interactive step types are used in non-TTY environments (CI, piped output), they **fail with a clear error**:

```text
Error: Step 'select_env' requires interactive terminal

The step type 'choose' requires a TTY for user input.
This workflow cannot run in non-interactive mode (CI, piped output).

Hints:
  - Use --dry-run to preview workflow without interactive steps
  - Set default values in workflow configuration
  - Use environment variables instead of interactive prompts in CI
```

**Rationale:** Interactive steps fundamentally require user input. Silent failures or default values could lead to unintended actions in CI.

### TTY Detection

Uses existing Atmos TTY detection:
- `term.IsTTYSupportForStdout()`
- `ATMOS_FORCE_TTY=true` to override
- `CI=true` environment detection

### Non-Interactive Alternatives

For CI environments, recommend:

```yaml
workflows:
  deploy-ci:
    description: "CI-friendly deployment (no interactive prompts)"
    steps:
      # Use environment variables instead of prompts
      - name: validate_env
        type: shell
        command: |
          if [ -z "$DEPLOY_ENV" ]; then
            echo "Error: DEPLOY_ENV must be set"
            exit 1
          fi

      - name: deploy
        type: atmos
        command: terraform apply vpc -s $DEPLOY_ENV -auto-approve
```

---

## Testing Strategy

### Interface Mocking

All handlers implement `StepHandler` interface for easy mocking:

```go
//go:generate go run go.uber.org/mock/mockgen@latest -source=registry.go -destination=mock_handler_test.go -package=step

func TestWorkflowExecution(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockHandler := NewMockStepHandler(ctrl)
    mockHandler.EXPECT().GetName().Return("input").AnyTimes()
    mockHandler.EXPECT().Execute(gomock.Any(), gomock.Any(), gomock.Any()).
        Return(&StepResult{Value: "test-value"}, nil)

    // Test logic
}
```

### Test Coverage Requirements

- Unit tests for all handlers (80% coverage minimum)
- Integration tests for variable passing between steps
- TTY behavior tests using mock terminal
- CLI command tests using `cmd.NewTestKit(t)`
- Table-driven tests for configuration validation

### Test Cases

1. **Handler Registration**: Verify all built-in handlers register correctly
2. **Variable Resolution**: Test Go template variable passing
3. **TTY Detection**: Test interactive step behavior with/without TTY
4. **Output Modes**: Test viewport, raw, log, none modes
5. **Theme Integration**: Verify theme colors applied to all components
6. **Error Handling**: Test validation errors, execution failures
7. **Timeout Handling**: Test spin step timeout behavior

---

## Implementation Plan

### Phase 1: Core Infrastructure

1. Create `pkg/workflow/step/` package structure
2. Define `StepHandler` interface and `Registry`
3. Implement `Variables` type with Go template support
4. Implement `BaseHandler` with common functionality
5. Add schema fields to `WorkflowStep`
6. Generate mocks for testing

### Phase 2: Interactive Handlers

1. Implement `InputHandler` (huh.NewInput)
2. Implement `ChooseHandler` (huh.NewSelect)
3. Implement `ConfirmHandler` (huh.NewConfirm)
4. Implement `WriteHandler` (huh.NewText)
5. Implement `FilterHandler` (huh.NewSelect with filtering)
6. Implement `FileHandler` (custom bubbletea)
7. Add theme integration via `NewAtmosHuhTheme()`

### Phase 3: Output Handlers

1. Implement `SpinHandler` (bubbles/spinner + command)
2. Implement `TableHandler` (reuse pkg/list/format/table)
3. Implement `PagerHandler` (reuse pkg/pager)
4. Implement `FormatHandler` (Go templates)
5. Implement `JoinHandler` (string joining)
6. Implement `StyleHandler` (lipgloss styling)

### Phase 4: UI Message Handlers

1. Implement `SuccessHandler` (ui.Success)
2. Implement `InfoHandler` (ui.Info)
3. Implement `WarnHandler` (ui.Warning)
4. Implement `ErrorHandler` (ui.Error)
5. Implement `MarkdownHandler` (ui.Markdown)

### Phase 5: Legacy Handlers & Output Modes

1. Implement `AtmosHandler` (existing atmos step)
2. Implement `ShellHandler` (existing shell step)
3. Implement output mode support (viewport, raw, log, none)
4. Integrate with `pkg/workflow/executor.go`

### Phase 6: Integration & Testing

1. Update `pkg/workflow/executor.go` to use registry
2. Update `internal/exec/workflow_utils.go` for compatibility
3. Write comprehensive unit tests
4. Write integration tests
5. Add CLI tests using NewTestKit

### Phase 7: Documentation

1. Create Docusaurus documentation for each step type
2. Add examples to workflow documentation
3. Create blog post announcement
4. Update CHANGELOG

---

## File Structure

```
pkg/workflow/step/
├── registry.go           # StepHandler interface + Registry
├── types.go              # StepCategory, StepResult, Variables
├── variables.go          # Variable resolution with Go templates
├── handler_base.go       # BaseHandler with common functionality
│
├── # Interactive handlers
├── input.go              # InputHandler
├── choose.go             # ChooseHandler
├── confirm.go            # ConfirmHandler
├── filter.go             # FilterHandler
├── file.go               # FileHandler
├── write.go              # WriteHandler
│
├── # Output handlers
├── spin.go               # SpinHandler
├── table.go              # TableHandler
├── pager.go              # PagerHandler
├── format.go             # FormatHandler
├── join.go               # JoinHandler
├── style.go              # StyleHandler
│
├── # UI message handlers
├── toast.go              # ToastHandler (success/info/warn/error)
├── markdown.go           # MarkdownHandler
│
├── # Terminal handlers
├── alert.go              # AlertHandler
├── title.go              # TitleHandler
├── clear.go              # ClearHandler
├── env.go                # EnvHandler
├── exit.go               # ExitHandler
│
├── # Command handlers
├── atmos.go              # AtmosHandler
├── shell.go              # ShellHandler
│
├── # Output modes
├── output_mode.go        # Output mode implementation
│
├── mock_handler_test.go  # Generated mocks
└── *_test.go             # Tests
```

---

## References

- [Charmbracelet Gum](https://github.com/charmbracelet/gum) - Inspiration for step types
- [Charmbracelet Huh](https://github.com/charmbracelet/huh) - Form library for interactive prompts
- [Taskfile Output Modes](https://taskfile.dev/usage/#output-syntax) - Inspiration for output modes
- [DEV-263: Add Input Type to Atmos Workflows](https://linear.app/cloudposse/issue/DEV-263)
- [DEV-2969: Atmos viewports for workflows](https://linear.app/cloudposse/issue/DEV-2969)
- [Atmos Command Registry Pattern](command-registry-pattern.md)
- [Atmos Theme System](../../pkg/ui/theme/)
