package cloudformation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/smithy-go"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ui"
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
	if err := deleteChangeSetWhenReady(cleanupCtx, client, stackName, result.ChangeSetName); err != nil {
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

// deleteChangeSetWhenReady waits out creation only after AWS rejects deletion for its status.
// All attempts share the cleanup deadline; other API errors remain terminal.
func deleteChangeSetWhenReady(ctx context.Context, client CloudFormationClient, stackName, name string) error {
	var deleteErr error
	creating := false
	for {
		if err := ctx.Err(); err != nil {
			return errors.Join(deleteErr, err)
		}
		if !creating {
			_, deleteErr = client.DeleteChangeSet(ctx, &cloudformation.DeleteChangeSetInput{
				StackName: awsString(stackName), ChangeSetName: awsString(name),
			})
			var apiErr smithy.APIError
			if !errors.As(deleteErr, &apiErr) || apiErr.ErrorCode() != "InvalidChangeSetStatus" {
				return deleteErr
			}
		}
		state, err := client.DescribeChangeSet(ctx, &cloudformation.DescribeChangeSetInput{
			StackName: awsString(stackName), ChangeSetName: awsString(name),
		})
		if err != nil {
			return errors.Join(deleteErr, err)
		}
		var retry bool
		creating, retry = changeSetCleanupStatus(state.Status)
		if !retry {
			return deleteErr
		}
		// Also delay repeated rejections of a ready change set, avoiding a tight API loop.
		select {
		case <-ctx.Done():
			return errors.Join(deleteErr, ctx.Err())
		case <-time.After(changeSetPollInterval):
		}
	}
}

// changeSetCleanupStatus permits a retry after creation; other states retain AWS's rejection.
func changeSetCleanupStatus(status cfntypes.ChangeSetStatus) (creating, retry bool) {
	switch status {
	case cfntypes.ChangeSetStatusCreatePending, cfntypes.ChangeSetStatusCreateInProgress:
		return true, true
	case cfntypes.ChangeSetStatusCreateComplete, cfntypes.ChangeSetStatusFailed:
		return false, true
	default:
		return false, false
	}
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
	if err := waitForStubDeletion(ctx, client, stackName); err != nil {
		return fmt.Errorf("%w: wait for preview stack %q deletion: %w", errUtils.ErrAwsCloudFormationChangeSetFailed, stackName, err)
	}
	return nil
}

// waitForStubDeletion observes completion within the caller's cleanup deadline.
// Deleted stacks can remain describable by their immutable ID as DELETE_COMPLETE.
func waitForStubDeletion(ctx context.Context, client CloudFormationClient, stackName string) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		state, err := describeStackState(ctx, client, stackName)
		if err != nil {
			return err
		}
		if !state.Found || state.Status == cfntypes.StackStatusDeleteComplete {
			return nil
		}
		if state.Status == cfntypes.StackStatusDeleteFailed {
			return fmt.Errorf("%w: stack reached %s", errUtils.ErrAwsCloudFormationChangeSetFailed, state.Status)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(eventPollInterval):
		}
	}
}

// discardChangeSetBestEffort preserves apply and explicit-create outcomes while warning about cleanup failures.
func discardChangeSetBestEffort(ctx context.Context, client CloudFormationClient, stackName string, result *changeSetResult) {
	if err := discardChangeSet(ctx, client, stackName, result); err != nil {
		ui.Warning(fmt.Sprintf("failed to clean up CloudFormation changeset: %v", err))
	}
}

// joinCleanupError retains the original diagnostic metadata when cleanup succeeds.
// Joining a sole error can hide hints from consumers that follow a single error chain.
func joinCleanupError(operationErr, cleanupErr error) error {
	if cleanupErr == nil {
		return operationErr
	}
	return errors.Join(operationErr, cleanupErr)
}
