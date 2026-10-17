package cloudformation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/providers/generic"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestDriftDescribeRendersActualResourceStatusInCISummary follows paginated API results through the
// real CI writer, distinguishing clean, unchecked, missing, and drifted results.
func TestDriftDescribeRendersActualResourceStatusInCISummary(t *testing.T) {
	tests := []struct {
		name       string
		statuses   []cfntypes.StackResourceDriftStatus
		wantStatus string
		wantCount  int32
	}{
		{name: "no results"},
		{name: "clean", statuses: []cfntypes.StackResourceDriftStatus{cfntypes.StackResourceDriftStatusInSync}, wantStatus: "IN_SYNC"},
		{name: "not checked", statuses: []cfntypes.StackResourceDriftStatus{cfntypes.StackResourceDriftStatusNotChecked}, wantStatus: "NOT_CHECKED"},
		{name: "partially checked", statuses: []cfntypes.StackResourceDriftStatus{cfntypes.StackResourceDriftStatusInSync, cfntypes.StackResourceDriftStatusNotChecked}, wantStatus: "NOT_CHECKED"},
		{name: "partially checked reverse order", statuses: []cfntypes.StackResourceDriftStatus{cfntypes.StackResourceDriftStatusNotChecked, cfntypes.StackResourceDriftStatusInSync}, wantStatus: "NOT_CHECKED"},
		{name: "modified", statuses: []cfntypes.StackResourceDriftStatus{cfntypes.StackResourceDriftStatusModified}, wantStatus: "DRIFTED", wantCount: 1},
		{name: "deleted", statuses: []cfntypes.StackResourceDriftStatus{cfntypes.StackResourceDriftStatusDeleted}, wantStatus: "DRIFTED", wantCount: 1},
		{name: "mixed pages", statuses: []cfntypes.StackResourceDriftStatus{cfntypes.StackResourceDriftStatusNotChecked, cfntypes.StackResourceDriftStatusModified, cfntypes.StackResourceDriftStatusInSync, cfntypes.StackResourceDriftStatusDeleted, cfntypes.StackResourceDriftStatusNotChecked}, wantStatus: "DRIFTED", wantCount: 2},
		{name: "unknown status", statuses: []cfntypes.StackResourceDriftStatus{"FUTURE_STATUS"}, wantStatus: "NOT_CHECKED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewMockCloudFormationClient(gomock.NewController(t))
			expectDriftResourcePages(t, client, tt.statuses)
			summary, err := runDriftDescribe(context.Background(), client, "app", map[string]any{})
			require.NoError(t, err)
			result := &schema.CloudFormationCIResult{}
			populateCloudFormationCIResultFromSummary(result, summary)
			assert.Equal(t, tt.wantStatus, result.DriftStatus)
			assert.EqualValues(t, tt.wantCount, result.DriftedCount)
			rendered := renderDriftDescribeCISummary(t, summary)
			assert.Contains(t, rendered, "atmos aws cloudformation drift describe app -s dev")
			if tt.wantStatus == "" {
				assert.NotContains(t, rendered, "Drift status:")
				assert.NotContains(t, rendered, "Drifted resources:")
			} else {
				assert.Contains(t, rendered, "Drift status: **"+tt.wantStatus+"**")
				assert.Contains(t, rendered, fmt.Sprintf("Drifted resources: **%d**", tt.wantCount))
			}
		})
	}
}

// Return one resource per page so aggregation must include every page and
// remain independent of resource order. An empty result still makes one call.
func expectDriftResourcePages(t *testing.T, client *MockCloudFormationClient, statuses []cfntypes.StackResourceDriftStatus) {
	t.Helper()
	if len(statuses) == 0 {
		client.EXPECT().DescribeStackResourceDrifts(gomock.Any(), &cloudformation.DescribeStackResourceDriftsInput{StackName: awsString("app")}).Return(&cloudformation.DescribeStackResourceDriftsOutput{}, nil)
		return
	}
	var token *string
	var calls []*gomock.Call
	for i, status := range statuses {
		out := &cloudformation.DescribeStackResourceDriftsOutput{StackResourceDrifts: []cfntypes.StackResourceDrift{{StackResourceDriftStatus: status}}}
		if i+1 < len(statuses) {
			out.NextToken = awsString(fmt.Sprintf("page-%d", i+1))
		}
		calls = append(calls, client.EXPECT().DescribeStackResourceDrifts(gomock.Any(), &cloudformation.DescribeStackResourceDriftsInput{StackName: awsString("app"), NextToken: token}).Return(out, nil))
		token = out.NextToken
	}
	for i := 1; i < len(calls); i++ {
		calls[i].After(calls[i-1])
	}
}

// Use the real CI hook dispatcher, CloudFormation plugin, embedded template,
// and generic summary writer rather than rendering a hand-built context.
func renderDriftDescribeCISummary(t *testing.T, summary map[string]any) string {
	t.Helper()
	t.Cleanup(ci.SwapRegistryForTest())
	path := filepath.Join(t.TempDir(), "summary.md")
	t.Setenv("ATMOS_CI_SUMMARY", path)
	ci.Register(generic.NewProvider())
	config := &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true}}
	config.Settings.Experimental = "silence"
	runCIHook(ciHookParams{
		event:       hooks.AfterAwsCloudFormationDriftDescribe,
		flags:       map[string]any{"ci": true},
		atmosConfig: config,
		info:        &schema.ConfigAndStacksInfo{ComponentFromArg: "app", Stack: "dev", SubCommand: "drift-describe"},
		summary:     summary,
	})
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(body)
}
