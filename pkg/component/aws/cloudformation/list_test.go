package cloudformation

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
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

// toStackStatuses must default an empty filter to every known status except DELETE_COMPLETE
// (AWS retains DELETE_COMPLETE summaries for 90 days, which would otherwise surface deleted
// stacks as live deployed ones), and must convert each string verbatim, unfiltered, when an
// explicit filter is given — even if the caller explicitly asks for DELETE_COMPLETE.
func TestToStackStatuses(t *testing.T) {
	for _, empty := range [][]string{nil, {}} {
		got := toStackStatuses(empty)
		assert.NotEmpty(t, got, "an empty filter must default to a non-empty status allowlist")
		assert.NotContains(t, got, cfntypes.StackStatusDeleteComplete, "the default allowlist must exclude DELETE_COMPLETE")
		assert.Contains(t, got, cfntypes.StackStatusCreateComplete)
		assert.Contains(t, got, cfntypes.StackStatusUpdateComplete)
	}

	got := toStackStatuses([]string{"CREATE_COMPLETE", "UPDATE_COMPLETE"})
	assert.Equal(t, []cfntypes.StackStatus{cfntypes.StackStatusCreateComplete, cfntypes.StackStatusUpdateComplete}, got)

	// An explicit request for DELETE_COMPLETE must pass through untouched — only the
	// no-filter-given default excludes it.
	explicit := toStackStatuses([]string{"DELETE_COMPLETE"})
	assert.Equal(t, []cfntypes.StackStatus{cfntypes.StackStatusDeleteComplete}, explicit)
}

// defaultDeployedStackStatuses must track the SDK's own StackStatus.Values() (minus
// DELETE_COMPLETE), not a hand-maintained literal count, so a future SDK-added status is
// included automatically instead of silently missing from the default allowlist.
func TestDefaultDeployedStackStatuses_TracksSDKValues(t *testing.T) {
	got := defaultDeployedStackStatuses()
	want := cfntypes.StackStatus("").Values()

	assert.Len(t, got, len(want)-1, "must be every known status except DELETE_COMPLETE")
	for _, s := range got {
		assert.NotEqual(t, cfntypes.StackStatusDeleteComplete, s)
	}
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
// auth/config resolution succeeds. Static test credentials are configured so
// the SDK never falls back to (slow, ultimately-failing) IMDS credential
// resolution — without them, the call fails during credential resolution and
// never actually reaches the test server, so the test would not exercise the
// ListStacks failure path it claims to.
func TestListDeployedStacks_ClientError(t *testing.T) {
	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	credsFile := filepath.Join(tmpDir, "credentials")
	configFile := filepath.Join(tmpDir, "config")
	require.NoError(t, os.WriteFile(credsFile, []byte("[test]\naws_access_key_id = AKIAIOSFODNN7EXAMPLE\naws_secret_access_key = wJalrXUtnFEMI\n"), 0o600))
	require.NoError(t, os.WriteFile(configFile, []byte("[profile test]\nregion = us-east-1\n"), 0o600))

	info := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{AWS: &schema.AWSAuthContext{
			CredentialsFile: credsFile,
			ConfigFile:      configFile,
			Profile:         "test",
			EndpointURL:     srv.URL,
		}},
	}

	_, err := ListDeployedStacks(context.Background(), info, "us-east-1", nil, map[string]bool{})
	require.Error(t, err)
	assert.GreaterOrEqual(t, atomic.LoadInt32(&requests), int32(1), "the dispatched call must have actually hit the test endpoint")
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
