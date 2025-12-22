# Product Requirements Document: Auth Credential Invalidation Handling

## Executive Summary

This PRD addresses the issue where `atmos auth login` fails persistently after AWS credentials have been rotated or revoked on the AWS side. The solution automatically detects the `InvalidClientTokenId` error from AWS STS, clears stale credentials from the keyring, prompts for new credentials inline, and retries authentication.

Additionally, this PRD addresses a bug where session duration configured via `atmos auth user configure` was not being preserved, causing tokens to expire after 12 hours instead of the configured duration (up to 36 hours with MFA).

## 1. Problem Statement

### Current Challenges

- **Persistent Authentication Failures**: When AWS access keys are rotated or revoked, `atmos auth login` fails with `InvalidClientTokenId` error
- **Stale Credentials in Keyring**: The keyring retains old credentials even after they become invalid on AWS side
- **Logout Doesn't Fix It**: `atmos auth logout` followed by `auth login` doesn't resolve the issue because keyring credentials are preserved by default
- **Poor Error Messages**: The error message doesn't explain why authentication failed or how to fix it
- **Manual Intervention Required**: Users must manually reconfigure their AWS credentials to recover
- **Session Duration Bug**: Session duration configured in keyring was not being passed through, causing 36h MFA sessions to expire after 12h

### User Impact

- Breaks developer workflows unpredictably (often after ~24 hours due to session duration bug)
- Undermines confidence in extended MFA session configuration
- Forces disruptive manual remediation (full user reconfiguration)

### Root Cause Analysis

1. Session token expires (normal behavior after configured duration)
2. `atmos auth login` attempts to generate new session token using cached long-lived credentials
3. AWS STS returns `InvalidClientTokenId` because the underlying access keys were rotated/revoked
4. Atmos stores credentials in keyring with `--keychain` flag required to clear them
5. Without clearing keyring, subsequent login attempts keep using stale credentials
6. **Bug**: `resolveLongLivedCredentials()` was not copying `SessionDuration` from keyring credentials

## 2. Solution Design

### 2.1 Error Detection

Detect specific AWS STS error codes that indicate credential problems:

| Error Code | Meaning | Action |
|------------|---------|--------|
| `InvalidClientTokenId` | Access keys rotated/revoked | Clear keyring, prompt for new credentials, retry |
| `ExpiredTokenException` | Session token expired | Guide user to re-login |
| `AccessDenied` | Missing IAM permissions | Guide user to check IAM policies |

### 2.2 Automatic Remediation with Inline Prompting

When `InvalidClientTokenId` is detected:

1. **Automatically clear stale credentials** from keyring for the affected identity
2. **Prompt for new credentials inline** using interactive form
3. **Store new credentials** in keyring
4. **Retry STS call** with new credentials
5. **Provide actionable error message** if prompting fails or is cancelled

### 2.3 Session Duration Bug Fix

When resolving long-lived credentials from keyring:

1. **Copy `SessionDuration` field** from keyring credentials to the result struct
2. **Pass session duration** to `getSessionDuration()` function
3. **Use configured duration** when calling AWS STS GetSessionToken

### 2.4 User Experience Flow

```
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

## 3. Implementation

### 3.1 New Sentinel Error

Add `ErrAwsCredentialsInvalid` to `errors/errors.go`:

```go
// AWS IAM User credential errors.
ErrAwsCredentialsInvalid = errors.New("aws credentials are invalid or have been revoked")
```

### 3.2 Credential Prompting Function Variable

Add `PromptCredentialsFunc` variable in `pkg/auth/identities/aws/user.go`:

```go
// PromptCredentialsFunc is a helper indirection to allow the cmd package to provide
// credential prompting UI. When set, it's called when credentials are missing or invalid.
var PromptCredentialsFunc func(identityName string, mfaArn string) (*types.AWSCredentials, error)
```

### 3.3 Error Handler Function (Updated)

Update `handleSTSError()` method to return new credentials when prompting succeeds:

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

            return nil, errUtils.Build(errUtils.ErrAwsCredentialsInvalid).
                WithHint("Your AWS access keys have been rotated or revoked on the AWS side").
                WithHintf("Run: atmos auth user configure --identity %s", i.name).
                WithHint("Stale credentials have been automatically cleared from keychain").
                WithContext("identity", i.name).
                WithContext("error_code", apiErr.ErrorCode()).
                Err()
        // ... handle other error codes
        }
    }
    return nil, errors.Join(errUtils.ErrAuthenticationFailed, err)
}
```

### 3.4 Session Duration Fix

Update `resolveLongLivedCredentials()` to copy `SessionDuration` from keyring:

```go
result := &types.AWSCredentials{
    AccessKeyID:     keystoreCreds.AccessKeyID,
    SecretAccessKey: keystoreCreds.SecretAccessKey,
    MfaArn:          keystoreCreds.MfaArn,
    SessionDuration: keystoreCreds.SessionDuration, // FIX: Preserve session duration from keyring
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

### 4.1 Unit Tests

Add/update tests for each error code scenario:

- `TestUserIdentity_HandleSTSError_InvalidClientTokenId` - Verify credentials cleared, prompting called
- `TestUserIdentity_HandleSTSError_ExpiredTokenException` - Verify correct error message
- `TestUserIdentity_HandleSTSError_AccessDenied` - Verify IAM guidance
- `TestUserIdentity_HandleSTSError_GenericError` - Verify fallback behavior

### 4.2 Test Implementation

Use `mockAPIError` type implementing `smithy.APIError` interface:

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

| File                                   | Changes                                                                                     |
|----------------------------------------|---------------------------------------------------------------------------------------------|
| `errors/errors.go`                     | Add `ErrAwsCredentialsInvalid` sentinel error                                               |
| `pkg/auth/identities/aws/user.go`      | Add `PromptCredentialsFunc`, update `handleSTSError()`, fix `resolveLongLivedCredentials()` |
| `pkg/auth/identities/aws/user_test.go` | Update tests for new return signature                                                       |
| `cmd/auth_credential_prompt.go`        | New file - credential prompting implementation                                              |

## 6. Success Metrics

- **Single Command Recovery**: Users can recover from credential invalidation with just `atmos auth login`
- **Session Duration Works**: 36-hour MFA sessions last the full 36 hours
- **Error Clarity**: Error messages provide specific, actionable remediation steps
- **Automatic Cleanup**: Stale credentials are cleared automatically, preventing repeated failures
- **Test Coverage**: All error scenarios have unit test coverage

## 7. Future Considerations

- Add similar error detection for other AWS API errors (AssumeRole, AssumeRoleWithSAML)
- Consider adding a `atmos auth repair` command for manual credential cleanup
- Add telemetry for credential invalidation events to track frequency
