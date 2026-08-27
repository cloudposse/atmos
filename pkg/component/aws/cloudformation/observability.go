package cloudformation

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/perf"
)

// nestedStackResourceType is the resource type a nested stack shows up as in
// its parent's ListStackResources output.
const nestedStackResourceType = "AWS::CloudFormation::Stack"

// maxNestedStackDepth bounds tree/logs' recursion into nested stacks, as a
// defensive guard against unexpectedly deep (or, in principle, cyclic) nesting.
const maxNestedStackDepth = 10

// stackNode is one stack in the nested-stack tree: its own name plus every
// child (nested) stack discovered under it.
type stackNode struct {
	StackName string
	Children  []*stackNode
}

// buildStackTree walks a stack's resources, recursing into every nested stack
// (AWS::CloudFormation::Stack resources) up to maxNestedStackDepth.
func buildStackTree(ctx context.Context, client CloudFormationClient, stackName string, depth int) (*stackNode, error) {
	defer perf.Track(nil, "cloudformation.buildStackTree")()

	node := &stackNode{StackName: stackName}
	if depth >= maxNestedStackDepth {
		return node, nil
	}

	resources, err := listAllStackResources(ctx, client, stackName)
	if err != nil {
		return nil, err
	}

	for i := range resources {
		r := &resources[i]
		if stringValue(r.ResourceType) != nestedStackResourceType {
			continue
		}
		childID := stringValue(r.PhysicalResourceId)
		if childID == "" {
			// Nested stack not yet created (still IN_PROGRESS) — nothing to recurse into yet.
			continue
		}
		child, err := buildStackTree(ctx, client, childID, depth+1)
		if err != nil {
			return nil, err
		}
		node.Children = append(node.Children, child)
	}
	return node, nil
}

// listAllStackResources fetches every resource for a stack, paginating through NextToken.
func listAllStackResources(ctx context.Context, client CloudFormationClient, stackName string) ([]cfntypes.StackResourceSummary, error) {
	var resources []cfntypes.StackResourceSummary
	var nextToken *string
	for {
		out, err := client.ListStackResources(ctx, &cloudformation.ListStackResourcesInput{
			StackName: awsString(stackName),
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf(errWrapFmt, errUtils.ErrAwsCloudFormationAPICallFailed, err)
		}
		resources = append(resources, out.StackResourceSummaries...)
		if out.NextToken == nil {
			return resources, nil
		}
		nextToken = out.NextToken
	}
}

// flattenStackNames returns every stack name in a tree (the root and every
// descendant), depth-first.
func flattenStackNames(node *stackNode) []string {
	names := []string{node.StackName}
	for _, child := range node.Children {
		names = append(names, flattenStackNames(child)...)
	}
	return names
}

// renderStackTree writes an indented tree of a stack and its nested stacks.
func renderStackTree(node *stackNode, prefix string) {
	_ = data.Writeln(prefix + node.StackName)
	for i, child := range node.Children {
		branch := "├─ "
		nextPrefix := prefix + "│  "
		if i == len(node.Children)-1 {
			branch = "└─ "
			nextPrefix = prefix + "   "
		}
		_ = data.Writeln(prefix + branch + child.StackName)
		for _, grandchild := range child.Children {
			renderStackTree(grandchild, nextPrefix)
		}
	}
}

// runTree renders the nested-stack dependency tree for a component.
func runTree(ctx context.Context, client CloudFormationClient, stackName string, summary map[string]any) (map[string]any, error) {
	root, err := buildStackTree(ctx, client, stackName, 0)
	if err != nil {
		return summary, err
	}
	summary["tree"] = root
	renderStackTree(root, "")
	return summary, nil
}

// runLogs renders the combined event log for a stack and every nested stack
// beneath it, sorted chronologically. --chart renders a lightweight
// per-resource timeline instead of a flat chronological list.
func runLogs(ctx context.Context, client CloudFormationClient, stackName string, chart bool, summary map[string]any) (map[string]any, error) {
	root, err := buildStackTree(ctx, client, stackName, 0)
	if err != nil {
		return summary, err
	}

	var allEvents []cfntypes.StackEvent
	for _, name := range flattenStackNames(root) {
		events, _, err := pollStackEvents(ctx, client, name, map[string]bool{})
		if err != nil {
			return summary, err
		}
		allEvents = append(allEvents, events...)
	}
	sort.Slice(allEvents, func(i, j int) bool {
		return timeValue(allEvents[i].Timestamp).Before(timeValue(allEvents[j].Timestamp))
	})
	summary["event_count"] = len(allEvents)

	if chart {
		renderEventChart(allEvents)
		return summary, nil
	}
	for i := range allEvents {
		printStackEvent(&allEvents[i])
	}
	return summary, nil
}

// renderEventChart groups events by logical resource ID and prints each
// resource's status transitions on one line — a compact per-resource timeline
// rather than a flat chronological event stream.
func renderEventChart(events []cfntypes.StackEvent) {
	order := []string{}
	byResource := map[string][]string{}
	for i := range events {
		e := &events[i]
		id := stringValue(e.LogicalResourceId)
		if _, seen := byResource[id]; !seen {
			order = append(order, id)
		}
		byResource[id] = append(byResource[id], string(e.ResourceStatus))
	}
	for _, id := range order {
		_ = data.Writeln(fmt.Sprintf("%-30s %s", id, strings.Join(byResource[id], " -> ")))
	}
}

// runWatch attaches to a stack's in-progress (or already-terminal) operation
// and streams events until it reaches a terminal status — the same polling
// loop apply/delete use internally, exposed as its own verb for attaching to
// an operation already in progress, including one started outside Atmos.
func runWatch(ctx context.Context, client CloudFormationClient, stackName string, summary map[string]any) (map[string]any, error) {
	status, err := streamStackEvents(ctx, client, stackName)
	if err != nil {
		return summary, err
	}
	summary["final_status"] = string(status)
	if isFailedStackStatus(status) {
		return summary, fmt.Errorf("%w: stack %s ended in status %s", errUtils.ErrAwsCloudFormationChangeSetFailed, stackName, status)
	}
	return summary, nil
}
