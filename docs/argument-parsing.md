# Developer Guide: Argument Parsing

This guide explains how to safely parse command-line arguments in Atmos, particularly when supporting the `--` separator pattern.

## Quick Reference

**Use Case** | **Solution** | **Example Command**
---|---|---
Pass args to subprocess | `ExtractSeparatedArgs()` → array | `atmos auth exec -- terraform apply`
Pass args to shell | `GetAfterSeparatorAsQuotedString()` | Custom commands with `{{ .TrailingArgs }}`
Parse everything | `DisableFlagParsing=true` | `atmos terraform plan -var foo=bar`

## The `--` Separator Pattern

The `--` (double dash) separator is a Unix convention that marks the end of command flags. Everything after `--` is passed through unchanged.

```bash
atmos auth exec --identity admin -- terraform apply -auto-approve
                ^^^^^^^^^^^^^^^^    ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
                Atmos flags         Passed to subprocess
```

## Safe Argument Parsing Functions

### `ExtractSeparatedArgs()` - The Unified Utility

**Location**: `cmd/args_separator.go`

Use this for all commands that need to separate Atmos flags from subprocess arguments.

```go
func ExtractSeparatedArgs(
    cmd *cobra.Command,
    args []string,      // From RunE
    osArgs []string,    // Usually os.Args
) *SeparatedCommandArgs

type SeparatedCommandArgs struct {
    BeforeSeparator []string  // Args before --
    AfterSeparator  []string  // Args after --
    SeparatorIndex  int       // Position of -- in osArgs
    HasSeparator    bool      // Whether -- was found
}
```

### Basic Usage

```go
func executeMyCommand(cmd *cobra.Command, args []string) error {
    // Extract separated args
    separated := cmd.ExtractSeparatedArgs(cmd, args, os.Args)

    if !separated.HasSeparator {
        return errors.New("command requires -- separator")
    }

    // Use args before separator for Atmos logic
    atmosArgs := separated.BeforeSeparator

    // Pass args after separator to subprocess
    subprocessArgs := separated.AfterSeparator
    execCmd := exec.Command(subprocessArgs[0], subprocessArgs[1:]...)
    return execCmd.Run()
}
```

## Passing Arguments to Subprocesses

### Option 1: Array (Preferred) ✅

**When**: Calling `exec.Command()`, SDK functions, any non-shell execution

**Why**: Preserves argument boundaries, no re-parsing, safest option

```go
separated := ExtractSeparatedArgs(cmd, args, os.Args)
cmdArgs := separated.AfterSeparator

// Pass directly to exec.Command - SAFE
execCmd := exec.Command(cmdPath, cmdArgs...)
```

### Option 2: Shell-Quoted String ✅

**When**: Custom commands, shell templates, shell script execution

**Why**: Properly quotes each argument using shell quoting rules

```go
separated := ExtractSeparatedArgs(cmd, args, os.Args)

// Get shell-safe quoted string
quotedString, err := separated.GetAfterSeparatorAsQuotedString()
if err != nil {
    return err
}

// Use in template - SAFE
data := map[string]any{
    "TrailingArgs": quotedString,  // Properly quoted!
}
```

**Quoting behavior**:

Input Args | Quoted Output | Why
---|---|---
`["echo", "hello  world"]` | `echo 'hello  world'` | Preserves spaces
`["echo", "$VAR"]` | `echo '$VAR'` | Prevents expansion
`["echo", "foo;bar"]` | `echo 'foo;bar'` | Prevents injection

### Option 3: Plain String ❌ UNSAFE

**DO NOT USE** `GetAfterSeparatorAsString()` for shell execution!

```go
// WRONG - SECURITY RISK!
unquoted := separated.GetAfterSeparatorAsString()
ExecuteShell(unquoted)  // Vulnerable to injection!
```

This method exists only for backwards compatibility. It loses argument boundaries and creates security vulnerabilities.

## Common Patterns

### Pattern 1: Auth Exec Pattern

Command with identity flag that wraps subprocess:

```go
var authExecCmd = &cobra.Command{
    Use:   "exec [flags] -- COMMAND [args...]",
    Short: "Execute command with authenticated identity",
    Args:  cobra.MinimumNArgs(1),
    RunE:  executeAuthExec,
}

func executeAuthExec(cmd *cobra.Command, args []string) error {
    separated := ExtractSeparatedArgs(cmd, args, os.Args)

    // Get identity from flag
    identity, _ := cmd.Flags().GetString("identity")

    // Authenticate...
    authEnv, err := authManager.GetEnvironment(identity)
    if err != nil {
        return err
    }

    // Execute subprocess with auth environment
    subCmd := exec.Command(separated.AfterSeparator[0], separated.AfterSeparator[1:]...)
    subCmd.Env = authEnv
    return subCmd.Run()
}
```

### Pattern 2: Custom Commands with Shell

Using trailing args in shell templates:

```yaml
commands:
  - name: deploy
    arguments:
      - name: environment
    steps:
      - "kubectl apply -f {{ .Arguments.environment }}.yaml {{ .TrailingArgs }}"
```

```go
func executeCustomCommand(cmd *cobra.Command, args []string) error {
    separated := ExtractSeparatedArgs(cmd, args, os.Args)

    // Get shell-safe string for template
    quotedTrailing, err := separated.GetAfterSeparatorAsQuotedString()
    if err != nil {
        return err
    }

    data := map[string]any{
        "Arguments": parseArguments(separated.BeforeSeparator),
        "TrailingArgs": quotedTrailing,  // Safely quoted
    }

    commandStr, _ := ProcessTemplate(step, data)
    return ExecuteShell(commandStr)
}
```

## Commands with `DisableFlagParsing`

For commands that pass ALL arguments through (like terraform, helmfile, packer):

```go
var terraformCmd = &cobra.Command{
    Use:                "terraform",
    DisableFlagParsing: true,  // Pass everything through
    RunE:              terraformRun,
}

func terraformRun(cmd *cobra.Command, args []string) error {
    // When DisableFlagParsing=true, use args or osArgs directly
    separated := ExtractSeparatedArgs(cmd, args, os.Args)

    // Everything before -- goes to Atmos
    atmosArgs := separated.BeforeSeparator

    // Everything after -- goes to terraform
    terraformArgs := separated.AfterSeparator

    // Pass array directly to terraform SDK - SAFE
    return executeTerraform(atmosArgs, terraformArgs)
}
```

## Testing Your Implementation

Always test with edge cases:

```bash
# Test whitespace preservation
atmos mycmd -- echo "hello  world"

# Test special characters
atmos mycmd -- echo '$HOME'

# Test empty arguments
atmos mycmd -- echo "" test

# Test shell metacharacters (should NOT execute as separate commands)
atmos mycmd -- echo "foo;bar"
```

## What NOT to Do

### ❌ Manual Argument Parsing

```go
// WRONG - fragile and error-prone
var afterSeparator []string
for i, arg := range args {
    if arg == "--" {
        afterSeparator = args[i+1:]
        break
    }
}
```

Use `ExtractSeparatedArgs()` instead.

### ❌ Plain String Join for Shell

```go
// WRONG - loses argument boundaries, security risk
trailingArgs := strings.Join(args, " ")
ExecuteShell(trailingArgs)
```

Use `GetAfterSeparatorAsQuotedString()` instead.

### ❌ Mixing Parsing Styles

```go
// WRONG - inconsistent with rest of codebase
func myCustomParsing(args []string) []string {
    // Custom logic...
}
```

Use the unified `ExtractSeparatedArgs()` utility.

## Migration from Legacy Code

If you're migrating from old parsing code:

**Old**: `extractTrailingArgs()`
```go
args, trailingStr := extractTrailingArgs(args, os.Args)
data["TrailingArgs"] = trailingStr  // UNSAFE
```

**New**: `ExtractSeparatedArgs()` with quoting
```go
separated := ExtractSeparatedArgs(cmd, args, os.Args)
quotedStr, _ := separated.GetAfterSeparatorAsQuotedString()
data["TrailingArgs"] = quotedStr  // SAFE
```

## See Also

- [Safe Argument Parsing PRD](../docs/prd/safe-argument-parsing.md) - Complete problem analysis and solution
- [Developing Atmos Commands](./developing-atmos-commands.md) - General command development guide
- [Command Registry Pattern](../docs/prd/command-registry-pattern.md) - How to register new commands
