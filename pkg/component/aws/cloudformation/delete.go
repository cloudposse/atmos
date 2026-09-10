package cloudformation

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

	if err := guardTerminationProtection(ctx, client, spec, opts); err != nil {
		return err
	}
	if err := guardRetainResources(ctx, client, spec, opts); err != nil {
		return err
	}

	input := deleteStackInput(spec, opts)
	if _, err := client.DeleteStack(ctx, input); err != nil {
		return handleDeleteStackError(ctx, client, spec, opts, err)
	}
	return nil
}

// guardTerminationProtection enforces the termination_protection gate: delete
// a protected stack fails with an actionable hint unless
// --disable-termination-protection is passed (which calls
// UpdateTerminationProtection first) — silent auto-disable would defeat the
// point of the setting.
func guardTerminationProtection(ctx context.Context, client CloudFormationClient, spec *stackSpec, opts deleteOptions) error {
	if !spec.TerminationProtection {
		return nil
	}
	if !opts.DisableTerminationProtection {
		return errUtils.Build(errUtils.ErrAwsCloudFormationChangeSetFailed).
			WithExplanationf("Stack %q has termination_protection enabled.", spec.StackName).
			WithHint("Set termination_protection: false in the component config and re-apply, " +
				"or pass --disable-termination-protection to delete it anyway.").
			Err()
	}
	return disableTerminationProtection(ctx, client, spec.StackName)
}

// guardRetainResources enforces that --retain-resources is only used against
// a stack already in DELETE_FAILED status (the only status AWS accepts it
// for).
func guardRetainResources(ctx context.Context, client CloudFormationClient, spec *stackSpec, opts deleteOptions) error {
	if len(opts.RetainResources) == 0 {
		return nil
	}
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
	return nil
}

// deleteStackInput builds the DeleteStack request from spec and opts.
func deleteStackInput(spec *stackSpec, opts deleteOptions) *cloudformation.DeleteStackInput {
	input := &cloudformation.DeleteStackInput{
		StackName: awsString(spec.StackName),
	}
	if len(opts.RetainResources) > 0 {
		input.RetainResources = opts.RetainResources
	}
	if spec.RoleArn != "" {
		input.RoleARN = awsString(spec.RoleArn)
	}
	return input
}

// handleDeleteStackError wraps a DeleteStack failure and, when
// guardTerminationProtection disabled termination protection above (only in
// direct response to the user's explicit --disable-termination-protection
// flag), attempts to restore it before returning -- otherwise a failed
// delete attempt would silently leave a previously-protected stack
// unprotected.
func handleDeleteStackError(ctx context.Context, client CloudFormationClient, spec *stackSpec, opts deleteOptions, deleteAPIErr error) error {
	deleteErr := fmt.Errorf("%w: %w", errUtils.ErrAwsCloudFormationAPICallFailed, deleteAPIErr)
	if !spec.TerminationProtection || !opts.DisableTerminationProtection {
		return deleteErr
	}
	if restoreErr := restoreTerminationProtectionAfterFailedDelete(ctx, client, spec.StackName); restoreErr != nil {
		return errors.Join(deleteErr, restoreErr)
	}
	return deleteErr
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

// restoreTerminationProtectionAfterFailedDelete re-enables termination
// protection after deleteStack disabled it (via --disable-termination-protection)
// but the subsequent DeleteStack call failed. Without this, a failed delete
// attempt would silently leave a previously-protected stack unprotected.
//
// AWS rejects UpdateTerminationProtection once a stack has actually entered
// DELETE_IN_PROGRESS or DELETE_COMPLETE status (the delete request was
// accepted server-side even though this call observed a transport-level
// error, e.g. a timeout): in that case there is nothing to restore — the
// stack is being (or has been) deleted regardless of its protection flag —
// so that specific rejection is deliberately not surfaced as a restoration
// failure.
func restoreTerminationProtectionAfterFailedDelete(ctx context.Context, client CloudFormationClient, stackName string) error {
	defer perf.Track(nil, "cloudformation.restoreTerminationProtectionAfterFailedDelete")()

	_, err := client.UpdateTerminationProtection(ctx, &cloudformation.UpdateTerminationProtectionInput{
		StackName:                   awsString(stackName),
		EnableTerminationProtection: awsBool(true),
	})
	if err == nil {
		return nil
	}
	if isDeleteInProgressError(err) {
		return nil
	}
	return fmt.Errorf("restoring termination protection for stack %q after failed delete: %w", stackName, err)
}

// isDeleteInProgressError reports whether err is CloudFormation's rejection of
// UpdateTerminationProtection because the stack has already entered
// DELETE_IN_PROGRESS or DELETE_COMPLETE status.
func isDeleteInProgressError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "DELETE_IN_PROGRESS") || strings.Contains(msg, "DELETE_COMPLETE")
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
