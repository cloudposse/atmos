# PRD: Disable Identity Flag

## Problem Statement

Users need a way to explicitly disable Atmos identity authentication in specific contexts (e.g., CI/CD environments) where different credential mechanisms are preferred. Currently, when identities are configured in `atmos.yaml`, there are three possible states for the `--identity` flag:

1. **Not provided** (`""`) - Use default identity from configuration
2. **Provided without value** (`"__SELECT__"`) - Show interactive identity selector
3. **Provided with identity name** (`"aws-sso"`) - Use that specific identity

There is **no fourth state** that means "skip authentication entirely and use AWS SDK defaults." This gap forces users to modify configuration files or use complex workarounds.

### Use Case

A developer has Atmos configured with AWS SSO identities for local development:

```yaml
# atmos.yaml
integrations:
  aws:
    auth:
      default:
        identity: aws-sso
```

When running the same components in GitHub Actions, they want to use GitHub's OIDC assume role mechanism instead of Atmos-managed identities. Currently, this requires either:
1. Maintaining separate `atmos.yaml` files for different environments
2. Using complex `yq` commands to remove the auth configuration at runtime
3. Creating environment-specific overrides

## Proposed Solution

Add support for a boolean `false` value for the `--identity` flag and `ATMOS_IDENTITY` environment variable that disables all identity authentication, reverting to standard AWS SDK credential resolution.

### User Interface

#### CLI Flag
```bash
# Disable identity authentication for a single command
atmos terraform plan my-component --stack=dev --identity=false

# Normal usage (uses configured identity)
atmos terraform plan my-component --stack=dev --identity=aws-sso
```

#### Environment Variable
```bash
# Disable identity authentication globally
export ATMOS_IDENTITY=false
atmos terraform plan my-component --stack=dev

# In CI/CD (GitHub Actions example)
- name: Deploy infrastructure
  env:
    ATMOS_IDENTITY: false
  run: atmos terraform apply my-component --stack=prod
```

### Behavior

When `--identity=false` or `ATMOS_IDENTITY=false` is set:

1. **Skip all identity authentication** - Do not attempt to load or use any configured identities
2. **Revert to AWS SDK defaults** - Use standard AWS credential resolution:
   - Environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, etc.)
   - Shared credentials file (`~/.aws/credentials`)
   - IAM role from EC2 instance metadata (IMDS)
   - Web identity token from environment (for OIDC/GitHub Actions)
3. **Apply to all AWS operations** - Affects Terraform, component operations, and any AWS SDK calls
4. **Override configuration** - Takes precedence over `atmos.yaml` auth settings

### Precedence

The identity resolution order should be:

1. `--identity=false` flag (highest priority - disables identity)
2. `--identity=<name>` flag (specific identity)
3. `ATMOS_IDENTITY=false` environment variable (disables identity)
4. `ATMOS_IDENTITY=<name>` environment variable (specific identity)
5. Default identity from `atmos.yaml` configuration
6. No identity (AWS SDK defaults) if no configuration exists

## Current Implementation Analysis

### Existing Identity Flag States

The `--identity` flag currently supports three distinct behaviors (see `pkg/auth/docs/identity-flag-behavior.md`):

#### State 1: Flag Not Provided (Empty String)
```bash
atmos terraform plan mycomponent --stack=dev
# stackInfo.Identity = ""
```
**Behavior:** Calls `authManager.GetDefaultIdentity(false)` to use default identity from config. If no default exists, returns error.

#### State 2: Flag Without Value (Interactive Selector)
```bash
atmos terraform plan mycomponent --stack=dev --identity
# stackInfo.Identity = "__SELECT__" (cfg.IdentityFlagSelectValue)
```
**Behavior:** Calls `authManager.GetDefaultIdentity(true)` which shows interactive selector in TTY, or errors in CI.

#### State 3: Flag With Explicit Value
```bash
atmos terraform plan mycomponent --stack=dev --identity=aws-sso
# stackInfo.Identity = "aws-sso"
```
**Behavior:** Uses specified identity name directly. Errors if identity doesn't exist.

### What's Missing

**State 4: Authentication Explicitly Disabled** - This is what we need to add.

Currently, there is no way to set `stackInfo.Identity` to a value that means "skip authentication." The empty string `""` already has a meaning (use default), and there is no sentinel value for "disabled."

### Implementation Location

Identity resolution happens in two places:

1. **Flag Parsing** (`cmd/identity_flag.go:GetIdentityFromFlags()`) - Extracts flag value from CLI args or env var
2. **Authentication Hook** (`pkg/auth/hooks.go:TerraformPreHook()`) - Calls `resolveTargetIdentityName()` which:
   - Line 83: If `stackInfo.Identity != ""`, uses that value
   - Line 87: If `stackInfo.Identity == ""`, calls `GetDefaultIdentity(false)`
   - Line 92: If no default, returns error

To add disable functionality, we need:
1. Parse `--identity=false` and set `stackInfo.Identity` to a new sentinel value (e.g., `"__DISABLED__"`)
2. In `TerraformPreHook`, check for this sentinel and **return early** before calling `authenticateAndWriteEnv()`

### Edge Cases

#### Case 1: Conflicting Values
```bash
# Flag takes precedence
export ATMOS_IDENTITY=aws-sso
atmos terraform plan --identity=false  # Identity is disabled
```

#### Case 2: Boolean String Variants
Accept common boolean representations:
- `false`, `False`, `FALSE`
- `0`
- `no`, `No`, `NO`
- `off`, `Off`, `OFF`

All other values are treated as identity names.

#### Case 3: Empty String
```bash
# Empty string is treated as "no identity specified"
# Falls back to next precedence level
atmos terraform plan --identity=""
```

#### Case 4: Multiple Identities Configured
When multiple identities are configured and `--identity=false`:
```yaml
integrations:
  aws:
    auth:
      default:
        identity: aws-sso
      prod:
        identity: aws-prod-sso
```

All identities are bypassed, regardless of context.

## Implementation Details

### Configuration Schema

No changes to `atmos.yaml` schema required. The flag/environment variable accepts:
- Identity name (string): `"aws-sso"`, `"production"`, etc.
- Disable flag (boolean/string): `false`, `"false"`, `"0"`, `"no"`, `"off"`

### Code Changes

1. **Flag Parsing** (`cmd/root.go`)
   - Update `--identity` flag to accept both string and boolean-like values
   - Add validation for boolean representations

2. **Environment Variable Binding** (`cmd/root.go`)
   - Ensure `ATMOS_IDENTITY` binding handles boolean values

3. **Identity Resolution** (`pkg/aws/auth.go` or similar)
   - Check for disable flag before loading identities
   - Skip identity configuration when disabled
   - Return early to allow AWS SDK default resolution

4. **Validation** (`pkg/validate/`)
   - Validate that `false` value is properly recognized
   - Ensure no errors when identities exist but are disabled

### Backward Compatibility

This change is **fully backward compatible**:

- Existing configurations continue to work unchanged
- Existing `--identity=<name>` usage is unaffected
- Only adds new functionality for boolean `false` value
- No breaking changes to API or configuration schema

## Testing Strategy

### Unit Tests

```go
// pkg/aws/auth_test.go
func TestIdentityDisabledViaFlag(t *testing.T) {
    // Test --identity=false disables authentication
}

func TestIdentityDisabledViaEnvVar(t *testing.T) {
    // Test ATMOS_IDENTITY=false disables authentication
}

func TestIdentityPrecedence(t *testing.T) {
    // Test flag overrides environment variable
}

func TestBooleanVariants(t *testing.T) {
    // Test false, False, FALSE, 0, no, off
}
```

### Integration Tests

```bash
# Test in CI environment
export ATMOS_IDENTITY=false
atmos terraform plan test-component --stack=dev
# Should use AWS SDK default credentials, not SSO
```

### Manual Testing

1. Configure identity in `atmos.yaml`
2. Run with `--identity=false`, verify AWS SDK credentials used
3. Run with `ATMOS_IDENTITY=false`, verify same behavior
4. Run normally, verify identity authentication still works

## Documentation

### Updates Required

1. **CLI Reference** (`website/docs/cli/commands/`)
   - Document `--identity=false` usage
   - Add examples for CI/CD scenarios

2. **Authentication Guide** (`website/docs/core-concepts/stacks/authentication.md`)
   - Add section on disabling identities
   - Provide GitHub Actions example
   - Explain use cases (local vs CI)

3. **Configuration Reference** (`website/docs/cli/configuration/`)
   - Document `ATMOS_IDENTITY=false` environment variable
   - Explain precedence order

4. **Migration Guide**
   - Help users migrate from `yq` workarounds
   - Show before/after examples

### Example Documentation

```markdown
## Disabling Identity Authentication

In some scenarios (e.g., CI/CD environments), you may want to disable Atmos-managed
identity authentication and use standard AWS credential resolution instead.

### Using CLI Flag

```bash
atmos terraform plan my-component --stack=prod --identity=false
```

### Using Environment Variable

```bash
export ATMOS_IDENTITY=false
atmos terraform apply my-component --stack=prod
```

### GitHub Actions Example

```yaml
- name: Deploy with GitHub OIDC
  env:
    ATMOS_IDENTITY: false
  run: |
    atmos terraform apply my-component --stack=prod
```

When identity is disabled, Atmos uses standard AWS SDK credential resolution order.
```

## Success Criteria

1. ✅ Users can disable identity authentication via `--identity=false`
2. ✅ Users can disable identity authentication via `ATMOS_IDENTITY=false`
3. ✅ Flag takes precedence over environment variable
4. ✅ AWS SDK default credential resolution works when identity disabled
5. ✅ Existing identity configurations continue to work unchanged
6. ✅ Boolean variants (`false`, `0`, `no`, `off`) are recognized
7. ✅ Documentation covers CI/CD use cases
8. ✅ Tests validate all scenarios
9. ✅ No breaking changes to existing functionality

## Open Questions

1. **Should we support identity disabling at the configuration level?**
   - e.g., `enabled: false` in `atmos.yaml`
   - Current proposal: No, flag/env var provides sufficient control

2. **Should we add a `--no-identity` flag alias?**
   - e.g., `atmos terraform plan --no-identity`
   - Current proposal: No, `--identity=false` is clear and consistent

3. **Should we warn users when identity is disabled?**
   - Current proposal: No warning, as this is intentional behavior
   - Could add debug-level logging: `Identity authentication disabled, using AWS SDK defaults`

## Timeline

- **Phase 1: Implementation** (1-2 days)
  - Flag parsing and validation
  - Identity resolution logic
  - Unit tests

- **Phase 2: Testing** (1 day)
  - Integration tests
  - Manual testing in CI/CD scenarios
  - Edge case validation

- **Phase 3: Documentation** (1 day)
  - CLI reference updates
  - Authentication guide updates
  - Examples and migration guidance

**Total Estimated Effort:** 3-4 days

## Related Work

- [RFC: Atmos Profiles](https://github.com/cloudposse/atmos/pull/1752) - Long-term solution for environment-specific configurations
- Identity authentication system - Current implementation that needs disable capability

## Future Enhancements

This feature provides an **interim solution** while the broader Atmos Profiles feature is developed. Once profiles are implemented, users will be able to define environment-specific configurations more comprehensively:

```yaml
# Future: Profiles approach
profiles:
  local:
    integrations:
      aws:
        auth:
          default:
            identity: aws-sso

  ci:
    integrations:
      aws:
        auth:
          default:
            identity: false  # Or omit entirely
```

The disable flag will remain useful for ad-hoc overrides even after profiles are implemented.
