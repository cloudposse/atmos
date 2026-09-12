package cloudformation

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
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
// protected stack fails with an actionable hint unless
// --disable-termination-protection is passed (which calls
// UpdateTerminationProtection first) — silent auto-disable would defeat the
// point of the setting.
//
// The local, resolved `termination_protection:` config value can legitimately
// drift from the stack's actual live AWS state: applyTerminationProtection
// only ever turns protection ON during apply, never OFF, so a component whose
// config was edited to `termination_protection: false` (without ever calling
// delete with --disable-termination-protection) can still be protected in AWS.
// Trusting the local value alone for this gate would let that drift silently
// defeat the safety net, so a local `false` is verified against the stack's
// live EnableTerminationProtection before the gate is skipped. A local `true`
// still short-circuits without an extra API call, and
// --disable-termination-protection skips the live lookup entirely since
// UpdateTerminationProtection(false) is unconditionally issued regardless of
// what triggered detection of protection being on.
func deleteStack(ctx context.Context, client CloudFormationClient, spec *stackSpec, opts deleteOptions) error {
	defer perf.Track(nil, "cloudformation.deleteStack")()

	// describedStack is populated lazily by whichever gate below needs a live
	// DescribeStacks lookup first, and reused by the other gate if it also
	// needs one — so a single delete call never issues more than one
	// DescribeStacks request even when both the termination-protection and
	// --retain-resources checks apply.
	describedStack, err := checkTerminationProtectionGate(ctx, client, spec, opts)
	if err != nil {
		return err
	}

	if len(opts.RetainResources) > 0 {
		if err := checkRetainResourcesGate(ctx, client, spec, describedStack); err != nil {
			return err
		}
	}

	input := deleteStackInput(spec, opts)
	if _, err := client.DeleteStack(ctx, input); err != nil {
		return handleDeleteStackError(ctx, client, spec, opts, err)
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
// checkTerminationProtectionGate disabled termination protection above (in
// direct response to the user's explicit --disable-termination-protection
// flag — checkTerminationProtectionGate calls disableTerminationProtection
// unconditionally whenever that flag is set, regardless of what
// spec.TerminationProtection's local, possibly-drifted value says), attempts
// to restore it before returning -- otherwise a failed delete attempt would
// silently leave a previously-protected stack unprotected.
func handleDeleteStackError(ctx context.Context, client CloudFormationClient, spec *stackSpec, opts deleteOptions, deleteAPIErr error) error {
	deleteErr := fmt.Errorf("%w: %w", errUtils.ErrAwsCloudFormationAPICallFailed, deleteAPIErr)
	if !opts.DisableTerminationProtection {
		return deleteErr
	}
	if restoreErr := restoreTerminationProtectionAfterFailedDelete(ctx, client, spec.StackName); restoreErr != nil {
		return errors.Join(deleteErr, restoreErr)
	}
	return deleteErr
}

// checkTerminationProtectionGate enforces the termination-protection gate.
// When --disable-termination-protection is set, protection is disabled
// unconditionally via UpdateTerminationProtection — no live lookup is needed
// first, regardless of which signal (local config or live AWS state) would
// otherwise have reported protection on. Otherwise, a local `true` short-
// circuits without an API call; a local `false` is verified against the
// stack's live EnableTerminationProtection, since local config can drift
// (apply only ever turns protection on, never off). Returns the described
// stack (nil if no live lookup was needed) so checkRetainResourcesGate can
// reuse it instead of issuing a second DescribeStacks call.
func checkTerminationProtectionGate(ctx context.Context, client CloudFormationClient, spec *stackSpec, opts deleteOptions) (*cfntypes.Stack, error) {
	defer perf.Track(nil, "cloudformation.checkTerminationProtectionGate")()

	if opts.DisableTerminationProtection {
		return nil, disableTerminationProtection(ctx, client, spec.StackName)
	}

	protected := spec.TerminationProtection
	var describedStack *cfntypes.Stack
	if !protected {
		var err error
		describedStack, err = describeStack(ctx, client, spec.StackName)
		if err != nil {
			return nil, err
		}
		protected = aws.ToBool(describedStack.EnableTerminationProtection)
	}
	if protected {
		return describedStack, errUtils.Build(errUtils.ErrAwsCloudFormationChangeSetFailed).
			WithExplanationf("Stack %q has termination_protection enabled.", spec.StackName).
			WithHint("Pass --disable-termination-protection to delete it anyway. " +
				"Setting termination_protection: false and re-applying does not disable " +
				"protection on the stack; apply only ever turns protection on, never off.").
			Err()
	}
	return describedStack, nil
}

// checkRetainResourcesGate enforces that --retain-resources is only used
// against a stack in DELETE_FAILED status (AWS semantics), reusing an
// already-described stack from checkTerminationProtectionGate when one is
// available instead of issuing a second DescribeStacks call.
func checkRetainResourcesGate(ctx context.Context, client CloudFormationClient, spec *stackSpec, describedStack *cfntypes.Stack) error {
	defer perf.Track(nil, "cloudformation.checkRetainResourcesGate")()

	if describedStack == nil {
		var err error
		describedStack, err = describeStack(ctx, client, spec.StackName)
		if err != nil {
			return err
		}
	}
	if !isDeleteFailedStack(describedStack.StackStatus) {
		return errUtils.Build(errUtils.ErrAwsCloudFormationChangeSetFailed).
			WithExplanationf("--retain-resources is only valid for a stack in DELETE_FAILED status; %s is currently %s.", spec.StackName, describedStack.StackStatus).
			WithHint("Retry the delete without --retain-resources, or wait for the stack to reach DELETE_FAILED.").
			Err()
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

// describeStack fetches the stack's full live description (status,
// termination protection, etc.) via a single DescribeStacks call, shared by
// deleteStack's termination-protection and --retain-resources gates so both
// can be answered from the same live lookup.
func describeStack(ctx context.Context, client CloudFormationClient, stackName string) (*cfntypes.Stack, error) {
	defer perf.Track(nil, "cloudformation.describeStack")()

	out, err := client.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{StackName: awsString(stackName)})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrAwsCloudFormationAPICallFailed, err)
	}
	if len(out.Stacks) == 0 {
		return nil, fmt.Errorf("%w: stack %s not found", errUtils.ErrAwsCloudFormationChangeSetFailed, stackName)
	}
	return &out.Stacks[0], nil
}
