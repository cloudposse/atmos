# PRD: Safe Argument Parsing and Shell Quoting

**Status**: Implementation Ready
**Priority**: Critical (Security & Correctness)
**Author**: Analysis from Session 2025-10-31
**Impact**: Custom Commands, Terraform, Helmfile, Packer, Auth Commands

## Executive Summary

This PRD documents a **critical security and correctness issue** discovered in Atmos's argument parsing implementation and provides a comprehensive solution. The issue affects how arguments are passed to shell execution, potentially causing:

1. **Data Loss**: Whitespace and special characters are lost when arguments are re-parsed by the shell
2. **Security Risk**: Command injection vulnerabilities when shell metacharacters are not properly quoted
3. **User Confusion**: Users may have been working around this bug with excessive shell quoting

## Problem Statement

### Root Cause

Atmos has multiple fragmented implementations for parsing arguments separated by the `--` (end-of-args) delimiter. These implementations all share a fundamental flaw: **they join argument arrays using `strings.Join()` without proper shell quoting**, then pass the resulting string to a shell parser.

#### The Bug Flow

```go
// User types:
atmos mycmd run -- echo "hello  world"

// Shell parses to os.Args (CORRECT):
["atmos", "mycmd", "run", "--", "echo", "hello  world"]
//                                      ^^^^^^^^^^^^^^^^
//                                      ONE argument with 2 spaces

// Current buggy code (extractTrailingArgs):
trailingArgs := strings.Join(["echo", "hello  world"], " ")
// Result: "echo hello  world" (raw string, no quotes)

// Template uses it:
{{ .TrailingArgs }}  →  "echo hello  world"

// ExecuteShell passes to shell parser:
ShellRunner("echo hello  world")

// Shell parser RE-PARSES without quotes:
["echo", "hello", "world"]  // BUG: 2 spaces became separator!
//       ^^^^^^^  ^^^^^^^
//       TWO arguments instead of one!
```

**Result**: The user's carefully quoted argument `"hello  world"` is split into two arguments, losing the double space.

### Security Implications

The bug is not just about whitespace - it's a **command injection vulnerability**:

```bash
# User input with semicolon
atmos mycmd -- echo "foo;bar"

# Without quoting, shell sees:
echo foo;bar

# This executes TWO commands:
# 1. echo foo
# 2. bar (potential malicious command)
```

Any shell metacharacter (`;`, `|`, `&`, `$`, `` ` ``, etc.) can be exploited to execute unintended commands.

### Affected Components

#### 1. **Custom Commands** ❌ CRITICAL

**File**: `cmd/cmd_utils.go:332,513`
**Function**: `extractTrailingArgs()`
**Impact**: HIGH - Used in production custom commands via `{{ .TrailingArgs }}`

```go
// BUGGY CODE:
trailingArgs = strings.Join(lo.Slice(osArgs, doubleDashIndex+1, len(osArgs)), " ")
// Returns: "arg1 arg2 arg3" (no shell quoting!)

// Then used in template:
data := map[string]any{
    "TrailingArgs": trailingArgs,  // Raw string, no quoting
}
commandToRun, _ := e.ProcessTmpl(&atmosConfig, step, data, false)
err = e.ExecuteShell(commandToRun, ...)  // Shell re-parses!
```

**Evidence in Documentation**:
- `website/docs/core-concepts/custom-commands/custom-commands.mdx` shows `{{ .TrailingArgs }}` pattern
- Example: `ansible-playbook {{ .Arguments.playbook }} {{ .TrailingArgs }}`
- This pattern is **UNSAFE** without proper quoting

#### 2. **Terraform** ✅ NOT AFFECTED

**File**: `cmd/cmd_utils.go:696-700`
**Function**: `getConfigAndStacksInfo()`
**Impact**: NONE - Passes `[]string` directly to SDK, no shell re-parsing

```go
// SAFE: Extracts as array
argsAfterDoubleDash = lo.Slice(args, doubleDashIndex+1, len(args))
info, err := e.ProcessCommandLineArgs(commandName, cmd, finalArgs, argsAfterDoubleDash)
// ProcessCommandLineArgs passes []string to terraform SDK directly
```

#### 3. **Helmfile** ✅ NOT AFFECTED

**File**: `cmd/helmfile.go:34`
**Impact**: NONE - Uses same `getConfigAndStacksInfo()` as Terraform

#### 4. **Packer** ✅ NOT AFFECTED

**File**: `cmd/packer.go:37`
**Impact**: NONE - Uses same `getConfigAndStacksInfo()` as Terraform

#### 5. **Auth Exec** ✅ NOT AFFECTED

**File**: `cmd/auth_exec.go:47,181`
**Function**: `extractIdentityFlag()`
**Impact**: NONE - Passes `[]string` directly to `exec.Command()`

```go
// SAFE: Uses array directly
func executeCommandWithEnv(args []string, envVars map[string]string) error {
    cmdName := args[0]
    cmdArgs := args[1:]
    execCmd := exec.Command(cmdPath, cmdArgs...)  // Direct array, no shell!
}
```

#### 6. **Auth Shell** ✅ NOT AFFECTED

**File**: `cmd/auth_shell.go:78,185`
**Function**: `extractAuthShellFlags()`
**Impact**: NONE - Passes `[]string` directly to shell startup

### Current Fragmented Implementations

We have **FIVE different implementations** of argument parsing logic:

1. `cmd/cmd_utils.go:507` - `extractTrailingArgs()` (custom commands) ❌ BUGGY
2. `cmd/cmd_utils.go:696` - `getConfigAndStacksInfo()` (terraform/helmfile/packer) ✅ Safe
3. `cmd/auth_exec.go:181` - `extractIdentityFlag()` (auth exec) ✅ Safe
4. `cmd/auth_shell.go:185` - `extractAuthShellFlags()` (auth shell) ✅ Safe
5. `cmd/flag_utils.go` - Custom parsing attempt (now deprecated)

**This fragmentation makes bugs inevitable.**

## Evidence of User Workarounds

While we didn't find extensive examples of users working around this specific bug in documentation, the potential is there:

1. **Custom Commands Documentation** shows the vulnerable `{{ .TrailingArgs }}` pattern
2. Users working with complex shell commands may have encountered issues with:
   - Whitespace preservation
   - Special characters
   - Variable expansion

3. The `--query` flag examples show complex quoting:
   ```bash
   atmos terraform apply --query '.vars.tags.team == "eks"'
   ```
   This works because `--query` is a FLAG (parsed by Cobra), not a trailing arg.

## Solution

### Unified Argument Parsing Utility

**File**: `cmd/args_separator.go`
**Status**: ✅ Implemented and Tested (70+ test cases)

```go
// Unified utility that works for ALL commands
type SeparatedCommandArgs struct {
    BeforeSeparator []string  // Args before --
    AfterSeparator  []string  // Args after --
    SeparatorIndex  int       // Position of -- in os.Args
    HasSeparator    bool      // Whether -- was found
}

func ExtractSeparatedArgs(cmd *cobra.Command, args []string, osArgs []string) *SeparatedCommandArgs
```

### Safe String Conversion Methods

#### Method 1: Array (Preferred)

**When to use**: Passing to `exec.Command()`, SDK calls, any non-shell execution
**Why safe**: Preserves argument boundaries, no re-parsing

```go
separated := ExtractSeparatedArgs(cmd, args, os.Args)
trailingArgs := separated.AfterSeparator  // []string

// Pass directly to exec.Command
execCmd := exec.Command(cmdPath, trailingArgs...)  // SAFE
```

#### Method 2: Shell-Quoted String

**When to use**: Custom commands, shell templates, any shell execution
**Why safe**: Properly quotes each argument using `syntax.Quote()`

```go
separated := ExtractSeparatedArgs(cmd, args, os.Args)
quotedString, err := separated.GetAfterSeparatorAsQuotedString()
// Result: "echo 'hello  world'" (properly quoted!)

// Use in template
data := map[string]any{
    "TrailingArgs": quotedString,  // SAFE - shell will parse correctly
}
```

**Implementation**:

```go
func (s *SeparatedCommandArgs) GetAfterSeparatorAsQuotedString() (string, error) {
    if !s.HasSeparator || len(s.AfterSeparator) == 0 {
        return "", nil
    }

    var quotedArgs []string
    for _, arg := range s.AfterSeparator {
        // Use mvdan.cc/sh/v3/syntax.Quote for proper shell quoting
        quoted, err := syntax.Quote(arg, syntax.LangBash)
        if err != nil {
            return "", fmt.Errorf("failed to quote argument %q: %w", arg, err)
        }
        quotedArgs = append(quotedArgs, quoted)
    }

    return strings.Join(quotedArgs, " "), nil
}
```

**Quoting Examples**:

| Input Args | Naive Join | Shell-Quoted |
|------------|-----------|--------------|
| `["echo", "hello  world"]` | `"echo hello  world"` ❌ | `"echo 'hello  world'"` ✅ |
| `["echo", "$VAR"]` | `"echo $VAR"` ❌ (expands!) | `"echo '$VAR'"` ✅ (literal) |
| `["echo", "foo;bar"]` | `"echo foo;bar"` ❌ (injection!) | `"echo 'foo;bar'"` ✅ (safe) |
| `["echo", ""]` | `"echo "` ❌ (lost arg) | `"echo ''"` ✅ (preserved) |
| `["echo", "line1\nline2"]` | `"echo line1\nline2"` ❌ | `"echo $'line1\nline2'"` ✅ |

#### Method 3: Unsafe String (Deprecated)

**When to use**: NEVER for new code. Exists only for backwards compatibility.
**Why unsafe**: Loses argument boundaries and exposes security risks

```go
// DO NOT USE THIS FOR NEW CODE
unquoted := separated.GetAfterSeparatorAsString()  // strings.Join without quoting
```

This method is marked with a deprecation warning in code comments.

## Implementation Plan

### Phase 1: Fix Custom Commands (CRITICAL) ✅ Ready

**Impact**: Fixes security vulnerability and data corruption

1. Update `cmd/cmd_utils.go:411`:
   ```go
   // OLD (BUGGY):
   data := map[string]any{
       "TrailingArgs": trailingArgs,  // strings.Join result
   }

   // NEW (FIXED):
   separated := ExtractSeparatedArgs(cmd, args, os.Args)
   quotedTrailing, err := separated.GetAfterSeparatorAsQuotedString()
   errUtils.CheckErrorPrintAndExit(err, "", "")

   data := map[string]any{
       "TrailingArgs": quotedTrailing,  // Properly quoted!
   }
   ```

2. Delete `extractTrailingArgs()` function (line 507-531)

### Phase 2: Consolidate All Parsing ✅ Ready

Replace all custom parsing with `ExtractSeparatedArgs()`:

1. **Terraform/Helmfile/Packer**: Replace inline parsing in `getConfigAndStacksInfo()`
   ```go
   // OLD:
   doubleDashIndex := lo.IndexOf(args, "--")
   if doubleDashIndex > 0 {
       finalArgs = lo.Slice(args, 0, doubleDashIndex)
       argsAfterDoubleDash = lo.Slice(args, doubleDashIndex+1, len(args))
   }

   // NEW:
   separated := ExtractSeparatedArgs(cmd, args, args)
   finalArgs = separated.BeforeSeparator
   argsAfterDoubleDash = separated.AfterSeparator
   ```

2. **Auth Exec**: Replace `extractIdentityFlag()` with unified approach
   - Keep identity flag parsing logic
   - Use `ExtractSeparatedArgs()` for separator handling
   - Delete custom `extractIdentityFlag()` function

3. **Auth Shell**: Replace `extractAuthShellFlags()` with unified approach
   - Keep flag parsing logic
   - Use `ExtractSeparatedArgs()` for separator handling
   - Delete custom `extractAuthShellFlags()` function

### Phase 3: Update Documentation 📝 Required

1. **Custom Commands Guide**:
   ```markdown
   ### Trailing Arguments

   ⚠️ **Security Note**: When using `{{ .TrailingArgs }}` in shell commands,
   Atmos automatically quotes each argument for shell safety. This preserves:
   - Whitespace within arguments
   - Special characters
   - Empty strings

   Example:
   ```yaml
   steps:
     - "my-script {{ .TrailingArgs }}"
   ```

   When you run:
   ```bash
   atmos mycmd -- arg1 "hello  world" '$VAR'
   ```

   The template expands to:
   ```bash
   my-script arg1 'hello  world' '$VAR'
   ```

   Each argument is properly quoted to preserve its original value.
   ```

2. **Migration Guide** for users who may have worked around the bug

## Test Coverage

✅ **70+ test cases** covering:

- **Baseline behavior**: Documents current parsing for all commands
- **Edge cases**: Whitespace, special chars, unicode, empty args, very long args
- **Security**: Command injection scenarios
- **Integration**: Actual shell parser behavior validation
- **Quoting**: Comprehensive shell quoting tests

### Key Test Files

1. `cmd/args_separator_test.go` - Core utility tests
2. `cmd/terraform_separator_test.go` - Terraform baseline (14 cases)
3. `cmd/helmfile_separator_test.go` - Helmfile baseline (8 cases)
4. `cmd/packer_separator_test.go` - Packer baseline (8 cases)
5. `cmd/custom_commands_separator_test.go` - Custom commands (20 cases)
6. `cmd/custom_commands_whitespace_bug_test.go` - Bug demonstration
7. `cmd/integration_shell_parsing_test.go` - Shell parser integration

## Breaking Changes

### For Custom Commands

**Potential Impact**: LOW - This is a bug fix that makes behavior CORRECT

**Scenario**: If users have worked around the bug by doing manual quoting:

```yaml
# If users did this as workaround:
steps:
  - "echo {{ .TrailingArgs }}"  # They might have been manually escaping

# And ran:
atmos mycmd -- 'hello\ \ world'  # Manual escaping for double space

# After fix, this would be double-quoted
```

**Mitigation**:
1. Add migration guide to release notes
2. Provide examples of old vs new behavior
3. Offer `TrailingArgsRaw` for backward compatibility (if needed)

### For Other Commands

**Impact**: NONE - Terraform/Helmfile/Packer/Auth already use safe array-based approach

## Success Metrics

1. ✅ All 70+ tests pass
2. ✅ No command injection vulnerabilities in custom commands
3. ✅ Whitespace and special characters preserved correctly
4. ✅ Single unified implementation (reduced from 5 to 1)
5. ✅ Comprehensive documentation of the pattern

## Security Assessment

### Before Fix

- **Severity**: HIGH
- **CVSS**: Potentially 7.5+ (Command Injection)
- **Attack Vector**: Malicious input in custom command trailing args
- **Exploitability**: HIGH - Simple to exploit with shell metacharacters

### After Fix

- **Severity**: NONE
- **Mitigation**: Complete - All arguments properly quoted
- **Defense in Depth**: Shell parser sees properly quoted strings

## References

1. Shell Quoting Library: `mvdan.cc/sh/v3/syntax`
2. Go Template Processing: `internal/exec/template_funcs.go`
3. Shell Execution: `pkg/utils/shell_utils.go`
4. Command Registry Pattern: `docs/prd/command-registry-pattern.md`

## Lessons Learned

1. **Never use `strings.Join()` for shell arguments** - Always use proper shell quoting
2. **Avoid fragmentation** - Unified utilities prevent bugs
3. **Test with edge cases** - Whitespace, special chars, unicode, injection attempts
4. **Document security implications** - Make the "why" clear for future developers
5. **Integration tests matter** - Testing with actual shell parser caught the bug

## Appendix: Shell Quoting Reference

| Character | Risk | Unquoted Behavior | Quoted Behavior |
|-----------|------|-------------------|-----------------|
| Space | Data loss | Word separator | Preserved |
| Tab | Data loss | Word separator | Preserved |
| `;` | Command injection | Command separator | Literal semicolon |
| `\|` | Command injection | Pipe to command | Literal pipe |
| `&` | Command injection | Background execution | Literal ampersand |
| `$` | Variable expansion | Expands `$VAR` | Literal dollar sign |
| `` ` `` | Command substitution | Executes command | Literal backtick |
| `'` | Quote mismatch | Starts/ends quote | Escaped or in double quotes |
| `"` | Quote mismatch | Starts/ends quote | Escaped or in single quotes |
| `\` | Escape sequence | Escapes next char | Literal backslash (in single quotes) |

**Golden Rule**: When in doubt, use `syntax.Quote()` before passing to shell.
