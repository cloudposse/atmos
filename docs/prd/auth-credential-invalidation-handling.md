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

### Error Detection and Response

| Error Code | Meaning | Action |
|------------|---------|--------|
| `InvalidClientTokenId` | Access keys rotated/revoked | Clear stale credentials, prompt for new ones, retry |
| `ExpiredTokenException` | Session token expired | Guide user to re-login |
| `AccessDenied` (MFA-related) | Invalid/expired MFA token | Re-prompt for MFA token only, retry |
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

### 3.3 Error Handler with Automatic Recovery

Update `handleSTSError()` to detect specific AWS errors and recover. The function now accepts `longLivedCreds` to enable MFA-only re-prompting:

```go
func (i *userIdentity) handleSTSError(err error, longLivedCreds *types.AWSCredentials, isRetry bool) (*types.AWSCredentials, error) {
    var apiErr smithy.APIError
    if errors.As(err, &apiErr) {
        switch apiErr.ErrorCode() {
        case "InvalidClientTokenId":
            return i.handleInvalidClientTokenId(apiErr, isRetry)
        case "ExpiredTokenException":
            return i.handleExpiredToken(apiErr)
        case "AccessDenied":
            return i.handleAccessDenied(apiErr, longLivedCreds, isRetry)
        }
    }
    return nil, errors.Join(errUtils.ErrAuthenticationFailed, err)
}
```

### 3.4 MFA-Related Error Detection

Detect MFA-related AccessDenied errors and re-prompt for MFA token only:

```go
// handleAccessDenied handles the case where the user doesn't have permission to call GetSessionToken.
// It also detects MFA-related failures and re-prompts for the MFA token.
func (i *userIdentity) handleAccessDenied(apiErr smithy.APIError, longLivedCreds *types.AWSCredentials, isRetry bool) (*types.AWSCredentials, error) {
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

### 3.5 Session Credentials Detection

Validate that keyring credentials are long-lived (not session credentials):

```go
// In resolveLongLivedCredentials()
// Validate that keyring credentials are long-lived (not session credentials).
// Session credentials have a SessionToken and cannot be used with GetSessionToken API.
if keystoreCreds.SessionToken != "" {
    log.Warn("Keyring contains session credentials instead of long-lived credentials",
        "identity", i.name,
        "hint", "This can happen if session credentials were accidentally stored. Re-configuring will fix this.")
    // Treat as if credentials don't exist - prompt for new long-lived credentials.
    if PromptCredentialsFunc != nil {
        return PromptCredentialsFunc(i.name, yamlMfaArn)
    }
    return nil, fmt.Errorf("%w: keyring contains session credentials (not long-lived) for identity %q",
        errUtils.ErrAwsUserNotConfigured, i.name)
}
```

### 3.6 Session Duration Preservation

Ensure `SessionDuration` is copied from keyring credentials:

```go
result := &types.AWSCredentials{
    AccessKeyID:     keystoreCreds.AccessKeyID,
    SecretAccessKey: keystoreCreds.SecretAccessKey,
    MfaArn:          keystoreCreds.MfaArn,          // Start with keyring MFA ARN.
    SessionDuration: keystoreCreds.SessionDuration, // Preserve session duration from keyring.
}
```

### 3.7 Credential Prompting Implementation

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

## 4. Testing

### Unit Tests

- `TestUserIdentity_HandleSTSError_InvalidClientTokenId` — Verify credentials cleared, prompting called
- `TestUserIdentity_HandleSTSError_ExpiredTokenException` — Verify correct error message
- `TestUserIdentity_HandleSTSError_AccessDenied` — Verify IAM guidance for non-MFA errors
- `TestUserIdentity_HandleSTSError_GenericError` — Verify fallback behavior
- `TestUserIdentity_HandleSTSError_WithPromptFunc` — Verify prompting returns new credentials
- `TestUserIdentity_HandleSTSError_PromptFuncFails` — Verify error when prompting fails
- `TestUserIdentity_HandleAccessDenied` — Verify MFA-related error detection and retry flow
- `TestUser_resolveLongLivedCredentials_DetectsSessionCredentials` — Verify session credentials detection
- `TestIsMfaRelatedError` — Verify MFA error pattern matching

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
| `pkg/auth/identities/aws/user.go` | Update error handling, fix session duration, add MFA retry flow, add session credentials detection |
| `pkg/auth/identities/aws/user_test.go` | Add tests for new functionality |
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
- **Test Coverage**: All error scenarios have unit test coverage

## 7. Future Considerations

- Extend error detection to other AWS API calls (AssumeRole, AssumeRoleWithSAML)
- Add `atmos auth repair` command for manual credential cleanup
- Generalize credential prompting interface for multi-cloud support
- Add telemetry for credential invalidation events to help users identify patterns
