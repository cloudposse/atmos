# PRD: AWS Browser-Based Authentication for Root and IAM Identities

**Status:** Draft
**Author:** Cloud Posse
**Created:** 2025-12-17
**Linear Issue:** [DEV-3829](https://linear.app/cloudposse/issue/DEV-3829)

## Overview

Add support for AWS browser-based authentication using the new `aws login` command introduced in AWS CLI 2.32.0. This enables developers to authenticate to AWS using the same credentials they use for the AWS Management Console, eliminating the need for long-term IAM access keys.

## Problem Statement

Currently, atmos supports AWS authentication via:
- **IAM Identity Center (SSO)** - `aws/iam-identity-center` provider
- **SAML** - `aws/saml` provider
- **Static IAM User Credentials** - `aws/user` identity

However, many AWS users authenticate to the console using:
- Root user credentials
- IAM user username/password
- Federated identity providers (non-SSO)

These users currently have no secure way to obtain programmatic credentials in atmos without creating long-term access keys, which AWS discourages as a security risk.

## Solution

Introduce a new `aws/login` provider that leverages the AWS CLI's `aws login` command to obtain temporary credentials through browser-based OAuth2 authentication with PKCE.

## User Stories

### US-1: IAM User Authentication
**As** a developer with an IAM user account
**I want** to authenticate using my console username and password
**So that** I can use atmos without creating long-term access keys

### US-2: Root User Authentication
**As** an AWS account administrator
**I want** to authenticate using root credentials when necessary
**So that** I can perform privileged operations securely

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
**I want** to assume IAM roles using my login credentials
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
| NFR-1 | No new Go dependencies (use AWS CLI wrapper) | P0 |
| NFR-2 | Unit tests with mocked CLI executor | P0 |
| NFR-3 | Documentation in Docusaurus | P0 |
| NFR-4 | Error messages include actionable hints | P1 |

## Configuration Schema

### Provider Configuration

```yaml
auth:
  providers:
    <provider-name>:
      kind: aws/login
      region: <aws-region>        # Required: AWS region
      session:
        duration: <duration>      # Optional: Session duration (default: 12h, max: 12h)
      spec:
        remote: <bool>            # Optional: Force headless mode (default: false)
        profile: <string>         # Optional: AWS CLI profile name (default: atmos-<provider-name>)
```

### Example Configurations

#### Basic Usage
```yaml
auth:
  providers:
    aws-console:
      kind: aws/login
      region: us-east-1
```

#### With Role Assumption
```yaml
auth:
  providers:
    aws-console:
      kind: aws/login
      region: us-east-1

  identities:
    prod-admin:
      kind: aws/assume-role
      via:
        provider: aws-console
      principal:
        role_arn: arn:aws:iam::123456789012:role/AdminRole
```

#### Headless Mode
```yaml
auth:
  providers:
    aws-console-remote:
      kind: aws/login
      region: us-east-1
      spec:
        remote: true
```

## User Experience

### Interactive Flow (Browser Available)

```
$ atmos auth login --provider aws-console

╭──────────────────────────────────────────────────────────╮
│  🔐 AWS Browser Authentication                           │
│                                                          │
│  Opening browser for authentication...                   │
│  If the browser doesn't open, visit:                     │
│  https://us-east-1.signin.aws.amazon.com/authorize?...   │
╰──────────────────────────────────────────────────────────╯

⠋ Waiting for authentication...

✓ Authentication successful!

  Provider    aws-console
  Account     123456789012
  Principal   arn:aws:iam::123456789012:user/developer
  Region      us-east-1
  Expires     2025-12-17 22:00:00 UTC (11h 59m)
```

### Headless Flow (Remote Mode)

```
$ atmos auth login --provider aws-console-remote

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

## Prerequisites

### User Requirements

1. **AWS CLI 2.32.0+** must be installed
2. **IAM Permissions**: Principal must have `SignInLocalDevelopmentAccess` managed policy or equivalent:
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
3. **Console Access**: IAM user must have console sign-in enabled

### Organizational Controls

- Organizations can deny these actions via SCPs to prevent usage
- Centralized root access management can prevent root login on member accounts

## Implementation Approach

### Recommended: AWS CLI Wrapper

Wrap the AWS CLI `aws login` command rather than implementing OAuth2 natively:

**Advantages:**
- AWS CLI handles credential refresh (15-minute rotation)
- Maintains compatibility as AWS evolves the protocol
- Credential caching follows AWS conventions
- Simpler implementation and maintenance

**Disadvantages:**
- Requires AWS CLI 2.32.0+ as external dependency
- Less control over the authentication UX

### Alternative: Native OAuth2 Implementation

Implement the OAuth2 + PKCE flow directly in Go:

**Advantages:**
- No external dependency
- Full control over UX
- Could potentially work without AWS CLI

**Disadvantages:**
- Must implement credential refresh mechanism
- Token endpoint details not fully documented
- Higher maintenance burden

**Recommendation:** Start with AWS CLI wrapper approach. Consider native implementation if CLI dependency becomes problematic.

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
- `website/docs/cli/commands/auth/login.mdx` - Update with new provider
- `website/docs/core-concepts/authentication/aws-login.mdx` - New concept page
- Schema documentation updates

## Rollout Plan

### Phase 1: Core Implementation
- Create `aws/login` provider
- Implement CLI wrapper with version validation
- Add error handling and hints

### Phase 2: UI and Integration
- Implement browser opening and spinner UI
- Add headless mode support
- Integration with `aws/assume-role`

### Phase 3: Documentation and Polish
- Docusaurus documentation
- Example configurations
- Blog post for release

## Success Metrics

1. **Adoption**: Number of users configuring `aws/login` provider
2. **Error Rate**: Authentication failures due to implementation issues
3. **User Feedback**: GitHub issues and community feedback

## Open Questions

1. **Profile Naming**: Should atmos auto-generate profile names or let users specify?
2. **Auto-Detection**: Should we auto-detect non-TTY and switch to remote mode?
3. **Credential Source**: When using role chaining, should we write credentials to files or pass via environment?

## References

- [AWS Blog: Simplified developer access to AWS with 'aws login'](https://aws.amazon.com/blogs/security/simplified-developer-access-to-aws-with-aws-login/)
- [AWS CLI Login Documentation](https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-sign-in.html)
- [AWS CLI IAM Identity Center Configuration](https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-sso.html)
- [Beyond IAM access keys: Modern authentication approaches](https://aws.amazon.com/blogs/security/beyond-iam-access-keys-modern-authentication-approaches/)
