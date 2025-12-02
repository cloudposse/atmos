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

### Approach: Pre-scan Stack Auth Defaults

Before creating the `AuthManager`, perform a lightweight scan of stack configurations
to extract auth identity defaults. This avoids full stack processing while still
respecting stack-level default identity configuration.

### Implementation

1. **Add stack auth scanner** (`pkg/config/stack_auth_scanner.go`):
   - Scans stack manifest files for `auth.identities.*.default: true`
   - Uses minimal YAML parsing without template/function processing
   - Returns a map of identity names to their default status

2. **Update `CreateAuthManagerFromIdentity`** (`pkg/auth/manager_helpers.go`):
   - If no identity specified and no default in atmos.yaml
   - Scan stack configs for auth defaults
   - Merge discovered defaults into auth config before creating manager

3. **Merge order** (lowest to highest priority):
   - Atmos config defaults
   - Stack config defaults
   - CLI flag / environment variable

### Key Files Changed

**New Files:**
- `pkg/config/stack_auth_scanner.go` - Scanner for stack-level auth defaults
- `pkg/config/stack_auth_scanner_test.go` - Unit tests for scanner

**Updated CLI Commands:**
- `cmd/describe_component.go` - Uses `CreateAuthManagerFromIdentityWithAtmosConfig`
- `cmd/describe_stacks.go` - Uses `CreateAuthManagerFromIdentityWithAtmosConfig`
- `cmd/describe_affected.go` - Uses `CreateAuthManagerFromIdentityWithAtmosConfig`
- `cmd/describe_dependents.go` - Uses `CreateAuthManagerFromIdentityWithAtmosConfig`

**Updated Internal Execution:**
- `internal/exec/terraform.go` - Uses `CreateAndAuthenticateManagerWithAtmosConfig`
- `internal/exec/terraform_nested_auth_helper.go` - Uses `CreateAndAuthenticateManagerWithAtmosConfig` (for YAML functions)
- `internal/exec/workflow_utils.go` - Scans stack defaults when creating AuthManager

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

### CLI Commands
- `atmos describe component`
- `atmos describe stacks`
- `atmos describe affected`
- `atmos describe dependents`
- `atmos terraform *` (all terraform subcommands)

### YAML Functions
- `!terraform.state` - Now respects stack-level default identity
- `!terraform.output` - Now respects stack-level default identity

### Workflows
- Workflow execution now scans for stack-level defaults when no explicit identity is specified

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
