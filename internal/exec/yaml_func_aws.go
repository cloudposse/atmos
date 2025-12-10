package exec

import (
	"context"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	execAWSYAMLFunction = "Executing Atmos YAML function"
	invalidYAMLFunction = "Invalid YAML function"
	failedGetIdentity   = "Failed to get AWS caller identity"
	functionKey         = "function"
)

// processTagAwsValue is a shared helper for AWS YAML functions.
// It validates the input tag, retrieves AWS caller identity, and returns the requested value.
func processTagAwsValue(
	atmosConfig *schema.AtmosConfiguration,
	input string,
	expectedTag string,
	stackInfo *schema.ConfigAndStacksInfo,
	extractor func(*AWSCallerIdentity) string,
) any {
	log.Debug(execAWSYAMLFunction, functionKey, input)

	// Validate the tag matches expected.
	if input != expectedTag {
		log.Error(invalidYAMLFunction, functionKey, input, "expected", expectedTag)
		errUtils.CheckErrorPrintAndExit(errUtils.ErrYamlFuncInvalidArguments, "", "")
		return nil
	}

	// Get auth context from stack info if available.
	var authContext *schema.AWSAuthContext
	if stackInfo != nil && stackInfo.AuthContext != nil && stackInfo.AuthContext.AWS != nil {
		authContext = stackInfo.AuthContext.AWS
	}

	// Get the AWS caller identity (cached).
	ctx := context.Background()
	identity, err := getAWSCallerIdentityCached(ctx, atmosConfig, authContext)
	if err != nil {
		log.Error(failedGetIdentity, "error", err)
		errUtils.CheckErrorPrintAndExit(err, "", "")
		return nil
	}

	// Extract the requested value.
	return extractor(identity)
}

// processTagAwsAccountID processes the !aws.account_id YAML function.
// It returns the AWS account ID of the current caller identity.
// The function takes no parameters.
//
// Usage in YAML:
//
//	account_id: !aws.account_id
func processTagAwsAccountID(
	atmosConfig *schema.AtmosConfiguration,
	input string,
	stackInfo *schema.ConfigAndStacksInfo,
) any {
	defer perf.Track(atmosConfig, "exec.processTagAwsAccountID")()

	result := processTagAwsValue(atmosConfig, input, u.AtmosYamlFuncAwsAccountID, stackInfo, func(id *AWSCallerIdentity) string {
		return id.Account
	})

	if result != nil {
		log.Debug("Resolved !aws.account_id", "account_id", result)
	}
	return result
}

// processTagAwsCallerIdentityArn processes the !aws.caller_identity_arn YAML function.
// It returns the ARN of the current AWS caller identity.
// The function takes no parameters.
//
// Usage in YAML:
//
//	caller_arn: !aws.caller_identity_arn
func processTagAwsCallerIdentityArn(
	atmosConfig *schema.AtmosConfiguration,
	input string,
	stackInfo *schema.ConfigAndStacksInfo,
) any {
	defer perf.Track(atmosConfig, "exec.processTagAwsCallerIdentityArn")()

	result := processTagAwsValue(atmosConfig, input, u.AtmosYamlFuncAwsCallerIdentityArn, stackInfo, func(id *AWSCallerIdentity) string {
		return id.Arn
	})

	if result != nil {
		log.Debug("Resolved !aws.caller_identity_arn", "arn", result)
	}
	return result
}

// processTagAwsCallerIdentityUserID processes the !aws.caller_identity_user_id YAML function.
// It returns the unique user ID of the current AWS caller identity.
// The function takes no parameters.
//
// Usage in YAML:
//
//	user_id: !aws.caller_identity_user_id
func processTagAwsCallerIdentityUserID(
	atmosConfig *schema.AtmosConfiguration,
	input string,
	stackInfo *schema.ConfigAndStacksInfo,
) any {
	defer perf.Track(atmosConfig, "exec.processTagAwsCallerIdentityUserID")()

	result := processTagAwsValue(atmosConfig, input, u.AtmosYamlFuncAwsCallerIdentityUserID, stackInfo, func(id *AWSCallerIdentity) string {
		return id.UserID
	})

	if result != nil {
		log.Debug("Resolved !aws.caller_identity_user_id", "user_id", result)
	}
	return result
}

// processTagAwsRegion processes the !aws.region YAML function.
// It returns the AWS region from the current configuration.
// The function takes no parameters.
//
// Usage in YAML:
//
//	region: !aws.region
func processTagAwsRegion(
	atmosConfig *schema.AtmosConfiguration,
	input string,
	stackInfo *schema.ConfigAndStacksInfo,
) any {
	defer perf.Track(atmosConfig, "exec.processTagAwsRegion")()

	result := processTagAwsValue(atmosConfig, input, u.AtmosYamlFuncAwsRegion, stackInfo, func(id *AWSCallerIdentity) string {
		return id.Region
	})

	if result != nil {
		log.Debug("Resolved !aws.region", "region", result)
	}
	return result
}
