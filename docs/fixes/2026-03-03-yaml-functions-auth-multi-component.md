# Fix: YAML functions and Go templates not using Atmos authentication in multi-component execution

**Date:** 2026-03-03

## Issue

[GitHub Issue #2081](https://github.com/cloudposse/atmos/issues/2081)

## Problem

When running `atmos terraform plan --all -s <stack>`, the `!terraform.state` YAML function fails
to read Terraform state from S3 because it does not use Atmos-managed authentication (SSO credentials).
The same command works correctly for single components: `atmos terraform plan <component> -s <stack>`.

### Error

```text
WARN  Failed to read Terraform state after all retries exhausted
  error="operation error S3: GetObject, get identity: get credentials: failed to refresh cached credentials,
  no EC2 IMDS role found"
```

### Log Comparison

**Single component (working):**

```text
DEBU  Using Atmos auth context for AWS SDK profile=platformops
```

**Entire stack --all (broken):**

```text
DEBU  Using standard AWS SDK credential resolution (no auth context provided)
```

## Root Cause

In `internal/exec/terraform_query.go`, the `ExecuteTerraformQuery` function (which handles `--all`
and multi-component execution) calls `ExecuteDescribeStacks` with `nil` for the `authManager`
parameter:

```go
stacks, err := ExecuteDescribeStacks(
    &atmosConfig,
    info.Stack,
    info.Components,
    []string{cfg.TerraformComponentType},
    nil,
    false,
    info.ProcessTemplates,
    info.ProcessFunctions,
    false,
    info.Skip,
    nil, // AuthManager - not needed for terraform query  <-- BUG
)
```

When `processYamlFunctions` is `true`, `ExecuteDescribeStacks` processes all YAML functions
including `!terraform.state` to fully resolve component configurations. Without an AuthManager,
the `configAndStacksInfo.AuthContext` is never populated (see `describe_stacks.go` lines 367-372),
so the S3 backend read uses standard AWS SDK credential resolution (IMDS, env vars) instead of
the configured SSO credentials.

### Execution Flow (Buggy)

1. User runs `atmos terraform plan --all -s test`
2. `terraformRunWithOptions` routes to `ExecuteTerraformQuery(&info)`
3. `ExecuteTerraformQuery` calls `ExecuteDescribeStacks(authManager=nil)`
4. `ExecuteDescribeStacks` processes `!terraform.state` YAML functions
5. `processTagTerraformStateWithContext` extracts nil auth from stackInfo
6. `GetTerraformState` → S3 read fails: no IMDS, no SSO credentials

### Single Component Flow (Working)

1. User runs `atmos terraform plan vpn -s test`
2. `terraformRunWithOptions` routes to `executeSingleComponent` → `ExecuteTerraform`
3. `ExecuteTerraform` creates AuthManager with SSO credentials (line 163)
4. `ProcessStacks` is called with the AuthManager
5. `ProcessComponentConfig` sets `AuthContext` from AuthManager (line 196)
6. YAML functions receive auth context → S3 read succeeds

## Fix

Create an AuthManager in `ExecuteTerraformQuery` before calling `ExecuteDescribeStacks`,
using the same pattern as `ExecuteTerraform`:

```go
// Create auth manager for YAML function processing during stack description.
mergedAuthConfig := auth.CopyGlobalAuthConfig(&atmosConfig.Auth)
authManager, err := auth.CreateAndAuthenticateManagerWithAtmosConfig(
    info.Identity, mergedAuthConfig, cfg.IdentityFlagSelectValue, &atmosConfig,
)
// ... error handling ...

stacks, err := ExecuteDescribeStacks(
    // ... same params ...
    authManager, // Pass auth manager instead of nil
)
```

Also store the auth manager in `info.AuthManager` so that downstream operations
(e.g., `processTerraformComponent` → `ExecuteTerraform`) can access it.

The `createQueryAuthManager` helper function encapsulates this logic and follows
the same pattern as `ExecuteTerraform` for auth manager creation. When auth is not
configured, it returns `nil, nil` (backward compatible).

Additionally, inject the auth resolver into identity-aware stores via
`atmosConfig.Stores.SetAuthContextResolver(authbridge.NewResolver(authManager, info))`,
matching the same pattern as `ExecuteTerraform`. This enables `!store` and `!store.get`
YAML functions to lazily resolve credentials on first access.

## Affected Functions

All YAML functions and Go template functions that use auth context are now fixed:

| Function                          | Uses AuthContext          | Uses AuthManager          | Fixed |
|-----------------------------------|---------------------------|---------------------------|-------|
| `!terraform.state`                | Yes (S3 reads)            | Yes (nested auth)         | Yes   |
| `!terraform.output`               | Yes (S3 reads)            | Yes (nested auth)         | Yes   |
| `!aws.account_id`                 | Yes (STS calls)           | No                        | Yes   |
| `!aws.caller_identity_arn`        | Yes (STS calls)           | No                        | Yes   |
| `!aws.caller_identity_user_id`    | Yes (STS calls)           | No                        | Yes   |
| `!aws.region`                     | Yes (STS calls)           | No                        | Yes   |
| `!aws.organization_id`            | Yes (Orgs calls)          | No                        | Yes   |
| `!store`                          | Via `authbridge.Resolver` | Via `authbridge.Resolver` | Yes   |
| `!store.get`                      | Via `authbridge.Resolver` | Via `authbridge.Resolver` | Yes   |
| `atmos.Component()` (Go template) | Yes (terraform output)    | Via wrapper               | Yes   |

## Files Changed

| File                                    | Change                                                                                                                   |
|-----------------------------------------|--------------------------------------------------------------------------------------------------------------------------|
| `internal/exec/terraform_query.go`      | Add `createQueryAuthManager` helper; call it before `ExecuteDescribeStacks`; add `SetAuthContextResolver` for store auth |
| `internal/exec/describe_stacks.go`      | Propagate `AuthManager` (not just `AuthContext`) on `configAndStacksInfo` for all 4 component types                      |
| `internal/exec/terraform_query_test.go` | Add 8 unit tests for auth context propagation                                                                            |

## Testing

- `TestCreateQueryAuthManager_NoAuthConfigured` - nil manager when no auth configured
- `TestCreateQueryAuthManager_ErrorReturned` - error returned when auth fails
- `TestCreateQueryAuthManager_EmptyIdentity` - auto-detect mode works correctly
- `TestDescribeStacksAuthPropagation` - both AuthContext and AuthManager flow through to `!terraform.state`
- `TestDescribeStacksNoAuthPropagation_Nil` - documents pre-fix behavior (nil auth when no manager)
- `TestTerraformOutputAuthPropagation` - auth context and auth manager flow through to `!terraform.output`
- `TestTerraformOutputNoAuth` - backward compatibility with nil auth for `!terraform.output`
- `TestMultipleYamlFunctionsAuthPropagation` - combined `!terraform.state` and `!terraform.output` in same input
- All existing tests pass (`go test ./internal/exec/ -count=1`)
- Full build passes (`go build ./...`)

## Status

- [x] Issue analyzed and confirmed
- [x] Tests added
- [x] Fix implemented
- [x] Tests passing
- [x] Fix verified
