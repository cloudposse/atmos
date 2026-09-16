package cloudformation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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

const terminationProtectionRestoreTimeout = 30 * time.Second

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
// still short-circuits without an extra API call.
//
// All non-mutating validation (termination-protection gate, --retain-resources
// gate) runs first; termination protection is only ever disabled immediately
// before the DeleteStack call itself. Ordering it this way -- rather than
// disabling protection as part of the termination-protection gate, before
// --retain-resources validation runs -- means a later gate failing (e.g. the
// stack isn't in DELETE_FAILED status) can never leave the stack's
// termination protection disabled with no DeleteStack call, and therefore no
// restoration path, ever having run.
func deleteStack(ctx context.Context, client CloudFormationClient, spec *stackSpec, opts deleteOptions) error {
	defer perf.Track(nil, "cloudformation.deleteStack")()

	// describedStack is populated lazily by whichever gate below needs a live
	// DescribeStacks lookup first, and reused by the others if they also need
	// one — so a single delete call never issues more DescribeStacks requests
	// than necessary even when the termination-protection gate,
	// --retain-resources gate, and disable-termination-protection's own live
	// read all apply.
	describedStack, err := validateTerminationProtectionGate(ctx, client, spec, opts)
	if err != nil {
		return err
	}

	if len(opts.RetainResources) > 0 {
		stack, err := checkRetainResourcesGate(ctx, client, spec, describedStack)
		if err != nil {
			return err
		}
		describedStack = stack
	}

	wasProtected := false
	if opts.DisableTerminationProtection {
		wasProtected, err = disableTerminationProtectionIfNeeded(ctx, client, spec.StackName, describedStack)
		if err != nil {
			return err
		}
	}

	input := deleteStackInput(spec, opts)
	if _, err := client.DeleteStack(ctx, input); err != nil {
		return handleDeleteStackError(ctx, deleteAttempt{Client: client, Spec: spec, Opts: opts, WasProtected: wasProtected}, err)
	}
	return nil
}

// deleteAttempt bundles deleteStack's shared state through to
// handleDeleteStackError, to stay under this repo's 5-argument function limit.
type deleteAttempt struct {
	Client CloudFormationClient
	Spec   *stackSpec
	Opts   deleteOptions
	// WasProtected records whether disableTerminationProtectionIfNeeded
	// actually found the stack protected (and therefore disabled it) before
	// this DeleteStack call. Restoration on failure must only happen when
	// this is true -- see handleDeleteStackError.
	WasProtected bool
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
// disableTerminationProtectionIfNeeded actually disabled termination
// protection above (attempt.WasProtected -- it only does so when the stack
// was genuinely protected live, never unconditionally), attempts to restore
// it before returning -- otherwise a failed delete attempt would silently
// leave a previously-protected stack unprotected. Conversely, when the stack
// was never protected to begin with (WasProtected is false, e.g.
// --disable-termination-protection was passed redundantly against an
// already-unprotected stack), no restoration happens -- unconditionally
// restoring here would turn an originally-unprotected stack into a protected
// one purely as a side effect of a failed delete attempt.
func handleDeleteStackError(ctx context.Context, attempt deleteAttempt, deleteAPIErr error) error {
	deleteErr := fmt.Errorf("%w: %w", errUtils.ErrAwsCloudFormationAPICallFailed, deleteAPIErr)
	if !attempt.WasProtected {
		return deleteErr
	}
	// A failed or canceled delete must not prevent restoring the protection we
	// disabled. Preserve context values, but give cleanup its own bounded lifetime.
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), terminationProtectionRestoreTimeout)
	defer cancel()
	if restoreErr := restoreTerminationProtectionAfterFailedDelete(cleanupCtx, attempt.Client, attempt.Spec.StackName); restoreErr != nil {
		return errors.Join(deleteErr, restoreErr)
	}
	return deleteErr
}

// validateTerminationProtectionGate enforces the termination-protection gate
// without mutating anything: when --disable-termination-protection is set,
// the check is skipped entirely and left to
// disableTerminationProtectionIfNeeded, called immediately before DeleteStack
// (see deleteStack) -- so a later validation gate (e.g. --retain-resources)
// failing can never leave protection disabled with no DeleteStack call, and
// therefore no restoration path, ever having run. Otherwise, a local `true`
// short-circuits without an API call; a local `false` is verified against the
// stack's live EnableTerminationProtection, since local config can drift
// (apply only ever turns protection on, never off). Returns the described
// stack (nil if no live lookup was needed) so later steps can reuse it
// instead of issuing another DescribeStacks call.
func validateTerminationProtectionGate(ctx context.Context, client CloudFormationClient, spec *stackSpec, opts deleteOptions) (*cfntypes.Stack, error) {
	defer perf.Track(nil, "cloudformation.validateTerminationProtectionGate")()

	if opts.DisableTerminationProtection {
		return nil, nil
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
// already-described stack from validateTerminationProtectionGate when one is
// available instead of issuing a second DescribeStacks call. Returns the
// described stack so later steps (disableTerminationProtectionIfNeeded) can
// reuse it in turn.
func checkRetainResourcesGate(ctx context.Context, client CloudFormationClient, spec *stackSpec, describedStack *cfntypes.Stack) (*cfntypes.Stack, error) {
	defer perf.Track(nil, "cloudformation.checkRetainResourcesGate")()

	if describedStack == nil {
		var err error
		describedStack, err = describeStack(ctx, client, spec.StackName)
		if err != nil {
			return nil, err
		}
	}
	if !isDeleteFailedStack(describedStack.StackStatus) {
		return nil, errUtils.Build(errUtils.ErrAwsCloudFormationChangeSetFailed).
			WithExplanationf("--retain-resources is only valid for a stack in DELETE_FAILED status; %s is currently %s.", spec.StackName, describedStack.StackStatus).
			WithHint("Retry the delete without --retain-resources, or wait for the stack to reach DELETE_FAILED.").
			Err()
	}
	return describedStack, nil
}

// disableTerminationProtectionIfNeeded checks the stack's live
// termination-protection state (reusing describedStack when an earlier gate
// already fetched one) and disables it only when currently enabled, reporting
// whether it did so via the returned bool. Skipping the mutating
// UpdateTerminationProtection call entirely when the stack was never
// protected is not just an optimization: it is also what lets
// handleDeleteStackError know it must NOT restore protection after a failed
// delete -- restoring unconditionally would turn an originally-unprotected
// stack into a protected one purely because --disable-termination-protection
// happened to be passed (even redundantly).
func disableTerminationProtectionIfNeeded(ctx context.Context, client CloudFormationClient, stackName string, describedStack *cfntypes.Stack) (bool, error) {
	defer perf.Track(nil, "cloudformation.disableTerminationProtectionIfNeeded")()

	if describedStack == nil {
		var err error
		describedStack, err = describeStack(ctx, client, stackName)
		if err != nil {
			return false, err
		}
	}
	if !aws.ToBool(describedStack.EnableTerminationProtection) {
		return false, nil
	}
	if err := disableTerminationProtection(ctx, client, stackName); err != nil {
		return false, err
	}
	return true, nil
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
