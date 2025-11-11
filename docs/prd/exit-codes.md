# PRD: Exit Code Strategy

## Overview

This document defines Atmos's exit code strategy to ensure consistent, predictable behavior across all commands and clear communication of error types to users, scripts, and CI/CD systems.

## Background

Exit codes are the primary mechanism for programmatic error detection in command-line tools. Different exit codes communicate different types of failures, enabling automation systems to make informed decisions about error handling and retry strategies.

### Current State

- Most Atmos errors previously returned exit code 1 (general error)
- No consistent distinction between error types
- Subprocess exit codes were sometimes lost or normalized
- Scripts couldn't distinguish between usage errors and system failures

### Goals

1. **Consistency**: Predictable exit codes across all Atmos commands
2. **Standards Compliance**: Follow Unix/POSIX/Linux exit code conventions
3. **Subprocess Transparency**: Preserve exit codes from subprocesses (terraform, packer, helmfile)
4. **Error Classification**: Enable programmatic distinction between error types
5. **CI/CD Friendly**: Support automated retry and failure handling strategies

## Exit Code Standards

### POSIX/Unix Conventions

Atmos follows POSIX.1-2017 and Linux Standard Base (LSB) conventions:

- **0**: Success (command completed without errors)
- **1**: General error (catch-all for errors not fitting other categories)
- **2**: Usage/misuse error (invalid arguments, missing required flags, configuration errors)
- **126**: Command cannot execute (permission denied, not executable)
- **127**: Command not found
- **128+N**: Fatal signal N (e.g., 130 = killed by SIGINT/Ctrl+C)

### References

- [POSIX Exit Status](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/V3_chap02.html#tag_18_08_02)
- [Linux Standard Base - Exit Codes](https://refspecs.linuxfoundation.org/LSB_5.0.0/LSB-Core-generic/LSB-Core-generic/iniscrptact.html)
- [sysexits.h](https://man.openbsd.org/sysexits) - BSD exit codes
- [Bash Exit Codes](https://www.gnu.org/software/bash/manual/html_node/Exit-Status.html)

## Atmos Exit Code Mapping

### Exit Code 0: Success

**When to use**: Command completed successfully without errors.

**Examples**:
```bash
$ atmos terraform plan component -s stack
# Plan generated successfully
$ echo $?
0

$ atmos describe component vpc -s dev
# Component described successfully
$ echo $?
0
```

### Exit Code 1: General Error

**When to use**:
- System-level errors (file I/O, network failures, OS errors)
- Internal errors (panics, assertion failures)
- External service failures (API errors, authentication failures)
- Unexpected runtime errors

**Examples**:
```go
// Network failure
return errUtils.Build(fmt.Errorf("%w: failed to connect to API",
    errUtils.ErrAPIConnectionFailed,
)).
    WithHint("Check your network connection").
    WithExitCode(1).
    Err()

// File I/O error
return errUtils.Build(fmt.Errorf("%w: failed to read file %s",
    errUtils.ErrReadFile, path,
)).
    WithExitCode(1).
    Err()
```

**CLI Examples**:
```bash
$ atmos terraform plan component -s stack
# Network timeout connecting to remote backend
$ echo $?
1

$ atmos describe stacks
# Failed to read stack file due to permission error
$ echo $?
1
```

### Exit Code 2: Usage/Configuration Error

**When to use**:
- Missing required arguments or flags
- Invalid component/stack names
- Configuration errors (invalid YAML, schema violations)
- Locked/abstract components being provisioned
- Missing required configuration files
- Policy violations (OPA, JSON Schema)
- Invalid workflow definitions

**Examples**:
```go
// Missing required flag
return errUtils.Build(errUtils.ErrMissingStack).
    WithHint("Specify stack using `--stack <stack>` (shorthand: `-s`)").
    WithExitCode(2).
    Err()

// Locked component
return errUtils.Build(fmt.Errorf("%w: component cannot be modified",
    errUtils.ErrLockedComponent,
)).
    WithContext("component", component).
    WithHint("Remove `metadata.locked: true` to enable modifications").
    WithExitCode(2).
    Err()

// Invalid configuration
return errUtils.Build(fmt.Errorf("%w: schema validation failed",
    errUtils.ErrValidation,
)).
    WithHint("Fix validation errors in your configuration").
    WithExitCode(2).
    Err()
```

**CLI Examples**:
```bash
$ atmos terraform plan component
# Error: missing required flag --stack
$ echo $?
2

$ atmos terraform deploy locked-component -s prod
# Error: component is locked and cannot be modified
$ echo $?
2

$ atmos validate stacks
# Error: schema validation failed
$ echo $?
2
```

## Subprocess Exit Code Preservation

**Critical Rule**: When Atmos invokes subprocesses (terraform, packer, helmfile, etc.), **always preserve the subprocess exit code**.

### Implementation

```go
// ✅ CORRECT: Preserve subprocess exit code
func ExecuteShellCommand(cmd string, args []string) error {
    execCmd := exec.Command(cmd, args...)
    err := execCmd.Run()

    if err != nil {
        // Check if it's an exit error
        var exitErr *exec.ExitError
        if errors.As(err, &exitErr) {
            // Preserve the subprocess exit code
            return errUtils.WithExitCode(err, exitErr.ExitCode())
        }
        // Other errors (command not found, etc.) use default
        return err
    }
    return nil
}
```

```go
// ❌ WRONG: Overriding subprocess exit code
func ExecuteShellCommand(cmd string, args []string) error {
    execCmd := exec.Command(cmd, args...)
    err := execCmd.Run()

    if err != nil {
        // BAD: Overrides terraform's exit code with 1
        return fmt.Errorf("%w: terraform failed", errUtils.ErrTerraformFailed)
    }
    return nil
}
```

### Subprocess Exit Code Examples

#### Terraform Exit Codes
Terraform uses standard exit codes:
- `0`: Success
- `1`: Error (general)
- `2`: Usage error (invalid arguments)

```bash
$ atmos terraform plan vpc -s dev
# Terraform exits with 2 (invalid var file)
$ echo $?
2  # ✅ Preserved

$ atmos terraform apply vpc -s dev
# Terraform exits with 1 (provider authentication failed)
$ echo $?
1  # ✅ Preserved
```

#### Packer Exit Codes
```bash
$ atmos packer validate bastion -s dev
# Packer exits with 1 (invalid template)
$ echo $?
1  # ✅ Preserved
```

#### Helmfile Exit Codes
```bash
$ atmos helmfile apply app -s dev
# Helmfile exits with 1 (k8s connection failed)
$ echo $?
1  # ✅ Preserved
```

### Error Wrapping with Exit Code Preservation

When wrapping subprocess errors with additional context, preserve the exit code:

```go
err := ExecuteShellCommand("terraform", args)
if err != nil {
    // Extract original exit code
    originalExitCode := errUtils.GetExitCode(err)

    // Wrap with context while preserving exit code
    return errUtils.Build(fmt.Errorf("%w: terraform execution failed", err)).
        WithContext("component", component).
        WithContext("stack", stack).
        WithHint("Check terraform output above for details").
        WithExitCode(originalExitCode).  // ✅ Preserve original exit code
        Err()
}
```

## Special Cases

### Exit Code 130: User Interrupt (SIGINT)

When a user presses Ctrl+C, the process receives SIGINT (signal 2), resulting in exit code 128+2=130.

**Implementation**: Let the OS handle this naturally. Don't catch SIGINT unless you have cleanup tasks.

```go
// Let OS handle SIGINT naturally
// Exit code will be 130 automatically
```

### Exit Code from --help

**Standard**: `--help` should exit with code 0 (success).

**Rationale**: Help is not an error condition; the user requested information and received it successfully.

```bash
$ atmos terraform --help
# Help text displayed
$ echo $?
0  # ✅ Success
```

### Exit Code from Usage Errors

**Standard**: Usage errors (invalid flags, missing arguments) should exit with code 2.

```bash
$ atmos terraform plan --invalid-flag
# Error: unknown flag --invalid-flag
$ echo $?
2  # ✅ Usage error
```

## Testing Strategy

### Unit Tests

Test exit code extraction and wrapping:

```go
func TestExitCodePreservation(t *testing.T) {
    tests := []struct {
        name     string
        err      error
        expected int
    }{
        {
            name:     "nil error returns 0",
            err:      nil,
            expected: 0,
        },
        {
            name:     "general error defaults to 1",
            err:      errors.New("general error"),
            expected: 1,
        },
        {
            name: "custom exit code 2",
            err: errUtils.Build(errors.New("usage error")).
                WithExitCode(2).
                Err(),
            expected: 2,
        },
        {
            name:     "exec.ExitError preserves subprocess code",
            err:      &exec.ExitError{ProcessState: /* exit code 42 */},
            expected: 42,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := errUtils.GetExitCode(tt.err)
            assert.Equal(t, tt.expected, got)
        })
    }
}
```

### Integration Tests

Test actual command exit codes:

```go
func TestCLIExitCodes(t *testing.T) {
    tests := []struct {
        name     string
        args     []string
        expected int
    }{
        {
            name:     "help returns 0",
            args:     []string{"--help"},
            expected: 0,
        },
        {
            name:     "missing stack returns 2",
            args:     []string{"terraform", "plan", "vpc"},
            expected: 2,
        },
        {
            name:     "locked component returns 2",
            args:     []string{"terraform", "deploy", "locked-component", "-s", "prod"},
            expected: 2,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            cmd := exec.Command("atmos", tt.args...)
            err := cmd.Run()

            exitCode := errUtils.GetExitCode(err)
            assert.Equal(t, tt.expected, exitCode)
        })
    }
}
```

## Implementation Checklist

- [x] Implement `WithExitCode()` in error builder
- [x] Implement `GetExitCode()` extraction
- [x] Preserve subprocess exit codes in `ExecuteShellCommand()`
- [ ] Update all usage errors to use exit code 2
- [ ] Update tests to expect correct exit codes
- [ ] Document exit codes in user-facing documentation
- [ ] Add exit code examples to error messages
- [ ] Test subprocess exit code preservation
- [ ] Verify `--help` returns exit code 0

## Migration Guide

### For Developers

**Before**:
```go
return fmt.Errorf("component is locked")
// Implicitly exits with 1
```

**After**:
```go
return errUtils.Build(fmt.Errorf("%w: component is locked",
    errUtils.ErrLockedComponent,
)).
    WithHint("Remove metadata.locked to enable modifications").
    WithExitCode(2).  // ✅ Usage error
    Err()
```

### For Tests

**Before**:
```go
assert.Equal(t, 1, exitCode)  // Expected general error
```

**After**:
```go
assert.Equal(t, 2, exitCode)  // ✅ Expected usage error
```

### For Scripts/CI

Scripts can now distinguish error types:

```bash
#!/bin/bash
atmos terraform plan vpc -s dev

case $? in
    0)
        echo "✅ Plan succeeded"
        ;;
    1)
        echo "❌ System error - notify ops team"
        # Don't retry, likely infrastructure issue
        ;;
    2)
        echo "⚠️  Configuration error - notify dev team"
        # Don't retry, needs human intervention
        ;;
    130)
        echo "🛑 User cancelled"
        ;;
    *)
        echo "❓ Unexpected exit code: $?"
        ;;
esac
```

## Error Categories and Exit Codes

| Error Category | Exit Code | Examples |
|----------------|-----------|----------|
| Success | 0 | Command completed successfully |
| System Errors | 1 | Network failures, file I/O errors, API errors |
| Usage Errors | 2 | Missing flags, invalid config, locked components |
| Subprocess Errors | Varies | Terraform (0-2), Packer (0-1), Helmfile (0-1) |
| User Interrupt | 130 | Ctrl+C (SIGINT) |

## Best Practices

### DO ✅

- **Always preserve subprocess exit codes**
- Use exit code 2 for user/configuration errors
- Use exit code 1 for system/runtime errors
- Return 0 for `--help` and successful operations
- Test exit codes in integration tests
- Document exit codes in user-facing error messages

### DON'T ❌

- Override subprocess exit codes with generic codes
- Use exit code 1 for usage/configuration errors
- Return non-zero for `--help`
- Use arbitrary exit codes (stick to standards)
- Ignore exit codes in tests

## References

- [POSIX.1-2017 Exit Status](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/V3_chap02.html#tag_18_08_02)
- [Linux Standard Base Core Specification](https://refspecs.linuxfoundation.org/LSB_5.0.0/LSB-Core-generic/LSB-Core-generic/iniscrptact.html)
- [Advanced Bash-Scripting Guide - Exit Codes](https://tldp.org/LDP/abs/html/exitcodes.html)
- [FreeBSD sysexits(3)](https://man.freebsd.org/cgi/man.cgi?query=sysexits)
- [GNU Coding Standards - Errors](https://www.gnu.org/prep/standards/html_node/Errors.html)

## Related Documents

- [Error Handling Strategy](./atmos-error-handling.md)
- [Error Builder API](../errors.md)
- [Testing Strategy](./testing-strategy.md)
