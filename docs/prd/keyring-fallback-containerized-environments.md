# PRD: Keyring Fallback for Containerized Environments

## Overview

Enable graceful degradation when system keyring is unavailable in containerized environments, while maintaining credential validation and expiration checking.

## Motivation

### Problem Statement

When running Atmos inside containers (e.g., Geodesic):
1. User authenticates on **host machine** via `atmos auth login`
2. Credentials stored in **system keyring** (with expiration)
3. Credentials written to **files**: `~/.aws/atmos/<provider>/credentials`
4. User enters **container** (mounts `~/.aws/atmos/` from host)
5. Container lacks dbus/system keychain → **alarming errors**
6. Credentials exist in files but expiration info only in (unavailable) keyring

### Current Behavior

```
credential store: failed to retrieve credentials from keyring: exec: "dbus-launch": executable file not found in $PATH
```

### AWS Credentials Flow

**Key Insight:** The keyring stores temporary AWS credentials (including expiration), but the **AWS SDK actually uses credentials files**, not the keyring.

**On Host:**
1. `atmos auth login` → Authenticate with AWS SSO/SAML
2. Get temporary credentials (access key, secret, session token, **expiration**)
3. Store in **system keyring** (full credentials + expiration)
4. Write to **files**: `~/.aws/atmos/<provider>/credentials` (access key, secret, token - **NO expiration**)
5. Set environment: `AWS_SHARED_CREDENTIALS_FILE=/path/to/credentials`

**In Container:**
1. Mount `~/.aws/atmos/` from host
2. Set `AWS_SHARED_CREDENTIALS_FILE` env var
3. AWS SDK reads credentials from files ✅
4. **Problem:** Keyring unavailable → can't check expiration → no validation

## Goals

1. **Graceful degradation** when system keyring unavailable
2. **Validate credentials** from files using provider APIs
3. **Report expiration** to user (warn if expiring soon, error if expired)
4. **No alarming errors** when running in containers
5. **Provider-agnostic** interface (AWS implements, others can too)

## Out of Scope

- Container-specific detection or configuration
- Automatic credential refresh in containers
- Browser-based authentication flows in containers
- Writing expiration to credentials files

## Detailed Requirements

### 1. Credential Validation Interface

Add `Validate()` method to `ICredentials` interface:

```go
type ICredentials interface {
    IsExpired() bool
    GetExpiration() (*time.Time, error)
    BuildWhoamiInfo(info *WhoamiInfo)

    // Validate validates credentials by making an API call to the provider.
    // Returns expiration time if available, error if credentials are invalid.
    // Returns ErrNotImplemented if validation is not supported for this credential type.
    Validate(ctx context.Context) (*time.Time, error)
}
```

### 2. AWS Credentials Validation

**Implementation:** `pkg/auth/types/aws_credentials.go`

```go
func (c *AWSCredentials) Validate(ctx context.Context) (*time.Time, error) {
    // Create AWS config with these credentials
    cfg := aws.Config{
        Region: c.Region,
        Credentials: credentials.NewStaticCredentialsProvider(
            c.AccessKeyID, c.SecretAccessKey, c.SessionToken,
        ),
    }

    // Create STS client
    stsClient := sts.NewFromConfig(cfg)

    // Call GetCallerIdentity to validate credentials
    _, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
    if err != nil {
        return nil, fmt.Errorf("failed to validate AWS credentials: %w", err)
    }

    // Return expiration time if available
    if expTime, err := c.GetExpiration(); err == nil && expTime != nil {
        return expTime, nil
    }

    return nil, nil // Long-term credentials (no expiration)
}
```

**Behavior:**
- Makes AWS STS GetCallerIdentity API call
- Returns error if credentials invalid/expired
- Returns expiration time from credential's Expiration field
- Works with both temporary (session) and long-term (IAM user) credentials

### 3. OIDC Credentials Validation

**Implementation:** `pkg/auth/types/github_oidc_credentials.go`

```go
func (c *OIDCCredentials) Validate(ctx context.Context) (*time.Time, error) {
    return nil, errUtils.ErrNotImplemented
}
```

**Rationale:** OIDC token validation requires provider-specific logic. Return `ErrNotImplemented` for now.

### 4. No-Op Keyring Store

**Purpose:** Used when system keyring is unavailable (e.g., containers without dbus).

**Current Behavior:**
- `Store()` → No-op (succeeds)
- `Retrieve()` → Returns `ErrCredentialsNotFound`
- All other methods → No-op or empty

**Required Behavior:**

**Option A: Keep Simple (Recommended)**
- Keep current behavior (Retrieve returns "not found")
- Let higher-level code handle credential loading from files
- No-op keyring just prevents alarming dbus errors

**Option B: Load and Validate**
- `Retrieve()` loads credentials from AWS files
- Calls `Validate()` to check with provider
- Returns validated credentials with expiration
- More complex but provides full validation

**Decision Point:** Which approach should we use?

### 5. System Keyring Detection

**Implementation:** `pkg/auth/credentials/keyring_system.go`

```go
func newSystemKeyringStore() (*systemKeyringStore, error) {
    // Test keyring availability by attempting to get a non-existent key
    testKey := "atmos-keyring-test"
    _, err := keyring.Get(testKey, KeyringUser)

    // If error is ErrNotFound, keyring is available
    if err != nil && !errors.Is(err, keyring.ErrNotFound) {
        // Any other error indicates keyring is not available
        return nil, fmt.Errorf("system keyring not available: %w", err)
    }

    return &systemKeyringStore{}, nil
}
```

**Behavior:**
- Proactively tests keyring availability during initialization
- Returns error if dbus/keychain unavailable
- Prevents runtime dbus errors later

### 6. Fallback Logic

**Implementation:** `pkg/auth/credentials/store.go`

```go
if err != nil {
    if keyringType == "system" {
        // System keyring unavailable (e.g., no dbus in containers)
        // Use no-op keyring to gracefully handle missing keyring
        log.Debug("System keyring not available, using no-op keyring", "error", err.Error())
        store = newNoopKeyringStore()
    } else {
        // Non-system keyring failed - try system keyring
        log.Debug("Keyring type failed, attempting system keyring fallback", "keyringType", keyringType)
        store, err = newSystemKeyringStore()
        if err != nil {
            log.Debug("System keyring not available, using no-op keyring")
            store = newNoopKeyringStore()
        }
    }
}
```

**Behavior:**
- System keyring unavailable → Use no-op keyring (not file/memory)
- Other keyring fails → Try system → fallback to no-op
- Debug logging for visibility

### 7. Expiration Warning/Error Behavior

**Requirements:**

When credentials are validated:
- **If expired** → Error: "Credentials expired. Please refresh on host machine and re-enter container."
- **If expiring soon** (< 15 minutes) → Warning: "Credentials expire in X minutes. Consider refreshing soon."
- **If valid** → Continue normally, log expiration time at debug level

### 8. Environment Variables

**Container Workflow:**

User sets these environment variables (already standard practice):
- `AWS_SHARED_CREDENTIALS_FILE=/path/to/mounted/credentials`
- `AWS_CONFIG_FILE=/path/to/mounted/config`
- `AWS_PROFILE=<identity-name>` (corresponds to Atmos identity)

Atmos should:
1. Detect system keyring unavailable
2. Use no-op keyring (no dbus errors)
3. Load credentials from files (via environment variables)
4. Validate credentials with provider
5. Check expiration and warn/error appropriately

## Implementation Notes

### Provider Interface

Validation is **credential-specific**, not provider-specific:
- Each credential type implements `Validate()`
- AWS credentials call STS GetCallerIdentity
- OIDC credentials return ErrNotImplemented
- Mock credentials return success (testing)

### Error Handling

```go
// New error in errors/errors.go
ErrNotImplemented = errors.New("not implemented")

// Usage
if errors.Is(err, errUtils.ErrNotImplemented) {
    // Validation not supported for this credential type
    // Continue without validation
}
```

### Logging

Use structured logging with appropriate levels:
- Debug: System keyring unavailable, using no-op
- Debug: Credentials validated successfully, expire at X
- Warn: Credentials expire soon (< 15 minutes)
- Error: Credentials expired or validation failed

### Cross-Platform

- Works on Linux, macOS, Windows
- Container detection is implicit (keyring unavailable)
- No container-specific code paths
- Standard AWS SDK usage

## Test Plan

### Unit Tests

1. **Credential Validation Tests**
   - AWS credentials: Valid credentials succeed
   - AWS credentials: Invalid credentials fail
   - AWS credentials: Expired credentials fail
   - OIDC credentials: Returns ErrNotImplemented

2. **No-Op Keyring Tests**
   - Store succeeds (no-op)
   - Retrieve returns appropriate response
   - Delete succeeds (no-op)
   - List returns empty

3. **System Keyring Detection Tests**
   - Available keyring: Success
   - Unavailable keyring: Returns error
   - Mock environment for testing

### Integration Tests

1. **Container Simulation**
   - Set ATMOS_KEYRING_TYPE=system (but make unavailable)
   - Verify no-op keyring used
   - No alarming errors logged

2. **Credential Validation Flow**
   - Load credentials from files
   - Validate with AWS (mock STS)
   - Check expiration warnings

### Manual Testing

1. **In Container**
   - Enter container without dbus
   - Run `atmos terraform plan`
   - Verify: No dbus errors
   - Verify: Credentials validated
   - Verify: Appropriate expiration warnings

2. **On Host**
   - Run `atmos auth login`
   - Verify: System keyring used
   - Verify: Credentials stored and validated

## Acceptance Criteria

- ✅ No dbus errors when system keyring unavailable
- ✅ ICredentials interface has Validate() method
- ✅ AWS credentials validate via STS GetCallerIdentity
- ✅ OIDC credentials return ErrNotImplemented
- ✅ No-op keyring used when system keyring unavailable
- ✅ Debug logging shows keyring backend selection
- ✅ Expired credentials produce clear error message
- ✅ Expiring credentials produce warning
- ✅ All tests pass
- ✅ Works in containers without code changes

## Open Questions

1. **No-Op Keyring Behavior:** Should it:
   - **Option A:** Just prevent errors (Retrieve returns "not found")
   - **Option B:** Load from files and validate credentials

2. **Expiration Warning Threshold:** 15 minutes reasonable?

3. **Validation Timing:** When should Validate() be called?
   - During Retrieve()?
   - By the auth manager?
   - On-demand when needed?

## References

- AWS SDK Go v2: https://github.com/aws/aws-sdk-go-v2
- STS GetCallerIdentity: https://docs.aws.amazon.com/STS/latest/APIReference/API_GetCallerIdentity.html
- AWS Credentials File Format: https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-files.html
