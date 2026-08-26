package cloudformation

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// listDeployedStacks must return the single page's stacks when ListStacks
// reports no NextToken.
func TestListDeployedStacks_SinglePage(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	client.EXPECT().ListStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.ListStacksOutput{
		StackSummaries: []cfntypes.StackSummary{
			{StackName: awsString("vpc"), StackStatus: cfntypes.StackStatusCreateComplete},
		},
	}, nil)

	stacks, err := listDeployedStacks(context.Background(), client, nil)
	require.NoError(t, err)
	require.Len(t, stacks, 1)
	assert.Equal(t, "vpc", *stacks[0].StackName)
}

// listDeployedStacks must page through NextToken, accumulating every page's
// stacks and threading the token to the following ListStacks call.
func TestListDeployedStacks_Pagination(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	token := "page-2-token"
	gomock.InOrder(
		client.EXPECT().ListStacks(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, input *cloudformation.ListStacksInput, _ ...func(*cloudformation.Options)) (*cloudformation.ListStacksOutput, error) {
				assert.Nil(t, input.NextToken, "the first call must not carry a NextToken")
				return &cloudformation.ListStacksOutput{
					StackSummaries: []cfntypes.StackSummary{{StackName: awsString("vpc")}},
					NextToken:      &token,
				}, nil
			},
		),
		client.EXPECT().ListStacks(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, input *cloudformation.ListStacksInput, _ ...func(*cloudformation.Options)) (*cloudformation.ListStacksOutput, error) {
				require.NotNil(t, input.NextToken)
				assert.Equal(t, token, *input.NextToken)
				return &cloudformation.ListStacksOutput{
					StackSummaries: []cfntypes.StackSummary{{StackName: awsString("dns")}},
				}, nil
			},
		),
	)

	stacks, err := listDeployedStacks(context.Background(), client, nil)
	require.NoError(t, err)
	require.Len(t, stacks, 2)
	assert.Equal(t, "vpc", *stacks[0].StackName)
	assert.Equal(t, "dns", *stacks[1].StackName)
}

// listDeployedStacks must wrap a ListStacks API error with
// ErrAwsCloudFormationAPICallFailed.
func TestListDeployedStacks_APIError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	sentinel := errors.New("throttled")
	client.EXPECT().ListStacks(gomock.Any(), gomock.Any()).Return(nil, sentinel)

	_, err := listDeployedStacks(context.Background(), client, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
	assert.ErrorIs(t, err, sentinel)
}

// toStackStatuses must return nil for an empty filter (ListStacks defaults to
// all non-deleted statuses in that case) and convert each string otherwise.
func TestToStackStatuses(t *testing.T) {
	assert.Nil(t, toStackStatuses(nil))
	assert.Nil(t, toStackStatuses([]string{}))

	got := toStackStatuses([]string{"CREATE_COMPLETE", "UPDATE_COMPLETE"})
	assert.Equal(t, []cfntypes.StackStatus{cfntypes.StackStatusCreateComplete, cfntypes.StackStatusUpdateComplete}, got)
}

// annotateManagedStacks must set Managed=true only for stacks whose name is
// present (and true) in configuredStackNames, and Managed=false for every
// other stack — asserted per-entry, not just "returns without error".
func TestAnnotateManagedStacks(t *testing.T) {
	stacks := []cfntypes.StackSummary{
		{StackName: awsString("vpc"), StackStatus: cfntypes.StackStatusCreateComplete},
		{StackName: awsString("orphaned-stack"), StackStatus: cfntypes.StackStatusUpdateComplete},
	}
	configured := map[string]bool{"vpc": true}

	got := annotateManagedStacks(stacks, configured)
	require.Len(t, got, 2)

	assert.Equal(t, DeployedStackSummary{StackName: "vpc", Status: "CREATE_COMPLETE", Managed: true}, got[0])
	assert.Equal(t, DeployedStackSummary{StackName: "orphaned-stack", Status: "UPDATE_COMPLETE", Managed: false}, got[1])
}

// annotateManagedStacks must return an empty (not nil-panicking) slice for no
// stacks.
func TestAnnotateManagedStacks_Empty(t *testing.T) {
	got := annotateManagedStacks(nil, map[string]bool{})
	assert.Empty(t, got)
}

// ListDeployedStacks must propagate a buildAWSConfig failure without ever
// reaching listDeployedStacks/the API.
func TestListDeployedStacks_AuthConfigError(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{
			AWS: &schema.AWSAuthContext{
				CredentialsFile: "/this/path/does/not/exist/credentials",
				ConfigFile:      "/this/path/does/not/exist/config",
				Profile:         "bogus-profile",
			},
		},
	}

	_, err := ListDeployedStacks(context.Background(), info, "us-east-1", nil, map[string]bool{})
	require.Error(t, err, "an unresolvable shared-config profile must surface as an error before any API call")
}

// ListDeployedStacks must propagate the underlying ListStacks failure once
// auth/config resolution succeeds — exercised via an endpoint that refuses
// connections, mirroring executor_test.go's TestRunOperation_DispatchesToHandler.
func TestListDeployedStacks_ClientError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	unreachable := srv.URL
	srv.Close()

	info := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{AWS: &schema.AWSAuthContext{EndpointURL: unreachable}},
	}

	_, err := ListDeployedStacks(context.Background(), info, "us-east-1", nil, map[string]bool{})
	require.Error(t, err, "the dispatched call must have actually hit the (unreachable) endpoint")
}

// RenderDeployedStacksList must print a "no stacks" message for an empty
// list, never a blank table.
func TestRenderDeployedStacksList_Empty(t *testing.T) {
	out := captureStdout(t, func() {
		RenderDeployedStacksList(nil)
	})
	assert.Contains(t, out, "No stacks found.")
}

// RenderDeployedStacksList must print one line per stack, distinguishing
// managed from unmanaged stacks in the marker column.
func TestRenderDeployedStacksList_Populated(t *testing.T) {
	stacks := []DeployedStackSummary{
		{StackName: "vpc", Status: "CREATE_COMPLETE", Managed: true},
		{StackName: "orphaned-stack", Status: "UPDATE_COMPLETE", Managed: false},
	}

	out := captureStdout(t, func() {
		RenderDeployedStacksList(stacks)
	})
	// "managed" is a substring of "unmanaged", so assert full rendered lines
	// (not bare substrings) to actually distinguish the two rows, rather than
	// a check that would still pass if vpc's row were also marked unmanaged.
	assert.Contains(t, out, fmt.Sprintf("%-9s %-30s %s", "managed", "CREATE_COMPLETE", "vpc"))
	assert.Contains(t, out, fmt.Sprintf("%-9s %-30s %s", "unmanaged", "UPDATE_COMPLETE", "orphaned-stack"))
}
