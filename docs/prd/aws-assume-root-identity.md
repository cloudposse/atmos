# PRD: AWS Assume Root Identity (`aws/assume-root`)

## Overview

**Feature**: `aws/assume-root` identity kind for centralized root access management
**Status**: Proposed
**Created**: 2025-12-02
**Author**: Daniel Miller (@milldr)
**Target Release**: TBD

## Executive Summary

The `aws/assume-root` identity kind enables Atmos users to leverage AWS's centralized root access feature via the `sts:AssumeRoot` API. This allows organizations that have enabled centralized root access to perform privileged root-level operations (such as deleting root credentials in delegated accounts) through a single permission set in the management account, rather than managing individual root credentials across all member accounts.

**Use Case**: Organizations implementing AWS best practices for centralized root access can use `atmos auth exec --identity core-audit/iam-audit-root-access` to assume root-level privileges on member accounts. This enables specific task policies for operations like credential auditing, root password management, and S3/SQS bucket policy unlocking.

## Problem Statement

### Current State

Organizations that have enabled [AWS Centralized Root Access](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_root-enable-root-access.html) need to:

1. Authenticate to a permission set with `sts:AssumeRoot` privileges
2. Manually run `aws sts assume-root` with the target account and task policy
3. Parse the output and export the credentials as environment variables
4. Execute their commands with the assumed root credentials

This multi-step process is error-prone and not integrated into Atmos's identity chaining model.

**Current Workaround (Manual)**:
```bash
# Step 1: Authenticate to the permission set
atmos auth shell --identity organizational-root-access

# Step 2: Assume root on member account (manually)
eval $(aws sts assume-root \
  --target-principal <MEMBER_ACCOUNT_ID> \
  --task-policy-arn arn:aws:iam::aws:policy/root-task/IAMDeleteRootUserCredentials \
  --query 'Credentials.[AccessKeyId,SecretAccessKey,SessionToken]' \
  --output text | awk '{print "export AWS_ACCESS_KEY_ID="$1" AWS_SECRET_ACCESS_KEY="$2" AWS_SESSION_TOKEN="$3}')

# Step 3: Verify and use
aws sts get-caller-identity
```

### Pain Points

1. **Multi-Step Process**: Users must manually chain authentication steps
2. **Error-Prone**: Parsing STS output and exporting variables manually is fragile
3. **No Integration**: AssumeRoot is not integrated into Atmos identity chains
4. **Poor UX**: Cannot use standard `atmos auth exec --identity` pattern
5. **Documentation Burden**: Users must remember the exact AWS CLI commands and task policy ARNs

### User Impact

- DevOps engineers spend time on manual credential management
- Security teams cannot easily audit root access operations
- Automation of root-level tasks is difficult
- Risk of credential exposure from manual handling

## Goals and Non-Goals

### Goals

1. **Native Integration**: Support `aws/assume-root` as a first-class identity kind
2. **Identity Chaining**: Allow assume-root to chain from permission sets or other identities
3. **Task Policy Support**: Support AWS-managed task policies for scoped root access
4. **Consistent UX**: Enable `atmos auth exec --identity my-root-identity` pattern
5. **Credential Management**: Handle temporary credentials like other identity types
6. **Test Coverage**: Achieve 85-90% test coverage

### Non-Goals

1. **Root Credential Creation**: Does not create or manage actual root user credentials
2. **Root User Authentication**: Does not support direct root user login (only assume-root)
3. **Custom Task Policies**: Only supports AWS-managed task policies (not custom policies)
4. **Organization Management**: Does not manage centralized root access settings

## User Stories

### US-1: Assume Root for Credential Deletion

**As a** security engineer
**I want to** assume root on member accounts to delete root credentials
**So that** I can enforce centralized root access without managing individual root passwords

**Acceptance Criteria**:
- Can configure `aws/assume-root` identity in `atmos.yaml`
- Can specify target account and task policy
- Can execute commands with root credentials via `atmos auth exec`
- Credentials are temporary and scoped to the task policy

### US-2: Chain from Permission Set

**As a** platform engineer
**I want to** chain assume-root from my existing permission set identity
**So that** I can leverage existing SSO authentication for root operations

**Acceptance Criteria**:
- `aws/assume-root` can use `via.identity` to chain from permission set
- Authentication flows through the full chain automatically
- No manual intermediate steps required

### US-3: Audit Root Credentials

**As a** compliance officer
**I want to** audit root credentials across all member accounts
**So that** I can verify compliance with security policies

**Acceptance Criteria**:
- Can use `IAMAuditRootUserCredentials` task policy
- Can iterate over multiple accounts (scripting support)
- Audit operations are logged and traceable

### US-4: Unlock S3 Bucket Policy

**As an** operations engineer
**I want to** unlock an S3 bucket with a restrictive policy
**So that** I can recover from misconfigured bucket policies

**Acceptance Criteria**:
- Can use `S3UnlockBucketPolicy` task policy
- Can target specific member account
- Operation completes without root user password

## Functional Requirements

### FR-1: Identity Kind Registration

**Requirement**: Register `aws/assume-root` as a valid identity kind

**Configuration**:
```yaml
auth:
  identities:
    core-audit/iam-audit-root-access:
      kind: aws/assume-root
      via:
        identity: organizational-root-access  # Permission set with sts:AssumeRoot
      principal:
        target_principal: "123456789012"      # Target member account ID
        task_policy_arn: arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials
```

**Implementation**:
- Add `"aws/assume-root"` case to `factory.NewIdentity()`.
- Create `awsIdentities.NewAssumeRootIdentity()` constructor.
- Add constant `IdentityKindAWSAssumeRoot = "aws/assume-root"` to `types/constants.go`.

### FR-2: Principal Configuration

**Requirement**: Support required and optional principal fields

**Required Fields**:
- `target_principal`: AWS account ID of the member account (12-digit string)
- `task_policy_arn`: ARN of the AWS-managed task policy

**Optional Fields**:
- `duration`: Session duration (default: 900 seconds / 15 minutes, max: 900 seconds per AWS limit)

**Validation**:
- `target_principal` must be a valid 12-digit AWS account ID
- `task_policy_arn` must match pattern `arn:aws:iam::aws:policy/root-task/*`
- `duration` must be between 1 and 900 seconds

### FR-3: AWS-Managed Task Policies

**Requirement**: Support all AWS-managed root task policies

**Supported Task Policies**:

| Task Policy ARN | Description |
|-----------------|-------------|
| `arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials` | Audit root user credentials (MFA, access keys, etc.) |
| `arn:aws:iam::aws:policy/root-task/IAMCreateRootUserPassword` | Create/reset root user password |
| `arn:aws:iam::aws:policy/root-task/IAMDeleteRootUserCredentials` | Delete root user credentials |
| `arn:aws:iam::aws:policy/root-task/S3UnlockBucketPolicy` | Unlock restrictive S3 bucket policies |
| `arn:aws:iam::aws:policy/root-task/SQSUnlockQueuePolicy` | Unlock restrictive SQS queue policies |

**Implementation**:
- Validate task policy ARN against known patterns
- Provide helpful hints for invalid ARNs
- Document all supported task policies

### FR-4: Authentication Flow

**Requirement**: Implement `sts:AssumeRoot` API call

**Authentication Steps**:
1. Authenticate via parent identity (permission set with `sts:AssumeRoot`)
2. Call `sts.AssumeRoot()` with target principal and task policy
3. Extract temporary credentials from response
4. Return `AWSCredentials` with scoped root access.

**Error Handling**:
- `AccessDenied`: Permission set lacks `sts:AssumeRoot` permission
- `InvalidParameterValue`: Invalid account ID or task policy ARN
- `MalformedPolicyDocument`: Unsupported task policy
- `RegionDisabledException`: Target account is in a disabled region

### FR-5: Identity Chaining

**Requirement**: Support chaining from other identity types

**Supported Via**:
- `via.identity`: Chain from another identity (permission-set, assume-role)
- `via.provider`: Direct from provider (less common)

**Example Chains**:
```yaml
# Chain: SSO Provider -> Permission Set -> Assume Root
auth:
  providers:
    company-sso:
      kind: aws/iam-identity-center
      region: us-east-1
      start_url: https://company.awsapps.com/start

  identities:
    root-access-permission-set:
      kind: aws/permission-set
      via:
        provider: company-sso
      principal:
        name: RootAccess
        account:
          name: management-account

    member-account-root:
      kind: aws/assume-root
      via:
        identity: root-access-permission-set
      principal:
        target_principal: "123456789012"
        task_policy_arn: arn:aws:iam::aws:policy/root-task/IAMDeleteRootUserCredentials
```

### FR-6: Credential Output

**Requirement**: Return standard AWS credentials

**Credential Structure**:
```go
type AWSCredentials struct {
    AccessKeyID     string
    SecretAccessKey string
    SessionToken    string
    Region          string
    Expiration      string  // RFC3339 timestamp
}
```

**Behavior**:
- Credentials are temporary (max 15 minutes)
- Session token is required (no long-term credentials)
- Region inherited from parent identity or default

### FR-7: Environment Preparation

**Requirement**: Prepare environment for subprocess execution

**Environment Variables**:
- `AWS_ACCESS_KEY_ID`: Temporary access key
- `AWS_SECRET_ACCESS_KEY`: Temporary secret key
- `AWS_SESSION_TOKEN`: Session token
- `AWS_REGION`: Region (inherited)
- `AWS_DEFAULT_REGION`: Region (inherited)

**Credential Files**:
- Write to isolated credential/config files (per-provider path)
- Set `AWS_SHARED_CREDENTIALS_FILE` and `AWS_CONFIG_FILE`
- Use identity name as profile name

## Non-Functional Requirements

### NFR-1: Performance

**Requirement**: Authentication completes within acceptable time

**Targets**:
- Full chain authentication: < 5 seconds (excluding SSO browser flow)
- AssumeRoot API call: < 2 seconds
- Environment preparation: < 100ms

### NFR-2: Test Coverage

**Requirement**: Achieve 85-90% test coverage

**Test Categories**:
1. **Unit Tests**: Identity creation, validation, credential handling
2. **Mock Tests**: STS API interactions with mocked AWS SDK
3. **Integration Tests**: Full chain authentication (with mock provider)

### NFR-3: Security

**Requirement**: Follow security best practices

**Security Considerations**:
- Never log credentials
- Clear credentials from memory after use
- Validate all input parameters
- Use short session durations
- Mask credentials in error messages

### NFR-4: Documentation

**Requirement**: Comprehensive user documentation

**Documentation Components**:
1. Identity kind reference
2. Configuration examples
3. Task policy descriptions
4. Troubleshooting guide
5. AWS prerequisites

## Technical Design

### Architecture

```text
pkg/auth/identities/aws/
├─ assume_root.go          # Main implementation
├─ assume_root_test.go     # Unit tests
└─ (existing files)

pkg/auth/factory/
└─ factory.go              # Add aws/assume-root case

pkg/auth/types/
└─ constants.go            # Add IdentityKindAWSAssumeRoot
```

### Data Structures

```go
// assume_root.go

// assumeRootIdentity implements AWS assume root identity.
type assumeRootIdentity struct {
    name             string
    config           *schema.Identity
    region           string
    targetPrincipal  string          // Target member account ID
    taskPolicyArn    string          // AWS-managed task policy ARN
    manager          types.AuthManager
    rootProviderName string
}
```

### Key Functions

```go
// NewAssumeRootIdentity creates a new AWS assume root identity.
func NewAssumeRootIdentity(name string, config *schema.Identity) (types.Identity, error)

// Kind returns the identity kind.
func (i *assumeRootIdentity) Kind() string

// Validate validates the identity configuration.
func (i *assumeRootIdentity) Validate() error

// Authenticate performs authentication using sts:AssumeRoot.
func (i *assumeRootIdentity) Authenticate(ctx context.Context, baseCreds types.ICredentials) (types.ICredentials, error)

// Environment returns environment variables for this identity.
func (i *assumeRootIdentity) Environment() (map[string]string, error)

// PrepareEnvironment prepares environment variables for external processes.
func (i *assumeRootIdentity) PrepareEnvironment(ctx context.Context, environ map[string]string) (map[string]string, error)

// PostAuthenticate sets up AWS files after authentication.
func (i *assumeRootIdentity) PostAuthenticate(ctx context.Context, params *types.PostAuthenticateParams) error

// LoadCredentials loads credentials from storage.
func (i *assumeRootIdentity) LoadCredentials(ctx context.Context) (types.ICredentials, error)

// CredentialsExist checks if credentials exist for this identity.
func (i *assumeRootIdentity) CredentialsExist() (bool, error)

// Logout removes identity-specific credential storage.
func (i *assumeRootIdentity) Logout(ctx context.Context) error
```

### AWS STS Integration

```go
// AssumeRoot API call
func (i *assumeRootIdentity) callAssumeRoot(ctx context.Context, stsClient *sts.Client) (*sts.AssumeRootOutput, error) {
    input := &sts.AssumeRootInput{
        TargetPrincipal: aws.String(i.targetPrincipal),
        TaskPolicyArn: &ststypes.PolicyDescriptorType{
            Arn: aws.String(i.taskPolicyArn),
        },
    }

    // Optional duration (default and max: 900 seconds)
    if i.config.Principal["duration"] != nil {
        if durationStr, ok := i.config.Principal["duration"].(string); ok {
            if duration, err := time.ParseDuration(durationStr); err == nil {
                input.DurationSeconds = aws.Int32(int32(duration.Seconds()))
            }
        }
    }

    return stsClient.AssumeRoot(ctx, input)
}
```

### Error Handling

**Error Types**:
- `ErrInvalidIdentityConfig`: Configuration errors
- `ErrMissingPrincipal`: Missing required principal fields
- `ErrAuthenticationFailed`: AssumeRoot API failures
- `ErrInvalidTargetPrincipal`: Invalid account ID format
- `ErrInvalidTaskPolicyArn`: Invalid or unsupported task policy

**Error Builder Usage**:
```go
return errUtils.Build(errUtils.ErrAuthenticationFailed).
    WithExplanationf("Failed to assume root on account '%s'", i.targetPrincipal).
    WithHint("Verify the target account is a member of your organization").
    WithHint("Ensure centralized root access is enabled for the target account").
    WithHint("Check that your permission set has sts:AssumeRoot permission").
    WithContext("identity", i.name).
    WithContext("target_principal", i.targetPrincipal).
    WithContext("task_policy_arn", i.taskPolicyArn).
    WithExitCode(1).
    Err()
```

## Implementation Plan

### Phase 1: Core Implementation

**Tasks**:
1. Create `pkg/auth/identities/aws/assume_root.go`
2. Implement `NewAssumeRootIdentity()` constructor
3. Implement `Kind()` and `Validate()` methods
4. Add constant to `pkg/auth/types/constants.go`
5. Register in `pkg/auth/factory/factory.go`
6. Write unit tests for validation

**Deliverable**: Identity registration and validation working

### Phase 2: Authentication

**Tasks**:
1. Implement `Authenticate()` method
2. Implement STS AssumeRoot API call
3. Implement credential conversion
4. Handle error cases
5. Write mock tests for authentication

**Deliverable**: AssumeRoot authentication working

### Phase 3: Credential Management

**Tasks**:
1. Implement `Environment()` method
2. Implement `PrepareEnvironment()` method
3. Implement `PostAuthenticate()` method
4. Implement `LoadCredentials()` method
5. Implement `CredentialsExist()` method
6. Implement `Logout()` method
7. Write tests for credential management

**Deliverable**: Full credential lifecycle working

### Phase 4: Testing & Documentation

**Tasks**:
1. Write integration tests
2. Achieve 85-90% coverage
3. Create user documentation
4. Add configuration examples
5. Update schema definitions

**Deliverable**: Production-ready with documentation

## Testing Strategy

### Unit Tests

```go
// assume_root_test.go
func TestNewAssumeRootIdentity_Valid(t *testing.T)
func TestNewAssumeRootIdentity_MissingTargetPrincipal(t *testing.T)
func TestNewAssumeRootIdentity_MissingTaskPolicyArn(t *testing.T)
func TestNewAssumeRootIdentity_InvalidTargetPrincipal(t *testing.T)
func TestNewAssumeRootIdentity_InvalidTaskPolicyArn(t *testing.T)
func TestAssumeRootIdentity_Kind(t *testing.T)
func TestAssumeRootIdentity_Validate(t *testing.T)
func TestAssumeRootIdentity_Authenticate_Success(t *testing.T)
func TestAssumeRootIdentity_Authenticate_AccessDenied(t *testing.T)
func TestAssumeRootIdentity_Authenticate_InvalidAccount(t *testing.T)
func TestAssumeRootIdentity_Environment(t *testing.T)
func TestAssumeRootIdentity_PrepareEnvironment(t *testing.T)
func TestAssumeRootIdentity_PostAuthenticate(t *testing.T)
func TestAssumeRootIdentity_LoadCredentials(t *testing.T)
func TestAssumeRootIdentity_CredentialsExist(t *testing.T)
func TestAssumeRootIdentity_Logout(t *testing.T)
```

### Integration Tests

```go
func TestAssumeRoot_FullChain_PermissionSet(t *testing.T)
func TestAssumeRoot_ExecCommand(t *testing.T)
func TestAssumeRoot_ShellEnvironment(t *testing.T)
```

## Acceptance Criteria

### Functional

- [ ] `aws/assume-root` identity kind is recognized
- [ ] `target_principal` validation works (12-digit account ID)
- [ ] `task_policy_arn` validation works (AWS root-task policy pattern)
- [ ] Authentication via `sts:AssumeRoot` succeeds with valid credentials
- [ ] Identity chaining from permission set works
- [ ] `atmos auth exec --identity my-root` executes commands with root credentials
- [ ] `atmos auth shell --identity my-root` exports root credentials
- [ ] `atmos auth whoami --identity my-root` shows root identity
- [ ] `atmos auth logout --identity my-root` clears credentials
- [ ] All AWS-managed task policies are supported

### Technical

- [ ] Test coverage >= 85%
- [ ] All tests pass
- [ ] `make lint` passes
- [ ] Pre-commit hooks pass
- [ ] Code follows CLAUDE.md conventions
- [ ] Error builder pattern used consistently
- [ ] Performance tracking added

### Documentation

- [ ] Identity kind documented
- [ ] Configuration examples provided
- [ ] Task policy reference included
- [ ] Troubleshooting guide written
- [ ] AWS prerequisites documented

## Risks and Mitigations

### Risk 1: AWS SDK Support

**Risk**: AWS SDK may not have `AssumeRoot` API support yet
**Impact**: High
**Probability**: Low (API is GA)
**Mitigation**: Verify SDK v2 includes `sts.AssumeRoot()`; if not, use raw API call.

### Risk 2: Centralized Root Access Prerequisites

**Risk**: Users may not have centralized root access enabled
**Impact**: Medium
**Probability**: Medium
**Mitigation**: Clear error messages and documentation about prerequisites.

### Risk 3: Task Policy Limitations

**Risk**: AWS may add new task policies
**Impact**: Low
**Probability**: Medium
**Mitigation**: Allow any ARN matching `arn:aws:iam::aws:policy/root-task/*` pattern.

## Success Metrics

### Usage Metrics

- Number of `aws/assume-root` identity configurations
- Most common task policies used
- Authentication success/failure rates

### Quality Metrics

- Test coverage achieved
- Bug reports in first 30 days
- Documentation completeness

## Future Enhancements

**Out of Scope for Initial Release**:
1. **Multiple Target Accounts**: Support for iterating over multiple accounts
2. **Account Discovery**: Auto-discover member accounts from Organizations API
3. **Task Policy Aliases**: Short names for common task policies
4. **Audit Logging**: Enhanced logging for compliance requirements

## References

- [AWS Centralized Root Access Documentation](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_root-enable-root-access.html)
- [AWS STS AssumeRoot API Reference](https://docs.aws.amazon.com/STS/latest/APIReference/API_AssumeRoot.html)
- [AWS Root Task Policies](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_root-user-access-management.html)
- [Atmos Auth Identity Chaining](https://atmos.tools/cli/commands/auth/)
- [Existing `aws/assume-role` Implementation](../../pkg/auth/identities/aws/assume_role.go)

## Appendix

### Example Configuration

```yaml
# atmos.yaml
auth:
  providers:
    ins-sso:
      kind: aws/iam-identity-center
      region: us-east-2
      start_url: https://inspatial.awsapps.com/start
      auto_provision_identities: true

  identities:
    # Permission set with sts:AssumeRoot permission
    organizational-root-access:
      kind: aws/permission-set
      via:
        provider: ins-sso
      principal:
        name: RootAccess
        account:
          name: InSpatial AWS CP  # core-root / management account

    # Assume root for auditing root credentials
    core-audit/iam-audit-root-access:
      kind: aws/assume-root
      via:
        identity: organizational-root-access
      principal:
        target_principal: "123456789012"  # Member account ID
        task_policy_arn: arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials

    # Assume root for deleting root credentials
    core-audit/iam-delete-root-credentials:
      kind: aws/assume-root
      via:
        identity: organizational-root-access
      principal:
        target_principal: "123456789012"  # Member account ID
        task_policy_arn: arn:aws:iam::aws:policy/root-task/IAMDeleteRootUserCredentials

    # Assume root for unlocking S3 bucket
    member-account/s3-unlock:
      kind: aws/assume-root
      via:
        identity: organizational-root-access
      principal:
        target_principal: "987654321098"  # Different member account
        task_policy_arn: arn:aws:iam::aws:policy/root-task/S3UnlockBucketPolicy
```

### Example Usage

```bash
# Authenticate and execute command with root credentials
atmos auth exec --identity core-audit/iam-audit-root-access -- aws sts get-caller-identity

# Start shell with root credentials
atmos auth shell --identity core-audit/iam-audit-root-access

# Check current root identity
atmos auth whoami --identity core-audit/iam-audit-root-access

# Delete root credentials on member account
atmos auth exec --identity core-audit/iam-delete-root-credentials -- \
  aws iam delete-login-profile --user-name root

# Unlock S3 bucket policy
atmos auth exec --identity member-account/s3-unlock -- \
  aws s3api delete-bucket-policy --bucket my-locked-bucket

# Logout and clear credentials
atmos auth logout --identity core-audit/iam-audit-root-access
```

### AWS Prerequisites

To use `aws/assume-root` identities, your AWS organization must have:

1. **Centralized Root Access Enabled**: In the AWS Organizations management account, enable centralized root access for member accounts.

2. **Permission Set with AssumeRoot**: Create an IAM Identity Center permission set with the `sts:AssumeRoot` permission:
   ```json
   {
     "Version": "2012-10-17",
     "Statement": [
       {
         "Effect": "Allow",
         "Action": "sts:AssumeRoot",
         "Resource": "*",
         "Condition": {
           "StringEquals": {
             "sts:TaskPolicyArn": [
               "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
               "arn:aws:iam::aws:policy/root-task/IAMDeleteRootUserCredentials",
               "arn:aws:iam::aws:policy/root-task/IAMCreateRootUserPassword",
               "arn:aws:iam::aws:policy/root-task/S3UnlockBucketPolicy",
               "arn:aws:iam::aws:policy/root-task/SQSUnlockQueuePolicy"
             ]
           }
         }
       }
     ]
   }
   ```

3. **Assignment to Management Account**: The permission set must be assigned to users/groups in the organization's management account.

4. **Member Account Enablement**: Target member accounts must have centralized root access enabled (this is the default for new accounts when the feature is enabled organization-wide).

---

**Document Version**: 1.0
**Last Updated**: 2025-12-02
**Status**: Proposed
