# Fix: terraform providers lock command not working

**Date**: 2025-01-28

**GitHub Issue**: [#2018](https://github.com/cloudposse/atmos/issues/2018)

## Problem

The `atmos terraform "providers lock"` command (and similar two-word terraform commands) stopped working. When using quotes around the two-word command, atmos was not properly recognizing it as a valid subcommand.

### Example

```bash
# This was failing:
atmos terraform "providers lock" my-component -s my-stack -- -platform=windows_amd64 -platform=darwin_amd64 -platform=linux_amd64

# Custom command in atmos.yaml also affected:
commands:
  - name: tf-lock
    steps:
      - atmos terraform "providers lock" $ATMOS_COMPONENT -s $ATMOS_STACK -- -platform=windows_amd64
```

**Expected behavior**: The command should execute `terraform providers lock` on the specified component.

**Actual behavior (before fix)**: Atmos would fail to recognize the command or would not validate it correctly.

## Root Cause

The command-line argument parsing in `processArgsAndFlags` had two issues:

1. **Early return for single arguments**: When the input had only one argument (like the quoted `"providers lock"`), the function returned early at line 221-224 before reaching the two-word command handling logic.

2. **Hardcoded argument indices**: The two-word command handling only checked for separate arguments (e.g., `["providers", "lock", "component"]`), not for quoted single arguments (e.g., `["providers lock", "component"]`).

## Solution

Modified `internal/exec/cli_utils.go` with a modular, well-tested approach:

1. **Helper functions for two-word command parsing**:
   - `parseTwoWordCommand()`: Main entry point that handles both quoted and separate forms.
   - `parseQuotedTwoWordCommand()`: Parses quoted forms like `"providers lock"`.
   - `parseSeparateTwoWordCommand()`: Parses separate forms like `["providers", "lock"]`.
   - `processTerraformTwoWordCommand()`: Processes two-word commands and extracts component.
   - `processSingleCommand()`: Handles standard single-word commands.

2. **Configurable subcommand lists**: Known two-word command subcommands are defined as package-level variables for easy maintenance:
   - `workspaceSubcommands`: list, select, new, delete, show
   - `stateSubcommands`: list, mv, pull, push, replace-provider, rm, show
   - `providersSubcommands`: lock, mirror, schema

3. **Dynamic component argument indexing**: Uses `argCount` to track whether the command was 1 argument (quoted) or 2 arguments (separate), supporting both:
   - Quoted form: `["providers lock", "component"]` → component at index 1
   - Separate form: `["providers", "lock", "component"]` → component at index 2

## Supported Two-Word Commands

The following terraform two-word commands are now supported in both quoted and separate forms:

| Command | Subcommands |
|---------|-------------|
| `providers` | lock, mirror, schema |
| `state` | list, mv, pull, push, replace-provider, rm, show |
| `workspace` | list, select, new, delete, show |
| `write` | varfile (legacy, use `generate varfile`) |

## Files Changed

- `internal/exec/cli_utils.go`: Modified argument parsing to support quoted two-word commands
- `internal/exec/cli_utils_test.go`: Added comprehensive tests for two-word command handling

## Testing

Added comprehensive tests for all new functionality:

**Integration tests (`TestProcessArgsAndFlags_TwoWordCommands`):**
- All providers subcommands (lock, mirror, schema) in both quoted and separate forms
- State subcommands (list, mv) in both forms
- Workspace subcommands (select, list) in both forms
- Write varfile in both forms
- Error cases for missing component arguments

**Unit tests for helper functions:**
- `TestParseTwoWordCommand`: Tests the main parsing entry point
- `TestParseQuotedTwoWordCommand`: Tests parsing of quoted forms
- `TestParseSeparateTwoWordCommand`: Tests parsing of separate word forms
- `TestProcessTerraformTwoWordCommand`: Tests processing with component extraction
- `TestProcessSingleCommand`: Tests single-word command processing

## Usage Examples

After the fix, all these forms work correctly:

```bash
# Quoted form (single argument with space)
atmos terraform "providers lock" my-component -s my-stack

# Separate form (two arguments)
atmos terraform providers lock my-component -s my-stack

# With additional terraform flags
atmos terraform "providers lock" my-component -s my-stack -- -platform=linux_amd64 -platform=darwin_amd64

# In custom commands
commands:
  - name: tf-lock
    steps:
      - atmos terraform "providers lock" $ATMOS_COMPONENT -s $ATMOS_STACK -- -platform=linux_amd64
```
