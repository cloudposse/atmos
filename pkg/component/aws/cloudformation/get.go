package cloudformation

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/perf"
)

// getDeployedTemplate fetches a deployed stack's template body. Original selects
// the user-submitted template (TemplateStageOriginal); otherwise CloudFormation
// returns the fully-processed template (TemplateStageProcessed, its default) —
// the shape a diff against the local render should compare against.
func getDeployedTemplate(ctx context.Context, client CloudFormationClient, stackName string, original bool) (string, error) {
	defer perf.Track(nil, "cloudformation.getDeployedTemplate")()

	input := &cloudformation.GetTemplateInput{StackName: awsString(stackName)}
	if original {
		input.TemplateStage = cfntypes.TemplateStageOriginal
	}

	out, err := client.GetTemplate(ctx, input)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrAwsCloudFormationChangeSetFailed, err)
	}
	return stringValue(out.TemplateBody), nil
}

// getDeployedStackPolicy fetches a deployed stack's current stack policy. Returns
// "" (no error) when the stack has no policy set — CloudFormation's default.
func getDeployedStackPolicy(ctx context.Context, client CloudFormationClient, stackName string) (string, error) {
	defer perf.Track(nil, "cloudformation.getDeployedStackPolicy")()

	out, err := client.GetStackPolicy(ctx, &cloudformation.GetStackPolicyInput{StackName: awsString(stackName)})
	if err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrAwsCloudFormationChangeSetFailed, err)
	}
	return stringValue(out.StackPolicyBody), nil
}

// runGetTemplate renders the deployed stack's template to the data channel.
func runGetTemplate(ctx context.Context, client CloudFormationClient, stackName string, flags map[string]any, summary map[string]any) (map[string]any, error) {
	original, _ := flags["original"].(bool)
	body, err := getDeployedTemplate(ctx, client, stackName, original)
	if err != nil {
		return summary, err
	}
	summary["template"] = body
	_ = data.Write(body)
	return summary, nil
}

// runGetPolicy renders the deployed stack's policy to the data channel.
func runGetPolicy(ctx context.Context, client CloudFormationClient, stackName string, summary map[string]any) (map[string]any, error) {
	body, err := getDeployedStackPolicy(ctx, client, stackName)
	if err != nil {
		return summary, err
	}
	summary["stack_policy"] = body
	if body == "" {
		_ = data.Writeln(fmt.Sprintf("%s: no stack policy set", stackName))
		return summary, nil
	}
	_ = data.Write(body)
	return summary, nil
}
