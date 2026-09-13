package cloudformation

import (
	"context"
	"fmt"
	"strconv"
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

// wrapFmt is the shared "%w: %w" format for wrapping a sentinel error around an
// underlying AWS API error, used throughout this file's changeset API calls.
const wrapFmt = "%w: %w"

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
	// ChangeSetType records whether this changeset creates a new stack or
	// updates an existing one — executeChangeSet needs this to know whether
	// OnStackFailure was already set on CreateChangeSet (CREATE-only), which
	// AWS's API forbids combining with ExecuteChangeSet's DisableRollback.
	ChangeSetType cfntypes.ChangeSetType
}

// changeSetNameMaxLength is CloudFormation's hard limit on ChangeSetName length
// (same limit as stack names): https://docs.aws.amazon.com/AWSCloudFormation/latest/APIReference/API_CreateChangeSet.html
const changeSetNameMaxLength = 128

// decimalBase is passed to strconv.FormatInt for the changeset name's
// timestamp suffix (a plain base-10 decimal, not hex/octal).
const decimalBase = 10

// changeSetName generates a unique, stack-scoped changeset name for this operation.
// The sanitized suffix is truncated so the final "atmos-<suffix>-<timestamp>" name
// never exceeds CloudFormation's 128-character ChangeSetName limit, even for a
// maximum-length stack name.
func changeSetName(stackName string) string {
	timestamp := strconv.FormatInt(time.Now().UnixNano(), decimalBase)
	suffix := sanitizeChangeSetSuffix(stackName)
	maxSuffixLength := changeSetNameMaxLength - len("atmos--") - len(timestamp)
	if len(suffix) > maxSuffixLength {
		suffix = suffix[:maxSuffixLength]
	}
	return fmt.Sprintf("atmos-%s-%s", suffix, timestamp)
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

// wrapAPICallError wraps a raw AWS CloudFormation SDK error with the shared
// ErrAwsCloudFormationAPICallFailed sentinel. For the common "stack does not
// exist" validation error — e.g. a post-apply follow-up call (SetStackPolicy,
// UpdateTerminationProtection, the DescribeStacks behind describeStackOutputs)
// reaching a stack that was never actually deployed — this uses the error
// builder to add an explanation and an actionable hint, instead of surfacing
// AWS's raw, unexplained message verbatim (the confusing error a user hit
// running `apply --target <publish-only-target>` before runApply learned to
// skip these follow-up calls for non-direct-deploy targets; see runApply).
// Every other AWS error shape falls back to the existing plain "%w: %w"
// wrap: forcing a hint onto an error shape this helper hasn't specifically
// recognized risks being wrong or misleading.
func wrapAPICallError(stackName string, err error) error {
	if !isStackNotFoundError(err) {
		return fmt.Errorf(wrapFmt, errUtils.ErrAwsCloudFormationAPICallFailed, err)
	}
	return errUtils.Build(errUtils.ErrAwsCloudFormationAPICallFailed).
		WithCause(err).
		WithExplanationf("Stack %q doesn't exist yet.", stackName).
		WithHint("Check `--target`/`-s`/component name, or run `apply` without `--target` first if you need to create the stack directly.").
		Err()
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
		Parameters:       spec.Parameters,
		Capabilities:     spec.Capabilities,
		Tags:             spec.Tags,
		NotificationARNs: spec.NotificationArns,
	}
	// CreateChangeSet accepts exactly one of TemplateBody/TemplateURL. A
	// packaged template (spec.TemplateURL set by deliverApply, e.g. because it
	// exceeds the 51,200-byte inline limit) must use TemplateURL -- sending
	// both, or falling back to an oversized TemplateBody, is rejected by AWS.
	if spec.TemplateURL != "" {
		input.TemplateURL = awsString(spec.TemplateURL)
	} else {
		input.TemplateBody = awsString(spec.TemplateBody)
	}
	if spec.RoleArn != "" {
		input.RoleARN = awsString(spec.RoleArn)
	}
	if spec.DisableRollback && changeSetType == cfntypes.ChangeSetTypeCreate {
		input.OnStackFailure = cfntypes.OnStackFailureDoNothing
	}

	if _, err := client.CreateChangeSet(ctx, input); err != nil {
		return nil, fmt.Errorf(wrapFmt, errUtils.ErrAwsCloudFormationChangeSetFailed, err)
	}

	return waitForChangeSet(ctx, client, spec.StackName, name, changeSetType)
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
func waitForChangeSet(ctx context.Context, client CloudFormationClient, stackName, name string, changeSetType cfntypes.ChangeSetType) (*changeSetResult, error) {
	defer perf.Track(nil, "cloudformation.waitForChangeSet")()

	deadline := time.Now().Add(changeSetTimeout)
	for {
		out, err := client.DescribeChangeSet(ctx, &cloudformation.DescribeChangeSetInput{
			ChangeSetName: awsString(name),
			StackName:     awsString(stackName),
		})
		if err != nil {
			return nil, fmt.Errorf(wrapFmt, errUtils.ErrAwsCloudFormationChangeSetFailed, err)
		}

		result := &changeSetResult{
			ChangeSetID:   stringValue(out.ChangeSetId),
			ChangeSetName: name,
			StackID:       stringValue(out.StackId),
			Status:        out.Status,
			StatusReason:  stringValue(out.StatusReason),
			Changes:       out.Changes,
			ChangeSetType: changeSetType,
		}

		if decision, err := evaluateChangeSetStatus(result); decision != changeSetPollContinue {
			if err == nil && out.NextToken != nil {
				pageArgs := changeSetPageArgs{Client: client, StackName: stackName, Name: name, NextToken: out.NextToken}
				if pageErr := fetchRemainingChangeSetPages(ctx, pageArgs, result); pageErr != nil {
					return result, pageErr
				}
			}
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

// changeSetPageArgs bundles fetchRemainingChangeSetPages' parameters to stay
// under this repo's 5-argument function limit.
type changeSetPageArgs struct {
	Client    CloudFormationClient
	StackName string
	Name      string
	NextToken *string
}

// fetchRemainingChangeSetPages follows DescribeChangeSet's NextToken, appending
// each page's Changes to result — a changeset with enough resource changes to
// paginate would otherwise silently under-report the diff a user reviews before
// approving apply. Called once the changeset has reached a terminal, successful
// status; a page-fetch error still leaves result populated with what was
// gathered so far.
func fetchRemainingChangeSetPages(ctx context.Context, args changeSetPageArgs, result *changeSetResult) error {
	defer perf.Track(nil, "cloudformation.fetchRemainingChangeSetPages")()

	nextToken := args.NextToken
	for nextToken != nil {
		out, err := args.Client.DescribeChangeSet(ctx, &cloudformation.DescribeChangeSetInput{
			ChangeSetName: awsString(args.Name),
			StackName:     awsString(args.StackName),
			NextToken:     nextToken,
		})
		if err != nil {
			return fmt.Errorf(wrapFmt, errUtils.ErrAwsCloudFormationChangeSetFailed, err)
		}
		result.Changes = append(result.Changes, out.Changes...)
		nextToken = out.NextToken
	}
	return nil
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
	// AWS rejects an ExecuteChangeSet that sets DisableRollback when OnStackFailure
	// was already set on the CREATE changeset (createChangeSet does this for the same
	// spec.DisableRollback/CREATE combination) — omit it here to avoid that conflict.
	onStackFailureAlreadySet := spec.DisableRollback && result.ChangeSetType == cfntypes.ChangeSetTypeCreate
	if spec.DisableRollback && !onStackFailureAlreadySet {
		input.DisableRollback = awsBool(true)
	}

	_, err := client.ExecuteChangeSet(ctx, input)
	if err != nil {
		return fmt.Errorf(wrapFmt, errUtils.ErrAwsCloudFormationChangeSetFailed, err)
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
