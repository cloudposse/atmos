# Fix: Auth identity not used for backend state reads in describe affected

**Date:** 2026-03-25

## Problem

When running `atmos list affected --ref refs/heads/main` (or `atmos describe affected`), the command
fails to authenticate to the S3 state bucket even though the user is authenticated via `atmos auth`
with a valid identity. The same identity works correctly with `atmos tf plan`.

### Error

```text
Failed to read Terraform state file from the S3 bucket
  error="operation error S3: GetObject, get identity: get credentials:
  failed to refresh cached credentials, no EC2 IMDS role found,
  operation error ec2imds: GetMetadata, request canceled, context deadline exceeded"
```

### Debug Output Pattern

```text
DEBU  Component has auth config with default identity, creating component-specific AuthManager component=mycomponent stack=mystack
DEBU  Authentication chain discovered identity=account-admin chainLength=2 chain="[my-sso account-admin]"
DEBU  Successfully loaded credentials from identity storage identity=account-admin
DEBU  Created component-specific AuthManager component=mycomponent stack=mystack
DEBU  Using standard AWS SDK credential resolution (no auth context provided)  ← BUG
DEBU  Failed to read Terraform state file from the S3 bucket error="...context deadline exceeded"
```

## Root Cause

Four independent bugs:

### Bug 1: AuthManager not threaded through describe affected

`DescribeAffectedCmdArgs.AuthManager` is created from the `--identity` flag but never passed through
the execution chain to `ExecuteDescribeStacks()`. All intermediate functions (`executeDescribeAffected`,
the three helper functions, `addDependentsToAffected`) lacked an `authManager` parameter and passed
`nil` to `ExecuteDescribeStacks()`.

**Call chain (before fix):**

```
DescribeAffectedCmdArgs.AuthManager (has value)
  → Execute() calls helpers WITHOUT passing AuthManager
    → executeDescribeAffected() has NO authManager parameter
      → ExecuteDescribeStacks(..., nil)  ← dropped here
```

### Bug 2: GetTerraformState ignores resolved AuthManager credentials for backend read

Even when `resolveAuthManagerForNestedComponent()` successfully creates a component-specific
AuthManager (visible in debug logs), the actual S3 backend read used the original `authContext`
parameter (nil) instead of extracting credentials from the resolved AuthManager.

**Code (before fix):**

```go
// terraform_state_utils.go
resolvedAuthMgr, _ := resolveAuthManagerForNestedComponent(...)  // ✓ valid AuthManager
componentSections, _ := ExecuteDescribeComponent(... AuthManager: resolvedAuthMgr ...)  // ✓ used
backend, _ := tb.GetTerraformBackend(atmosConfig, &componentSections, authContext)  // ✗ nil authContext!
```

### Bug 3: No per-component identity resolution in ExecuteDescribeStacks

`ExecuteDescribeStacks` applied a single `authManager` to all components via `propagateAuth()`.
Components with their own `auth:` section defining different identities were not resolved
individually during stack description.

## Fix

### Bug 1 Fix

Added `authManager auth.AuthManager` parameter to the entire describe affected call chain:

- `executeDescribeAffected()` in `describe_affected_utils.go`
- `ExecuteDescribeAffectedWithTargetRefClone()` in `describe_affected_helpers.go`
- `ExecuteDescribeAffectedWithTargetRefCheckout()` in `describe_affected_helpers.go`
- `ExecuteDescribeAffectedWithTargetRepoPath()` in `describe_affected_helpers.go`
- `addDependentsToAffected()` in `describe_affected_utils_2.go`
- Function type fields in `describeAffectedExec` struct
- `Execute()` method passes `a.AuthManager` to all calls
- `terraform_affected_graph.go` passes `args.AuthManager`
- `terraform_affected.go` passes `args.AuthManager`

### Bug 2 Fix

In `GetTerraformState()` (`terraform_state_utils.go`), after resolving the component-specific
AuthManager, extract its AuthContext and use it for the backend read:

```go
resolvedAuthContext := authContext
if resolvedAuthMgr != nil {
    if si := resolvedAuthMgr.GetStackInfo(); si != nil && si.AuthContext != nil {
        resolvedAuthContext = si.AuthContext
    }
}
backend, err := tb.GetTerraformBackend(atmosConfig, &componentSections, resolvedAuthContext)
```

### Bug 3 Fix

In `processComponentEntry()` (`describe_stacks_component_processor.go`), resolve per-component
auth when YAML functions will be processed. Uses the component section data already in-hand
(no extra `ExecuteDescribeComponent` call):

```go
componentAuthManager := p.authManager
if p.processYamlFunctions {
    authSection, hasAuth := componentSection[cfg.AuthSectionName].(map[string]any)
    if hasAuth && hasDefaultIdentity(authSection) {
        resolved, err := createComponentAuthManager(...)
        if err == nil && resolved != nil {
            componentAuthManager = resolved
        }
    }
}
propagateAuth(&info, componentAuthManager)
```

Gated behind `processYamlFunctions` to avoid unnecessary auth resolution when functions aren't
being processed (the only consumer of auth context in this path).

### Bug 4: `list affected` never reads `--identity` flag or creates AuthManager

The `--identity` / `-i` flag IS registered on `listCmd` as a PersistentFlag (inherited by all
subcommands including `list affected`). However, `cmd/list/affected.go` never read the flag
and never created an AuthManager. All three code paths in `executeAffectedLogic()` passed `nil`
for authManager.

**Why `atmos auth shell` worked but `-i` didn't:**
- `atmos auth shell` sets `ATMOS_IDENTITY` env var, picked up by viper fallback
- `-i admin-account` requires the command handler to read the flag and create an AuthManager

### Bug 4 Fix

Read the `--identity` flag in the `list affected` RunE handler (`cmd/list/affected.go`) and
pass the identity name through to `ExecuteListAffectedCmd` (`pkg/list/list_affected.go`),
which creates an AuthManager after config initialization and passes it to all three
`ExecuteDescribeAffectedWith*` helper functions.

```go
// cmd/list/affected.go - read identity flag in RunE
var identityName string
if cmd.Flags().Changed(cfg.IdentityFlagName) {
    identityName, _ = cmd.Flags().GetString(cfg.IdentityFlagName)
} else {
    identityName = v.GetString(cfg.IdentityFlagName)
}
identityName = cfg.NormalizeIdentityValue(identityName)

// pkg/list/list_affected.go - create AuthManager after config init
authManager, err := auth.CreateAndAuthenticateManagerWithAtmosConfig(
    opts.IdentityName, &atmosConfig.Auth, cfg.IdentityFlagSelectValue, &atmosConfig,
)
```

## Files Changed

- `internal/exec/describe_affected.go` — struct types, Execute()
- `internal/exec/describe_affected_helpers.go` — 3 helper function signatures
- `internal/exec/describe_affected_utils.go` — executeDescribeAffected() signature
- `internal/exec/describe_affected_utils_2.go` — addDependentsToAffected()
- `internal/exec/terraform_affected.go` — pass AuthManager to helpers
- `internal/exec/terraform_affected_graph.go` — pass AuthManager
- `internal/exec/terraform_state_utils.go` — use resolved AuthContext for backend read
- `internal/exec/describe_stacks_component_processor.go` — per-component identity resolution
- `internal/exec/atlantis_generate_repo_config.go` — pass nil (no auth context)
- `pkg/ai/tools/atmos/describe_affected.go` — pass nil
- `pkg/list/list_affected.go` — add IdentityName field, create AuthManager, pass to helpers
- `cmd/list/affected.go` — read identity flag, pass IdentityName through opts
- Test files updated for new signatures

## Related

- `docs/fixes/nested-terraform-state-auth-context-propagation.md` — original nested auth fix
- `docs/fixes/2026-03-03-yaml-functions-auth-multi-component.md` — multi-component auth fix
