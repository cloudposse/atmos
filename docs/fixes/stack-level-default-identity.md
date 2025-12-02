# Stack-Level Default Identity Not Recognized

## Issue Summary

When a default identity is configured only in stack config (e.g., `stacks/orgs/vbpt/_defaults`),
the identity is not passed to `describe component` and other commands. Users are prompted to
select an identity even though one is configured as default in their stack configuration.

## Symptoms

```bash
❯ atmos describe component runs-on -s core-usw2-auto
┃ No default identity configured. Please choose an identity:
┃ > identity-1
┃   identity-2
┃   ...
```

Even when stack config has:
```yaml
# stacks/orgs/vbpt/_defaults.yaml
auth:
  identities:
    vbpt-identity:
      default: true
```

## Workaround

Setting the default in profile config works:
```yaml
# profiles/managers/atmos.yaml
auth:
  identities:
    vbpt-identity:
      kind: aws/assume-role
      default: true
      via:
        identity: vbpt-identity/permission-set
      principal:
        assume_role: arn:aws:iam::123456789012:role/managers
```

## Root Cause Analysis

### The Timing Problem

The issue is a **chicken-and-egg problem** in how default identities are resolved:

1. **Command execution flow** (`cmd/describe_component.go`):
   ```go
   // Load atmos configuration - processStacks = FALSE
   atmosConfig, err := cfg.InitCliConfig(..., false)

   // Create AuthManager using ONLY atmos.yaml + profile auth config
   authManager, err := CreateAuthManagerFromIdentity(identityName, &atmosConfig.Auth)
   ```

2. **`InitCliConfig(processStacks=false)`** only loads:
   - System atmos config
   - User atmos config (`~/.atmos/atmos.yaml`)
   - Project atmos config (`atmos.yaml`)
   - Profile configs (e.g., `profiles/managers/atmos.yaml`)

3. **Stack configs are NOT loaded** at this point because:
   - Processing stacks may require authentication (for YAML functions)
   - Authentication requires knowing the identity
   - Identity resolution happens before stack processing

### Code Flow

```
describe component runs-on -s core-usw2-auto
    │
    ▼
InitCliConfig(processStacks=false)  ←── Only loads atmos.yaml + profiles
    │
    ▼
CreateAuthManagerFromIdentity("")
    │
    ▼
autoDetectDefaultIdentity(&atmosConfig.Auth)
    │
    ▼
GetDefaultIdentity(forceSelect=false)
    │
    ▼
for name, identity := range m.config.Identities {
    if identity.Default { ... }  ←── Stack defaults NOT present!
}
    │
    ▼
No defaults found → prompts user (or errors in CI)
```

### Why Profile Config Works

Profile configs are loaded during `InitCliConfig` **before** stacks are processed:
- Profiles are part of the atmos configuration layer
- They're merged into `atmosConfig.Auth` immediately
- The `AuthManager` sees them when checking for defaults

### Why Stack Config Doesn't Work

Stack configs are only processed when `processStacks=true`:
- Stack manifests contain component-specific auth overrides
- They're designed to be processed after base configuration
- The auth section in stack config isn't available during identity resolution

## Solution

Two approaches are used depending on whether a specific component+stack pair is available:

### Approach 1: Component Auth Merge (for commands with specific component+stack)

For commands like `describe component` and `terraform *` where both component and stack are known,
we leverage the **existing stack inheritance and merge functionality**:

1. Call `ExecuteDescribeComponent()` with `ProcessTemplates=false, ProcessYamlFunctions=false, AuthManager=nil`
2. The component config output includes the merged auth section from stack inheritance
3. Merge with global auth using `auth.MergeComponentAuthFromConfig()`
4. Create auth manager with the merged config

This approach is preferred because:
- It uses the existing stack merge logic (no duplication)
- The auth section includes all inherited defaults from stack hierarchy
- Component-level auth overrides are respected

**Example flow in `terraform.go` and `describe_component.go`:**
```go
// 1. Start with global auth config
mergedAuthConfig := auth.CopyGlobalAuthConfig(&atmosConfig.Auth)

// 2. Get component config (includes stack-level auth with default flag)
componentConfig, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
    Component:            component,
    Stack:                stack,
    ProcessTemplates:     false,
    ProcessYamlFunctions: false, // Avoid circular dependency
    AuthManager:          nil,   // No auth manager yet
})

// 3. Merge component auth (including stack defaults) with global auth
if err == nil {
    mergedAuthConfig, err = auth.MergeComponentAuthFromConfig(...)
}

// 4. Create auth manager with fully merged config
authManager, err := CreateAuthManagerFromIdentity(identityName, mergedAuthConfig)
```

### Approach 2: Stack Scanning (for commands without specific component)

For commands that operate on multiple stacks/components (e.g., `describe stacks`, `describe affected`),
we perform a lightweight pre-scan of stack configurations to extract auth identity defaults:

1. **Stack auth scanner** (`pkg/config/stack_auth_scanner.go`):
   - Scans stack manifest files for `auth.identities.*.default: true`
   - Uses minimal YAML parsing without template/function processing
   - Returns a map of identity names to their default status

2. **Merge into auth config** before creating manager:
   - Stack defaults take **precedence** over atmos.yaml defaults
   - Follows Atmos inheritance model (more specific config overrides global)

### Precedence Order (lowest to highest priority)

1. Atmos config defaults (`atmos.yaml`)
2. Stack config defaults (scanned or from component merge)
3. CLI flag (`--identity`) / environment variable (`ATMOS_IDENTITY`)

### Key Files Changed

**New Files:**
- `pkg/config/stack_auth_scanner.go` - Scanner for stack-level auth defaults
- `pkg/config/stack_auth_scanner_test.go` - Unit tests for scanner

**Commands Using Component Auth Merge (Approach 1):**
- `cmd/describe_component.go` - Uses terraform.go pattern with `ExecuteDescribeComponent` + `MergeComponentAuthFromConfig`
- `internal/exec/terraform.go` - Original implementation of this pattern

**Commands Using Stack Scanning (Approach 2):**
- `cmd/describe_stacks.go` - Uses `CreateAuthManagerFromIdentityWithAtmosConfig`
- `cmd/describe_affected.go` - Uses `CreateAuthManagerFromIdentityWithAtmosConfig`
- `cmd/describe_dependents.go` - Uses `CreateAuthManagerFromIdentityWithAtmosConfig`
- `internal/exec/workflow_utils.go` - Scans stack defaults when creating AuthManager

**Updated Internal Execution:**
- `internal/exec/terraform_nested_auth_helper.go` - Uses `CreateAndAuthenticateManagerWithAtmosConfig` (for YAML functions)

**Updated Auth Helpers:**
- `pkg/auth/manager_helpers.go` - New `CreateAndAuthenticateManagerWithAtmosConfig` function
- `cmd/identity_flag.go` - New `CreateAuthManagerFromIdentityWithAtmosConfig` wrapper

## Testing

### Unit Tests
- `pkg/config/stack_auth_scanner_test.go` - Scanner logic tests
- `pkg/auth/manager_helpers_test.go` - Integration with auth manager

### Integration Tests
- Test fixture in `tests/fixtures/scenarios/stack-auth-defaults/`
- CLI test verifying stack-level defaults work without prompting

## Commands/Features Now Supporting Stack-Level Auth Defaults

### CLI Commands Using Component Auth Merge (Approach 1)
- `atmos describe component` - Has specific component+stack
- `atmos terraform *` (all terraform subcommands) - Has specific component+stack

### CLI Commands Using Stack Scanning (Approach 2)
- `atmos describe stacks` - Operates on multiple stacks/components
- `atmos describe affected` - Operates on all affected components
- `atmos describe dependents` - Operates on multiple stacks
- `atmos list instances` - Lists all instances across stacks

### YAML Functions
- `!terraform.state` - Now respects stack-level default identity (uses Approach 2)
- `!terraform.output` - Now respects stack-level default identity (uses Approach 2)

### Workflows
- Workflow execution now scans for stack-level defaults when no explicit identity is specified (uses Approach 2)

### Not Applicable
- `atmos helmfile *` - Currently doesn't use Atmos Auth (uses external credentials)
- `atmos packer *` - Currently doesn't use Atmos Auth (uses external credentials)

## Related Issues

- Commands affected: All describe commands, terraform commands, YAML functions
- Version introduced: v1.200.0 (auth system)

## References

- Auth Manager implementation: `pkg/auth/manager.go`
- Identity flag handling: `cmd/identity_flag.go`
- Config loading: `pkg/config/config.go`
