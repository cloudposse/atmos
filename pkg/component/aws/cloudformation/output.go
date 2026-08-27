package cloudformation

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// describeStackOutputs fetches the deployed stack's Outputs via DescribeStacks,
// returning them as a plain map — the shape both the standalone `output` verb
// and the end-of-deploy summary render (via the shared pkg/output formatter).
func describeStackOutputs(ctx context.Context, client CloudFormationClient, stackName string) (map[string]any, error) {
	defer perf.Track(nil, "cloudformation.describeStackOutputs")()

	out, err := client.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{StackName: awsString(stackName)})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrAwsCloudFormationAPICallFailed, err)
	}
	if len(out.Stacks) == 0 {
		return map[string]any{}, nil
	}

	outputs := make(map[string]any, len(out.Stacks[0].Outputs))
	for _, o := range out.Stacks[0].Outputs {
		if o.OutputKey == nil {
			continue
		}
		outputs[*o.OutputKey] = stringValue(o.OutputValue)
	}
	return outputs, nil
}
