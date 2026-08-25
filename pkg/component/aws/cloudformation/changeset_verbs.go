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

// listChangeSets returns every changeset for a stack, paginating through
// ListChangeSets until CloudFormation stops returning a NextToken.
func listChangeSets(ctx context.Context, client CloudFormationClient, stackName string) ([]cfntypes.ChangeSetSummary, error) {
	defer perf.Track(nil, "cloudformation.listChangeSets")()

	var summaries []cfntypes.ChangeSetSummary
	var nextToken *string
	for {
		out, err := client.ListChangeSets(ctx, &cloudformation.ListChangeSetsInput{
			StackName: awsString(stackName),
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errUtils.ErrAwsCloudFormationChangeSetFailed, err)
		}
		summaries = append(summaries, out.Summaries...)
		if out.NextToken == nil {
			return summaries, nil
		}
		nextToken = out.NextToken
	}
}

// deleteChangeSet deletes a named changeset without touching the stack itself.
func deleteChangeSet(ctx context.Context, client CloudFormationClient, stackName, changeSetName string) error {
	defer perf.Track(nil, "cloudformation.deleteChangeSet")()

	_, err := client.DeleteChangeSet(ctx, &cloudformation.DeleteChangeSetInput{
		StackName:     awsString(stackName),
		ChangeSetName: awsString(changeSetName),
	})
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrAwsCloudFormationChangeSetFailed, err)
	}
	return nil
}

// describeNamedChangeSet fetches a previously-created changeset by name, for the
// `changeset execute`/`changeset delete` verbs, which act on an existing changeset
// rather than creating a fresh one (unlike apply/deploy/diff/plan).
func describeNamedChangeSet(ctx context.Context, client CloudFormationClient, stackName, changeSetName string) (*changeSetResult, error) {
	defer perf.Track(nil, "cloudformation.describeNamedChangeSet")()

	out, err := client.DescribeChangeSet(ctx, &cloudformation.DescribeChangeSetInput{
		ChangeSetName: awsString(changeSetName),
		StackName:     awsString(stackName),
	})
	if err != nil {
		if isStackNotFoundError(err) {
			return nil, fmt.Errorf("%w: %q on stack %q", errUtils.ErrAwsCloudFormationChangeSetNotFound, changeSetName, stackName)
		}
		return nil, fmt.Errorf("%w: %w", errUtils.ErrAwsCloudFormationChangeSetFailed, err)
	}

	return &changeSetResult{
		ChangeSetID:   stringValue(out.ChangeSetId),
		ChangeSetName: changeSetName,
		StackID:       stringValue(out.StackId),
		Status:        out.Status,
		StatusReason:  stringValue(out.StatusReason),
		Changes:       out.Changes,
	}, nil
}

// runChangesetCreate creates a changeset and leaves it in place for later manual
// review/execution — the explicit-control complement to diff/plan's implicit,
// preview-only changeset (which is also left in place, but framed as a preview
// rather than a named, reusable artifact).
func runChangesetCreate(ctx context.Context, client CloudFormationClient, spec *stackSpec, summary map[string]any) (map[string]any, error) {
	result, err := createChangeSet(ctx, client, spec)
	if err != nil {
		return summary, err
	}
	summary["changeset_id"] = result.ChangeSetID
	summary["changeset_name"] = result.ChangeSetName
	summary["no_op"] = result.NoOp
	summary["changes"] = result.Changes
	renderDiffSummary(spec.StackName, result)
	if !result.NoOp {
		_ = data.Writeln(fmt.Sprintf("changeset: %s", result.ChangeSetName))
	}
	return summary, nil
}

// runChangesetExecute executes a previously-created, named changeset and streams
// stack events until the operation reaches a terminal state.
func runChangesetExecute(ctx context.Context, client CloudFormationClient, spec *stackSpec, changeSetName string, summary map[string]any) (map[string]any, error) {
	result, err := describeNamedChangeSet(ctx, client, spec.StackName, changeSetName)
	if err != nil {
		return summary, err
	}
	summary["changeset_id"] = result.ChangeSetID
	summary["changeset_name"] = result.ChangeSetName

	if err := executeChangeSet(ctx, client, spec, result); err != nil {
		return summary, err
	}

	status, err := streamStackEvents(ctx, client, spec.StackName)
	if err != nil {
		return summary, err
	}
	summary["final_status"] = string(status)
	if isFailedStackStatus(status) {
		return summary, fmt.Errorf("%w: stack %s ended in status %s", errUtils.ErrAwsCloudFormationChangeSetFailed, spec.StackName, status)
	}
	return summary, nil
}

// runChangesetList lists a stack's changesets, newest first (ListChangeSets'
// own ordering), rendering each one's name, status, and creation time.
func runChangesetList(ctx context.Context, client CloudFormationClient, spec *stackSpec, summary map[string]any) (map[string]any, error) {
	summaries, err := listChangeSets(ctx, client, spec.StackName)
	if err != nil {
		return summary, err
	}
	summary["changesets"] = summaries

	if len(summaries) == 0 {
		_ = data.Writeln(fmt.Sprintf("%s: no changesets", spec.StackName))
		return summary, nil
	}
	for i := range summaries {
		cs := &summaries[i]
		line := fmt.Sprintf("%-40s %-20s %s", stringValue(cs.ChangeSetName), cs.Status, stringValue(cs.Description))
		_ = data.Writeln(line)
	}
	return summary, nil
}

// runChangesetDelete deletes a named changeset without touching the stack.
func runChangesetDelete(ctx context.Context, client CloudFormationClient, spec *stackSpec, changeSetName string, summary map[string]any) (map[string]any, error) {
	if err := deleteChangeSet(ctx, client, spec.StackName, changeSetName); err != nil {
		return summary, err
	}
	summary["changeset_name"] = changeSetName
	_ = data.Writeln(fmt.Sprintf("changeset %q deleted", changeSetName))
	return summary, nil
}
