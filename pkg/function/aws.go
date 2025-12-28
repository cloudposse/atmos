package function

import (
	"context"

	awsIdentity "github.com/cloudposse/atmos/pkg/aws/identity"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// errMsgAWSIdentityFailed is a constant for the AWS identity error message.
const errMsgAWSIdentityFailed = "Failed to get AWS caller identity"

// getAWSIdentity is a helper that retrieves the AWS caller identity from the execution context.
func getAWSIdentity(ctx context.Context, execCtx *ExecutionContext) (*awsIdentity.CallerIdentity, error) {
	defer perf.Track(nil, "function.getAWSIdentity")()

	// Get auth context from stack info if available.
	var authContext *schema.AWSAuthContext
	if execCtx != nil && execCtx.StackInfo != nil &&
		execCtx.StackInfo.AuthContext != nil && execCtx.StackInfo.AuthContext.AWS != nil {
		authContext = execCtx.StackInfo.AuthContext.AWS
	}

	// Get AtmosConfig from execution context.
	var atmosConfig *schema.AtmosConfiguration
	if execCtx != nil {
		atmosConfig = execCtx.AtmosConfig
	}

	// Get the AWS caller identity (cached).
	return awsIdentity.GetCallerIdentityCached(ctx, atmosConfig, authContext)
}

// AwsAccountIDFunction implements the aws.account_id function.
type AwsAccountIDFunction struct {
	BaseFunction
}

// NewAwsAccountIDFunction creates a new aws.account_id function handler.
func NewAwsAccountIDFunction() *AwsAccountIDFunction {
	defer perf.Track(nil, "function.NewAwsAccountIDFunction")()

	return &AwsAccountIDFunction{
		BaseFunction: BaseFunction{
			FunctionName:    TagAwsAccountID,
			FunctionAliases: nil,
			FunctionPhase:   PostMerge,
		},
	}
}

// Execute processes the aws.account_id function.
// Usage:
//
//	!aws.account_id   - Returns the AWS account ID of the current caller identity
func (f *AwsAccountIDFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.AwsAccountIDFunction.Execute")()

	log.Debug("Executing aws.account_id function")

	identity, err := getAWSIdentity(ctx, execCtx)
	if err != nil {
		log.Error(errMsgAWSIdentityFailed, "error", err)
		return nil, err
	}

	log.Debug("Resolved !aws.account_id", "account_id", identity.Account)
	return identity.Account, nil
}

// AwsCallerIdentityArnFunction implements the aws.caller_identity_arn function.
type AwsCallerIdentityArnFunction struct {
	BaseFunction
}

// NewAwsCallerIdentityArnFunction creates a new aws.caller_identity_arn function handler.
func NewAwsCallerIdentityArnFunction() *AwsCallerIdentityArnFunction {
	defer perf.Track(nil, "function.NewAwsCallerIdentityArnFunction")()

	return &AwsCallerIdentityArnFunction{
		BaseFunction: BaseFunction{
			FunctionName:    TagAwsCallerIdentityArn,
			FunctionAliases: nil,
			FunctionPhase:   PostMerge,
		},
	}
}

// Execute processes the aws.caller_identity_arn function.
// Usage:
//
//	!aws.caller_identity_arn   - Returns the ARN of the current caller identity
func (f *AwsCallerIdentityArnFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.AwsCallerIdentityArnFunction.Execute")()

	log.Debug("Executing aws.caller_identity_arn function")

	identity, err := getAWSIdentity(ctx, execCtx)
	if err != nil {
		log.Error(errMsgAWSIdentityFailed, "error", err)
		return nil, err
	}

	log.Debug("Resolved !aws.caller_identity_arn", "arn", identity.Arn)
	return identity.Arn, nil
}

// AwsCallerIdentityUserIDFunction implements the aws.caller_identity_user_id function.
type AwsCallerIdentityUserIDFunction struct {
	BaseFunction
}

// NewAwsCallerIdentityUserIDFunction creates a new aws.caller_identity_user_id function handler.
func NewAwsCallerIdentityUserIDFunction() *AwsCallerIdentityUserIDFunction {
	defer perf.Track(nil, "function.NewAwsCallerIdentityUserIDFunction")()

	return &AwsCallerIdentityUserIDFunction{
		BaseFunction: BaseFunction{
			FunctionName:    TagAwsCallerIdentityUserID,
			FunctionAliases: nil,
			FunctionPhase:   PostMerge,
		},
	}
}

// Execute processes the aws.caller_identity_user_id function.
// Usage:
//
//	!aws.caller_identity_user_id   - Returns the user ID of the current caller identity
func (f *AwsCallerIdentityUserIDFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.AwsCallerIdentityUserIDFunction.Execute")()

	log.Debug("Executing aws.caller_identity_user_id function")

	identity, err := getAWSIdentity(ctx, execCtx)
	if err != nil {
		log.Error(errMsgAWSIdentityFailed, "error", err)
		return nil, err
	}

	log.Debug("Resolved !aws.caller_identity_user_id", "user_id", identity.UserID)
	return identity.UserID, nil
}

// AwsRegionFunction implements the aws.region function.
type AwsRegionFunction struct {
	BaseFunction
}

// NewAwsRegionFunction creates a new aws.region function handler.
func NewAwsRegionFunction() *AwsRegionFunction {
	defer perf.Track(nil, "function.NewAwsRegionFunction")()

	return &AwsRegionFunction{
		BaseFunction: BaseFunction{
			FunctionName:    TagAwsRegion,
			FunctionAliases: nil,
			FunctionPhase:   PostMerge,
		},
	}
}

// Execute processes the aws.region function.
// Usage:
//
//	!aws.region   - Returns the AWS region from the current configuration
func (f *AwsRegionFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.AwsRegionFunction.Execute")()

	log.Debug("Executing aws.region function")

	identity, err := getAWSIdentity(ctx, execCtx)
	if err != nil {
		log.Error(errMsgAWSIdentityFailed, "error", err)
		return nil, err
	}

	log.Debug("Resolved !aws.region", "region", identity.Region)
	return identity.Region, nil
}
