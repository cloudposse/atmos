package exec

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	awscfn "github.com/cloudposse/atmos/pkg/aws/cloudformation"
	awsIdentity "github.com/cloudposse/atmos/pkg/aws/identity"
	awsOrg "github.com/cloudposse/atmos/pkg/aws/organization"
	cfg "github.com/cloudposse/atmos/pkg/config"
	fnparser "github.com/cloudposse/atmos/pkg/function/parser"
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
	extractor func(*awsIdentity.CallerIdentity) string,
) (any, error) {
	log.Debug(execAWSYAMLFunction, functionKey, input)

	// Validate the tag matches expected.
	if input != expectedTag {
		log.Error(invalidYAMLFunction, functionKey, input, "expected", expectedTag)
		return nil, errUtils.ErrYamlFuncInvalidArguments
	}

	// Get auth context from stack info if available.
	var authContext *schema.AWSAuthContext
	if stackInfo != nil && stackInfo.AuthContext != nil && stackInfo.AuthContext.AWS != nil {
		authContext = stackInfo.AuthContext.AWS
	}

	// Get the AWS caller identity (cached).
	ctx := context.Background()
	identity, err := awsIdentity.GetCallerIdentityCached(ctx, atmosConfig, authContext)
	if err != nil {
		log.Error(failedGetIdentity, "error", err)
		return nil, err
	}

	// Extract the requested value.
	return extractor(identity), nil
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
) (any, error) {
	defer perf.Track(atmosConfig, "exec.processTagAwsAccountID")()

	result, err := processTagAwsValue(atmosConfig, input, u.AtmosYamlFuncAwsAccountID, stackInfo, func(id *awsIdentity.CallerIdentity) string {
		return id.Account
	})

	if result != nil {
		log.Debug("Resolved !aws.account_id", "account_id", result)
	}
	return result, err
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
) (any, error) {
	defer perf.Track(atmosConfig, "exec.processTagAwsCallerIdentityArn")()

	result, err := processTagAwsValue(atmosConfig, input, u.AtmosYamlFuncAwsCallerIdentityArn, stackInfo, func(id *awsIdentity.CallerIdentity) string {
		return id.Arn
	})

	if result != nil {
		log.Debug("Resolved !aws.caller_identity_arn", "arn", result)
	}
	return result, err
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
) (any, error) {
	defer perf.Track(atmosConfig, "exec.processTagAwsCallerIdentityUserID")()

	result, err := processTagAwsValue(atmosConfig, input, u.AtmosYamlFuncAwsCallerIdentityUserID, stackInfo, func(id *awsIdentity.CallerIdentity) string {
		return id.UserID
	})

	if result != nil {
		log.Debug("Resolved !aws.caller_identity_user_id", "user_id", result)
	}
	return result, err
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
) (any, error) {
	defer perf.Track(atmosConfig, "exec.processTagAwsRegion")()

	result, err := processTagAwsValue(atmosConfig, input, u.AtmosYamlFuncAwsRegion, stackInfo, func(id *awsIdentity.CallerIdentity) string {
		return id.Region
	})

	if result != nil {
		log.Debug("Resolved !aws.region", "region", result)
	}
	return result, err
}

// processTagAwsOrganizationID processes the !aws.organization_id YAML function.
// It returns the AWS Organization ID of the current account.
// The function takes no parameters.
//
// Usage in YAML:
//
//	org_id: !aws.organization_id
func processTagAwsOrganizationID(
	atmosConfig *schema.AtmosConfiguration,
	input string,
	stackInfo *schema.ConfigAndStacksInfo,
) (any, error) {
	defer perf.Track(atmosConfig, "exec.processTagAwsOrganizationID")()

	log.Debug(execAWSYAMLFunction, functionKey, input)

	// Validate the tag matches expected.
	if input != u.AtmosYamlFuncAwsOrganizationID {
		log.Error(invalidYAMLFunction, functionKey, input, "expected", u.AtmosYamlFuncAwsOrganizationID)
		return nil, errUtils.ErrYamlFuncInvalidArguments
	}

	// Get auth context from stack info if available.
	var authContext *schema.AWSAuthContext
	if stackInfo != nil && stackInfo.AuthContext != nil && stackInfo.AuthContext.AWS != nil {
		authContext = stackInfo.AuthContext.AWS
	}

	// Get the AWS organization info (cached).
	ctx := context.Background()
	orgInfo, err := awsOrg.GetOrganizationCached(ctx, atmosConfig, authContext)
	if err != nil {
		log.Error("Failed to get AWS organization info", "error", err)
		return nil, err
	}

	if orgInfo == nil || orgInfo.ID == "" {
		log.Error("Failed to get AWS organization info", "error", errUtils.ErrAwsDescribeOrganization)
		return nil, errUtils.ErrAwsDescribeOrganization
	}

	log.Debug("Resolved !aws.organization_id", "organization_id", orgInfo.ID)
	return orgInfo.ID, nil
}

// parseAwsCloudFormationOutputArgs parses `!aws.cloudformation.output`'s
// `component [stack] output-key` arguments, defaulting Stack to currentStack
// when omitted. The result's Expression field holds the output key.
func parseAwsCloudFormationOutputArgs(input, currentStack string) (fnparser.TerraformArgs, error) {
	str, err := getStringAfterTag(input, u.AtmosYamlFuncAwsCloudFormationOutput)
	if err != nil {
		return fnparser.TerraformArgs{}, err
	}

	parsed, err := fnparser.ParseTerraform(str)
	if err != nil {
		return fnparser.TerraformArgs{}, err
	}
	if parsed.Stack == "" {
		parsed.Stack = currentStack
	}
	return parsed, nil
}

// processTagAwsCloudFormationOutputWithContext processes the
// `!aws.cloudformation.output` YAML tag: the Terraform<->CloudFormation
// interop bridge, sibling to !terraform.output. Syntax is `component [stack]
// output-key`, reusing the same "component [stack] expression" grammar
// (fnparser.ParseTerraform is not Terraform-specific despite the name — it's
// this shared 2-3-token shape).
func processTagAwsCloudFormationOutputWithContext(
	atmosConfig *schema.AtmosConfiguration,
	input string,
	currentStack string,
	resolutionCtx *ResolutionContext,
	stackInfo *schema.ConfigAndStacksInfo,
) (any, error) {
	defer perf.Track(atmosConfig, "exec.processTagAwsCloudFormationOutputWithContext")()

	log.Debug(execAWSYAMLFunction, functionKey, input)

	parsed, err := parseAwsCloudFormationOutputArgs(input, currentStack)
	if err != nil {
		return nil, err
	}
	component, stack, output := parsed.Component, parsed.Stack, parsed.Expression

	cleanup, err := trackOutputDependency(atmosConfig, resolutionCtx, component, stack, "aws.cloudformation.output", input)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	authDisabled := authdeferred.AuthDisabled(atmosConfig.AuthManager) || (stackInfo != nil && stackInfo.AuthDisabled)
	resolvedAuthManager, err := resolveCloudFormationOutputAuth(atmosConfig, component, stack, stackInfo)
	if err != nil {
		return nil, err
	}
	var callerContext *schema.AuthContext
	if stackInfo != nil {
		callerContext = stackInfo.AuthContext
	}
	resolvedAuthContext := resolvedTargetAuthContext(atmosConfig, resolvedAuthManager, callerContext, authDisabled)

	sections, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		AtmosConfig:          atmosConfig,
		ResolveSecrets:       stackInfo == nil || !stackInfo.SecretsMaskOnly,
		AuthManager:          resolvedAuthManager,
		AuthDisabled:         authDisabled,
		Component:            component,
		Stack:                stack,
		ComponentType:        cfg.CloudFormationComponentType,
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe aws/cloudformation component %s in stack %s: %w", component, stack, err)
	}

	resolvedAuthContext, err = cloudFormationOutputAuthForSections(atmosConfig, sections, cloudFormationOutputScope(stack, stackInfo, authDisabled), resolvedAuthContext)
	if err != nil {
		return nil, err
	}
	outputs, err := cloudFormationOutputsForSections(atmosConfig, component, sections, resolvedAuthContext)
	if err != nil {
		return nil, fmt.Errorf("failed to get aws/cloudformation output for component %s in stack %s, output %s: %w", component, stack, output, err)
	}

	return lookupCloudFormationOutput(sections, outputs, component, stack, output)
}

// lookupCloudFormationOutput returns one Output's value. A missing key must
// fail loudly: resolving it to null would flow into the consuming component as
// an empty value and deploy it silently.
func lookupCloudFormationOutput(sections, outputs map[string]any, component, stack, output string) (any, error) {
	cfnStackName, _ := sections[cfg.StackNameSectionName].(string)
	value, err := awscfn.LookupOutput(outputs, cfnStackName, output)
	if err != nil {
		return nil, fmt.Errorf("failed to get aws/cloudformation output for component %s in stack %s, output %s: %w", component, stack, output, err)
	}
	return value, nil
}

// resolveCloudFormationOutputAuth resolves the target identity before any output
// read. Resolver errors are fatal; they must not select the caller's credentials.
func resolveCloudFormationOutputAuth(ac *schema.AtmosConfiguration, component, stack string, info *schema.ConfigAndStacksInfo) (auth.AuthManager, error) {
	if authdeferred.AuthDisabled(ac.AuthManager) || (info != nil && info.AuthDisabled) {
		return nil, nil
	}
	var parent auth.AuthManager
	if info != nil {
		parent, _ = info.AuthManager.(auth.AuthManager)
		// Deferred resolution needs the caller's auth configuration, not just its
		// already-resolved credentials, to bind inherited defaults to the target.
		if parent == nil || authdeferred.IsDeferred(ac.AuthManager) {
			parent = &authContextWrapper{stackInfo: info}
		}
	}
	return resolveAuthManagerForNestedComponent(ac, component, stack, parent)
}
