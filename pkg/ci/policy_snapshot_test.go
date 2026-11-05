package ci_test

import (
	"context"
	"testing"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/providers/github"
	"github.com/stretchr/testify/require"
)

func TestPolicySnapshotUsesNativeRegistry(t *testing.T) {
	restore := ci.SwapRegistryForTest()
	defer restore()
	snapshot, err := ci.CollectPolicySnapshot(context.Background())
	require.NoError(t, err)
	require.Nil(t, snapshot)
	ci.Register(github.NewProvider())
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_EVENT_PATH", "")
	snapshot, err = ci.CollectPolicySnapshot(context.Background())
	require.NoError(t, err)
	require.Equal(t, "github-actions", snapshot.Provider)
}
