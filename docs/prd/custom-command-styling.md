# PRD: Custom Command Styling

**Status**: Draft
**Created**: 2025-12-20
**Updated**: 2025-12-20

## Executive Summary

This PRD proposes extending Atmos custom commands with native styling primitives that eliminate the need for external tools like `gum` (Charmbracelet). The feature includes:

1. **Automatic header generation** from the command's `description` field
2. **Structured step syntax** with optional styling configuration (spinners, progress indicators)

By providing these capabilities, custom commands become dramatically simpler, more readable, and visually consistent with Atmos's established UI conventions.

### Before: Verbose with External Dependencies

```yaml
commands:
  - name: build
    description: 'Build all container images'
    steps:
      - |
        gum style --bold --foreground 99 "Building Docker Images"
        gum style --faint "Tag: {{ .Flags.tag }}"
      - |
        gum spin --spinner dot --title "Building opennext..." -- docker build -t opennext .
        gum log --level info "opennext built"
```

### After: Clean and Native

```yaml
commands:
  - name: build
    description: 'Build all container images'
    steps:
      - command: docker build -t opennext .
        settings:
          title: "Building opennext"
          type: spinner
```

The header "Build all container images" is automatically displayed from the `description` field. Success (✓) or failure (✗) is shown automatically based on the command's exit status.

## Problem Statement

### Current State

Custom commands in Atmos currently rely on external tools like `gum` for styled CLI output:

```yaml
commands:
  - name: helm
    commands:
      - name: deploy
        steps:
          - |
            gum style --bold --foreground 99 "Deploying to EKS"
            gum style --faint "Namespace: {{ .Flags.namespace }}"
          - |
            gum spin --spinner dot --title "Updating dependencies..." -- helm dependency update
          - |
            gum log --level info "Deployment complete"
```

### Pain Points

1. **External Dependency**: Requires `gum` to be installed on developer machines and CI environments.
2. **Inconsistent Styling**: `gum` uses its own color palette and styling conventions that may not match Atmos's theme system.
3. **Verbose Syntax**: Styling commands are long and repetitive (`gum style --bold --foreground 99`).
4. **Readability**: The actual command logic is obscured by styling boilerplate.
5. **Maintenance Burden**: Updates to styling require changing every custom command that uses `gum`.
6. **Learning Curve**: Users must learn both Atmos templates AND `gum` syntax.

### Opportunity

Atmos already has a comprehensive UI layer (`pkg/ui/`) with:
- Theme-aware styles via `pkg/ui/theme/`
- Semantic formatting (Success, Warning, Error, Info)
- Spinner support via `pkg/ui/spinner/`
- Markdown rendering
- Icon constants (✓, ✗, ⚠, ℹ)

Exposing these capabilities to custom commands would:
- Eliminate the `gum` dependency
- Ensure consistent styling across all Atmos output
- Simplify custom command authoring
- Respect user's theme preferences and terminal capabilities

## Design Goals

1. **Zero External Dependencies**: Custom commands should not require `gum` or other styling tools.
2. **Theme Consistency**: All styling should respect the user's configured Atmos theme.
3. **Simple Syntax**: Structured steps should be intuitive and concise.
4. **Graceful Degradation**: Styling should degrade gracefully in non-TTY environments (CI, pipes).
5. **Backward Compatibility**: Existing custom commands using `gum` should continue to work.
6. **Convention Over Configuration**: Automatic header from `description` eliminates boilerplate.

## Technical Specification

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           CUSTOM COMMAND EXECUTION                           │
│                              (cmd/cmd_utils.go)                              │
│                                                                              │
│   1. Display auto-header from command description                            │
│   2. Parse each step (string OR structured object)                           │
│   3. Apply step styling (spinner, progress) if configured                    │
│   4. Execute step                                                            │
└─────────────────────────────────────────────────────────────────────────────┘
                                       │
                                       │ uses
                                       ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                           STEP PROCESSOR                                     │
│                        (pkg/command/step.go)                                 │
│                                                                              │
│   ParseStep(raw any) → Step                                                  │
│                                                                              │
│   Step Types:                                                                │
│   - String step      → Execute as shell command                              │
│   - Structured step  → { command, settings: { title, type } }               │
│                                                                              │
│   Execution Types:                                                           │
│   - stream           → Show command output in real-time (default)            │
│   - tail             → Show last 10 lines in real-time (sliding window)      │
│   - spin             → Animated spinner, hide output, show ✓/✗ on completion │
│   - quiet            → Hide output, show ✓/✗ on completion                   │
└─────────────────────────────────────────────────────────────────────────────┘
                                       │
                                       │ delegates to
                                       ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              UI LAYER                                        │
│                              (pkg/ui/)                                       │
│                                                                              │
│   Spinner        → Progress indicator for long operations                    │
│   Theme/Styles   → Color palette and lipgloss styles                         │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Feature 1: Automatic Header from Description

When a custom command has a `description` field, Atmos automatically displays it as a styled header before executing steps. The `description` field supports Go templates for dynamic content (e.g., `'Deploy {{ .Flags.app }} to {{ .Flags.env }}'`).

#### Behavior

```yaml
commands:
  - name: build
    description: 'Build all container images'
    steps:
      - docker build -t myapp .
```

**Output:**
```
Build all container images

$ docker build -t myapp .
[docker build output...]
```

#### Configuration Options

Command display settings can be configured at two levels:

1. **Global defaults** in `atmos.yaml` under `settings.commands`
2. **Per-command overrides** in the command's `settings` field

**Global configuration (atmos.yaml):**

```yaml
settings:
  list_merge_strategy: replace
  terminal:
    color: true
    max_width: 120
    pager: false
  commands:
    header: true          # Show description as header (default: true)
    show_flags: true      # Show all flag values under header (default: true)
    show_steps: true      # Show step command before execution (default: true)
    show_count: false     # Show step count (e.g., "[1/3]") (default: false)

commands:
  - name: build
    description: 'Build all container images'
    steps:
      - docker build -t myapp .
```

**Per-command configuration:**

```yaml
commands:
  - name: build
    description: 'Build all container images'
    settings:
      header: true          # Show description as header (default: true)
      show_flags: true      # Show all flag values under header (default: true)
      show_steps: true      # Show step command before execution (default: true)
      show_count: false     # Show step count (e.g., "[1/3]") (default: false)
    steps:
      - docker build -t myapp .
```

**Precedence:** Command-level settings override global settings. If neither is set, the built-in defaults apply.

**With `show_flags: true` (shows all flags):**
```
Build all container images
  Tag: latest
  Registry: 123456.dkr.ecr.us-west-2.amazonaws.com

$ docker build -t myapp .
[docker build output...]
```

**With `show_steps: true` (default):**
```
Build all container images
  Tag: latest
  Registry: 123456.dkr.ecr.us-west-2.amazonaws.com

$ docker build -t myapp .
[docker build output...]
```

**With `show_count: true`:**
```
Build all container images
  Tag: latest
  Registry: 123456.dkr.ecr.us-west-2.amazonaws.com

[1/3] $ docker build -t myapp .
[docker build output...]

[2/3] ✓ Tagging image

[3/3] ✓ Pushing to registry
```

#### Disabling Defaults

All display settings default to `true` except `show_count` which defaults to `false`. To disable them:

**Globally (affects all custom commands):**

```yaml
# atmos.yaml
settings:
  commands:
    header: false      # Don't display header for any command
    show_flags: false  # Don't show flag values for any command
    show_steps: false  # Don't show step commands for any command

commands:
  - name: build
    description: 'Build all container images'  # Still used for --help
    steps:
      - docker build -t myapp .
```

**Per-command:**

```yaml
commands:
  - name: build
    description: 'Build all container images'  # Still used for --help
    settings:
      header: false      # Don't display header
      show_flags: false  # Don't show flag values
      show_steps: false  # Don't show step commands
    steps:
      - docker build -t myapp .
```

**Mixed (global defaults with per-command overrides):**

```yaml
# atmos.yaml
settings:
  commands:
    header: false      # Disable headers globally
    show_flags: false  # Disable flag display globally

commands:
  - name: build
    description: 'Build all container images'
    settings:
      header: true     # Override: enable header for this command only
    steps:
      - docker build -t myapp .
```

### Feature 2: Structured Step Syntax

Steps can now be either strings (current behavior) OR structured objects with styling configuration.

#### Step Schema

```yaml
# Simple string step (unchanged)
steps:
  - echo "hello"

# Structured step with styling
steps:
  - command: echo "hello"
    settings:
      title: "Saying hello..."
      type: spinner
```

#### Structured Step Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `command` | string | Yes | The shell command to execute |
| `settings.title` | string | No | Custom title to display instead of the command (supports templates, takes precedence over `show_step`) |
| `settings.type` | string | No | Execution type: `stream`, `tail`, `spin`, `quiet` (default: `stream`) |
| `settings.show_step` | bool | No | Show this step's command before execution (overrides command-level `show_steps`) |

**Display precedence (for `stream` and `tail` types):**
1. If `title` is set → display the title as text (regardless of `show_step`)
2. If `show_step` is true (or defaults to true) → display the command as code
3. Otherwise → display nothing

**Note:** For `spin` and `quiet` types, the title/command is displayed *with* the spinner or checkmark—not separately before execution. This avoids duplication.

**Formatting:**
- **Title** → displayed as plain text (human-readable message)
- **Command** → displayed as code with `$` prefix (machine-executable)

Success (✓) or failure (✗) is determined automatically from the command's exit status.

#### Execution Types

All execution types share the same **completion format**: `✓ title` on success or `✗ title` on failure (with stderr displayed after failures). The difference is what's shown **during execution**:

| Type | During Execution | On Completion |
|------|------------------|---------------|
| `stream` | All output in real-time | `✓ title` or `✗ title` |
| `tail` | Last 10 lines (sliding window) | `✓ title` or `✗ title` |
| `spin` | `⠋ title...` (animated spinner) | `✓ title` or `✗ title` |
| `quiet` | Nothing | `✓ title` or `✗ title` |

**On completion:** All types clear their execution output and display the uniform result. For `stream` and `tail`, the output is cleared and replaced with the success/failure line. For `spin`, the spinner is replaced in-place. For `quiet`, the result appears immediately.

**Non-TTY behavior:** In non-TTY environments (CI, pipes), behavior degrades gracefully:

| Type | Non-TTY During Execution | Non-TTY On Completion |
|------|--------------------------|----------------------|
| `stream` | All output (unchanged) | `✓ title` or `✗ title` |
| `tail` | All output (falls back to stream) | `✓ title` or `✗ title` |
| `spin` | `title...` (no animation) | `✓ title` or `✗ title` |
| `quiet` | Nothing (unchanged) | `✓ title` or `✗ title` |

Key differences in non-TTY:
- **`stream`**: No clearing of output (escape codes don't work), so output remains visible followed by the completion result
- **`tail`**: Falls back to `stream` behavior since the sliding window requires terminal escape codes
- **`spin`**: Shows static `title...` instead of animated spinner, then completion result on new line
- **`quiet`**: Unchanged behavior—nothing during execution, completion result only

**Output capture:** The `tail`, `spin`, and `quiet` types all capture stdout and stderr together.

**Step failure behavior:** When a step fails (non-zero exit code), execution stops immediately and the failure is reported. Captured stderr is displayed after the `✗` indicator for all types. Subsequent steps are not executed.

**Keyboard interrupt:** Ctrl+C during any step type gracefully cleans up (e.g., clears spinner, clears tail output) before exiting.

#### Examples

**Stream (default) - show output in real-time:**
```yaml
steps:
  - command: npm install
    settings:
      title: "Installing dependencies"
      type: stream  # Optional - this is the default
```

**Output (during execution):**
```
Installing dependencies
added 847 packages in 3s
npm warn deprecated package@1.0.0: this package is deprecated
added 153 packages in 2s
...
```

**Output (on completion):**
```
✓ Installing dependencies
```

**Tail - show last 10 lines of output:**
```yaml
steps:
  - command: docker build -t myapp .
    settings:
      title: "Building image"
      type: tail
```

**Output (during execution):**
```
 ---> 3a2b4c5d6e7f
Step 8/10 : COPY package*.json ./
 ---> Using cache
 ---> 4b5c6d7e8f9a
Step 9/10 : RUN npm install --production
 ---> Running in 5c6d7e8f9a0b
 ---> 6d7e8f9a0b1c
Step 10/10 : CMD ["node", "server.js"]
 ---> 7e8f9a0b1c2d
Successfully built 7e8f9a0b1c2d
```

As new lines arrive, older lines scroll off the top, keeping only the 10 most recent visible. Useful for long-running commands where you want live progress without flooding the terminal.

**Output (on completion):**
```
✓ Building image
```

The tail output is cleared and replaced with the success indicator.

**Spin - animated spinner, hide output:**
```yaml
steps:
  - command: helm dependency update ./chart
    settings:
      title: "Updating dependencies"
      type: spin
```

**Output (during execution):**
```
⠋ Updating dependencies...
```

**Output (on completion - success):**
```
✓ Updating dependencies
```

**Output (on completion - error):**
```
✗ Updating dependencies
Error: chart not found
```

**Quiet - hide output, show result:**
```yaml
steps:
  - command: rm -rf node_modules
    settings:
      title: "Cleaning node_modules"
      type: quiet
```

**Output (during execution):**
```
[nothing displayed]
```

**Output (on completion):**
```
✓ Cleaning node_modules
```

Unlike `spin`, there's no animated spinner during execution—the result appears only when complete.

**Mixed steps:**
```yaml
steps:
  # Stream output for builds (so user sees progress)
  - command: docker build -t myapp .
    settings:
      title: "Building container"
      type: stream

  # Spin for quick operations
  - command: docker tag myapp registry/myapp:latest
    settings:
      title: "Tagging image"
      type: spin

  # Quiet for cleanup (no output needed)
  - command: docker image prune -f
    settings:
      title: "Pruning old images"
      type: quiet
```

**Output (during first step - stream):**
```
Sending build context to Docker daemon  2.048kB
Step 1/3 : FROM alpine:latest
 ---> 9c6f07244728
Step 2/3 : COPY . /app
 ---> Using cache
 ---> 3a2b4c5d6e7f
Step 3/3 : CMD ["./app"]
 ---> Running in 1a2b3c4d5e6f
 ---> 7f8e9d0c1b2a
Successfully built 7f8e9d0c1b2a
Successfully tagged myapp:latest
```

**Output (during second step - spin):**
```
✓ Building container
⠋ Tagging image...
```

**Output (final state):**
```
✓ Building container
✓ Tagging image
✓ Pruning old images
```

All steps show the same uniform completion format regardless of their execution type.

**Step commands shown by default:**

Since `show_steps` defaults to `true`, step commands are automatically displayed during execution:
```yaml
commands:
  - name: deploy
    description: 'Deploy to production'
    steps:
      - helm upgrade --install myapp ./chart
```

**Output (during execution):**
```
Deploy to production

$ helm upgrade --install myapp ./chart
[helm output...]
```

**Output (on completion):**
```
Deploy to production

✓ helm upgrade --install myapp ./chart
```

**Using title to customize display:**

The `title` field replaces the command in the output, displayed as plain text (no `$` prefix):
```yaml
commands:
  - name: deploy
    description: 'Deploy to production'
    steps:
      - command: helm upgrade --install myapp ./chart
        settings:
          title: "Upgrading helm release"
```

**Output (during execution):**
```
Deploy to production

Upgrading helm release
[helm output...]
```

**Output (on completion):**
```
Deploy to production

✓ Upgrading helm release
```

**Hiding step display:**

Set `show_step: false` at the step level or `show_steps: false` at the command level:
```yaml
# Option 1: Hide all steps at command level
commands:
  - name: deploy
    description: 'Deploy to production'
    settings:
      show_steps: false
    steps:
      - helm upgrade --install myapp ./chart

# Option 2: Hide specific step
commands:
  - name: deploy
    description: 'Deploy to production'
    steps:
      - command: helm upgrade --install myapp ./chart
        settings:
          show_step: false
```

**Output (during execution):**
```
Deploy to production

[helm output...]
```

**Output (on completion):**
```
Deploy to production

✓ helm upgrade --install myapp ./chart
```

Note: The completion message always shows, even if `show_step: false` hides the command during execution.

**Title takes precedence over show_step:**

Even with `show_step: false`, if a `title` is set it will be displayed (as plain text):
```yaml
commands:
  - name: deploy
    description: 'Deploy to production'
    steps:
      - command: helm upgrade --install myapp ./chart
        settings:
          title: "Upgrading helm release"
          show_step: false  # Ignored because title is set
```

**Output (during execution):**
```
Deploy to production

Upgrading helm release
[helm output...]
```

**Output (on completion):**
```
Deploy to production

✓ Upgrading helm release
```

### Implementation Details

#### Schema Changes

**Global settings** are added to the `Settings` struct in `pkg/schema/schema.go`:

```go
// pkg/schema/schema.go

// Settings defines the global atmos settings.
type Settings struct {
    ListMergeStrategy string            `yaml:"list_merge_strategy,omitempty" json:"list_merge_strategy,omitempty"`
    Terminal          *TerminalSettings `yaml:"terminal,omitempty" json:"terminal,omitempty"`
    Commands          *CommandSettings  `yaml:"commands,omitempty" json:"commands,omitempty"` // NEW: global command defaults
    // ... existing fields
}
```

**Per-command settings** are added to the `Command` struct in `pkg/schema/command.go`:

```go
// pkg/schema/command.go

// Command defines a custom command configuration.
type Command struct {
    Name        string         `yaml:"name" json:"name"`
    Description string         `yaml:"description" json:"description"`
    Commands    []Command      `yaml:"commands,omitempty" json:"commands,omitempty"`
    Steps       []any          `yaml:"steps,omitempty" json:"steps,omitempty"` // string OR Step
    Flags       []Flag         `yaml:"flags,omitempty" json:"flags,omitempty"`
    Arguments   []Argument     `yaml:"arguments,omitempty" json:"arguments,omitempty"`
    Env         []EnvVar       `yaml:"env,omitempty" json:"env,omitempty"`
    Settings    *CommandSettings  `yaml:"settings,omitempty" json:"settings,omitempty"` // NEW: per-command overrides
    // ... existing fields
}

// CommandSettings configures styling behavior for a custom command.
// Used both globally (settings.commands) and per-command (command.settings).
type CommandSettings struct {
    Header    *bool `yaml:"header,omitempty" json:"header,omitempty"`         // Show description as header (default: true)
    ShowFlags *bool `yaml:"show_flags,omitempty" json:"show_flags,omitempty"` // Show flag values under header (default: true)
    ShowSteps *bool `yaml:"show_steps,omitempty" json:"show_steps,omitempty"` // Show step commands before execution (default: true)
    ShowCount *bool `yaml:"show_count,omitempty" json:"show_count,omitempty"` // Show step count e.g. "[1/3]" (default: false)
}

// MergeCommandSettings merges global and per-command settings.
// Per-command settings override global settings. Returns effective settings with defaults applied.
func MergeCommandSettings(global, command *CommandSettings) CommandSettings {
    // Start with defaults
    result := CommandSettings{
        Header:    boolPtr(true),
        ShowFlags: boolPtr(true),
        ShowSteps: boolPtr(true),
        ShowCount: boolPtr(false),
    }

    // Apply global settings
    if global != nil {
        if global.Header != nil {
            result.Header = global.Header
        }
        if global.ShowFlags != nil {
            result.ShowFlags = global.ShowFlags
        }
        if global.ShowSteps != nil {
            result.ShowSteps = global.ShowSteps
        }
        if global.ShowCount != nil {
            result.ShowCount = global.ShowCount
        }
    }

    // Apply per-command overrides
    if command != nil {
        if command.Header != nil {
            result.Header = command.Header
        }
        if command.ShowFlags != nil {
            result.ShowFlags = command.ShowFlags
        }
        if command.ShowSteps != nil {
            result.ShowSteps = command.ShowSteps
        }
        if command.ShowCount != nil {
            result.ShowCount = command.ShowCount
        }
    }

    return result
}

func boolPtr(b bool) *bool {
    return &b
}

// Step defines a structured step with optional settings.
type Step struct {
    Command  string        `yaml:"command" json:"command"`
    Settings *StepSettings `yaml:"settings,omitempty" json:"settings,omitempty"`
}

// StepSettings configures execution settings for a single step.
type StepSettings struct {
    Title    string `yaml:"title,omitempty" json:"title,omitempty"`         // Message during/after execution
    Type     string `yaml:"type,omitempty" json:"type,omitempty"`           // stream (default), tail, spin, quiet
    ShowStep *bool  `yaml:"show_step,omitempty" json:"show_step,omitempty"` // Override command-level show_steps
}
```

#### Step Processor

A new package will handle parsing and executing steps:

```go
// pkg/command/step.go

package command

import (
    "github.com/cloudposse/atmos/pkg/schema"
    "github.com/cloudposse/atmos/pkg/ui/spinner"
    u "github.com/cloudposse/atmos/pkg/utils"
)

// ParseStep converts a raw step (string or map) into a Step struct.
func ParseStep(raw any) (*schema.Step, error) {
    switch v := raw.(type) {
    case string:
        // Simple string step - no styling
        return &schema.Step{Command: v}, nil
    case map[string]any:
        // Structured step - parse into Step struct
        return parseStructuredStep(v)
    default:
        return nil, fmt.Errorf("invalid step type: %T", raw)
    }
}

// ExecuteStep runs a step with appropriate styling.
// showSteps is the command-level setting (nil means default true); step.Settings.ShowStep can override it.
// All execution types show uniform completion format: ✓ title (success) or ✗ title + stderr (failure).
func ExecuteStep(step *schema.Step, showSteps *bool, workDir string, env []string) error {
    // Determine execution type (default: stream)
    execType := "stream"
    if step.Settings != nil && step.Settings.Type != "" {
        execType = step.Settings.Type
    }

    // Get title for completion message
    title := getStepTitle(step)

    switch execType {
    case "tail":
        return executeWithTail(step, title, workDir, env)
    case "spin":
        return executeWithSpin(step, title, workDir, env)
    case "quiet":
        return executeQuiet(step, title, workDir, env)
    default: // "stream"
        return executeWithStream(step, title, showSteps, workDir, env)
    }
}

// getStepTitle returns the display title for a step (title if set, otherwise command).
func getStepTitle(step *schema.Step) string {
    if step.Settings != nil && step.Settings.Title != "" {
        return step.Settings.Title
    }
    return step.Command
}

// displayStepHeader shows the title or command before execution (for stream/tail types).
func displayStepHeader(step *schema.Step, showSteps *bool) {
    styles := theme.GetCurrentStyles()

    // Precedence: title (as text) > command (as code) > nothing
    if step.Settings != nil && step.Settings.Title != "" {
        // Title takes precedence - display as plain text
        ui.Writeln(step.Settings.Title)
    } else {
        // Check show_step settings
        showThisStep := true // default
        if showSteps != nil {
            showThisStep = *showSteps
        }
        if step.Settings != nil && step.Settings.ShowStep != nil {
            showThisStep = *step.Settings.ShowStep
        }
        if showThisStep {
            // Display command as code with $ prefix
            ui.Writef("%s %s\n", styles.Muted.Render("$"), styles.Code.Render(step.Command))
        }
    }
}

// showCompletionResult displays the uniform completion message for all step types.
func showCompletionResult(title string, err error, stderr string) {
    styles := theme.GetCurrentStyles()
    if err != nil {
        ui.Writef("%s %s\n", styles.XMark, title)
        if stderr != "" {
            ui.Writeln(stderr)
        }
    } else {
        ui.Writef("%s %s\n", styles.Checkmark, title)
    }
}

// executeWithStream shows output in real-time, then clears and shows completion result.
func executeWithStream(step *schema.Step, title string, showSteps *bool, workDir string, env []string) error {
    // Display header during execution
    displayStepHeader(step, showSteps)

    // Track output lines for clearing
    lineCount := 0
    var stderrBuf strings.Builder

    err := u.ExecuteShellWithCallback(step.Command, "step", workDir, env, func(line string, isStderr bool) {
        lineCount++
        ui.Writeln(line)
        if isStderr {
            stderrBuf.WriteString(line + "\n")
        }
    })

    // Clear all output lines (including header)
    totalLines := lineCount + 1 // +1 for header
    for i := 0; i < totalLines; i++ {
        ui.Write(terminal.EscClearLine + terminal.EscMoveUp)
    }
    ui.Write(terminal.EscClearLine)

    // Show uniform completion result
    showCompletionResult(title, err, stderrBuf.String())
    return err
}

// executeWithTail shows last 10 lines during execution, then clears and shows completion result.
func executeWithTail(step *schema.Step, title string, workDir string, env []string) error {
    const maxLines = 10

    // Create a ring buffer to hold the last N lines
    buffer := make([]string, 0, maxLines)
    lineCount := 0
    var stderrBuf strings.Builder

    // Execute with line-by-line callback
    err := u.ExecuteShellWithCallback(step.Command, "step", workDir, env, func(line string, isStderr bool) {
        lineCount++
        if isStderr {
            stderrBuf.WriteString(line + "\n")
        }

        // Add line to buffer (ring buffer behavior)
        if len(buffer) < maxLines {
            buffer = append(buffer, line)
        } else {
            // Shift buffer and add new line
            copy(buffer, buffer[1:])
            buffer[maxLines-1] = line
        }

        // Clear previous lines and redraw
        if lineCount > 1 {
            linesToClear := min(lineCount-1, maxLines)
            for i := 0; i < linesToClear; i++ {
                ui.Write(terminal.EscClearLine + terminal.EscMoveUp)
            }
            ui.Write(terminal.EscClearLine)
        }

        // Draw current buffer
        for i, l := range buffer {
            if i > 0 {
                ui.Writeln("")
            }
            ui.Write(l)
        }
    })

    // Clear tail output
    linesToClear := min(lineCount, maxLines)
    for i := 0; i < linesToClear; i++ {
        ui.Write(terminal.EscClearLine + terminal.EscMoveUp)
    }
    ui.Write(terminal.EscClearLine)

    // Show uniform completion result
    showCompletionResult(title, err, stderrBuf.String())
    return err
}

// executeWithSpin shows spinner during execution, then shows completion result.
func executeWithSpin(step *schema.Step, title string, workDir string, env []string) error {
    var stderrBuf strings.Builder

    // Capture stderr while running
    err := spinner.ExecWithSpinnerAndCapture(title, title, func() error {
        return u.ExecuteShellWithStderr(step.Command, "step", workDir, env, &stderrBuf)
    })

    // Spinner already shows ✓/✗, but we need to show stderr on error
    if err != nil && stderrBuf.Len() > 0 {
        ui.Writeln(stderrBuf.String())
    }
    return err
}

// executeQuiet hides all output during execution, then shows completion result.
func executeQuiet(step *schema.Step, title string, workDir string, env []string) error {
    var stderrBuf strings.Builder

    // Execute silently, capturing stderr
    err := u.ExecuteShellWithStderr(step.Command, "step", workDir, env, &stderrBuf)

    // Show uniform completion result
    showCompletionResult(title, err, stderrBuf.String())
    return err
}
```

#### Auto-Header Display

The auto-header is displayed in `executeCustomCommand` before processing steps. Global and per-command settings are merged:

```go
// cmd/cmd_utils.go - in executeCustomCommand

// Merge global and per-command settings
settings := schema.MergeCommandSettings(
    atmosConfig.Settings.Commands,  // Global defaults from settings.commands
    commandConfig.Settings,          // Per-command overrides
)

if *settings.Header && commandConfig.Description != "" {
    styles := theme.GetCurrentStyles()
    ui.Writeln(styles.Heading.Render(commandConfig.Description))

    // Show flag values if enabled
    if *settings.ShowFlags {
        for _, flag := range commandConfig.Flags {
            value, _ := cmd.Flags().GetString(flag.Name)
            if value != "" {
                ui.Writef("  %s: %s\n", styles.Muted.Render(flag.Name), value)
            }
        }
    }
    ui.Writeln("") // Blank line after header
}

// Execute steps with merged settings
for i, rawStep := range commandConfig.Steps {
    step, err := command.ParseStep(rawStep)
    if err != nil {
        return err
    }
    if err := command.ExecuteStep(step, settings.ShowSteps, settings.ShowCount, i, len(commandConfig.Steps), workDir, env); err != nil {
        return err
    }
}
```

### Configuration

No new configuration is required. Styling automatically uses the user's configured theme.

Users who want to disable colors can use existing mechanisms:
- `--no-color` flag
- `NO_COLOR=1` environment variable
- Non-TTY detection (automatic)

### Complete Example: Before and After

#### Before (with gum)

```yaml
commands:
  - name: docker
    commands:
      - name: build
        description: 'Build all container images'
        flags:
          - name: tag
            shorthand: t
            description: 'Image tag'
            type: string
            default: latest
          - name: registry
            description: 'ECR registry URL'
            type: string
            default: 123456.dkr.ecr.us-west-2.amazonaws.com
        steps:
          - |
            gum style --bold --foreground 99 "Building Docker Images"
            gum style --faint "Tag: {{ .Flags.tag }}"
            echo ""
          - |
            gum spin --spinner dot --title "Building opennext..." -- docker build -t {{ .Flags.registry }}/opennext:{{ .Flags.tag }} -f apps/web/Dockerfile .
            gum log --level info "opennext built"
          - |
            gum spin --spinner dot --title "Building ai-streaming..." -- docker build -t {{ .Flags.registry }}/ai-streaming:{{ .Flags.tag }} -f apps/ai-streaming/Dockerfile .
            gum log --level info "ai-streaming built"
```

#### After (native styling)

```yaml
commands:
  - name: docker
    commands:
      - name: build
        description: 'Build all container images'
        # No settings needed - header, show_flags, and show_steps all default to true
        flags:
          - name: tag
            shorthand: t
            description: 'Image tag'
            type: string
            default: latest
          - name: registry
            description: 'ECR registry URL'
            type: string
            default: 123456.dkr.ecr.us-west-2.amazonaws.com
        steps:
          - command: docker build -t {{ .Flags.registry }}/opennext:{{ .Flags.tag }} -f apps/web/Dockerfile .
            settings:
              title: "Building opennext"
              type: spin

          - command: docker build -t {{ .Flags.registry }}/ai-streaming:{{ .Flags.tag }} -f apps/ai-streaming/Dockerfile .
            settings:
              title: "Building ai-streaming"
              type: spin

          # Step without title - command is displayed as code
          - command: docker image prune -f
            settings:
              type: quiet
```

#### After (native styling with show_count)

```yaml
commands:
  - name: docker
    commands:
      - name: build
        description: 'Build all container images'
        settings:
          show_count: true  # Enable step counting
        flags:
          - name: tag
            shorthand: t
            description: 'Image tag'
            type: string
            default: latest
          - name: registry
            description: 'ECR registry URL'
            type: string
            default: 123456.dkr.ecr.us-west-2.amazonaws.com
        steps:
          - command: docker build -t {{ .Flags.registry }}/opennext:{{ .Flags.tag }} -f apps/web/Dockerfile .
            settings:
              title: "Building opennext"
              type: spin

          - command: docker build -t {{ .Flags.registry }}/ai-streaming:{{ .Flags.tag }} -f apps/ai-streaming/Dockerfile .
            settings:
              title: "Building ai-streaming"
              type: spin

          - command: docker image prune -f
            settings:
              type: quiet
```

#### Output Comparison

**Before (gum):**
```
Building Docker Images
Tag: latest

⠋ Building opennext...
INFO opennext built
⠋ Building ai-streaming...
INFO ai-streaming built
```

**After (native) - during execution:**
```
Build all container images
  tag: latest
  registry: 123456.dkr.ecr.us-west-2.amazonaws.com

✓ Building opennext
⠋ Building ai-streaming...
```

**After (native) - final state:**
```
Build all container images
  tag: latest
  registry: 123456.dkr.ecr.us-west-2.amazonaws.com

✓ Building opennext
✓ Building ai-streaming
✓ docker image prune -f
```

Note: The last step has no `title`, so the command itself is displayed.

**After (native with show_count) - during execution:**
```
Build all container images
  tag: latest
  registry: 123456.dkr.ecr.us-west-2.amazonaws.com

[1/3] ✓ Building opennext
[2/3] ⠋ Building ai-streaming...
```

**After (native with show_count) - final state:**
```
Build all container images
  tag: latest
  registry: 123456.dkr.ecr.us-west-2.amazonaws.com

[1/3] ✓ Building opennext
[2/3] ✓ Building ai-streaming
[3/3] ✓ docker image prune -f
```

**Benefits:**
- **60% reduction in YAML** - from complex shell scripts to declarative configuration
- **No external dependencies** - no need to install `gum`
- **Theme consistency** - matches other Atmos commands
- **Auto-header** - description is automatically displayed as styled header
- **Auto-flags** - flag values displayed without manual echo
- **Auto success/failure** - ✓/✗ icons based on exit status
- **Graceful degradation** - works in CI without special handling

## Testing Strategy

### Unit Tests

1. **Step Parser Tests** (`pkg/command/step_test.go`)
   - Test parsing of string steps
   - Test parsing of structured steps with settings
   - Test error handling for invalid step types

2. **Step Execution Tests** (`pkg/command/step_execution_test.go`)
   - Test `stream` type shows output in real-time, clears on completion
   - Test `tail` type shows rolling 10-line window, clears on completion
   - Test `tail` type falls back to `stream` in non-TTY
   - Test `spin` type shows spinner during execution
   - Test `quiet` type hides output during execution
   - Test all types show uniform `✓ title` on success
   - Test all types show uniform `✗ title` + stderr on failure
   - Test error handling for each type
   - Test non-TTY fallback behavior for `spin` and `tail`
   - Test stdout and stderr are captured together for all types
   - Test Ctrl+C gracefully cleans up (clears output, shows clean state)

3. **Auto-Header Tests** (`cmd/custom_command_header_test.go`)
   - Test header display from description
   - Test `show_flags` option
   - Test `header: false` disabling

4. **Show Steps Tests** (`cmd/custom_command_show_steps_test.go`)
   - Test command-level `show_steps: true` displays all step commands
   - Test step-level `show_step: false` overrides command-level setting
   - Test step-level `show_step: true` when command-level is false
   - Test `title` displays instead of command
   - Test `title` takes precedence over `show_step: false`
   - Test interaction with each execution type (title shown before execution)

5. **Show Count Tests** (`cmd/custom_command_count_test.go`)
   - Test `show_count: true` displays step count prefix (e.g., "[1/3]")
   - Test `show_count: false` (default) does not display count
   - Test count works with all execution types

6. **Global Settings Tests** (`pkg/schema/command_settings_test.go`)
   - Test `MergeCommandSettings` with nil global and nil command returns defaults
   - Test `MergeCommandSettings` with global settings applies them
   - Test `MergeCommandSettings` with command settings overrides globals
   - Test `MergeCommandSettings` with partial overrides (some global, some command)
   - Test global `settings.commands` is loaded from atmos.yaml
   - Test per-command settings override global settings in execution

### Integration Tests

1. **Custom Command Styling Tests** (`cmd/custom_command_styling_test.go`)
   - Test end-to-end execution of styled commands
   - Verify output matches expected patterns
   - Test mixed string and structured steps

### Manual Testing

1. **Visual Verification**
   - Run styled commands in terminal, verify appearance
   - Test in CI environment, verify graceful degradation
   - Test with different themes

## Success Criteria

1. **Developer Experience**
   - Custom command authors can style output without external tools
   - Documentation includes clear examples for structured steps
   - Styling is intuitive and discoverable

2. **Visual Consistency**
   - All styled output respects user's theme
   - Colors match other Atmos commands
   - Spinners use same style as built-in commands

3. **Reliability**
   - Styling degrades gracefully in non-TTY
   - No crashes when terminal doesn't support colors
   - CI environments work without modification

4. **Adoption**
   - At least 80% of new custom commands use native styling
   - Migration guide enables easy conversion from `gum`

## Implementation Plan

### Phase 1: Schema and Step Processor
- Extend `schema.Command` with `Settings` field
- Add `schema.Step` and `schema.StepSettings` types
- Implement `ParseStep()` to handle string and structured steps
- Update JSON schema in `pkg/datafetcher/schema/`
- Write unit tests for step parsing

### Phase 2: Auto-Header and Display Features
- Add header display logic in `executeCustomCommand`
- Implement `show_flags` option
- Implement `show_steps` option (command-level and step-level)
- Add `settings.header` toggle
- Write unit tests

### Phase 3: Structured Step Execution
- Implement `ExecuteStep()` with spinner integration
- Write integration tests

### Phase 4: Documentation
- Update custom commands documentation in Docusaurus
- Add migration guide from `gum`
- Include examples and best practices
- Update CLI reference

## References

- [Atmos Custom Commands Documentation](https://atmos.tools/cli/configuration/commands)
- [PRD: I/O Handling Strategy](./io-handling-strategy.md)
- [PRD: Help System Architecture](./help-system-architecture.md)
- [Charmbracelet Gum](https://github.com/charmbracelet/gum)
- [pkg/ui/theme/](../../pkg/ui/theme/) - Theme system implementation
- [pkg/ui/spinner/](../../pkg/ui/spinner/) - Spinner implementation
