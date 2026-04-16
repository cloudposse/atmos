# Fix: terraform compound subcommands (providers lock, state list, workspace select, etc.)

**Date**: 2025-01-28

**GitHub Issue**: [#2018](https://github.com/cloudposse/atmos/issues/2018)

## Problem

Terraform compound subcommands — `providers lock`, `state list`, `workspace select`, and others — were not working correctly.
When using quotes around the compound subcommand (e.g., `"providers lock"`), atmos was not properly recognizing it as a valid subcommand.
Additionally, these commands were handled via hardcoded argument parsing rather than the Cobra command tree.

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

1. **Early return for single arguments**: When the input had only one argument (like the quoted `"providers lock"`), the function returned early at line 221-224 before reaching the compound subcommand handling logic.

2. **Hardcoded argument indices**: The compound subcommand handling only checked for separate arguments (e.g., `["providers", "lock", "component"]`), not for quoted single arguments (e.g., `["providers lock", "component"]`).

## Solution

### Part 1: Argument parsing fix (`internal/exec/cli_utils.go`)

Modified `internal/exec/cli_utils.go` with a modular, well-tested approach:

1. **Helper functions for compound subcommand parsing**:
   - `parseCompoundSubcommand()`: Main entry point that handles both quoted and separate forms.
   - `parseQuotedCompoundSubcommand()`: Parses quoted forms like `"providers lock"`.
   - `parseSeparateCompoundSubcommand()`: Parses separate forms like `["providers", "lock"]`.
   - `processTerraformCompoundSubcommand()`: Processes compound subcommands and extracts component.
   - `processSingleCommand()`: Handles standard single-word commands.

2. **Configurable subcommand lists**: Known compound subcommand lists are defined as package-level variables for easy maintenance:
   - `workspaceSubcommands`: list, select, new, delete, show
   - `stateSubcommands`: list, mv, pull, push, replace-provider, rm, show
   - `providersSubcommands`: lock, mirror, schema

3. **Dynamic component argument indexing**: Uses `argCount` to track whether the subcommand was passed as 1 argument (quoted) or 2 arguments (separate), supporting both:
   - Quoted form: `["providers lock", "component"]` → component at index 1
   - Separate form: `["providers", "lock", "component"]` → component at index 2

### Part 2: Cobra command tree registration (`cmd/terraform/`)

Following the command registry pattern, compound subcommands are now registered as proper
Cobra child commands in the command tree, enabling standard Cobra routing:

1. **`cmd/terraform/utils.go`** - `newTerraformPassthroughSubcommand()` helper creates Cobra
   child commands that delegate to the parent command's execution flow.

2. **`cmd/terraform/state.go`** - Registers `list`, `mv`, `pull`, `push`, `replace-provider`,
   `rm`, `show` as children of `stateCmd`.

3. **`cmd/terraform/providers.go`** - Registers `lock`, `mirror`, `schema` as children
   of `providersCmd`.

4. **`cmd/terraform/workspace.go`** - Registers `list`, `select`, `new`, `delete`, `show`
   as children of `workspaceCmd`. Uses `RegisterPersistentFlags` so sub-subcommands inherit
   backend execution flags. Has a workspace-specific `newWorkspacePassthroughSubcommand()`
   that binds both `terraformParser` and `workspaceParser`.

The legacy compound subcommand parsing in `processArgsAndFlags` is retained as a fallback
for the interactive UI path (which bypasses Cobra) and backward compatibility.

## Supported Compound Subcommands

The following terraform compound subcommands are now supported in both quoted and separate forms:

| Command     | Subcommands                                      |
|-------------|--------------------------------------------------|
| `providers` | lock, mirror, schema                             |
| `state`     | list, mv, pull, push, replace-provider, rm, show |
| `workspace` | list, select, new, delete, show                  |
| `write`     | varfile (legacy, use `generate varfile`)         |

## Files Changed

- `internal/exec/cli_utils.go`: Modified argument parsing to support quoted compound subcommands
- `internal/exec/cli_utils_test.go`: Added comprehensive tests for compound subcommand handling
- `cmd/terraform/utils.go`: Added `newTerraformPassthroughSubcommand()` helper
- `cmd/terraform/state.go`: Registered state sub-subcommands as Cobra children
- `cmd/terraform/providers.go`: Registered providers sub-subcommands as Cobra children
- `cmd/terraform/workspace.go`: Registered workspace sub-subcommands as Cobra children, changed to persistent flags

## Testing

Added comprehensive tests for all new functionality:

**Integration tests (`TestProcessArgsAndFlags_CompoundSubcommands`):**
- All providers subcommands (lock, mirror, schema) in both quoted and separate forms
- State subcommands (list, mv) in both forms
- Workspace subcommands (select, list) in both forms
- Write varfile in both forms
- Error cases for missing component arguments

**Unit tests for helper functions:**
- `TestParseCompoundSubcommand`: Tests the main parsing entry point
- `TestParseQuotedCompoundSubcommand`: Tests parsing of quoted forms
- `TestParseSeparateCompoundSubcommand`: Tests parsing of separate word forms
- `TestProcessTerraformCompoundSubcommand`: Tests processing with component extraction
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
