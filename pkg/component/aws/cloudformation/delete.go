package cloudformation

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// deleteOptions carries the delete-specific flags (--retain-resources,
// --disable-termination-protection) through to deleteStack.
type deleteOptions struct {
	RetainResources              []string
	DisableTerminationProtection bool
}

// deleteStack deletes the stack, respecting termination protection: deleting a
// stack with termination_protection: true fails with an actionable hint unless
// --disable-termination-protection is passed (which calls
// UpdateTerminationProtection first) — silent auto-disable would defeat the
// point of the setting.
func deleteStack(ctx context.Context, client CloudFormationClient, spec *stackSpec, opts deleteOptions) error {
	defer perf.Track(nil, "cloudformation.deleteStack")()

	if spec.TerminationProtection {
		if !opts.DisableTerminationProtection {
			return errUtils.Build(errUtils.ErrAwsCloudFormationChangeSetFailed).
				WithExplanationf("Stack %q has termination_protection enabled.", spec.StackName).
				WithHint("Set termination_protection: false in the component config and re-apply, " +
					"or pass --disable-termination-protection to delete it anyway.").
				Err()
		}
		if err := disableTerminationProtection(ctx, client, spec.StackName); err != nil {
			return err
		}
	}

	if len(opts.RetainResources) > 0 {
		status, err := currentStackStatus(ctx, client, spec.StackName)
		if err != nil {
			return err
		}
		if !isDeleteFailedStack(status) {
			return errUtils.Build(errUtils.ErrAwsCloudFormationChangeSetFailed).
				WithExplanationf("--retain-resources is only valid for a stack in DELETE_FAILED status; %s is currently %s.", spec.StackName, status).
				WithHint("Retry the delete without --retain-resources, or wait for the stack to reach DELETE_FAILED.").
				Err()
		}
	}

	input := &cloudformation.DeleteStackInput{
		StackName: awsString(spec.StackName),
	}
	if len(opts.RetainResources) > 0 {
		input.RetainResources = opts.RetainResources
	}
	if spec.RoleArn != "" {
		input.RoleARN = awsString(spec.RoleArn)
	}

	if _, err := client.DeleteStack(ctx, input); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrAwsCloudFormationAPICallFailed, err)
	}
	return nil
}

// disableTerminationProtection calls UpdateTerminationProtection to clear the
// flag before a delete, only ever in direct response to the user's explicit
// --disable-termination-protection flag (see deleteStack).
func disableTerminationProtection(ctx context.Context, client CloudFormationClient, stackName string) error {
	defer perf.Track(nil, "cloudformation.disableTerminationProtection")()

	_, err := client.UpdateTerminationProtection(ctx, &cloudformation.UpdateTerminationProtectionInput{
		StackName:                   awsString(stackName),
		EnableTerminationProtection: awsBool(false),
	})
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrAwsCloudFormationAPICallFailed, err)
	}
	return nil
}

// isDeleteFailedStack reports whether a stack is in DELETE_FAILED status, the
// only status --retain-resources is valid against (AWS semantics).
func isDeleteFailedStack(status cfntypes.StackStatus) bool {
	return status == cfntypes.StackStatusDeleteFailed
}

// currentStackStatus fetches the stack's current status.
func currentStackStatus(ctx context.Context, client CloudFormationClient, stackName string) (cfntypes.StackStatus, error) {
	out, err := client.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{StackName: awsString(stackName)})
	if err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrAwsCloudFormationAPICallFailed, err)
	}
	if len(out.Stacks) == 0 {
		return "", fmt.Errorf("%w: stack %s not found", errUtils.ErrAwsCloudFormationChangeSetFailed, stackName)
	}
	return out.Stacks[0].StackStatus, nil
}
