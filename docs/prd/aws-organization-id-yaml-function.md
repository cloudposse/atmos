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

## Testing

- Unit tests for `pkg/aws/organization/` - cache behavior, concurrent access, error caching.
- Unit tests for `pkg/function/aws_test.go` - modern layer function execution.
- Unit tests for `internal/exec/yaml_func_aws_test.go` - legacy layer processing.
- All tests use mock getters (no real AWS calls).

## Considerations

### Permissions

The `organizations:DescribeOrganization` permission is not universally available like
`sts:GetCallerIdentity`. Users may need to explicitly grant this permission.

### Not-in-Organization Error

When the account is not a member of an AWS Organization, the API returns
`AWSOrganizationsNotInUseException`. This is handled with a clear error message.

### Caching

Results are cached in a separate cache from the identity functions since they use different
AWS APIs. The cache uses the same per-auth-context key format.

## References

- GitHub Issue: [#2073](https://github.com/cloudposse/atmos/issues/2073)
- Terragrunt equivalent: `get_aws_org_id()`
- AWS API: [DescribeOrganization](https://docs.aws.amazon.com/organizations/latest/APIReference/API_DescribeOrganization.html)
