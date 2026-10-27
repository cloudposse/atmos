package exec

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE

import (
	"context"
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	awscfn "github.com/cloudposse/atmos/pkg/aws/cloudformation"
	cfnmanifest "github.com/cloudposse/atmos/pkg/component/aws/cloudformation/manifest"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// cloudFormationOutputsTimeout bounds the DescribeStacks lookup issued by
// cloudFormationOutputsForSections while preserving any earlier caller deadline
// and cancellation, so a stalled endpoint cannot block a template indefinitely.
const cloudFormationOutputsTimeout = 5 * time.Minute

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

// resolveCloudFormationRegion shares the deployment resolver so output lookups use
// the same inherited region and legacy-alias behavior as CloudFormation commands.
func resolveCloudFormationRegion(sections map[string]any) string {
	return cfnmanifest.ResolveRegion(sections)
}

// cloudFormationOutputsForSections resolves a target aws/cloudformation
// component's stack_name and region from its already-described sections, and
// fetches its deployed Outputs using the resolved AuthContext (the target's
// own auth when it authenticates independently, otherwise the enclosing
// component's — callers resolve this via resolveCloudFormationOutputAuth /
// resolveComponentFuncAuthManager before calling in).
func cloudFormationOutputsForSections(
	ctx context.Context,
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

	ctx, cancel := context.WithTimeout(ctx, cloudFormationOutputsTimeout)
	defer cancel()

	return GetCloudFormationOutputs(ctx, resolveCloudFormationRegion(sections), stackName, awsAuthContext)
}
