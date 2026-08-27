package cloudformation

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

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
	renderStackTreeChildren(node.Children, prefix)
}

// renderStackTreeChildren renders every descendant of a node with the
// appropriate ├─/└─ branch glyph and continuation prefix at every depth,
// recursing into each child's own children the same way — not just one level
// deep, which would leave grandchildren+ printed as bare lines with no
// branch glyph.
func renderStackTreeChildren(children []*stackNode, prefix string) {
	for i, child := range children {
		branch := "├─ "
		nextPrefix := prefix + "│  "
		if i == len(children)-1 {
			branch = "└─ "
			nextPrefix = prefix + "   "
		}
		_ = data.Writeln(prefix + branch + child.StackName)
		renderStackTreeChildren(child.Children, nextPrefix)
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

// logsOptions carries the logs-specific flags (--chart, --follow) through to
// runLogs. --chart and --follow are mutually exclusive, rejected at the flag
// layer (validateLogsFollowChart).
type logsOptions struct {
	Chart  bool
	Follow bool
}

// runLogs renders the combined event log for a stack and every nested stack
// beneath it, sorted chronologically. Chart renders a lightweight per-resource
// timeline instead of a flat chronological list. Follow continuously tails new
// events instead of returning after one pass.
func runLogs(ctx context.Context, client CloudFormationClient, stackName string, opts logsOptions, summary map[string]any) (map[string]any, error) {
	root, err := buildStackTree(ctx, client, stackName, 0)
	if err != nil {
		return summary, err
	}
	names := flattenStackNames(root)

	if opts.Follow {
		return followLogs(ctx, client, names, summary)
	}

	var allEvents []cfntypes.StackEvent
	for _, name := range names {
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

	if opts.Chart {
		renderEventChart(allEvents)
		return summary, nil
	}
	for i := range allEvents {
		writeLogLine(&allEvents[i])
	}
	return summary, nil
}

// writeLogLine renders one stack event on the data channel (stdout) — unlike
// printStackEvent (watch's UI/stderr channel), logs is a pipeable data command
// per docs/io-and-ui-output.md, so even FAILED events stay on stdout rather than
// splitting across channels.
func writeLogLine(event *cfntypes.StackEvent) {
	line, _ := formatStackEventLine(event)
	_ = data.Writeln(line)
}

// followLogs tails new events across every stack in names (a nested-stack tree
// flattened once, per runLogs — a child stack created after that initial walk
// will not be picked up) until ctx is canceled. Unlike watch, it does not stop
// at a terminal stack status: --follow is tail -f style, ended by the caller
// (Ctrl+C), matching this repo's existing --follow convention (cmd/container,
// cmd/devcontainer, cmd/composition).
func followLogs(ctx context.Context, client CloudFormationClient, names []string, summary map[string]any) (map[string]any, error) {
	seen := make(map[string]map[string]bool, len(names))
	for _, name := range names {
		seen[name] = make(map[string]bool)
	}

	eventCount := 0
	for {
		var batch []cfntypes.StackEvent
		for _, name := range names {
			events, _, err := pollStackEvents(ctx, client, name, seen[name])
			if err != nil {
				return summary, err
			}
			batch = append(batch, events...)
		}
		sort.Slice(batch, func(i, j int) bool {
			return timeValue(batch[i].Timestamp).Before(timeValue(batch[j].Timestamp))
		})
		for i := range batch {
			writeLogLine(&batch[i])
			eventCount++
		}
		summary["event_count"] = eventCount

		select {
		case <-ctx.Done():
			return summary, nil
		case <-time.After(eventPollInterval):
		}
	}
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
		return summary, fmt.Errorf("%w: stack %s ended in status %s", errUtils.ErrAwsCloudFormationOperationFailed, stackName, status)
	}
	return summary, nil
}
