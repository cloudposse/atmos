// Package cloudformation provides a minimal, direct AWS SDK v2 read of a
// deployed CloudFormation stack's Outputs, for consumers outside
// pkg/component/aws/cloudformation (which already owns the full CFN component
// implementation but cannot be imported back into internal/exec — it imports
// internal/exec itself). Kept intentionally narrow: a single DescribeStacks
// call, mirroring pkg/aws/identity's and pkg/aws/organization's shape as a
// small, provider-specific leaf package.
package cloudformation

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/aws/identity"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// GetOutputs fetches a deployed CloudFormation stack's Outputs, keyed by
// Output name. AuthContext's EndpointURL (when set) is honored so this also
// reaches a Floci-emulated CloudFormation endpoint, matching
// pkg/component/aws/cloudformation's own client construction.
func GetOutputs(ctx context.Context, region, stackName string, authContext *schema.AWSAuthContext) (map[string]any, error) {
	defer perf.Track(nil, "cloudformation.GetOutputs")()

	awsCfg, err := identity.LoadConfigWithAuth(ctx, region, "", 0, authContext)
	if err != nil {
		return nil, err
	}

	var optFns []func(*cloudformation.Options)
	if authContext != nil && authContext.EndpointURL != "" {
		optFns = append(optFns, func(o *cloudformation.Options) { o.BaseEndpoint = aws.String(authContext.EndpointURL) })
	}

	client := cloudformation.NewFromConfig(awsCfg, optFns...)
	out, err := client.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{StackName: aws.String(stackName)})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrAwsCloudFormationChangeSetFailed, err)
	}
	if len(out.Stacks) == 0 {
		return nil, fmt.Errorf("%w: stack %q", errUtils.ErrAwsCloudFormationChangeSetFailed, stackName)
	}

	outputs := make(map[string]any, len(out.Stacks[0].Outputs))
	for _, o := range out.Stacks[0].Outputs {
		if o.OutputKey == nil {
			continue
		}
		var value any
		if o.OutputValue != nil {
			value = *o.OutputValue
		}
		outputs[*o.OutputKey] = value
	}
	return outputs, nil
}
