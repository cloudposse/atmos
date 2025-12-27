# PRD: AWS Browser-Based Authentication for Root and IAM Identities

**Status:** Draft
**Author:** Cloud Posse
**Created:** 2025-12-17
**Linear Issue:** [DEV-3829](https://linear.app/cloudposse/issue/DEV-3829)

## Overview

Enhance the existing `aws/user` identity to support browser-based OAuth2 authentication as a fallback when no static credentials are configured. This enables developers to authenticate using the same credentials they use for the AWS Management Console, eliminating the need for long-term IAM access keys.

## Problem Statement

Currently, the `aws/user` identity requires one of:
1. Hardcoded credentials in YAML config
2. Credentials stored in keychain via `atmos auth user configure`

If neither is available, authentication fails with an error prompting the user to run `atmos auth user configure`.

Many AWS users authenticate to the console using:
- Root user credentials
- IAM user username/password
- Federated identity providers (non-SSO)

These users should be able to authenticate seamlessly without pre-configuring static credentials.

## Solution

Enhance the `aws/user` identity with a three-tier credential resolution strategy:

1. **YAML Config** - Hardcoded or environment-sourced credentials
2. **Keychain** - Credentials stored via `atmos auth user configure`
3. **Browser Webflow** - OAuth2 authentication via AWS console sign-in (NEW)

This is NOT a new provider type - it enhances the existing `aws/user` identity.

## User Stories

### US-1: Zero-Config Authentication
**As** a developer with an AWS account
**I want** to use `atmos auth login` without pre-configuring credentials
**So that** I can get started immediately using my console credentials

### US-2: Fallback Authentication
**As** a developer whose keychain credentials expired or were deleted
**I want** atmos to automatically fall back to browser authentication
**So that** I don't need to manually reconfigure credentials

### US-3: Federated User Authentication
**As** a developer who signs in via corporate identity provider
**I want** to use my existing console session for CLI access
**So that** I have a seamless authentication experience

### US-4: Headless Environment Authentication
**As** a developer working on a remote server without a browser
**I want** to authenticate using a manual authorization code flow
**So that** I can work in headless environments

### US-5: Role Chaining
**As** a developer
**I want** to assume IAM roles using my browser-authenticated credentials
**So that** I can access resources in different accounts

## Technical Details

### AWS Login OAuth2 Flow

The `aws login` command uses OAuth 2.0 Authorization Code flow with PKCE:

1. **Client Registration**: Uses fixed client ID `arn:aws:signin:::devtools/same-device`
2. **Authorization Endpoint**: `https://{region}.signin.aws.amazon.com/authorize`
3. **PKCE**: SHA-256 code challenge method
4. **Callback**: Local server at `http://127.0.0.1:<port>/oauth/callback`
5. **IAM Actions**: `signin:AuthorizeOAuth2Access`, `signin:CreateOAuth2Token`

### Credential Lifecycle

- **Auto-refresh**: Credentials are automatically rotated every 15 minutes
- **Session duration**: Valid up to 12 hours (limited by IAM principal's max session)
- **Cache location**: `~/.aws/login/cache` (or `AWS_LOGIN_CACHE_DIRECTORY`)

### CloudTrail Events

Two new events are logged:
- `AuthorizeOAuth2Access` - When authorization is granted
- `CreateOAuth2Token` - When tokens are exchanged

## Requirements

### Functional Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| FR-1 | Support browser-based OAuth2 authentication flow | P0 |
| FR-2 | Support headless/remote mode with manual code entry | P0 |
| FR-3 | Cache credentials following AWS CLI conventions | P0 |
| FR-4 | Validate AWS CLI version >= 2.32.0 | P0 |
| FR-5 | Integrate with existing `aws/assume-role` identity for role chaining | P0 |
| FR-6 | Support multiple profiles/providers | P1 |
| FR-7 | Display authentication status with spinner UI | P1 |
| FR-8 | Auto-detect non-TTY and switch to remote mode | P2 |

### Non-Functional Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| NFR-1 | Native SDK implementation (no AWS CLI dependency) | P0 |
| NFR-2 | Unit tests with mocked HTTP server and OAuth flow | P0 |
| NFR-3 | Documentation in Docusaurus | P0 |
| NFR-4 | Error messages include actionable hints | P1 |

## Configuration Schema

### Identity Configuration

The `aws/user` identity now supports three credential sources with automatic fallback:

```yaml
auth:
  identities:
    <identity-name>:
      kind: aws/user
      credentials:
        region: <aws-region>      # Optional: AWS region (default: us-east-1)
        # Optional: Static credentials (highest priority)
        access_key_id: <key>
        secret_access_key: <secret>
        mfa_arn: <arn>            # Optional: MFA device ARN
      session:
        duration: <duration>      # Optional: Session duration (default: 12h, max: 12h)
```

### Credential Resolution Order

1. **YAML Config** - If `access_key_id` and `secret_access_key` are set
2. **Keychain** - If credentials stored via `atmos auth user configure`
3. **Browser Webflow** - If neither above is available (NEW)

### Example Configurations

#### Zero-Config (Browser Auth Only)
```yaml
auth:
  identities:
    my-user:
      kind: aws/user
      # No credentials block - will use browser authentication
```

#### Static Credentials from Environment
```yaml
auth:
  identities:
    my-user:
      kind: aws/user
      credentials:
        access_key_id: !env MY_AWS_ACCESS_KEY_ID
        secret_access_key: !env MY_AWS_SECRET_ACCESS_KEY
```

#### With Role Chaining
```yaml
auth:
  identities:
    my-user:
      kind: aws/user
      # Browser auth as fallback

    prod-admin:
      kind: aws/assume-role
      via:
        identity: my-user
      principal:
        role_arn: arn:aws:iam::123456789012:role/AdminRole
```

## User Experience

### Interactive Flow (Browser Available)

```
$ atmos auth login --identity my-user

No stored credentials found for 'my-user'.
Starting browser authentication...

╭──────────────────────────────────────────────────────────╮
│  🔐 AWS Browser Authentication                           │
│                                                          │
│  Opening browser for authentication...                   │
│  If the browser doesn't open, visit:                     │
│  https://us-east-1.signin.aws.amazon.com/authorize?...   │
╰──────────────────────────────────────────────────────────╯

⠋ Waiting for authentication...

✓ Authentication successful!

  Identity    my-user
  Account     123456789012
  Principal   arn:aws:iam::123456789012:user/developer
  Region      us-east-1
  Expires     2025-12-17 22:00:00 UTC (11h 59m)
```

### Headless Flow (Remote Mode)

When running in a non-TTY environment or with `--remote` flag:

```
$ atmos auth login --identity my-user --remote

No stored credentials found for 'my-user'.
Starting browser authentication (remote mode)...

╭──────────────────────────────────────────────────────────╮
│  🔐 AWS Browser Authentication (Remote Mode)             │
│                                                          │
│  Visit this URL on a device with a browser:              │
│  https://us-east-1.signin.aws.amazon.com/authorize?...   │
│                                                          │
│  After signing in, paste the authorization code below.   │
╰──────────────────────────────────────────────────────────╯

Authorization code: █

✓ Authentication successful!
```

### Credential Resolution Flow

```
┌─────────────────────────────────────────────────────────────┐
│                    aws/user Identity                         │
├─────────────────────────────────────────────────────────────┤
│  1. Check YAML config for credentials                       │
│     └─ Found? → Use static credentials                      │
│                                                             │
│  2. Check keychain for stored credentials                   │
│     └─ Found? → Use keychain credentials                    │
│                                                             │
│  3. Initiate browser webflow                                │
│     └─ Interactive TTY? → Open browser automatically        │
│     └─ Headless/--remote? → Display URL for manual auth     │
└─────────────────────────────────────────────────────────────┘
```

## Prerequisites

### User Requirements

1. **IAM Permissions**: Principal must have `SignInLocalDevelopmentAccess` managed policy or equivalent:
   ```json
   {
     "Version": "2012-10-17",
     "Statement": [
       {
         "Effect": "Allow",
         "Action": [
           "signin:AuthorizeOAuth2Access",
           "signin:CreateOAuth2Token"
         ],
         "Resource": "*"
       }
     ]
   }
   ```
2. **Console Access**: IAM user must have console sign-in enabled
3. **Browser Access**: Default browser must be available (or use remote mode for headless)

### Organizational Controls

- Organizations can deny these actions via SCPs to prevent usage
- Centralized root access management can prevent root login on member accounts

## Implementation Approach

### Enhance `aws/user` Identity with Webflow Fallback

Modify `pkg/auth/identities/aws/user.go` to add browser authentication as a third credential source.

**Key Changes to `resolveLongLivedCredentials()`:**

```go
// Current flow (simplified):
// 1. Check YAML config → return if found
// 2. Check keychain → return if found
// 3. Return error "credentials not found"

// New flow:
// 1. Check YAML config → return if found
// 2. Check keychain → return if found
// 3. Initiate browser webflow → return credentials
```

**New Files:**

| File | Purpose |
|------|---------|
| `pkg/auth/identities/aws/webflow.go` | OAuth2 PKCE flow implementation |
| `pkg/auth/identities/aws/webflow_test.go` | Unit tests with mocked HTTP |

**Technical Implementation (in `webflow.go`):**

1. Start local HTTP server on `http://127.0.0.1:<port>/oauth/callback`
2. Generate PKCE code verifier (random 32-byte string, base64url encoded)
3. Generate code challenge (SHA-256 hash of verifier, base64url encoded)
4. Open browser to authorization URL:
   ```
   https://{region}.signin.aws.amazon.com/authorize?
     client_id=arn:aws:signin:::devtools/same-device
     &redirect_uri=http://127.0.0.1:{port}/oauth/callback
     &response_type=code
     &code_challenge={challenge}
     &code_challenge_method=S256
     &scope=openid
   ```
5. Receive authorization code via callback
6. Exchange code for tokens via AWS signin service
7. Return temporary AWS credentials

**Advantages:**
- No new provider type - extends existing `aws/user` identity
- Seamless fallback when no credentials configured
- Follows existing atmos auth patterns (similar to SSO device flow)
- No external AWS CLI dependency

## Security Considerations

1. **No Long-Term Credentials**: Only temporary credentials are issued
2. **PKCE Protection**: Authorization code interception attacks are mitigated
3. **Short-Lived Tokens**: Credentials rotate every 15 minutes
4. **CloudTrail Logging**: All authentication events are logged
5. **MFA Support**: MFA requirements are enforced by AWS during browser sign-in

## Testing Strategy

### Unit Tests
- Provider configuration validation
- AWS CLI version detection and validation
- Cache file parsing (valid, expired, malformed)
- Error handling for CLI failures

### Integration Tests
- Manual testing with real AWS account
- Headless mode flow verification

### Snapshot Tests
- UI output consistency across platforms

## Documentation

### Files to Create/Update
- `website/docs/core-concepts/authentication/aws-user.mdx` - Update with browser auth fallback
- `website/docs/cli/commands/auth/login.mdx` - Add `--remote` flag documentation
- Schema documentation updates for credential resolution order

## Rollout Plan

### Phase 1: Core Implementation
- Create `pkg/auth/identities/aws/webflow.go` with OAuth2 PKCE flow
- Modify `resolveLongLivedCredentials()` in `user.go` to add webflow fallback
- Add error handling and hints

### Phase 2: UI and Integration
- Implement browser opening and spinner UI (reuse patterns from SSO)
- Add headless/remote mode support with `--remote` flag
- Add unit tests with mocked HTTP server

### Phase 3: Documentation and Polish
- Update Docusaurus documentation for `aws/user` identity
- Add example configurations
- Blog post for release

## Success Metrics

1. **Adoption**: Number of users using `aws/user` without static credentials
2. **Error Rate**: Authentication failures due to implementation issues
3. **User Feedback**: GitHub issues and community feedback

## Open Questions

1. **Auto-Detection**: Should we auto-detect non-TTY and switch to remote mode automatically?
2. **Credential Caching**: Should webflow credentials be cached in keychain after successful auth?
3. **Disable Option**: Should there be a config option to disable webflow fallback (for security-conscious orgs)?

## References

- [AWS Blog: Simplified developer access to AWS with 'aws login'](https://aws.amazon.com/blogs/security/simplified-developer-access-to-aws-with-aws-login/)
- [AWS CLI Login Documentation](https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-sign-in.html)
- [AWS CLI IAM Identity Center Configuration](https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-sso.html)
- [Beyond IAM access keys: Modern authentication approaches](https://aws.amazon.com/blogs/security/beyond-iam-access-keys-modern-authentication-approaches/)
