package exec

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	awscfn "github.com/cloudposse/atmos/pkg/aws/cloudformation"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// CloudFormationOutputsGetter defines the interface for getting a deployed
// aws/cloudformation stack's Outputs. This interface allows dependency
// injection and testing, mirroring TerraformOutputGetter.
type CloudFormationOutputsGetter interface {
	GetOutputs(ctx context.Context, region, stackName string, authContext *schema.AWSAuthContext) (map[string]any, error)
}

// defaultCloudFormationOutputsGetter delegates to pkg/aws/cloudformation, a
// small leaf package that's allowed to import the AWS SDK directly (unlike
// internal/exec, which the provider-agnostic-auth depguard rule keeps
// cloud-SDK-free) and that pkg/component/aws/cloudformation cannot be reused
// for here: that package already imports internal/exec, so the reverse import
// would cycle.
type defaultCloudFormationOutputsGetter struct{}

func (defaultCloudFormationOutputsGetter) GetOutputs(
	ctx context.Context,
	region, stackName string,
	authContext *schema.AWSAuthContext,
) (map[string]any, error) {
	defer perf.Track(nil, "exec.defaultCloudFormationOutputsGetter.GetOutputs")()

	return awscfn.GetOutputs(ctx, region, stackName, authContext)
}

// cloudFormationOutputsGetter is replaceable in tests.
var cloudFormationOutputsGetter CloudFormationOutputsGetter = defaultCloudFormationOutputsGetter{}

// GetCloudFormationOutputs fetches a deployed aws/cloudformation stack's
// Outputs, keyed by Output name. Used by both the `!aws.cloudformation.output`
// YAML function and atmos.Component(...).outputs for a CFN target.
func GetCloudFormationOutputs(ctx context.Context, region, stackName string, authContext *schema.AWSAuthContext) (map[string]any, error) {
	defer perf.Track(nil, "exec.GetCloudFormationOutputs")()

	return cloudFormationOutputsGetter.GetOutputs(ctx, region, stackName, authContext)
}

// resolveCloudFormationRegion returns the explicit region override, if any,
// from a target component's `settings.aws_cloudformation.region` — the same
// precedence source pkg/component/aws/cloudformation's own resolveRegion reads
// (duplicated here rather than imported: that package already imports
// internal/exec, so the reverse import would cycle).
func resolveCloudFormationRegion(sections map[string]any) string {
	settings, ok := sections[cfg.SettingsSectionName].(map[string]any)
	if !ok {
		return ""
	}
	cfnSettings, ok := settings["aws_cloudformation"].(map[string]any)
	if !ok {
		return ""
	}
	region, _ := cfnSettings["region"].(string)
	return region
}

// cloudFormationOutputsForSections resolves a target aws/cloudformation
// component's stack_name and region from its already-described sections, and
// fetches its deployed Outputs using the resolved AuthContext (the target's
// own auth when it authenticates independently, otherwise the enclosing
// component's — callers resolve this via resolveNestedOutputAuth /
// resolveComponentFuncAuthManager before calling in).
func cloudFormationOutputsForSections(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	sections map[string]any,
	authContext *schema.AuthContext,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "exec.cloudFormationOutputsForSections")()

	stackName, _ := sections[cfg.StackNameSectionName].(string)
	if stackName == "" {
		return nil, fmt.Errorf("%w: component %q has no stack_name", errUtils.ErrMissingAwsCloudFormationStackName, component)
	}

	var awsAuthContext *schema.AWSAuthContext
	if authContext != nil {
		awsAuthContext = authContext.AWS
	}

	return GetCloudFormationOutputs(context.Background(), resolveCloudFormationRegion(sections), stackName, awsAuthContext)
}
