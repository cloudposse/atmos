package function

import (
	"context"

	"github.com/cloudposse/atmos/pkg/perf"
)

// AWS function tags are defined in tags.go.
// Use YAMLTag(TagAwsAccountID) etc. to get the YAML tag format.

// AwsAccountIDFunction implements the !aws.account_id YAML function.
// It returns the AWS account ID via STS GetCallerIdentity.
// Note: During HCL parsing (PreMerge), this returns a placeholder.
// The actual resolution happens during PostMerge processing.
type AwsAccountIDFunction struct {
	BaseFunction
}

// NewAwsAccountIDFunction creates a new AwsAccountIDFunction.
func NewAwsAccountIDFunction() *AwsAccountIDFunction {
	defer perf.Track(nil, "function.NewAwsAccountIDFunction")()

	return &AwsAccountIDFunction{
		BaseFunction: BaseFunction{
			FunctionName:    "aws.account_id",
			FunctionAliases: nil,
			FunctionPhase:   PostMerge,
		},
	}
}

// Execute returns a placeholder for post-merge resolution.
// The actual AWS API call is made during YAML post-merge processing.
func (f *AwsAccountIDFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.AwsAccountIDFunction.Execute")()

	// Return placeholder for post-merge resolution.
	return TagAwsAccountID, nil
}

// AwsCallerIdentityArnFunction implements the !aws.caller_identity_arn YAML function.
// It returns the AWS caller identity ARN via STS GetCallerIdentity.
type AwsCallerIdentityArnFunction struct {
	BaseFunction
}

// NewAwsCallerIdentityArnFunction creates a new AwsCallerIdentityArnFunction.
func NewAwsCallerIdentityArnFunction() *AwsCallerIdentityArnFunction {
	defer perf.Track(nil, "function.NewAwsCallerIdentityArnFunction")()

	return &AwsCallerIdentityArnFunction{
		BaseFunction: BaseFunction{
			FunctionName:    "aws.caller_identity_arn",
			FunctionAliases: nil,
			FunctionPhase:   PostMerge,
		},
	}
}

// Execute returns a placeholder for post-merge resolution.
func (f *AwsCallerIdentityArnFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.AwsCallerIdentityArnFunction.Execute")()

	return TagAwsCallerIdentityArn, nil
}

// AwsCallerIdentityUserIDFunction implements the !aws.caller_identity_user_id YAML function.
// It returns the AWS caller identity user ID via STS GetCallerIdentity.
type AwsCallerIdentityUserIDFunction struct {
	BaseFunction
}

// NewAwsCallerIdentityUserIDFunction creates a new AwsCallerIdentityUserIDFunction.
func NewAwsCallerIdentityUserIDFunction() *AwsCallerIdentityUserIDFunction {
	defer perf.Track(nil, "function.NewAwsCallerIdentityUserIDFunction")()

	return &AwsCallerIdentityUserIDFunction{
		BaseFunction: BaseFunction{
			FunctionName:    "aws.caller_identity_user_id",
			FunctionAliases: nil,
			FunctionPhase:   PostMerge,
		},
	}
}

// Execute returns a placeholder for post-merge resolution.
func (f *AwsCallerIdentityUserIDFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.AwsCallerIdentityUserIDFunction.Execute")()

	return TagAwsCallerIdentityUserID, nil
}

// AwsRegionFunction implements the !aws.region YAML function.
// It returns the AWS region from the SDK configuration.
type AwsRegionFunction struct {
	BaseFunction
}

// NewAwsRegionFunction creates a new AwsRegionFunction.
func NewAwsRegionFunction() *AwsRegionFunction {
	defer perf.Track(nil, "function.NewAwsRegionFunction")()

	return &AwsRegionFunction{
		BaseFunction: BaseFunction{
			FunctionName:    "aws.region",
			FunctionAliases: nil,
			FunctionPhase:   PostMerge,
		},
	}
}

// Execute returns a placeholder for post-merge resolution.
func (f *AwsRegionFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.AwsRegionFunction.Execute")()

	return TagAwsRegion, nil
}
