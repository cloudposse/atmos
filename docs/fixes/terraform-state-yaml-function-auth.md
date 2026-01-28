# Issue: !terraform.state YAML function not using internal authentication

**Date:** 2026-01-28

## Problem

The `!terraform.state` YAML function does not use Atmos internal authentication (AWS SSO)
when running certain commands:

- `atmos terraform plan --all -s <stack>`
- `atmos terraform shell <component> -s <stack>`

This results in errors like:
```
Error: failed to read Terraform state for component vpc in stack core-network
in YAML function: !terraform.state vpc ".vpc_id // \"vpc\""
```

## Working Scenarios

- `atmos terraform plan <component> -s <stack>` - Works correctly with authentication
- `!terraform.output` YAML function - Works correctly with authentication

## Non-Working Scenarios

- `atmos terraform plan --all -s <stack>` - Authentication not passed to !terraform.state
- `atmos terraform shell <component> -s <stack>` - Authentication not passed to !terraform.state

## Root Cause Analysis

Two issues were identified:

1. **Missing AuthManager propagation in ProcessComponentConfig**: In `internal/exec/utils.go`,
   the `ProcessComponentConfig` function only set `AuthContext` from the authManager, but did
   not store the `AuthManager` itself on `configAndStacksInfo`. This meant that YAML functions
   like `!terraform.state` could not access the AuthManager for authentication.

2. **Missing authentication setup in terraform shell**: The `ExecuteTerraformShell` function
   in `internal/exec/terraform_shell.go` was calling `ProcessStacks` with `nil` as the
   authManager parameter. This meant no authentication context was available when processing
   YAML functions during the shell command.

The `!terraform.state` function needs to read from the S3 backend, which requires AWS credentials.
When Atmos internal authentication is configured, these credentials should be obtained through
the AuthManager, but this context was not being propagated in certain code paths.

## Fix

### 1. Fixed AuthManager propagation in ProcessComponentConfig (`internal/exec/utils.go`)

Set `configAndStacksInfo.AuthManager = authManager` alongside setting `AuthContext`:

```go
// Populate AuthContext and AuthManager from AuthManager if provided (from --identity flag).
// The AuthManager is stored for YAML functions like !terraform.state that need the full
// authentication context to access remote state.
if authManager != nil {
    configAndStacksInfo.AuthManager = authManager
    managerStackInfo := authManager.GetStackInfo()
    if managerStackInfo != nil && managerStackInfo.AuthContext != nil {
        configAndStacksInfo.AuthContext = managerStackInfo.AuthContext
    }
}
```

### 2. Added authentication support to terraform shell command

- Added `Identity` field to `ShellOptions` struct (`pkg/terraform/options.go`)
- Added `--identity` flag support to shell command (`cmd/terraform/shell.go`)
- Added `createShellAuthManager` function in `ExecuteTerraformShell` (`internal/exec/terraform_shell.go`)
  that creates and authenticates an AuthManager before calling `ProcessStacks`

## Files Changed

- `internal/exec/utils.go` - Added `configAndStacksInfo.AuthManager = authManager` in ProcessComponentConfig
- `internal/exec/terraform_shell.go` - Added authentication setup before ProcessStacks call
- `pkg/terraform/options.go` - Added `Identity` field to ShellOptions struct
- `cmd/terraform/shell.go` - Added identity flag handling and passing to ShellOptions

## Testing

The fix ensures that:
1. The `--identity` flag works with `atmos terraform shell` command
2. YAML functions like `!terraform.state` can access authenticated credentials
3. Existing authentication behavior is preserved for other commands
