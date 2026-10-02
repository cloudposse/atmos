package backend

import (
	"io"
	"os"
	"strings"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	pkgcfn "github.com/cloudposse/atmos/pkg/component/aws/cloudformation"
)

// A missing component must fail up front naming this command group, never the terraform one.
func TestRequireComponentAndStack(t *testing.T) {
	tests := []struct {
		name      string
		verb      string
		component string
		stack     string
		wantErr   error
		wantHint  string
	}{
		{name: "both present", verb: "list", component: "vpc", stack: "dev"},
		{
			name: "missing component", verb: "list", stack: "dl-dev", wantErr: errUtils.ErrComponentRequired,
			wantHint: "atmos aws cloudformation backend list <component> -s <stack>",
		},
		{
			name: "missing component on create", verb: "create", stack: "dev", wantErr: errUtils.ErrComponentRequired,
			wantHint: "atmos aws cloudformation backend create <component> -s <stack>",
		},
		{name: "missing stack", verb: "list", component: "vpc", wantErr: errUtils.ErrRequiredFlagNotProvided, wantHint: "--stack"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := requireComponentAndStack(tt.verb, tt.component, tt.stack)
			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.wantErr)
			assert.Contains(t, strings.Join(cockroachErrors.GetAllHints(err), "\n"), tt.wantHint)
			assert.NotContains(t, err.Error(), "terraform")
			assert.NotContains(t, strings.Join(cockroachErrors.GetAllHints(err), "\n"), "terraform")
		})
	}
}

// Every verb rejects a missing component before any config or auth work.
func TestBackendVerbs_RequireComponent(t *testing.T) {
	setupTestWithMocks(t) // Strict mocks fail the test on any config initialization.
	ctx := t.Context()

	require.ErrorIs(t, executeCreateOrUpdate(ctx, createOrUpdateArgs{Verb: verbCreate, Stack: "dev"}), errUtils.ErrComponentRequired)
	require.ErrorIs(t, executeCreateOrUpdate(ctx, createOrUpdateArgs{Verb: verbUpdate, Stack: "dev", DryRun: true}), errUtils.ErrComponentRequired)
	require.ErrorIs(t, executeDelete(ctx, deleteRequest{Stack: "dev"}), errUtils.ErrComponentRequired)
	require.ErrorIs(t, executeDescribe(ctx, &describeRequest{Stack: "dev", Format: "table"}), errUtils.ErrComponentRequired)
	require.ErrorIs(t, executeList(ctx, "", "dev", "", "table"), errUtils.ErrComponentRequired)
}

// Re-running create/update without a terminal must say how to authorize it.
func TestConfirmExistingBackendOverwrite_NonTTYHintsAutoApprove(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockProv := NewMockProvisioner(ctrl)
	mockProv.EXPECT().BackendExists(gomock.Any(), gomock.Any()).Return(true, nil)
	t.Cleanup(ResetDependencies)
	SetProvisioner(mockProv)

	err := confirmExistingBackendOverwrite(t.Context(), &CreateBackendParams{Component: "vpc"}, false)

	require.ErrorIs(t, err, errUtils.ErrInteractiveNotAvailable)
	assert.Contains(t, strings.Join(cockroachErrors.GetAllHints(err), "\n"), "--auto-approve")
}

// describe/list tables carry a header row, and the machine-readable formats use snake_case keys.
func TestRenderBackendStatuses_HeaderAndKeys(t *testing.T) {
	statuses := []*pkgcfn.S3BackendStatus{}
	cfgMap := map[string]any{"provision": map[string]any{"targets": map[string]any{
		"artifacts": map[string]any{"kind": "aws/s3", "bucket": "my-bucket", "region": "us-east-1"},
	}}}
	targets := pkgcfn.FindS3BackendTargets(cfgMap["provision"].(map[string]any))
	require.Len(t, targets, 1)
	statuses = append(statuses, &pkgcfn.S3BackendStatus{Target: targets["artifacts"], Region: "us-east-1", Exists: true})

	capture := func(format string) string {
		oldStdout := os.Stdout
		r, w, err := os.Pipe()
		require.NoError(t, err)
		os.Stdout = w
		renderErr := renderBackendStatuses(format, statuses)
		require.NoError(t, w.Close())
		os.Stdout = oldStdout
		require.NoError(t, renderErr)
		out, err := io.ReadAll(r)
		require.NoError(t, err)
		return string(out)
	}

	table := capture("table")
	lines := strings.Split(strings.TrimSpace(table), "\n")
	require.Len(t, lines, 2)
	assert.Equal(t, []string{"TARGET", "BUCKET", "REGION", "STATUS"}, strings.Fields(lines[0]))
	assert.Equal(t, []string{"artifacts", "my-bucket", "us-east-1", "exists"}, strings.Fields(lines[1]))

	jsonOut := capture("json")
	assert.Contains(t, jsonOut, `"target"`)
	assert.Contains(t, jsonOut, `"bucket": "my-bucket"`)
	assert.Contains(t, jsonOut, `"exists": true`)
	assert.NotContains(t, jsonOut, `"Target"`)
	assert.NotContains(t, jsonOut, `"Exists"`)

	// An empty list still prints its explanatory line, without a stray header.
	statuses = nil
	assert.Contains(t, capture("table"), "No `kind: aws/s3` provision targets declared.")
}
