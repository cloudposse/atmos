# Fix: terraform shell not working from Atmos UI

**Date**: 2025-01-28

**GitHub Issue**: [#2017](https://github.com/cloudposse/atmos/issues/2017)

## Problem

The `atmos terraform shell` command works correctly when invoked directly from the CLI but fails when selected from the Atmos interactive UI (launched by running `atmos` with no arguments).

### Example

```bash
# This works:
atmos terraform shell my-component -s my-stack

# This fails (from UI):
# 1. Run `atmos` to open interactive UI
# 2. Select "terraform shell" from commands
# 3. Select a component and stack
# Error: "Terraform has no command named 'shell'"
```

**Expected behavior**: The shell command should start an interactive shell configured for the terraform component.

**Actual behavior (before fix)**: Terraform returns an error because "shell" is not a native terraform command.

## Root Cause

The Atmos interactive UI uses a different code path than direct CLI invocation:

**Direct CLI path (working)**:
```text
atmos terraform shell vpc -s dev
  ↓
cmd/terraform/shell.go (Cobra command)
  ↓
ExecuteTerraformShell() - called directly
  ↓
Interactive shell starts
```

**UI path (was broken)**:
```text
atmos (interactive UI)
  ↓
User selects "terraform shell"
  ↓
ExecuteAtmosCmd() in internal/exec/atmos.go
  ↓
ExecuteTerraform() dispatcher
  ↓
No "shell" case in switch statement ← BUG
  ↓
Falls through to execute "terraform shell" as native command
  ↓
Terraform error: "no command named shell"
```

The `ExecuteTerraform()` function in `internal/exec/terraform.go` has special handling for various terraform subcommands (plan, apply, destroy, init, etc.) but was missing handling for the Atmos-specific "shell" subcommand.

## Solution

Added handling for the "shell" subcommand in `ExecuteTerraform()` function:

```go
// Handle "shell" subcommand - this is an Atmos-specific command that opens an interactive shell
// configured for the terraform component. It should not be passed to terraform executable.
if info.SubCommand == "shell" {
    opts := &ShellOptions{
        Component:         info.ComponentFromArg,
        Stack:             info.Stack,
        DryRun:            info.DryRun,
        Identity:          info.Identity,
        ProcessingOptions: ProcessingOptions{ProcessTemplates: info.ProcessTemplates, ProcessFunctions: info.ProcessFunctions, Skip: info.Skip},
    }
    return ExecuteTerraformShell(opts, &atmosConfig)
}
```

This intercepts the "shell" subcommand early (similar to "version" handling) and routes it to `ExecuteTerraformShell()` instead of trying to execute it as a native terraform command.

## Atmos-Specific Terraform Commands

These are Atmos commands that extend terraform functionality and are not native terraform commands:

| Command | Description | Now works from UI |
| ------- | ----------- | ----------------- |
| `shell` | Opens interactive shell with component environment | ✓ |
| `clean` | Cleans up terraform artifacts | Separate implementation |
| `generate varfile` | Generates terraform.tfvars.json | Separate implementation |
| `generate backend` | Generates backend.tf.json | Separate implementation |

## Files Changed

- `internal/exec/terraform.go`: Added "shell" subcommand handling in ExecuteTerraform()
- `internal/exec/terraform_shell_test.go`: Added unit tests for shell options conversion

## Testing

Added tests:
- `TestShellOptionsFromConfigAndStacksInfo`: Validates correct conversion of ConfigAndStacksInfo to ShellOptions
- `TestShellSubcommandIdentification`: Validates shell subcommand is correctly identified

## Usage

After the fix, both paths work correctly:

```bash
# Direct CLI invocation
atmos terraform shell my-component -s my-stack

# From Atmos interactive UI
atmos                          # Launch UI
# Select "terraform shell"     # Choose command
# Select component             # Choose component
# Select stack                 # Choose stack
# Shell starts successfully
```
