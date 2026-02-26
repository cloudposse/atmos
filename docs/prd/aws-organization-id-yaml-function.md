# PRD: `!aws.organization_id` YAML Function

## Overview

Add a new `!aws.organization_id` YAML function that returns the current user's AWS Organization ID.
This complements the existing AWS context functions (`!aws.account_id`, `!aws.caller_identity_arn`,
`!aws.caller_identity_user_id`, `!aws.region`).

## Problem Statement

Users need to reference the AWS Organization ID in stack configurations for governance, tagging,
cross-account trust policies, and SCP scoping. Currently, the organization ID must be hardcoded
or retrieved through workarounds.

## Goals

1. Provide a `!aws.organization_id` YAML function that returns the AWS Organization ID.
2. Follow the same patterns as existing AWS functions for consistency.
3. Handle the case where the account is not in an organization with a clear error message.
4. Cache results to avoid repeated API calls within a single CLI invocation.

## Architecture

### Key Difference from Existing AWS Functions

All existing AWS functions (`!aws.account_id`, etc.) use the STS `GetCallerIdentity` API and share
one cached result. The new function requires:

- A different AWS service: Organizations (not STS).
- A different SDK client: `organizations.NewFromConfig()`.
- Different IAM permissions: `organizations:DescribeOrganization`.
- Different failure modes: Account may not be in an organization.

### Package Structure

- **`pkg/aws/organization/`** - New package parallel to `pkg/aws/identity/`.
  - `Getter` interface with `GetOrganization()` method.
  - `defaultGetter` that calls `organizations.DescribeOrganization`.
  - Per-auth-context caching with double-checked locking.
  - `SetGetter()` for test injection.
- Reuses `identity.LoadConfigWithAuth()` for AWS config loading.

### Registration

- Modern layer: `pkg/function/aws.go` - `AwsOrganizationIDFunction` struct.
- Legacy layer: `internal/exec/yaml_func_aws.go` - `processTagAwsOrganizationID()`.
- Tag constants in both `pkg/function/tags.go` and `pkg/utils/yaml_utils.go`.

## Authentication Integration

### Atmos Auth Support

The `!aws.organization_id` function fully integrates with [Atmos Authentication](/cli/commands/auth/usage),
following the same pattern as all other AWS YAML functions (`!aws.account_id`, `!aws.region`, etc.).

### Auth Context Flow

The authentication context flows through the system as follows:

1. **CLI invocation** - User runs a command with `--identity` flag or a default identity is configured
   in `settings.auth`.
2. **AuthManager creation** - `ExecuteTerraform()` creates an `AuthManager` from the identity.
3. **PostAuthenticate** - The identity's `PostAuthenticate()` method:
   - Writes credentials to `~/.atmos/auth/{realm}/aws/{provider}/credentials`
   - Populates `AWSAuthContext` with file paths, profile name, and region
   - Prepares environment variables for spawned processes
4. **Component processing** - `ProcessComponentConfig()` copies `AuthContext` from `AuthManager`
   into `ConfigAndStacksInfo`.
5. **YAML function execution** - Both modern layer (`getAWSOrganization()`) and legacy layer
   (`processTagAwsOrganizationID()`) extract `AWSAuthContext` from `stackInfo.AuthContext.AWS`.
6. **AWS config loading** - `LoadConfigWithAuth()` configures the AWS SDK to use Atmos-managed
   credential files instead of default `~/.aws/` paths.
7. **API call** - `organizations.DescribeOrganization` is called with the authenticated config.

### Credential Resolution

When `AWSAuthContext` is present (Atmos Auth active):
- `AWS_SHARED_CREDENTIALS_FILE` → Atmos-managed credentials file
- `AWS_CONFIG_FILE` → Atmos-managed config file
- `AWS_PROFILE` → Identity name (e.g., `core-auto/terraform`)
- IMDS is disabled to prevent accidental instance credential usage

When `AWSAuthContext` is nil (no Atmos Auth):
- Falls back to standard AWS SDK credential resolution chain

### Per-Auth-Context Caching

The cache key is `Profile:CredentialsFile:ConfigFile`. This ensures:
- Different identities (different profiles/credentials) get independent cache entries
- Same identity across multiple components shares the cached result
- No cross-contamination between auth contexts

### Stack-Level Auth Configuration

Auth can be specified at three levels (component-level takes highest precedence):
1. Global (`atmos.yaml` → `auth.identities`)
2. Stack-level (stack manifest → `settings.auth.identities`)
3. Component-level (component → `settings.auth.identities`)

### Nested Function Authentication

When `!aws.organization_id` is used inside a component that is referenced by another component
(e.g., via `!terraform.output`), the `AuthManager` is propagated to the nested component.
The nested component can also override with its own auth settings.

## Testing

- Unit tests for `pkg/aws/organization/` - cache behavior, concurrent access, error caching.
- Unit tests for `pkg/function/aws_test.go` - modern layer function execution with auth context.
- Unit tests for `internal/exec/yaml_func_aws_test.go` - legacy layer processing with auth context.
- All tests use mock getters (no real AWS calls).
- Tests verify auth context propagation from `stackInfo` to the organization getter.

## Considerations

### Permissions

The `organizations:DescribeOrganization` permission is not universally available like
`sts:GetCallerIdentity`. Users may need to explicitly grant this permission in their
IAM policies or permission sets.

### Not-in-Organization Error

When the account is not a member of an AWS Organization, the API returns
`AWSOrganizationsNotInUseException`. This is handled with a clear error message:
`"failed to describe AWS organization: the AWS account is not a member of an organization"`.

### Caching

Results are cached in a separate cache from the identity functions since they use different
AWS APIs. The cache uses the same per-auth-context key format (`Profile:CredentialsFile:ConfigFile`).

## References

- GitHub Issue: [#2073](https://github.com/cloudposse/atmos/issues/2073)
- Terragrunt equivalent: `get_aws_org_id()`
- AWS API: [DescribeOrganization](https://docs.aws.amazon.com/organizations/latest/APIReference/API_DescribeOrganization.html)
