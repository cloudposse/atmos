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
		return "", wrapGetError(stackName, err)
	}
	return stringValue(out.TemplateBody), nil
}

// getDeployedStackPolicy fetches a deployed stack's current stack policy. Returns
// "" (no error) when the stack has no policy set — CloudFormation's default.
func getDeployedStackPolicy(ctx context.Context, client CloudFormationClient, stackName string) (string, error) {
	defer perf.Track(nil, "cloudformation.getDeployedStackPolicy")()

	out, err := client.GetStackPolicy(ctx, &cloudformation.GetStackPolicyInput{StackName: awsString(stackName)})
	if err != nil {
		return "", wrapGetError(stackName, err)
	}
	return stringValue(out.StackPolicyBody), nil
}

// stackNotFoundError maps CloudFormation's "does not exist" error for a read
// verb to the stack-not-found sentinel with an actionable hint, instead of
// surfacing AWS's raw validation message. Unlike wrapAPICallError's hint, this
// one makes no mention of `--target`, which read verbs do not accept.
func stackNotFoundError(stackName string, err error) error {
	return errUtils.Build(errUtils.ErrAwsCloudFormationStackNotFound).
		WithCause(err).
		WithExplanationf("Stack %q doesn't exist.", stackName).
		WithHint("Check the component name and `--stack`, or deploy the stack first with `atmos aws cloudformation apply`.").
		Err()
}

// wrapGetError wraps a read-verb API error: a missing stack becomes the
// stack-not-found sentinel, anything else the generic API-call sentinel.
func wrapGetError(stackName string, err error) error {
	if isStackNotFoundError(err) {
		return stackNotFoundError(stackName, err)
	}
	return fmt.Errorf("%w: %w", errUtils.ErrAwsCloudFormationAPICallFailed, err)
}

// runGetTemplate renders the deployed stack's template to the data channel.
// CloudFormation may return the body as JSON even when the stack was
// originally deployed from YAML, so the processed template is re-serialized
// through formatTemplateAsBlockYAML for consistent, pretty-printed YAML output.
// With --original the user-submitted body is printed exactly as returned,
// byte for byte, so it can be diffed against the source file.
func runGetTemplate(ctx context.Context, client CloudFormationClient, stackName string, flags map[string]any, summary map[string]any) (map[string]any, error) {
	original, _ := flags["original"].(bool)
	body, err := getDeployedTemplate(ctx, client, stackName, original)
	if err != nil {
		return summary, err
	}
	if original {
		summary["template"] = body
		_ = data.Write(body)
		return summary, nil
	}
	formatted, err := formatTemplateAsBlockYAML(body)
	if err != nil {
		return summary, err
	}
	summary["template"] = formatted
	_ = data.Write(formatted)
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
