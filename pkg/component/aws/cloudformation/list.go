package cloudformation

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// DeployedStackSummary is one entry in `atmos aws cloudformation list`'s
// output: a live CloudFormation stack, annotated with whether its name matches
// an aws/cloudformation component's stack_name configured in the queried
// Atmos stack.
type DeployedStackSummary struct {
	StackName string
	Status    string
	Managed   bool
}

// listDeployedStacks fetches account stacks via ListStacks, paginating
// through NextToken, optionally filtered by status.
func listDeployedStacks(ctx context.Context, client CloudFormationClient, statusFilter []cfntypes.StackStatus) ([]cfntypes.StackSummary, error) {
	defer perf.Track(nil, "cloudformation.listDeployedStacks")()

	var stacks []cfntypes.StackSummary
	var nextToken *string
	for {
		out, err := client.ListStacks(ctx, &cloudformation.ListStacksInput{
			StackStatusFilter: statusFilter,
			NextToken:         nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errUtils.ErrAwsCloudFormationChangeSetFailed, err)
		}
		stacks = append(stacks, out.StackSummaries...)
		if out.NextToken == nil {
			return stacks, nil
		}
		nextToken = out.NextToken
	}
}

// ListDeployedStacks resolves credentials for the active identity, fetches the
// account's deployed CloudFormation stacks, and annotates each one with
// whether it matches a configured aws/cloudformation component's stack_name
// (configuredStackNames — built by the CLI layer from the queried Atmos
// stack's components, since matching stack_name against Atmos configuration is
// outside this SDK-only package's concern).
func ListDeployedStacks(
	ctx context.Context,
	info *schema.ConfigAndStacksInfo,
	region string,
	statusFilter []string,
	configuredStackNames map[string]bool,
) ([]DeployedStackSummary, error) {
	defer perf.Track(nil, "cloudformation.ListDeployedStacks")()

	awsCfg, err := buildAWSConfig(ctx, info, region)
	if err != nil {
		return nil, err
	}
	client := newClient(awsCfg, resolveEndpointURL(info))

	stacks, err := listDeployedStacks(ctx, client, toStackStatuses(statusFilter))
	if err != nil {
		return nil, err
	}

	return annotateManagedStacks(stacks, configuredStackNames), nil
}

// annotateManagedStacks converts raw ListStacks summaries into
// DeployedStackSummary entries, annotating each one's Managed field against
// configuredStackNames. Split out of ListDeployedStacks so the annotation
// logic (a pure function) is testable independent of AWS config/client
// construction.
func annotateManagedStacks(stacks []cfntypes.StackSummary, configuredStackNames map[string]bool) []DeployedStackSummary {
	summaries := make([]DeployedStackSummary, 0, len(stacks))
	for i := range stacks {
		s := &stacks[i]
		name := stringValue(s.StackName)
		summaries = append(summaries, DeployedStackSummary{
			StackName: name,
			Status:    string(s.StackStatus),
			Managed:   configuredStackNames[name],
		})
	}
	return summaries
}

// toStackStatuses converts CLI-provided status strings to the SDK's enum type.
func toStackStatuses(statusFilter []string) []cfntypes.StackStatus {
	if len(statusFilter) == 0 {
		return nil
	}
	statuses := make([]cfntypes.StackStatus, 0, len(statusFilter))
	for _, s := range statusFilter {
		statuses = append(statuses, cfntypes.StackStatus(s))
	}
	return statuses
}

// RenderDeployedStacksList writes the list to the data channel (stdout) as a
// simple table: one line per stack, with a "MANAGED"/"unmanaged" marker.
func RenderDeployedStacksList(stacks []DeployedStackSummary) {
	defer perf.Track(nil, "cloudformation.RenderDeployedStacksList")()

	if len(stacks) == 0 {
		_ = data.Writeln("No stacks found.")
		return
	}
	for _, s := range stacks {
		managed := "unmanaged"
		if s.Managed {
			managed = "managed"
		}
		_ = data.Writeln(fmt.Sprintf("%-9s %-30s %s", managed, s.Status, s.StackName))
	}
}
