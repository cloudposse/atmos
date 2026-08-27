package cloudformation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

// buildStackTree must return a node with no children for a flat stack (no
// nested stacks among its resources).
func TestBuildStackTree_FlatStack(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().ListStackResources(gomock.Any(), gomock.Any()).Return(&cloudformation.ListStackResourcesOutput{
		StackResourceSummaries: []cfntypes.StackResourceSummary{
			{ResourceType: awsString("AWS::S3::Bucket"), PhysicalResourceId: awsString("my-bucket")},
		},
	}, nil)

	node, err := buildStackTree(context.Background(), client, "root", 0)
	require.NoError(t, err)
	assert.Equal(t, "root", node.StackName)
	assert.Empty(t, node.Children)
}

// buildStackTree must recurse one level into a single nested stack.
func TestBuildStackTree_OneLevelNesting(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	gomock.InOrder(
		client.EXPECT().ListStackResources(gomock.Any(), &cloudformation.ListStackResourcesInput{StackName: awsString("root")}).Return(&cloudformation.ListStackResourcesOutput{
			StackResourceSummaries: []cfntypes.StackResourceSummary{
				{ResourceType: awsString(nestedStackResourceType), PhysicalResourceId: awsString("child-1")},
			},
		}, nil),
		client.EXPECT().ListStackResources(gomock.Any(), &cloudformation.ListStackResourcesInput{StackName: awsString("child-1")}).Return(&cloudformation.ListStackResourcesOutput{}, nil),
	)

	node, err := buildStackTree(context.Background(), client, "root", 0)
	require.NoError(t, err)
	require.Len(t, node.Children, 1)
	assert.Equal(t, "child-1", node.Children[0].StackName)
	assert.Empty(t, node.Children[0].Children)
}

// buildStackTree must recurse multiple levels deep (2-3 levels).
func TestBuildStackTree_MultipleLevelsNesting(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	gomock.InOrder(
		client.EXPECT().ListStackResources(gomock.Any(), &cloudformation.ListStackResourcesInput{StackName: awsString("root")}).Return(&cloudformation.ListStackResourcesOutput{
			StackResourceSummaries: []cfntypes.StackResourceSummary{
				{ResourceType: awsString(nestedStackResourceType), PhysicalResourceId: awsString("child-1")},
			},
		}, nil),
		client.EXPECT().ListStackResources(gomock.Any(), &cloudformation.ListStackResourcesInput{StackName: awsString("child-1")}).Return(&cloudformation.ListStackResourcesOutput{
			StackResourceSummaries: []cfntypes.StackResourceSummary{
				{ResourceType: awsString(nestedStackResourceType), PhysicalResourceId: awsString("grandchild-1")},
			},
		}, nil),
		client.EXPECT().ListStackResources(gomock.Any(), &cloudformation.ListStackResourcesInput{StackName: awsString("grandchild-1")}).Return(&cloudformation.ListStackResourcesOutput{}, nil),
	)

	node, err := buildStackTree(context.Background(), client, "root", 0)
	require.NoError(t, err)
	require.Len(t, node.Children, 1)
	require.Len(t, node.Children[0].Children, 1)
	assert.Equal(t, "grandchild-1", node.Children[0].Children[0].StackName)
}

// buildStackTree must skip (not recurse into) a nested-stack resource whose
// PhysicalResourceId is still empty (still-creating child).
func TestBuildStackTree_StillCreatingChildSkipped(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	// Only one ListStackResources call expected — for "root". A second call for
	// the empty-PhysicalResourceId child would fail via gomock's unexpected-call
	// panic, proving the skip.
	client.EXPECT().ListStackResources(gomock.Any(), gomock.Any()).Return(&cloudformation.ListStackResourcesOutput{
		StackResourceSummaries: []cfntypes.StackResourceSummary{
			{ResourceType: awsString(nestedStackResourceType), PhysicalResourceId: awsString("")},
		},
	}, nil)

	node, err := buildStackTree(context.Background(), client, "root", 0)
	require.NoError(t, err)
	assert.Empty(t, node.Children)
}

// buildStackTree must wrap a ListStackResources API error.
func TestBuildStackTree_APIError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().ListStackResources(gomock.Any(), gomock.Any()).Return(nil, errors.New("throttled"))

	_, err := buildStackTree(context.Background(), client, "root", 0)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
}

// buildStackTree must propagate an error from a recursive call (a nested
// stack's own ListStackResources failing), not just from the root call.
func TestBuildStackTree_RecursiveCallError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	gomock.InOrder(
		client.EXPECT().ListStackResources(gomock.Any(), &cloudformation.ListStackResourcesInput{StackName: awsString("root")}).Return(&cloudformation.ListStackResourcesOutput{
			StackResourceSummaries: []cfntypes.StackResourceSummary{
				{ResourceType: awsString(nestedStackResourceType), PhysicalResourceId: awsString("child-1")},
			},
		}, nil),
		client.EXPECT().ListStackResources(gomock.Any(), &cloudformation.ListStackResourcesInput{StackName: awsString("child-1")}).Return(nil, errors.New("throttled")),
	)

	_, err := buildStackTree(context.Background(), client, "root", 0)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
}

// buildStackTree must stop recursing once maxNestedStackDepth is reached,
// rather than looping forever against a mock that would otherwise recurse
// indefinitely (each "stack" nests itself under a new name).
func TestBuildStackTree_MaxDepthGuard(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	// Every stack (however deep) reports one nested-stack resource whose
	// PhysicalResourceId is just its own stack name suffixed with "-child" —
	// an unbounded chain without the depth guard.
	client.EXPECT().ListStackResources(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, input *cloudformation.ListStackResourcesInput, _ ...func(*cloudformation.Options)) (*cloudformation.ListStackResourcesOutput, error) {
			childName := stringValue(input.StackName) + "-child"
			return &cloudformation.ListStackResourcesOutput{
				StackResourceSummaries: []cfntypes.StackResourceSummary{
					{ResourceType: awsString(nestedStackResourceType), PhysicalResourceId: awsString(childName)},
				},
			}, nil
		},
	).AnyTimes()

	done := make(chan *stackNode, 1)
	errCh := make(chan error, 1)
	go func() {
		node, err := buildStackTree(context.Background(), client, "root", 0)
		if err != nil {
			errCh <- err
			return
		}
		done <- node
	}()

	select {
	case node := <-done:
		// Depth-count the chain: root has a child at every depth up to the guard.
		depth := 0
		cur := node
		for len(cur.Children) > 0 {
			depth++
			cur = cur.Children[0]
		}
		assert.LessOrEqual(t, depth, maxNestedStackDepth, "recursion must stop at maxNestedStackDepth")
	case err := <-errCh:
		t.Fatalf("buildStackTree returned an error instead of stopping at the depth guard: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("buildStackTree did not return — the maxNestedStackDepth guard failed to stop recursion")
	}
}

// listAllStackResources must return a single page as-is, and must paginate
// through NextToken to accumulate every page's resources in order. A single
// table-driven test (rather than two flat, near-identical mock-setup
// functions) covers both shapes of the same underlying pagination contract.
func TestListAllStackResources_Pagination(t *testing.T) {
	pageOfIDs := func(ids ...string) []cfntypes.StackResourceSummary {
		summaries := make([]cfntypes.StackResourceSummary, 0, len(ids))
		for _, id := range ids {
			summaries = append(summaries, cfntypes.StackResourceSummary{LogicalResourceId: awsString(id)})
		}
		return summaries
	}

	tests := map[string][]struct {
		ids       []string
		hasMore   bool
		wantAfter []string
	}{
		"single page": {
			{ids: []string{"R1"}, hasMore: false, wantAfter: []string{"R1"}},
		},
		"paginates across pages": {
			{ids: []string{"R1"}, hasMore: true, wantAfter: []string{"R1"}},
			{ids: []string{"R2"}, hasMore: false, wantAfter: []string{"R1", "R2"}},
		},
	}

	for name, pages := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := NewMockCloudFormationClient(ctrl)

			var calls []any
			for _, page := range pages {
				out := &cloudformation.ListStackResourcesOutput{StackResourceSummaries: pageOfIDs(page.ids...)}
				if page.hasMore {
					token := "next"
					out.NextToken = &token
				}
				calls = append(calls, client.EXPECT().ListStackResources(gomock.Any(), gomock.Any()).Return(out, nil))
			}
			gomock.InOrder(calls...)

			resources, err := listAllStackResources(context.Background(), client, "root")
			require.NoError(t, err)

			wantFinal := pages[len(pages)-1].wantAfter
			require.Len(t, resources, len(wantFinal))
			for i, want := range wantFinal {
				assert.Equal(t, want, stringValue(resources[i].LogicalResourceId))
			}
		})
	}
}

// listAllStackResources must wrap an API error.
func TestListAllStackResources_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().ListStackResources(gomock.Any(), gomock.Any()).Return(nil, errors.New("throttled"))

	_, err := listAllStackResources(context.Background(), client, "root")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
}

// flattenStackNames must return just the root's name for a single node.
func TestFlattenStackNames_SingleNode(t *testing.T) {
	node := &stackNode{StackName: "root"}
	assert.Equal(t, []string{"root"}, flattenStackNames(node))
}

// flattenStackNames must return names depth-first for a multi-level tree.
func TestFlattenStackNames_MultiLevel(t *testing.T) {
	tree := &stackNode{
		StackName: "root",
		Children: []*stackNode{
			{
				StackName: "child-a",
				Children: []*stackNode{
					{StackName: "grandchild-a1"},
				},
			},
			{StackName: "child-b"},
		},
	}

	assert.Equal(t, []string{"root", "child-a", "grandchild-a1", "child-b"}, flattenStackNames(tree))
}

// renderStackTree must render a 2+-level tree with the expected
// indentation/branch characters, not merely avoid panicking.
func TestRenderStackTree_MultiLevel(t *testing.T) {
	tree := &stackNode{
		StackName: "root",
		Children: []*stackNode{
			{
				StackName: "child-a",
				Children: []*stackNode{
					{StackName: "grandchild-a1"},
				},
			},
			{StackName: "child-b"},
		},
	}

	out := captureStdout(t, func() { renderStackTree(tree, "") })
	assert.Contains(t, out, "root")
	assert.Contains(t, out, "├─ child-a")
	assert.Contains(t, out, "└─ child-b")
	assert.Contains(t, out, "grandchild-a1")
}

// runTree's happy path: populates summary["tree"] and propagates
// buildStackTree's error otherwise.
func TestRunTree_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().ListStackResources(gomock.Any(), gomock.Any()).Return(&cloudformation.ListStackResourcesOutput{}, nil)

	out := captureStdout(t, func() {
		summary, err := runTree(context.Background(), client, "root", map[string]any{})
		require.NoError(t, err)
		tree, ok := summary["tree"].(*stackNode)
		require.True(t, ok)
		assert.Equal(t, "root", tree.StackName)
	})
	assert.Contains(t, out, "root")
}

// runTree must propagate a buildStackTree failure.
func TestRunTree_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().ListStackResources(gomock.Any(), gomock.Any()).Return(nil, errors.New("throttled"))

	_, err := runTree(context.Background(), client, "root", map[string]any{})
	require.Error(t, err)
}

// runLogs must combine events from a root stack and its nested stacks,
// sorted chronologically even when the two stacks' events arrive out of order.
func TestRunLogs_MergesAndSortsAcrossStacks(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	rootLater := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	childEarlier := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	gomock.InOrder(
		client.EXPECT().ListStackResources(gomock.Any(), &cloudformation.ListStackResourcesInput{StackName: awsString("root")}).Return(&cloudformation.ListStackResourcesOutput{
			StackResourceSummaries: []cfntypes.StackResourceSummary{
				{ResourceType: awsString(nestedStackResourceType), PhysicalResourceId: awsString("child")},
			},
		}, nil),
		client.EXPECT().ListStackResources(gomock.Any(), &cloudformation.ListStackResourcesInput{StackName: awsString("child")}).Return(&cloudformation.ListStackResourcesOutput{}, nil),
	)

	client.EXPECT().DescribeStackEvents(gomock.Any(), &cloudformation.DescribeStackEventsInput{StackName: awsString("root")}).Return(&cloudformation.DescribeStackEventsOutput{
		StackEvents: []cfntypes.StackEvent{
			{EventId: awsString("root-1"), LogicalResourceId: awsString("RootResource"), Timestamp: &rootLater},
		},
	}, nil)
	client.EXPECT().DescribeStacks(gomock.Any(), &cloudformation.DescribeStacksInput{StackName: awsString("root")}).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusCreateComplete}},
	}, nil)
	client.EXPECT().DescribeStackEvents(gomock.Any(), &cloudformation.DescribeStackEventsInput{StackName: awsString("child")}).Return(&cloudformation.DescribeStackEventsOutput{
		StackEvents: []cfntypes.StackEvent{
			{EventId: awsString("child-1"), LogicalResourceId: awsString("ChildResource"), Timestamp: &childEarlier},
		},
	}, nil)
	client.EXPECT().DescribeStacks(gomock.Any(), &cloudformation.DescribeStacksInput{StackName: awsString("child")}).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusCreateComplete}},
	}, nil)

	out := captureStderr(t, func() {
		summary, err := runLogs(context.Background(), client, "root", false, map[string]any{})
		require.NoError(t, err)
		assert.Equal(t, 2, summary["event_count"])
	})

	// The child event (earlier timestamp) must be printed before the root event.
	childIdx := indexOf(t, out, "ChildResource")
	rootIdx := indexOf(t, out, "RootResource")
	assert.Less(t, childIdx, rootIdx, "events must be merged in chronological order")
}

// indexOf is a small test helper returning the index of substr in s, failing
// the test if it's not found.
func indexOf(t *testing.T, s, substr string) int {
	t.Helper()
	idx := -1
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			idx = i
			break
		}
	}
	require.GreaterOrEqual(t, idx, 0, "expected %q to contain %q", s, substr)
	return idx
}

// runLogs with --chart must render the grouped-by-resource output
// (renderEventChart) instead of the flat chronological list.
func TestRunLogs_ChartMode(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().ListStackResources(gomock.Any(), gomock.Any()).Return(&cloudformation.ListStackResourcesOutput{}, nil)

	earlier := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	later := time.Date(2026, 1, 1, 11, 0, 0, 0, time.UTC)
	client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{
		StackEvents: []cfntypes.StackEvent{
			{EventId: awsString("e1"), LogicalResourceId: awsString("MyBucket"), ResourceStatus: cfntypes.ResourceStatusCreateInProgress, Timestamp: &earlier},
			{EventId: awsString("e2"), LogicalResourceId: awsString("MyBucket"), ResourceStatus: cfntypes.ResourceStatusCreateComplete, Timestamp: &later},
		},
	}, nil)
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusCreateComplete}},
	}, nil)

	out := captureStdout(t, func() {
		summary, err := runLogs(context.Background(), client, "root", true, map[string]any{})
		require.NoError(t, err)
		assert.Equal(t, 2, summary["event_count"])
	})
	assert.Contains(t, out, "MyBucket")
	assert.Contains(t, out, "CREATE_IN_PROGRESS -> CREATE_COMPLETE")
}

// runLogs must propagate a buildStackTree failure.
func TestRunLogs_BuildTreeError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().ListStackResources(gomock.Any(), gomock.Any()).Return(nil, errors.New("throttled"))

	_, err := runLogs(context.Background(), client, "root", false, map[string]any{})
	require.Error(t, err)
}

// runLogs must propagate a pollStackEvents failure.
func TestRunLogs_PollStackEventsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().ListStackResources(gomock.Any(), gomock.Any()).Return(&cloudformation.ListStackResourcesOutput{}, nil)
	client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(nil, errors.New("access denied"))

	_, err := runLogs(context.Background(), client, "root", false, map[string]any{})
	require.Error(t, err)
}

// renderEventChart must group events by resource, joining each resource's
// status transitions with " -> ", and preserve first-seen order across
// resources.
func TestRenderEventChart_GroupsAndPreservesOrder(t *testing.T) {
	events := []cfntypes.StackEvent{
		{LogicalResourceId: awsString("BucketA"), ResourceStatus: cfntypes.ResourceStatusCreateInProgress},
		{LogicalResourceId: awsString("RoleB"), ResourceStatus: cfntypes.ResourceStatusCreateInProgress},
		{LogicalResourceId: awsString("BucketA"), ResourceStatus: cfntypes.ResourceStatusCreateComplete},
		{LogicalResourceId: awsString("RoleB"), ResourceStatus: cfntypes.ResourceStatusCreateComplete},
	}

	out := captureStdout(t, func() { renderEventChart(events) })

	bucketIdx := indexOf(t, out, "BucketA")
	roleIdx := indexOf(t, out, "RoleB")
	assert.Less(t, bucketIdx, roleIdx, "resources must be rendered in first-seen order")
	assert.Contains(t, out, "CREATE_IN_PROGRESS -> CREATE_COMPLETE")
}

// runWatch's happy path: a non-failed terminal status returns no error.
func TestRunWatch_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{}, nil)
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusUpdateComplete}},
	}, nil)

	summary, err := runWatch(context.Background(), client, "root", map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, string(cfntypes.StackStatusUpdateComplete), summary["final_status"])
}

// runWatch must return a wrapped error when the stack ends in a failed
// terminal status.
func TestRunWatch_FailedStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{}, nil)
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusUpdateRollbackComplete}},
	}, nil)

	_, err := runWatch(context.Background(), client, "root", map[string]any{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationChangeSetFailed)
}

// runWatch must propagate a streamStackEvents failure.
func TestRunWatch_StreamError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(nil, errors.New("access denied"))

	_, err := runWatch(context.Background(), client, "root", map[string]any{})
	require.Error(t, err)
}
