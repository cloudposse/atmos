package cloudformation

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"

	"github.com/cloudposse/atmos/pkg/perf"
)

// describeStackOutputs fetches the deployed stack's Outputs via DescribeStacks,
// returning them as a plain map — the shape both the standalone `output` verb
// and the end-of-deploy summary render (via the shared pkg/output formatter).
func describeStackOutputs(ctx context.Context, client CloudFormationClient, stackName string) (map[string]any, error) {
	outputs, _, err := describeStackOutputValues(ctx, client, stackName)
	return outputs, err
}

func describeStackOutputValues(ctx context.Context, client CloudFormationClient, stackName string) (map[string]any, []cfntypes.Parameter, error) {
	defer perf.Track(nil, "cloudformation.describeStackOutputs")()

	out, err := client.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{StackName: awsString(stackName)})
	if err != nil {
		return nil, nil, wrapAPICallError(stackName, err)
	}
	if len(out.Stacks) == 0 {
		return map[string]any{}, nil, nil
	}

	outputs := make(map[string]any, len(out.Stacks[0].Outputs))
	for _, o := range out.Stacks[0].Outputs {
		if o.OutputKey == nil {
			continue
		}
		outputs[*o.OutputKey] = stringValue(o.OutputValue)
	}
	return outputs, out.Stacks[0].Parameters, nil
}
