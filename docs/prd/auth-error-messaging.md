# PRD: Improved Auth Error Messaging

## Overview

This PRD defines requirements for improving error messaging in `atmos auth` to provide better developer experience when authentication fails. Currently, auth errors lack context about why authentication failed, making troubleshooting difficult.

## Problem Statement

When authentication fails, users receive minimal error messages like:

```
Error: authentication failed: failed to authenticate via credential chain
for identity "plat-dev/terraform": authentication failed: identity=plat-
dev/terraform step=1: authentication failed
```

This error provides no visibility into:
- What command was being executed
- The identity configuration being used
- The current profile (or lack thereof)
- Actionable steps to resolve the issue

## Goals

1. Provide rich, contextual error messages for all auth-related failures
2. Use the standard ErrorBuilder pattern consistently across auth code
3. Include identity configuration, profile info, and actionable hints
4. Cover authentication failures, credential cache issues, and configuration validation errors

## Non-Goals

- Changing authentication logic or flow
- Adding new authentication providers
- Modifying the credential storage system

## Scope

This PRD covers error improvements for three categories:

### 1. Authentication Failures
- Provider authentication failures (AWS SSO, GitHub OIDC, etc.)
- Identity authentication failures (assume-role, permission-set)
- Credential chain failures (multi-step authentication)

### 2. Credential Cache Issues
- Missing credentials (`ErrNoCredentialsFound`)
- Expired credentials (`ErrExpiredCredentials`)
- Invalid credentials
- Credential store initialization failures

### 3. Configuration Validation Errors
- Invalid provider configuration
- Invalid identity configuration
- Missing required fields
- Circular dependencies in identity chains
- Identity/provider not found

## Requirements

### R1: Error Context Requirements

All auth errors MUST include the following context when available:

| Context | Description | Example |
|---------|-------------|---------|
| `identity` | Target identity name | `plat-dev/terraform` |
| `identity_kind` | Identity type | `aws/assume-role` |
| `provider` | Provider name | `aws-sso` |
| `provider_kind` | Provider type | `aws/iam-identity-center` |
| `profiles` | Active profiles or "(not set)" | `ATMOS_PROFILE=devops` |
| `chain_step` | Current step in auth chain | `2 of 3` |

### R2: Identity Configuration Display

When an identity-related error occurs, the error MUST display the full identity configuration in YAML format:

```
Identity configuration:
  plat-dev/terraform:
    kind: aws/assume-role
    via:
      provider: github-oidc
    principal:
      assume_role: arn:aws:iam::344349181611:role/ins-plat-gbl-dev-terraform
```

Special cases:
- If identity not found in config: Display `Identity configuration: (not found in current profile)`
- If identity config is partial: Display available fields only

### R3: Profile Status Display

All auth errors MUST include current profile status:

```
Current profile: ATMOS_PROFILE=devops
```

Or if not set:

```
Current profile: (not set)
```

Multiple profiles should be comma-separated:

```
Current profile: ATMOS_PROFILE=ci,developer
```

### R4: Actionable Hints

Each error type MUST include appropriate hints:

| Error Type | Required Hints |
|------------|----------------|
| Auth failure | "Run `atmos auth --help` for troubleshooting" |
| Expired credentials | "Run `atmos auth login` to refresh credentials" |
| Missing credentials | "Run `atmos auth login` to authenticate" |
| SSO session expired | "Run `atmos auth login --provider <name>` to re-authenticate" |
| Identity not found | "Run `atmos list identities` to see available identities" |
| Provider not found | "Run `atmos list providers` to see available providers" |
| No profile set | "Set ATMOS_PROFILE or use --profile flag" |
| Circular dependency | "Check identity chain configuration for cycles" |

### R5: ErrorBuilder Migration

All auth errors MUST use the ErrorBuilder pattern:

```go
return errUtils.Build(errUtils.ErrAuthenticationFailed).
    WithCause(underlyingErr).
    WithExplanation("Failed to authenticate identity").
    WithHint("Run `atmos auth --help` for troubleshooting").
    WithContext("identity", identityName).
    WithContext("identity_kind", identity.Kind()).
    WithContext("provider", providerName).
    WithContext("profiles", formatProfiles(profilesFromArg)).
    Err()
```

### R6: Error Categories and Sentinels

Use appropriate sentinel errors for each category:

**Authentication Failures:**
- `ErrAuthenticationFailed` - General auth failure
- `ErrPostAuthenticationHookFailed` - Post-auth hook failure

**Credential Issues:**
- `ErrNoCredentialsFound` - Missing credentials
- `ErrExpiredCredentials` - Expired/invalid credentials
- `ErrInitializingCredentialStore` - Store init failure

**Configuration Errors:**
- `ErrInvalidIdentityConfig` - Invalid identity config
- `ErrInvalidProviderConfig` - Invalid provider config
- `ErrIdentityNotFound` - Identity not in config
- `ErrProviderNotFound` - Provider not in config
- `ErrCircularDependency` - Circular chain dependency
- `ErrNoDefaultIdentity` - No default identity configured

**SSO-Specific:**
- `ErrSSOSessionExpired` - SSO session expired
- `ErrSSODeviceAuthFailed` - Device auth failed
- `ErrSSOTokenCreationFailed` - Token creation failed

## Example Error Output

### Example 1: Authentication Failed (Profile Set)

```markdown
# Authentication Error

**Error:** authentication failed

## Explanation

Failed to authenticate via credential chain for identity "plat-dev/terraform".

## Identity Configuration

```yaml
plat-dev/terraform:
  kind: aws/assume-role
  via:
    provider: github-oidc
  principal:
    assume_role: arn:aws:iam::344349181611:role/ins-plat-gbl-dev-terraform
```

## Hints

💡 Run `atmos auth --help` for troubleshooting
💡 Check that the GitHub OIDC provider is correctly configured

## Context

| Key | Value |
|-----|-------|
| identity | plat-dev/terraform |
| provider | github-oidc |
| profiles | ATMOS_PROFILE=devops |
| chain_step | 1 of 2 |
```

### Example 2: Credentials Expired (No Profile)

```markdown
# Authentication Error

**Error:** credentials for identity are expired or invalid

## Explanation

Cached credentials for identity "core-auto/terraform" have expired.

## Identity Configuration

```yaml
core-auto/terraform:
  kind: aws/assume-role
  via:
    provider: aws-sso
  principal:
    assume_role: arn:aws:iam::101071483060:role/ins-core-gbl-auto-terraform
```

## Hints

💡 Run `atmos auth login` to refresh credentials
💡 Set ATMOS_PROFILE or use --profile flag to select a profile

## Context

| Key | Value |
|-----|-------|
| identity | core-auto/terraform |
| provider | aws-sso |
| profiles | (not set) |
| expiration | 2024-01-15T10:30:00Z |
```

### Example 3: Identity Not Found in Profile

```markdown
# Authentication Error

**Error:** identity not found

## Explanation

Identity "plat-sandbox/terraform" not found in the current configuration.

## Identity Configuration

(not found in current profile)

## Hints

💡 Run `atmos list identities` to see available identities
💡 Check that the identity is defined in your auth configuration
💡 Current profile may not include this identity

## Context

| Key | Value |
|-----|-------|
| identity | plat-sandbox/terraform |
| profiles | ATMOS_PROFILE=github-plan |
| available_identities | plat-dev/terraform, plat-prod/terraform, core-auto/terraform |
```

## Implementation Notes

### Helper Functions

Create helper functions for consistent formatting:

```go
// FormatProfiles formats profile information for error display
func FormatProfiles(profiles []string) string {
    if len(profiles) == 0 {
        return "(not set)"
    }
    return "ATMOS_PROFILE=" + strings.Join(profiles, ",")
}

// FormatIdentityConfig formats identity config as YAML for error display
func FormatIdentityConfig(identity types.Identity) string {
    // Marshal identity to YAML format
}

// FormatIdentityNotFound returns placeholder for missing identity
func FormatIdentityNotFound() string {
    return "(not found in current profile)"
}
```

### Files to Modify

| File | Changes |
|------|---------|
| `pkg/auth/manager.go` | Migrate all error returns to ErrorBuilder pattern |
| `pkg/auth/manager_helpers.go` | Add profile context to errors |
| `pkg/auth/credentials/store.go` | Improve credential store errors |
| `pkg/auth/validation/validator.go` | Improve validation error messages |
| `pkg/auth/identities/aws/*.go` | Add identity-specific error context |
| `pkg/auth/providers/aws/*.go` | Add provider-specific error context |
| `pkg/auth/errors.go` (new) | Helper functions for error formatting |

### Testing Requirements

1. Unit tests for each error type with expected context
2. Test that `errors.Is()` works correctly with wrapped errors
3. Test profile formatting (set, not set, multiple)
4. Test identity config formatting (found, not found, partial)

## Success Criteria

1. All auth errors use ErrorBuilder pattern consistently
2. Every auth error includes identity, provider, and profile context
3. Every error includes at least one actionable hint
4. Users can understand why authentication failed from the error message alone
5. All existing tests pass with updated error format
6. New tests cover all error scenarios

## References

- [ErrorBuilder Documentation](../errors.md)
- [Auth Configuration](../auth/README.md)
- [Atmos Profiles PRD](./atmos-profiles.md)
- [Auth Default Settings PRD](./auth-default-settings.md)
- Linear Issue: DEV-3809
