# Product Requirements Document: Seamless Authentication Recovery

## Executive Summary

Enable Atmos to automatically recover from credential invalidation scenarios, providing a seamless single-command authentication experience. When credentials become invalid (rotated, revoked, or expired), Atmos should detect the issue, clean up stale state, and guide users through recovery inline—all within the same `atmos auth login` command.

## 1. Goals

### Primary Goals

- **Single Command Recovery**: Users should be able to recover from any credential state with just `atmos auth login`
- **Inline Credential Prompting**: When credentials are missing or invalid, prompt for new ones without requiring a separate configure command
- **Automatic Cleanup**: Stale credentials should be cleared automatically to prevent repeated failures
- **Actionable Guidance**: Error messages should clearly explain what happened and what to do next

### Secondary Goals

- **Extended Session Support**: Honor configured session durations (up to 36 hours with MFA)
- **Graceful Degradation**: When prompting isn't available (non-interactive), provide clear fallback instructions

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

### Error Detection and Response

| Error Code | Meaning | Action |
|------------|---------|--------|
| `InvalidClientTokenId` | Access keys rotated/revoked | Clear stale credentials, prompt for new ones, retry |
| `ExpiredTokenException` | Session token expired | Guide user to re-login |
| `AccessDenied` | Missing IAM permissions | Guide user to check IAM policies |

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

Update `handleSTSError()` to detect specific AWS errors and recover:

```go
func (i *userIdentity) handleSTSError(err error) (*types.AWSCredentials, error) {
    var apiErr smithy.APIError
    if errors.As(err, &apiErr) {
        switch apiErr.ErrorCode() {
        case "InvalidClientTokenId":
            // Clear stale credentials from keyring
            credStore := credentials.NewCredentialStore()
            _ = credStore.Delete(i.name)

            // If credential prompting is available, prompt for new credentials
            if PromptCredentialsFunc != nil {
                yamlMfaArn, _ := i.config.Credentials["mfa_arn"].(string)
                newCreds, promptErr := PromptCredentialsFunc(i.name, yamlMfaArn)
                if promptErr == nil {
                    return newCreds, nil // Success - return new credentials for retry
                }
            }

            return nil, errUtils.Build(errUtils.ErrCredentialsInvalid).
                WithExplanation("Your AWS access keys have been rotated or revoked on the AWS side").
                WithExplanation("Stale credentials have been automatically cleared from keychain").
                WithHintf("Run: atmos auth user configure --identity %s", i.name).
                WithContext("identity", i.name).
                WithContext("error_code", apiErr.ErrorCode()).
                Err()
        // ... handle other error codes
        }
    }
    return nil, errors.Join(errUtils.ErrAuthenticationFailed, err)
}
```

### 3.4 Session Duration Preservation

Ensure `SessionDuration` is copied from keyring credentials:

```go
result := &types.AWSCredentials{
    AccessKeyID:     keystoreCreds.AccessKeyID,
    SecretAccessKey: keystoreCreds.SecretAccessKey,
    MfaArn:          keystoreCreds.MfaArn,
    SessionDuration: keystoreCreds.SessionDuration, // Preserve session duration from keyring.
}
```

### 3.5 Credential Prompting Implementation

Create `cmd/auth_credential_prompt.go` to register the credential prompting function:

```go
func init() {
    aws.PromptCredentialsFunc = promptForAWSCredentials
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
- `TestUserIdentity_HandleSTSError_AccessDenied` — Verify IAM guidance
- `TestUserIdentity_HandleSTSError_GenericError` — Verify fallback behavior
- `TestUserIdentity_HandleSTSError_WithPromptFunc` — Verify prompting returns new credentials
- `TestUserIdentity_HandleSTSError_PromptFuncFails` — Verify error when prompting fails

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
| `pkg/auth/identities/aws/user.go` | Update error handling, fix session duration |
| `pkg/auth/identities/aws/user_test.go` | Add tests for new functionality |
| `pkg/auth/identities/aws/credential_prompt.go` | New file with AWS credential prompting implementation |
| `pkg/auth/identities/aws/credential_prompt_test.go` | Tests for credential validation and form building |

## 6. Success Metrics

- **Single Command Recovery**: Users can recover from credential invalidation with just `atmos auth login`
- **Session Duration Works**: Configured session durations (up to 36 hours with MFA) are honored
- **Error Clarity**: Error messages clearly explain what happened (explanation) and what to do (hint)
- **Automatic Cleanup**: Stale credentials are cleared automatically, preventing repeated failures
- **Test Coverage**: All error scenarios have unit test coverage

## 7. Future Considerations

- Extend error detection to other AWS API calls (AssumeRole, AssumeRoleWithSAML)
- Add `atmos auth repair` command for manual credential cleanup
- Generalize credential prompting interface for multi-cloud support
