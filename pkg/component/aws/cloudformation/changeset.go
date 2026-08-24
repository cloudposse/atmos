package cloudformation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// changeSetPollInterval is how often DescribeChangeSet is polled while a changeset
// is being created.
const changeSetPollInterval = 2 * time.Second

// changeSetTimeout bounds how long changeset creation may take before giving up.
const changeSetTimeout = 5 * time.Minute

// changeSetResult is the outcome of creating (and describing) a changeset.
type changeSetResult struct {
	ChangeSetID   string
	ChangeSetName string
	StackID       string
	Status        cfntypes.ChangeSetStatus
	StatusReason  string
	Changes       []cfntypes.Change
	// NoOp is true when CloudFormation reports the changeset would make no
	// changes (a stack already matching the desired state) — apply is a no-op.
	NoOp bool
}

// changeSetName generates a unique, stack-scoped changeset name for this operation.
func changeSetName(stackName string) string {
	return fmt.Sprintf("atmos-%s-%d", sanitizeChangeSetSuffix(stackName), time.Now().UnixNano())
}

// sanitizeChangeSetSuffix keeps changeset names within CloudFormation's
// alphanumeric-and-hyphen naming constraint.
func sanitizeChangeSetSuffix(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return b.String()
}

// stackExists reports whether the stack already exists (and is not itself
// mid-deletion), determining whether the changeset should be CREATE or UPDATE type.
func stackExists(ctx context.Context, client CloudFormationClient, stackName string) (bool, error) {
	defer perf.Track(nil, "cloudformation.stackExists")()

	out, err := client.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{StackName: awsString(stackName)})
	if err != nil {
		if isStackNotFoundError(err) {
			return false, nil
		}
		return false, err
	}
	for i := range out.Stacks {
		if out.Stacks[i].StackStatus == cfntypes.StackStatusReviewInProgress {
			// A stack left in REVIEW_IN_PROGRESS (a CREATE changeset was made but
			// never executed, then abandoned) has no real resources yet — treat
			// as not-existing so a fresh CREATE changeset can be generated.
			continue
		}
		return true, nil
	}
	return false, nil
}

// isStackNotFoundError reports whether err is CloudFormation's "does not exist" error.
func isStackNotFoundError(err error) bool {
	return strings.Contains(err.Error(), "does not exist")
}

// createChangeSet creates a changeset (CREATE or UPDATE, auto-detected) and waits
// for it to finish computing, returning the described changeset.
func createChangeSet(ctx context.Context, client CloudFormationClient, spec *stackSpec) (*changeSetResult, error) {
	defer perf.Track(nil, "cloudformation.createChangeSet")()

	exists, err := stackExists(ctx, client, spec.StackName)
	if err != nil {
		return nil, err
	}

	changeSetType := cfntypes.ChangeSetTypeCreate
	if exists {
		changeSetType = cfntypes.ChangeSetTypeUpdate
	}

	name := changeSetName(spec.StackName)

	input := &cloudformation.CreateChangeSetInput{
		ChangeSetName:    awsString(name),
		StackName:        awsString(spec.StackName),
		ChangeSetType:    changeSetType,
		TemplateBody:     awsString(spec.TemplateBody),
		Parameters:       spec.Parameters,
		Capabilities:     spec.Capabilities,
		Tags:             spec.Tags,
		NotificationARNs: spec.NotificationArns,
	}
	if spec.RoleArn != "" {
		input.RoleARN = awsString(spec.RoleArn)
	}
	if spec.DisableRollback && changeSetType == cfntypes.ChangeSetTypeCreate {
		input.OnStackFailure = cfntypes.OnStackFailureDoNothing
	}

	if _, err := client.CreateChangeSet(ctx, input); err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrAwsCloudFormationChangeSetFailed, err)
	}

	return waitForChangeSet(ctx, client, spec.StackName, name)
}

// changeSetPollDecision is the outcome of inspecting one DescribeChangeSet poll.
type changeSetPollDecision int

const (
	changeSetPollDone changeSetPollDecision = iota
	changeSetPollError
	changeSetPollContinue
)

// waitForChangeSet polls DescribeChangeSet until the changeset finishes computing
// (CREATE_COMPLETE, FAILED, or a no-op "didn't contain changes" failure).
func waitForChangeSet(ctx context.Context, client CloudFormationClient, stackName, name string) (*changeSetResult, error) {
	defer perf.Track(nil, "cloudformation.waitForChangeSet")()

	deadline := time.Now().Add(changeSetTimeout)
	for {
		out, err := client.DescribeChangeSet(ctx, &cloudformation.DescribeChangeSetInput{
			ChangeSetName: awsString(name),
			StackName:     awsString(stackName),
		})
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errUtils.ErrAwsCloudFormationChangeSetFailed, err)
		}

		result := &changeSetResult{
			ChangeSetID:   stringValue(out.ChangeSetId),
			ChangeSetName: name,
			StackID:       stringValue(out.StackId),
			Status:        out.Status,
			StatusReason:  stringValue(out.StatusReason),
			Changes:       out.Changes,
		}

		if decision, err := evaluateChangeSetStatus(result); decision != changeSetPollContinue {
			return result, err
		}

		if time.Now().After(deadline) {
			return result, fmt.Errorf("%w: timed out waiting for changeset to compute", errUtils.ErrAwsCloudFormationChangeSetFailed)
		}
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(changeSetPollInterval):
		}
	}
}

// evaluateChangeSetStatus classifies a changeset's current status: done (terminal
// success, possibly a no-op), error (terminal failure), or continue (keep polling).
// Mutates result.NoOp in place when the changeset is a no-op.
func evaluateChangeSetStatus(result *changeSetResult) (changeSetPollDecision, error) {
	switch result.Status {
	case cfntypes.ChangeSetStatusCreateComplete:
		return changeSetPollDone, nil
	case cfntypes.ChangeSetStatusFailed:
		if isNoOpChangeSet(result.StatusReason) {
			result.NoOp = true
			return changeSetPollDone, nil
		}
		return changeSetPollError, fmt.Errorf("%w: %s", errUtils.ErrAwsCloudFormationChangeSetFailed, result.StatusReason)
	case cfntypes.ChangeSetStatusCreateInProgress, cfntypes.ChangeSetStatusCreatePending:
		return changeSetPollContinue, nil
	default:
		if strings.HasSuffix(string(result.Status), "_IN_PROGRESS") || strings.HasSuffix(string(result.Status), "_PENDING") {
			return changeSetPollContinue, nil
		}
		return changeSetPollError, fmt.Errorf("%w: unexpected changeset status %s", errUtils.ErrAwsCloudFormationChangeSetFailed, result.Status)
	}
}

// isNoOpChangeSet reports whether a FAILED changeset status reason indicates
// CloudFormation found no changes to make (not a real failure).
func isNoOpChangeSet(reason string) bool {
	lower := strings.ToLower(reason)
	return strings.Contains(lower, "no updates are to be performed") ||
		strings.Contains(lower, "didn't contain changes")
}

// executeChangeSet executes a previously-created, computed changeset.
func executeChangeSet(ctx context.Context, client CloudFormationClient, spec *stackSpec, result *changeSetResult) error {
	defer perf.Track(nil, "cloudformation.executeChangeSet")()

	input := &cloudformation.ExecuteChangeSetInput{
		ChangeSetName: awsString(result.ChangeSetName),
		StackName:     awsString(spec.StackName),
	}
	if spec.DisableRollback {
		input.DisableRollback = awsBool(true)
	}

	_, err := client.ExecuteChangeSet(ctx, input)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrAwsCloudFormationChangeSetFailed, err)
	}
	return nil
}

func stringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func awsBool(b bool) *bool {
	return &b
}
