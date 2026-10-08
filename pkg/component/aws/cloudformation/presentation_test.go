package cloudformation

import (
	"strings"
	"testing"

	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ui/spinner"
)

func TestChangePreviewColumnAlignment(t *testing.T) {
	t.Setenv("COLUMNS", "120")
	result := &changeSetResult{Changes: []cfntypes.Change{
		{ResourceChange: &cfntypes.ResourceChange{Action: cfntypes.ChangeActionAdd, LogicalResourceId: awsString("Bucket"), ResourceType: awsString("AWS::S3::Bucket")}},
		{ResourceChange: &cfntypes.ResourceChange{Action: cfntypes.ChangeActionModify, LogicalResourceId: awsString("Function"), ResourceType: awsString("AWS::Lambda::Function"), Replacement: cfntypes.ReplacementConditional}},
	}}
	text := ansi.Strip(diffSummaryText("app", result))
	lines := strings.Split(text, "\n")
	var header, add, modify string
	for _, line := range lines {
		switch {
		case strings.Contains(line, "Action"):
			header = line
		case strings.Contains(line, "Bucket"):
			add = line
		case strings.Contains(line, "Function"):
			modify = line
		}
	}
	require.NotEmpty(t, header)
	require.NotEmpty(t, add)
	require.NotEmpty(t, modify)
	assert.Equal(t, strings.Index(header, "Action"), strings.Index(add, "Add"))
	assert.Equal(t, strings.Index(header, "Resource"), strings.Index(add, "Bucket"))
	assert.Equal(t, strings.Index(header, "Type"), strings.Index(modify, "AWS::Lambda::Function"))
	assert.Contains(t, text, "Conditional")
}

func TestOutputTableNamesItsFirstColumn(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, renderOutputsSummary(map[string]any{"BucketName": "example-bucket"}, nil))
	})
	assert.Contains(t, ansi.Strip(out), "Output")
	assert.NotContains(t, ansi.Strip(out), "Key")
}

// Root completion must appear once, whether it arrives with other events or
// is synthesized after polling. A nested stack must not suppress the root.
func TestLiveCompletionIsReportedOnce(t *testing.T) {
	for _, tc := range []struct {
		name   string
		events []cfntypes.StackEvent
		final  cfntypes.StackStatus
		want   string
	}{
		{"root event", []cfntypes.StackEvent{stackLevelEvent("done", cfntypes.ResourceStatusCreateComplete)}, cfntypes.StackStatusCreateComplete, "Created"},
		{"missing event", nil, cfntypes.StackStatusCreateComplete, "Created"},
		{"failure", []cfntypes.StackEvent{stackLevelEvent("failed", cfntypes.ResourceStatusCreateFailed)}, cfntypes.StackStatusCreateFailed, "CREATE_FAILED"},
		{"later rollback", []cfntypes.StackEvent{stackLevelEvent("failed", cfntypes.ResourceStatusCreateFailed)}, cfntypes.StackStatusRollbackComplete, "ROLLBACK_COMPLETE"},
		{"nested stack", []cfntypes.StackEvent{{LogicalResourceId: awsString("Nested"), ResourceType: awsString(stackResourceType), ResourceStatus: cfntypes.ResourceStatusCreateComplete}}, cfntypes.StackStatusCreateComplete, "Created"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sp := spinner.New("watching")
			out := ansi.Strip(captureStderr(t, func() {
				reported := dispatchStackEvents(sp, tc.events, "vpc", "")
				finishStreamSpinner(sp, "vpc", tc.final, reported)
			}))
			count := 0
			for _, line := range strings.Split(out, "\n") {
				if strings.Contains(line, tc.want) && strings.Contains(line, "vpc") {
					count++
				}
			}
			assert.Equal(t, 1, count, out)
			assert.NotContains(t, out, "AWS::CloudFormation::Stack")
		})
	}
}

func TestLiveEventVocabulary(t *testing.T) {
	for _, tc := range []struct {
		status cfntypes.ResourceStatus
		verb   string
	}{
		{cfntypes.ResourceStatusCreateInProgress, "Creating"},
		{cfntypes.ResourceStatusCreateComplete, "Created"},
		{cfntypes.ResourceStatusUpdateInProgress, "Updating"},
		{cfntypes.ResourceStatusUpdateComplete, "Updated"},
		{cfntypes.ResourceStatusDeleteInProgress, "Deleting"},
		{cfntypes.ResourceStatusDeleteComplete, "Deleted"},
		{cfntypes.ResourceStatusDeleteFailed, "DELETE_FAILED"},
	} {
		t.Run(string(tc.status), func(t *testing.T) {
			event := &cfntypes.StackEvent{LogicalResourceId: awsString("Bucket"), ResourceStatus: tc.status, ResourceStatusReason: awsString("provider detail")}
			got := formatLiveStackEvent(event)
			assert.True(t, strings.HasPrefix(got, tc.verb), got)
			assert.Contains(t, got, "Bucket — provider detail")
		})
	}
}
