# Interactive Prompts for Terraform Commands

## Overview

Terraform commands should provide interactive component and stack selection when users omit required arguments in TTY environments, enabling discoverability and reducing friction for new users unfamiliar with available components and stacks.

## Problem Statement

### Current State

When users run terraform commands without specifying required arguments, they receive unhelpful error messages:

```bash
$ atmos terraform plan
Error: `component` is required.

Usage:

`atmos terraform <command> <component> <arguments_and_flags>`

$ atmos terraform plan vpc
Error: stack is required; specify it on the command line using the flag '--stack <stack>' (shorthand '-s')
```

### Challenges

1. **Poor discoverability** - Users must know component/stack names upfront
2. **Error-driven workflow** - Users discover requirements through errors
3. **Friction for new users** - High barrier to entry for unfamiliar codebases
4. **Inconsistent UX** - Other commands (theme, auth) have interactive prompts

### Existing Infrastructure

Atmos already has a fully-implemented interactive prompt system:

- `pkg/flags/interactive.go` - Core prompting with TTY/CI detection
- `pkg/flags/options.go` - `WithPositionalArgPrompt()`, `WithCompletionPrompt()`
- Completion functions for components/stacks already exist
- Reference implementation: `cmd/theme/show.go`, `handleInteractiveIdentitySelection()`

## Solution: Centralized Prompt Handler

### Architecture

Add a single handler function in `terraformRunWithOptions()` that intercepts missing arguments before validation:

```
User runs: atmos terraform plan
    │
    ▼
terraformRunWithOptions()
    │
    ├─► ProcessCommandLineArgs()  ─► Extract component/stack from args
    │
    ├─► resolveComponentPath()    ─► Handle "." and path-based components
    │
    ├─► handleInteractiveComponentStackSelection()  ◄── NEW
    │       │
    │       ├─► If component missing + interactive → prompt
    │       └─► If stack missing + interactive → prompt (filtered by component)
    │
    ├─► checkTerraformFlags()     ─► Validate flag combinations
    │
    └─► Route to execution
```

### Command Coverage

#### Phase 1: Centralized Handler (22 commands)

One change to `terraformRunWithOptions()` covers all commands that route through it:

| Command | File | Notes |
|---------|------|-------|
| `plan` | plan.go | Calls `terraformRunWithOptions` directly |
| `apply` | apply.go | Calls `terraformRunWithOptions` directly |
| `deploy` | deploy.go | Calls `terraformRunWithOptions` directly |
| `init` | init.go | Via `terraformRun` |
| `destroy` | destroy.go | Via `terraformRun` |
| `validate` | validate.go | Via `terraformRun` |
| `output` | output.go | Via `terraformRun` |
| `refresh` | refresh.go | Via `terraformRun` |
| `show` | show.go | Via `terraformRun` |
| `state` | state.go | Via `terraformRun` |
| `taint` | taint.go | Via `terraformRun` |
| `untaint` | untaint.go | Via `terraformRun` |
| `console` | console.go | Via `terraformRun` |
| `fmt` | fmt.go | Via `terraformRun` |
| `get` | get.go | Via `terraformRun` |
| `graph` | graph.go | Via `terraformRun` |
| `import` | import.go | Via `terraformRun` |
| `force-unlock` | force_unlock.go | Via `terraformRun` |
| `providers` | providers.go | Via `terraformRun` |
| `test` | test.go | Via `terraformRun` |
| `workspace` | workspace.go | Via `terraformRun` |
| `modules` | modules.go | Via `terraformRun` |

#### Phase 2: Custom Commands (6 commands)

Commands with custom execution paths need individual handling:

| Command | File | Priority | Notes |
|---------|------|----------|-------|
| `shell` | shell.go | High | Required component/stack |
| `clean` | clean.go | Medium | Optional component/stack |
| `generate varfile` | generate/varfile.go | Medium | Required component/stack |
| `generate backend` | generate/backend.go | Medium | Required component/stack |
| `generate planfile` | generate/planfile.go | Medium | Required component/stack |
| `varfile` | varfile.go | Low | Deprecated |

#### No Changes Needed

| Command | Reason |
|---------|--------|
| `login`, `logout` | Terraform Cloud - no component/stack |
| `metadata`, `version`, `help` | No component/stack |
| `generate backends`, `generate varfiles` | Operate on all components |
| `backend create/delete/describe/list/update` | Backend CRUD - no component/stack |

### Prompt Behavior

#### Skip Prompts When

- Multi-component flags are set (`--all`, `--affected`, `--query`, `--components`)
- Help is requested (`--help`)
- Non-interactive environment (no TTY, CI detected, `--interactive=false`)
- Both component and stack are already provided

#### Prompt Sequence

1. **Component prompt** (if missing): Shows all terraform components
2. **Stack prompt** (if missing): Shows stacks filtered by selected component

#### User Experience

```bash
# Missing both
$ atmos terraform plan
? Choose a component
  > vpc
    eks
    rds
? Choose a stack
  > ue2-dev
    ue2-prod

# Missing only stack
$ atmos terraform plan vpc
? Choose a stack
  > ue2-dev
    ue2-prod

# All provided - no prompts
$ atmos terraform plan vpc -s ue2-dev
# Executes immediately

# Non-TTY - standard error
$ echo "" | atmos terraform plan
Error: `component` is required.
```

### Implementation

#### Core Handler Function

```go
// handleInteractiveComponentStackSelection prompts for missing component and stack
// when running in interactive mode. Skipped for multi-component operations.
func handleInteractiveComponentStackSelection(info *schema.ConfigAndStacksInfo, cmd *cobra.Command) error {
    // Skip if multi-component mode or help requested
    if hasMultiComponentFlags(info) || info.NeedHelp {
        return nil
    }

    // Both provided - nothing to do
    if info.ComponentFromArg != "" && info.Stack != "" {
        return nil
    }

    // Prompt for component if missing
    if info.ComponentFromArg == "" {
        component, err := promptForComponent(cmd)
        if err != nil {
            if errors.Is(err, errUtils.ErrUserAborted) {
                errUtils.Exit(errUtils.ExitCodeSIGINT)
            }
            if errors.Is(err, errUtils.ErrInteractiveModeNotAvailable) {
                return nil // Fall through to validation
            }
            return err
        }
        if component != "" {
            info.ComponentFromArg = component
        }
    }

    // Prompt for stack if missing
    if info.Stack == "" {
        stack, err := promptForStack(cmd, info.ComponentFromArg)
        if err != nil {
            if errors.Is(err, errUtils.ErrUserAborted) {
                errUtils.Exit(errUtils.ExitCodeSIGINT)
            }
            if errors.Is(err, errUtils.ErrInteractiveModeNotAvailable) {
                return nil
            }
            return err
        }
        if stack != "" {
            info.Stack = stack
        }
    }

    return nil
}

func promptForComponent(cmd *cobra.Command) (string, error) {
    return flags.PromptForPositionalArg(
        "component",
        "Choose a component",
        componentsArgCompletion,
        cmd,
        nil,
    )
}

func promptForStack(cmd *cobra.Command, component string) (string, error) {
    var args []string
    if component != "" {
        args = []string{component}
    }
    return flags.PromptForMissingRequired(
        "stack",
        "Choose a stack",
        stackFlagCompletion,
        cmd,
        args,
    )
}
```

#### Integration Point

In `cmd/terraform/utils.go` `terraformRunWithOptions()`:

```go
// After path resolution (line 212), before help check (line 214):

// Handle interactive component/stack selection for single-component operations.
if err := handleInteractiveComponentStackSelection(&info, actualCmd); err != nil {
    return err
}
```

## Edge Cases

1. **Path-based components** (`atmos terraform plan .`): Prompts run AFTER path resolution
2. **Stack-only provided**: Triggers multi-component execution (existing behavior preserved)
3. **User abort** (Ctrl+C/ESC): Exit with SIGINT code (130)
4. **CI environments**: Prompts auto-disabled, standard validation errors shown

## Success Criteria

1. `atmos terraform plan` (no args) prompts for component then stack in TTY
2. `atmos terraform plan vpc` prompts for stack in TTY
3. Non-TTY/CI environments show standard validation errors
4. Multi-component flags (`--all`, `--affected`) bypass prompts
5. User can abort with Ctrl+C/ESC
6. All existing tests continue to pass
7. Works for all 22 commands routed through `terraformRunWithOptions()`

## Files to Modify

| File | Change |
|------|--------|
| `cmd/terraform/utils.go` | Add handler function and call site |

## Dependencies

- `pkg/flags` package (exists)
- `errUtils.ErrUserAborted`, `errUtils.ErrInteractiveModeNotAvailable` (exist)
- `componentsArgCompletion`, `stackFlagCompletion` (exist in utils.go)
