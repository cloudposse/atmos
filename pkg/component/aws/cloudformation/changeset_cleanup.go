package cloudformation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"

	errUtils "github.com/cloudposse/atmos/errors"
)

// changeSetCleanupTimeout bounds cleanup even when the caller cancelled its operation.
const changeSetCleanupTimeout = 30 * time.Second

// discardChangeSet removes an abandoned changeset and its newly created empty stack.
// Errors are returned so a preview cannot report success while leaking AWS resources.
func discardChangeSet(ctx context.Context, client CloudFormationClient, stackName string, result *changeSetResult) error {
	if result == nil || result.ChangeSetName == "" {
		return nil
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), changeSetCleanupTimeout)
	defer cancel()
	_, err := client.DeleteChangeSet(cleanupCtx, &cloudformation.DeleteChangeSetInput{
		StackName: awsString(stackName), ChangeSetName: awsString(result.ChangeSetName),
	})
	if err != nil {
		return fmt.Errorf("%w: clean up changeset %q: %w", errUtils.ErrAwsCloudFormationChangeSetFailed, result.ChangeSetName, err)
	}
	if !result.StackStub {
		return nil
	}
	// Prefer the immutable stack ID so a replacement with the same name is safe.
	if result.StackID != "" {
		stackName = result.StackID
	}
	return removeStubStack(cleanupCtx, client, stackName)
}

// removeStubStack only deletes an owned stack while it is still empty and unexecuted.
func removeStubStack(ctx context.Context, client CloudFormationClient, stackName string) error {
	state, err := describeStackState(ctx, client, stackName)
	if err != nil {
		return fmt.Errorf("%w: inspect preview stack %q: %w", errUtils.ErrAwsCloudFormationAPICallFailed, stackName, err)
	}
	if !state.Found || state.Status != cfntypes.StackStatusReviewInProgress {
		return nil
	}
	_, err = client.DeleteStack(ctx, &cloudformation.DeleteStackInput{StackName: awsString(stackName)})
	if err != nil {
		return fmt.Errorf("%w: remove preview stack %q: %w", errUtils.ErrAwsCloudFormationAPICallFailed, stackName, err)
	}
	return nil
}

// joinCleanupError retains the original diagnostic metadata when cleanup succeeds.
// Joining a sole error can hide hints from consumers that follow a single error chain.
func joinCleanupError(operationErr, cleanupErr error) error {
	if cleanupErr == nil {
		return operationErr
	}
	return errors.Join(operationErr, cleanupErr)
}
