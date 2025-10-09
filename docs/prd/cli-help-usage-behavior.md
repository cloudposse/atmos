# CLI Help and Usage Behavior PRD

## Executive Summary

This document defines the user experience for help and usage messages in the Atmos CLI, establishing clear distinctions between minimal usage prompts, full help output, and interactive help commands. The goal is to provide users with appropriate levels of detail based on their intent while maintaining consistency across all commands.

## Problem Statement

### Background
Command-line tools typically provide multiple ways for users to get help:
- **Minimal usage**: Brief reminder shown when required arguments are missing
- **Full help**: Complete documentation including flags, examples, and descriptions
- **Interactive help**: Contextual help shown on demand

### Current Challenges
1. **Inconsistent behavior**: Some commands show full help when minimal usage is appropriate
2. **Information overload**: Users seeing complete flag documentation when they just need argument syntax
3. **Exit code confusion**: Help commands returning incorrect exit codes (1 instead of 0)
4. **Mixed output destinations**: Unclear separation between help output and error messages

## Design Goals

1. **Progressive disclosure**: Show minimal information by default, full details on request
2. **Consistent experience**: All commands follow the same help/usage patterns
3. **Correct exit codes**: Success (0) for help requests, error (1) for invalid usage
4. **Clear separation**: Usage hints go to stderr, command output goes to stdout
5. **Self-documenting**: Users can discover help options without external documentation

## Technical Specification

### Three Levels of Help

#### 1. Minimal Usage (Error Case)

**Trigger**: Command run without required arguments or with invalid arguments

**Behavior**:
- Display concise usage syntax
- Show available subcommands (for parent commands)
- Include brief examples if defined in `cmd/markdown/*_usage.md`
- Output to **stderr**
- Exit with code **1** (error)

**Example Output**:
```
Error: The command `atmos atlantis generate` requires a subcommand

Valid subcommands are:
• repo-config

Run `atmos atlantis generate --help` for usage

# Error

The command atmos atlantis generate requires a subcommand

Valid subcommands are:

• repo-config
```

**Implementation**:
```go
// In cmd/atlantis_generate.go
RunE: func(cmd *cobra.Command, args []string) error {
    // Handle "help" subcommand explicitly
    if len(args) > 0 && args[0] == "help" {
        cmd.Help()
        return nil
    }
    // Show minimal usage for invalid invocation
    return showUsageAndExit(cmd, args)
}
```

#### 2. Full Help Output (Requested Help)

**Trigger**: Command run with `--help` or `-h` flag

**Behavior**:
- Display complete command documentation
- Show all flags with descriptions and default values
- Include usage syntax and examples
- Show global flags
- Output to **stdout**
- Exit with code **0** (success)

**Example Output**:
```
Generate the repository configuration file required for Atlantis to manage
Terraform repositories.

Usage:

  atmos atlantis generate repo-config [flags]

Flags:

    --affected-only              Generate Atlantis projects only for components
                                 changed between two Git commits

    --clone-target-ref           Clone the target reference for comparison
                                 (default false)

    --components string          Generate projects for specified components
                                 (comma-separated)

Global Flags:

    --base-path string           Base path for Atmos project
    --config stringSlice         Paths to configuration files
    ...
```

**Implementation**:
- Cobra handles this automatically via the `--help` flag
- No custom code needed in RunE

#### 3. Interactive Help (Help Subcommand)

**Trigger**: Command run with `help` as a subcommand (e.g., `atmos atlantis generate help`)

**Behavior**:
- Same as `--help` flag (display full help)
- Output to **stdout**
- Exit with code **0** (success)

**Implementation**:
```go
// Parent commands handle "help" explicitly
RunE: func(cmd *cobra.Command, args []string) error {
    if len(args) > 0 && args[0] == "help" {
        cmd.Help()
        return nil
    }
    return showUsageAndExit(cmd, args)
}

// Child commands use handleHelpRequest helper
RunE: func(cmd *cobra.Command, args []string) error {
    if err := handleHelpRequest(cmd, args); err != nil {
        return err
    }
    // ... rest of command logic
}
```

### Exit Code Matrix

| Scenario | Exit Code | Output Stream |
|----------|-----------|---------------|
| Missing required arguments | 1 | stderr |
| Invalid arguments | 1 | stderr |
| `--help` flag | 0 | stdout |
| `-h` flag | 0 | stdout |
| `help` subcommand | 0 | stdout |
| Invalid subcommand | 1 | stderr |

### Command Type Behaviors

#### Parent Commands (No RunE)
Parent commands that only organize subcommands (e.g., `atmos atlantis`):
- **No arguments**: Show minimal usage with exit code 1
- **`--help`**: Show full help with exit code 0
- **No custom RunE needed** - Cobra handles this automatically

#### Parent Commands (With RunE)
Parent commands that have their own execution logic (e.g., `atmos atlantis generate`):
- **No arguments**: Execute RunE which shows minimal usage with exit code 1
- **`help` subcommand**: Explicitly handle in RunE, show full help with exit code 0
- **`--help`**: Cobra intercepts, shows full help with exit code 0

#### Leaf Commands
Commands that perform actual work (e.g., `atmos atlantis generate repo-config`):
- **Missing args**: Show minimal usage with exit code 1
- **`help` subcommand**: Use `handleHelpRequest`, show full help with exit code 0
- **`--help`**: Cobra intercepts, shows full help with exit code 0

### Usage Message Format

Minimal usage messages should include:

1. **Error context** (via structured markdown error format)
2. **Valid subcommands** (for parent commands)
3. **Suggestion to use --help** for more information
4. **Examples** (if defined in `cmd/markdown/*_usage.md`)

Example structure:
```
Error: <brief description>

<suggested actions>

Run `<command> --help` for usage

# Error

<error message>

## Hints

<helpful hints>

## Usage Examples:

<examples from markdown file>
```

### Helper Function Specifications

#### `handleHelpRequest(cmd, args) error`

**Purpose**: Detect and handle help requests in command arguments

**Behavior**:
- Checks if first arg is "help" or if "--help"/"-h" is in args
- Calls `cmd.Help()` to display full help
- Returns `nil` to indicate success (exit code 0)
- If no help request detected, returns `nil` without action

**Usage**:
```go
// At the start of RunE for leaf commands
if err := handleHelpRequest(cmd, args); err != nil {
    return err
}
```

#### `showUsageAndExit(cmd, args) error`

**Purpose**: Display minimal usage and return error for invalid invocation

**Behavior**:
- Determines appropriate error message based on context
- Shows valid subcommands if applicable
- Includes examples from `cmd/markdown/*_usage.md` if available
- Returns error with exit code 1

**Usage**:
```go
// When command is invoked incorrectly
return showUsageAndExit(cmd, args)
```

#### `showErrorExampleFromMarkdown(cmd, arg) error`

**Purpose**: Generate usage error with examples from markdown files

**Behavior**:
- Looks up examples in `examples` map from `cmd/markdown_help.go`
- Formats error message with valid subcommands
- Includes usage examples if available
- Returns formatted error with exit code 1

## Implementation Guidelines

### For New Commands

When adding a new command, follow these patterns:

**Parent Command (with subcommands)**:
```go
var myParentCmd = &cobra.Command{
    Use:   "mycommand",
    Short: "Brief description",
    Long:  "Detailed description",
    RunE: func(cmd *cobra.Command, args []string) error {
        // Handle "help" subcommand
        if len(args) > 0 && args[0] == "help" {
            cmd.Help()
            return nil
        }
        // Show usage for invalid invocation
        return showUsageAndExit(cmd, args)
    },
}
```

**Leaf Command (executes work)**:
```go
var myLeafCmd = &cobra.Command{
    Use:   "subcommand",
    Short: "Brief description",
    Long:  "Detailed description",
    RunE: func(cmd *cobra.Command, args []string) error {
        // Handle help requests first
        if err := handleHelpRequest(cmd, args); err != nil {
            return err
        }

        // Validate arguments
        if len(args) > 0 {
            return showUsageAndExit(cmd, args)
        }

        // Execute command logic
        return executeMyCommand(cmd, args)
    },
}
```

### Creating Usage Examples

1. Create markdown file: `cmd/markdown/atmos_mycommand_subcommand_usage.md`
2. Format with bullets and code blocks:
   ```markdown
   - Basic usage

   ```
   $ atmos mycommand subcommand
   ```

   - With options

   ```
   $ atmos mycommand subcommand --flag=value
   ```
   ```

3. Register in `cmd/markdown_help.go`:
   ```go
   examples["atmos_mycommand_subcommand"] = MarkdownExample{
       Content:    myCommandUsageMarkdown,
       Suggestion: "https://atmos.tools/cli/commands/mycommand/subcommand",
   }
   ```

## Testing Requirements

### Test Cases for Each Command

1. **No arguments** (if required):
   - Expect: Minimal usage on stderr
   - Expect: Exit code 1
   - Snapshot: `TestCLICommands_atmos_command.stderr.golden`

2. **`--help` flag**:
   - Expect: Full help on stdout
   - Expect: Exit code 0
   - Snapshot: `TestCLICommands_atmos_command_--help.stdout.golden`

3. **`help` subcommand**:
   - Expect: Full help on stdout
   - Expect: Exit code 0
   - Snapshot: `TestCLICommands_atmos_command_help.stdout.golden`

4. **Invalid subcommand**:
   - Expect: Error with valid subcommands on stderr
   - Expect: Exit code 1
   - Snapshot: `TestCLICommands_atmos_command_invalid.stderr.golden`

### Test Configuration Example

In `tests/test-cases/help-and-usage.yaml`:
```yaml
- name: atmos mycommand
  description: "Should show usage when run without args"
  command: "atmos"
  args:
    - "mycommand"
  expect:
    exit_code: 1
    stderr:
      - "The command"
      - "requires"

- name: atmos mycommand help
  description: "Should show full help"
  command: "atmos"
  args:
    - "mycommand"
    - "help"
  expect:
    exit_code: 0
    stdout:
      - "Usage:"
      - "Flags:"

- name: atmos mycommand --help
  description: "Should show full help"
  command: "atmos"
  args:
    - "mycommand"
    - "--help"
  expect:
    exit_code: 0
    stdout:
      - "Usage:"
      - "Flags:"
```

## Future Enhancements

1. **Contextual examples**: Show examples specific to the error (e.g., missing required flag)
2. **Did you mean?**: Suggest similar commands for typos
3. **Shortened help**: Option for medium-detail help between minimal and full
4. **Man page generation**: Generate Unix man pages from help content
5. **Interactive mode**: Shell-like interface with tab completion

## References

- [Cobra Documentation](https://github.com/spf13/cobra)
- [POSIX Utility Conventions](https://pubs.opengroup.org/onlinepubs/9699919799/basedefs/V1_chap12.html)
- [GNU Coding Standards - --help](https://www.gnu.org/prep/standards/html_node/_002d_002dhelp.html)
- [Atmos Error Handling Strategy](./error-handling-strategy.md)

## Changelog

- **2025-01-09**: Initial version defining three levels of help behavior
