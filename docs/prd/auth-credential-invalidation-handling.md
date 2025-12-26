# Product Requirements Document: Seamless Authentication Recovery

## Executive Summary

Enable Atmos to automatically recover from credential invalidation scenarios, providing a seamless single-command authentication experience. When credentials become invalid (rotated, revoked, or expired), Atmos should detect the issue, clean up stale state, and guide users through recovery inline—all within the same `atmos auth login` command.

## 1. Goals

### Primary Goals

- **Single Command Recovery**: Users should be able to recover from any credential state with just `atmos auth login`
- **Inline Credential Prompting**: When credentials are missing or invalid, prompt for new ones without requiring a separate configure command
- **Automatic Cleanup**: Stale credentials should be cleared automatically to prevent repeated failures
- **Actionable Guidance**: Error messages should clearly explain what happened and what to do next
- **MFA-Only Re-prompt**: When MFA token is invalid but long-lived credentials are still valid, only re-prompt for MFA token (not all credentials)

### Secondary Goals

- **Extended Session Support**: Honor configured session durations (up to 36 hours with MFA)
- **Graceful Degradation**: When prompting isn't available (non-interactive), provide clear fallback instructions
- **Session Credentials Detection**: Detect and handle cases where session credentials were accidentally stored in keyring
- **Non-Interactive Operations**: Read-only operations like `whoami` must not trigger credential prompts

## 2. User Experience

### Target Flow

```shell
$ atmos auth login dev-admin

⚠ AWS credentials are required for identity: dev-admin

AWS Access Key ID: AKIAXXXXXXXXXX
AWS Secret Access Key: ********
MFA ARN (optional): arn:aws:iam::123456789012:mfa/user
Session Duration (optional, default: 12h): 36h

✓ Credentials saved to keyring: dev-admin

Enter MFA Token: 123456

✓ Authentication successful!

  Provider   aws-user
  Identity   dev-admin
  Account    123456789012
  Region     us-east-1
  Expires    2024-12-24 04:58:00 MST (35h 59m)
```

### MFA Token Re-prompt Flow (Session Expired)

When session expires but long-lived credentials are still valid:

```shell
$ atmos auth login dev-admin

Enter MFA Token: 123456    # Invalid/expired token

⚠ MFA token was invalid, prompting for new token

Enter MFA Token: 789012    # User enters new token

✓ Authentication successful!
```

This avoids requiring users to re-enter all credentials when only the MFA token was wrong.

### Non-Interactive Whoami Flow

The `whoami` command should never prompt for credentials:

```shell
$ atmos auth whoami dev-admin
# If credentials are valid: shows identity info
# If credentials are missing/invalid: returns error with guidance, no prompts
```

### Error Detection and Response

| Error Code | Meaning | Action |
|------------|---------|--------|
| `InvalidClientTokenId` | Access keys rotated/revoked | Clear stale credentials, prompt for new ones (if interactive), retry |
| `ExpiredTokenException` | Session token expired | Guide user to re-login |
| `AccessDenied` (MFA-related) | Invalid/expired MFA token | Re-prompt for MFA token only (if interactive), retry |
| `AccessDenied` (permission) | Missing IAM permissions | Guide user to check IAM policies |

## 3. Implementation

### 3.1 Generic Credential Prompting Interface

Add generic types in `pkg/auth/types/credential_prompt.go`:

```go
// CredentialField describes a single credential input field.
type CredentialField struct {
    Name        string              // Field identifier (e.g., "access_key_id").
    Title       string              // Display title (e.g., "AWS Access Key ID").
    Description string              // Help text.
    Required    bool                // Must be non-empty.
    Secret      bool                // Mask input (password mode).
    Default     string              // Pre-populated value.
    Validator   func(string) error  // Optional validation function.
}

// CredentialPromptSpec defines what credentials to prompt for.
type CredentialPromptSpec struct {
    IdentityName string            // Name of the identity requiring credentials.
    CloudType    string            // Cloud provider: "aws", "azure", "gcp".
    Fields       []CredentialField // Fields to prompt for.
}

// CredentialPromptFunc is the generic prompting interface.
type CredentialPromptFunc func(spec CredentialPromptSpec) (map[string]string, error)
```

### 3.2 AWS Credential Spec Builder

Each identity type defines its credential fields in `pkg/auth/identities/aws/credential_prompt.go`:

```go
func buildAWSCredentialSpec(identityName string, mfaArn string) types.CredentialPromptSpec {
    return types.CredentialPromptSpec{
        IdentityName: identityName,
        CloudType:    "AWS",
        Fields: []types.CredentialField{
            {Name: "access_key_id", Title: "AWS Access Key ID", Required: true},
            {Name: "secret_access_key", Title: "AWS Secret Access Key", Required: true, Secret: true},
            {Name: "mfa_arn", Title: "MFA ARN", Required: false, Default: mfaArn},
            {Name: "session_duration", Title: "Session Duration", Required: false},
        },
    }
}
```

### 3.3 Non-Interactive Context Control

Add context helpers in `pkg/auth/types/context.go` to control whether prompting is allowed:

```go
// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

const (
    // ContextKeyAllowPrompts is the context key for controlling whether credential prompts are allowed.
    ContextKeyAllowPrompts contextKey = "atmos-auth-allow-prompts"
)

// WithAllowPrompts returns a new context with the allow-prompts flag set.
// When allowPrompts is false, authentication flows should not prompt for credentials.
func WithAllowPrompts(ctx context.Context, allowPrompts bool) context.Context {
    return context.WithValue(ctx, ContextKeyAllowPrompts, allowPrompts)
}

// AllowPrompts returns whether credential prompts are allowed in this context.
// Returns true if the flag is not set (default behavior allows prompts).
func AllowPrompts(ctx context.Context) bool {
    val := ctx.Value(ContextKeyAllowPrompts)
    if val == nil {
        return true // Default: allow prompts.
    }
    allow, ok := val.(bool)
    if !ok {
        return true // Default: allow prompts if value is wrong type.
    }
    return allow
}
```

Operations like `Whoami` must use non-interactive context:

```go
func (m *manager) Whoami(ctx context.Context, identityName string) (*types.WhoamiInfo, error) {
    // Use a non-interactive context to prevent credential prompts during whoami.
    nonInteractiveCtx := types.WithAllowPrompts(ctx, false)
    authInfo, authErr := m.Authenticate(nonInteractiveCtx, identityName)
    // ...
}
```

### 3.4 Error Handler with Automatic Recovery

Update `handleSTSError()` to detect specific AWS errors and recover. The function accepts context to check if prompting is allowed, and `longLivedCreds` to enable MFA-only re-prompting:

```go
func (i *userIdentity) handleSTSError(ctx context.Context, err error, longLivedCreds *types.AWSCredentials, isRetry bool) (*types.AWSCredentials, error) {
    var apiErr smithy.APIError
    if errors.As(err, &apiErr) {
        switch apiErr.ErrorCode() {
        case "InvalidClientTokenId":
            return i.handleInvalidClientTokenId(ctx, apiErr, isRetry)
        case "ExpiredTokenException":
            return i.handleExpiredToken(apiErr)
        case "AccessDenied":
            return i.handleAccessDenied(ctx, apiErr, longLivedCreds, isRetry)
        }
    }
    return nil, errors.Join(errUtils.ErrAuthenticationFailed, err)
}
```

### 3.5 MFA-Related Error Detection

Detect MFA-related AccessDenied errors and re-prompt for MFA token only:

```go
// handleAccessDenied handles the case where the user doesn't have permission to call GetSessionToken.
// It also detects MFA-related failures and re-prompts for the MFA token.
// The ctx parameter is used to check if interactive prompts are allowed.
func (i *userIdentity) handleAccessDenied(ctx context.Context, apiErr smithy.APIError, longLivedCreds *types.AWSCredentials, isRetry bool) (*types.AWSCredentials, error) {
    errorMsg := apiErr.ErrorMessage()

    // Check if this is an MFA-related failure (invalid/expired token).
    if isMfaRelatedError(errorMsg) {
        // Don't re-prompt for MFA if this is already a retry (prevents infinite loop).
        if isRetry {
            return nil, errUtils.Build(errUtils.ErrAuthenticationFailed).
                WithExplanation("The MFA token you entered is invalid or has expired").
                WithHint("MFA tokens are time-based and typically valid for 30 seconds").
                Err()
        }

        // MFA is configured and we have long-lived credentials - just re-prompt for MFA token.
        if longLivedCreds != nil && longLivedCreds.MfaArn != "" {
            log.Warn("MFA token was invalid, prompting for new token", "identity", i.name)
            // Return the same long-lived credentials - the retry will prompt for a new MFA token.
            return longLivedCreds, nil
        }
    }

    // Generic AccessDenied - likely a permission issue.
    return nil, errUtils.Build(errUtils.ErrAuthenticationFailed).
        WithExplanation("Your IAM user does not have permission to call sts:GetSessionToken").
        WithHint("Ensure your IAM user has the sts:GetSessionToken permission").
        Err()
}

// isMfaRelatedError checks if an AccessDenied error message indicates an MFA token issue.
func isMfaRelatedError(errorMsg string) bool {
    mfaPatterns := []string{
        "MultiFactorAuthentication failed",
        "invalid MFA",
        "MFA token",
        "one time pass code",
    }
    lowerMsg := strings.ToLower(errorMsg)
    for _, pattern := range mfaPatterns {
        if strings.Contains(lowerMsg, strings.ToLower(pattern)) {
            return true
        }
    }
    return false
}
```

### 3.6 Session Credentials Detection

Validate that keyring credentials are long-lived (not session credentials). Check `AllowPrompts(ctx)` before prompting:

```go
// In resolveLongLivedCredentials(ctx context.Context)
allowPrompts := types.AllowPrompts(ctx)

// Validate that keyring credentials are long-lived (not session credentials).
// Session credentials have a SessionToken and cannot be used with GetSessionToken API.
if keystoreCreds.SessionToken != "" {
    log.Warn("Keyring contains session credentials instead of long-lived credentials",
        "identity", i.name,
        "hint", "This can happen if session credentials were accidentally stored. Re-configuring will fix this.")
    // Treat as if credentials don't exist - prompt for new long-lived credentials if allowed.
    if PromptCredentialsFunc != nil && allowPrompts {
        return PromptCredentialsFunc(i.name, yamlMfaArn)
    }
    return nil, fmt.Errorf("%w: keyring contains session credentials (not long-lived) for identity %q",
        errUtils.ErrAwsUserNotConfigured, i.name)
}
```

### 3.7 Session Duration Preservation

Ensure `SessionDuration` is copied from keyring credentials:

```go
result := &types.AWSCredentials{
    AccessKeyID:     keystoreCreds.AccessKeyID,
    SecretAccessKey: keystoreCreds.SecretAccessKey,
    MfaArn:          keystoreCreds.MfaArn,          // Start with keyring MFA ARN.
    SessionDuration: keystoreCreds.SessionDuration, // Preserve session duration from keyring.
}
```

### 3.8 Credential Prompting Implementation

Create `pkg/auth/identities/aws/credential_prompt.go` to register the credential prompting function:

```go
func init() {
    PromptCredentialsFunc = promptForAWSCredentials
}

func promptForAWSCredentials(identityName string, mfaArn string) (*types.AWSCredentials, error) {
    // Display warning
    ui.Warning("AWS credentials are required for identity: " + identityName)

    // Build and run huh form for credential input
    // Store credentials in keyring
    // Return credentials for immediate use
}
```

### 3.9 Session Token Caching Prevention

**CRITICAL:** Session tokens must NEVER be cached in keyring as they would overwrite long-lived credentials that are needed for future authentication.

Add a helper function to detect session tokens:

```go
// isSessionToken checks if credentials are temporary session tokens.
// Session tokens are identified by the presence of a SessionToken field.
// These should not be cached in keyring as they overwrite long-lived credentials.
func isSessionToken(creds types.ICredentials) bool {
    if awsCreds, ok := creds.(*types.AWSCredentials); ok {
        return awsCreds.SessionToken != ""
    }
    // Add other credential types as needed.
    return false
}
```

All credential store operations must check for session tokens before caching:

**In `authenticateIdentityChain` (manager.go):**
```go
// Cache credentials for this level, but skip session tokens.
if isSessionToken(currentCreds) {
    log.Debug("Skipping keyring cache for session tokens", "identityStep", identityStep)
} else {
    if err := m.credentialStore.Store(identityStep, currentCreds); err != nil {
        log.Debug("Failed to cache credentials", "identityStep", identityStep, "error", err)
    }
}
```

**In `authenticateWithProvider` (manager.go):**
```go
// Cache provider credentials, but skip session tokens.
if isSessionToken(credentials) {
    log.Debug("Skipping keyring cache for session token provider credentials", logKeyProvider, providerName)
} else {
    if err := m.credentialStore.Store(providerName, credentials); err != nil {
        log.Debug("Failed to cache provider credentials", "error", err)
    }
}
```

**In `buildWhoamiInfo` (manager_whoami.go):**
```go
// Store credentials in the keystore and set a reference handle.
// CRITICAL: Skip caching session tokens to avoid overwriting long-lived credentials.
if !isSessionToken(creds) {
    if err := m.credentialStore.Store(identityName, creds); err == nil {
        info.CredentialsRef = identityName
    }
} else {
    log.Debug("Skipping keyring cache for session tokens in WhoamiInfo", logKeyIdentity, identityName)
    // Still set the reference for credential lookups - credentials can be loaded from identity storage.
    info.CredentialsRef = identityName
}
```

## 4. Testing

### Unit Tests

- `TestUserIdentity_HandleSTSError_InvalidClientTokenId` — Verify credentials cleared, prompting called
- `TestUserIdentity_HandleSTSError_ExpiredTokenException` — Verify correct error message
- `TestUserIdentity_HandleSTSError_AccessDenied` — Verify IAM guidance for non-MFA errors
- `TestUserIdentity_HandleSTSError_GenericError` — Verify fallback behavior
- `TestUserIdentity_HandleSTSError_WithPromptFunc` — Verify prompting returns new credentials
- `TestUserIdentity_HandleSTSError_PromptFuncFails` — Verify error when prompting fails
- `TestUserIdentity_HandleAccessDenied` — Verify MFA-related error detection and retry flow
- `TestUserIdentity_HandleInvalidClientTokenId` — Verify context-based prompting control
- `TestUser_resolveLongLivedCredentials_DetectsSessionCredentials` — Verify session credentials detection
- `TestUser_resolveLongLivedCredentials_PromptWhenMissing` — Verify prompting when credentials missing
- `TestUser_resolveLongLivedCredentials_ErrorWhenMissingAndNoPrompt` — Verify error when prompts disabled
- `TestIsMfaRelatedError` — Verify MFA error pattern matching
- `TestSessionTokenDoesNotOverwriteLongLivedCredentialsInKeyring` — Verify session tokens not cached
- `TestManager_Whoami_NonInteractive` — Verify Whoami doesn't trigger prompts
- `TestAllowPrompts_ContextHelpers` — Verify context flag propagation

### Mock Implementation

```go
type mockAPIError struct {
    code    string
    message string
}

func (e *mockAPIError) Error() string       { return e.message }
func (e *mockAPIError) ErrorCode() string   { return e.code }
func (e *mockAPIError) ErrorMessage() string { return e.message }
func (e *mockAPIError) ErrorFault() smithy.ErrorFault { return smithy.FaultClient }
```

## 5. Files Modified

| File | Changes |
|------|---------|
| `errors/errors.go` | Add `ErrCredentialsInvalid` sentinel error |
| `pkg/auth/types/credential_prompt.go` | New file with generic credential prompting interface |
| `pkg/auth/types/context.go` | **New file** with context helpers for non-interactive prompting control |
| `pkg/auth/manager.go` | Add session token check in `authenticateWithProvider`, pass context through error handlers |
| `pkg/auth/manager_whoami.go` | Add session token check in `buildWhoamiInfo`, use non-interactive context in `Whoami` |
| `pkg/auth/identities/aws/user.go` | Update error handling with context, fix session duration, add MFA retry flow, add session credentials detection |
| `pkg/auth/identities/aws/user_test.go` | Add tests for new functionality, update existing tests for context parameter |
| `pkg/auth/identities/aws/credential_prompt.go` | New file with AWS credential prompting implementation |
| `pkg/auth/identities/aws/credential_prompt_test.go` | Tests for credential validation and form building |
| `cmd/auth_shell_test.go` | Disable credential prompting in tests to prevent blocking |

## 6. Success Metrics

- **Single Command Recovery**: Users can recover from credential invalidation with just `atmos auth login`
- **MFA-Only Re-prompt**: Invalid MFA token only requires re-entering the MFA token, not all credentials
- **Session Duration Works**: Configured session durations (up to 36 hours with MFA) are honored
- **Error Clarity**: Error messages describe what happened (explanation) and what to do (hint)
- **Automatic Cleanup**: Stale credentials are cleared automatically, preventing repeated failures
- **Session Credentials Detection**: Accidentally stored session credentials are detected and user is prompted to reconfigure
- **Non-Interactive Whoami**: `atmos auth whoami` never triggers credential prompts
- **Session Token Protection**: Session tokens are never cached in keyring, protecting long-lived credentials
- **Test Coverage**: All error scenarios have unit test coverage

## 7. Manual Testing Guide

This section provides step-by-step instructions for manually testing the credential invalidation handling features.

### Prerequisites

1. Build Atmos from source:
   ```shell
   make build
   ```

2. Create a test identity in your `atmos.yaml`:
   ```yaml
   auth:
     identities:
       test-user:
         kind: aws/user
         region: us-east-1
   ```

3. Have valid AWS IAM User credentials available (access key ID + secret access key).

### Test 1: Single Command Recovery (Fresh Start)

**Purpose:** Verify that `atmos auth login` prompts for credentials when none exist.

```shell
# Step 1: Ensure no credentials exist for the identity
./build/atmos auth logout test-user

# Step 2: Run login - should prompt for credentials
./build/atmos auth login test-user

# Expected:
# ⚠ AWS credentials are required for identity: test-user
# [Credential prompt form appears]
# Enter: Access Key ID, Secret Access Key, optional MFA ARN, optional Session Duration
# ✓ Authentication successful!
```

### Test 2: Whoami Non-Interactive (No Prompts)

**Purpose:** Verify that `atmos auth whoami` never triggers credential prompts.

```shell
# Step 1: Logout to clear credentials
./build/atmos auth logout test-user

# Step 2: Run whoami - should NOT prompt, should show error
./build/atmos auth whoami test-user

# Expected:
# Error message indicating credentials are missing
# NO credential prompt should appear
# Exit code should be non-zero
```

```shell
# Step 3: Login with valid credentials
./build/atmos auth login test-user

# Step 4: Run whoami again - should show identity info
./build/atmos auth whoami test-user

# Expected:
# Identity info displayed (provider, identity, account, region, expiration)
# NO credential prompt should appear
```

### Test 3: Session Credentials Detection

**Purpose:** Verify that session credentials in keyring are detected and user is prompted to reconfigure.

This test requires manually corrupting the keyring with session credentials. Use the debug subcommand if available, or:

```shell
# Step 1: Login normally first
./build/atmos auth login test-user

# Step 2: Check the keyring contents (for debugging)
# On macOS: Keychain Access app, search for "atmos"
# On Linux: Check ~/.local/share/keyrings/ or secret-tool

# Step 3: If you can manually edit the keyring to add a SessionToken field,
# the next login should detect this and prompt for new credentials:
./build/atmos auth login test-user

# Expected (if session credentials detected):
# WARN Keyring contains session credentials instead of long-lived credentials
# [Credential prompt form appears]
```

### Test 4: Invalid Credentials Recovery

**Purpose:** Verify that invalid credentials trigger re-prompting.

```shell
# Step 1: Configure with INVALID credentials
./build/atmos auth user configure test-user
# Enter: AKIAINVALIDKEY12345, invalidsecretkey123456

# Step 2: Try to login - should detect invalid and prompt for new
./build/atmos auth login test-user

# Expected:
# ERROR AWS credentials are invalid or have been revoked
# [Credential prompt form appears for new credentials]
# Enter valid credentials
# ✓ Authentication successful!
```

### Test 5: Session Duration Preservation

**Purpose:** Verify that configured session duration is honored.

```shell
# Step 1: Login with custom session duration
./build/atmos auth login test-user
# When prompted for Session Duration, enter: 8h

# Step 2: Check the expiration time
./build/atmos auth whoami test-user

# Expected:
# Expires field should show ~8 hours from now
```

```shell
# Step 3: Logout and login again - duration should be preserved
./build/atmos auth logout test-user
./build/atmos auth login test-user
# Skip entering session duration (should use cached 8h)

./build/atmos auth whoami test-user

# Expected:
# Expires field should still show ~8 hours from now
```

### Test 6: MFA Token Re-prompt (Requires MFA-enabled IAM User)

**Purpose:** Verify that invalid MFA token only re-prompts for MFA, not all credentials.

```shell
# Step 1: Configure with MFA ARN
./build/atmos auth user configure test-user
# Enter: valid access key, valid secret key, MFA ARN

# Step 2: Login with WRONG MFA token
./build/atmos auth login test-user
# Enter wrong MFA token (e.g., 000000)

# Expected:
# WARN MFA token was invalid, prompting for new token
# [Only MFA token prompt appears - NOT full credential form]
# Enter correct MFA token
# ✓ Authentication successful!
```

### Test 7: Session Token Not Cached in Keyring

**Purpose:** Verify that session tokens don't overwrite long-lived credentials.

```shell
# Step 1: Login with credentials
./build/atmos auth login test-user
# Enter valid credentials

# Step 2: Note the access key ID shown in whoami
./build/atmos auth whoami test-user
# Note: Access key should start with "ASIA" (session credentials)

# Step 3: Logout and login again WITHOUT re-entering credentials
./build/atmos auth logout test-user
./build/atmos auth login test-user

# Expected:
# Should NOT prompt for credentials (keyring has long-lived creds)
# Should generate new session token from cached long-lived credentials
# ✓ Authentication successful!

# If this fails with "keyring contains session credentials" - BUG!
```

### Test 8: Credential Rotation Recovery

**Purpose:** Verify recovery after AWS credentials are rotated in AWS Console.

```shell
# Step 1: Login with current credentials
./build/atmos auth login test-user

# Step 2: In AWS Console, rotate the IAM User's access keys
# (Create new key, delete old key)

# Step 3: Wait for session to expire, or logout
./build/atmos auth logout test-user

# Step 4: Try to login with OLD credentials (still in keyring)
./build/atmos auth login test-user

# Expected:
# ERROR AWS credentials are invalid or have been revoked
# [Credential prompt form appears]
# Enter NEW credentials
# ✓ Authentication successful!
```

### Test 9: Non-Interactive Mode (CI/CD Simulation)

**Purpose:** Verify graceful degradation when prompting isn't possible.

```shell
# Step 1: Clear credentials
./build/atmos auth logout test-user

# Step 2: Try login with stdin closed (simulates CI/CD)
echo "" | ./build/atmos auth login test-user

# Expected:
# Should fail with clear error message
# Should NOT hang waiting for input
# Error should suggest: "Run: atmos auth user configure test-user"
```

### Test 10: Debug Logging

**Purpose:** Use debug logging to trace credential flow.

```shell
# Run with debug logging to see credential resolution
ATMOS_LOGS_LEVEL=Debug ./build/atmos auth login test-user

# Look for log messages:
# - "Resolving long-lived credentials"
# - "Retrieved credentials from keyring"
# - "Skipping keyring cache for session tokens"
# - "Cached credentials" or "Skipping keyring cache"
```

### Troubleshooting

**Keyring Issues:**
```shell
# Check keyring backend being used
ATMOS_LOGS_LEVEL=Debug ./build/atmos auth whoami test-user 2>&1 | grep -i keyring

# Force file-based keyring (for testing)
ATMOS_AUTH_KEYRING_BACKEND=file ./build/atmos auth login test-user
```

**Clear All Cached Credentials:**
```shell
# Logout all identities
./build/atmos auth logout --all

# Or manually clear keyring entries
# macOS: Keychain Access → search "atmos" → delete entries
# Linux: secret-tool clear service atmos
```

**View Stored Credentials (Debug Only):**
```shell
# Check what's in the credential files
ls -la ~/.config/atmos/aws/

# View AWS credentials file (contains session tokens)
cat ~/.config/atmos/aws/aws-user/credentials
```

## 8. Future Considerations

- Extend error detection to other AWS API calls (AssumeRole, AssumeRoleWithSAML)
- Add `atmos auth repair` command for manual credential cleanup
- Generalize credential prompting interface for multi-cloud support
- Add telemetry for credential invalidation events to help users identify patterns
