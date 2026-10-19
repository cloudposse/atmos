package github

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/pkg/policy"
	"github.com/stretchr/testify/require"
)

func TestPolicySnapshot(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "not-for-policy-input")
	t.Setenv("PRIVATE_SECRET", "not-for-policy-input")
	t.Setenv("GITHUB_EVENT_NAME", "pull_request")
	t.Setenv("GITHUB_REPOSITORY", "acme/infra")
	path := filepath.Join(t.TempDir(), "event.json")
	t.Setenv("GITHUB_EVENT_PATH", path)
	for _, tc := range []struct {
		name, payload string
		status        policy.Availability
	}{
		{"native fields", `{"action":"opened","pull_request":{"number":4},"future_field":42}`, policy.Available},
		{"malformed", `{`, policy.Incomplete},
		{"null", `null`, policy.Incomplete},
		{"array", `[]`, policy.Incomplete},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, os.WriteFile(path, []byte(tc.payload), 0o600))
			snapshot, err := NewProvider().CollectPolicySnapshot(context.Background())
			require.NoError(t, err)
			require.Equal(t, "github-actions", snapshot.Provider)
			require.Equal(t, PolicySnapshotSchema, snapshot.Schema)
			require.Equal(t, tc.status, snapshot.Event.Status)
			require.Equal(t, tc.status == policy.Available, snapshot.Event.Complete)
			require.NotContains(t, string(snapshot.Metadata.Data), "not-for-policy-input")
			require.Contains(t, string(snapshot.Metadata.Data), "acme/infra")
			require.Empty(t, snapshot.Facts)
			if tc.status == policy.Available {
				require.JSONEq(t, tc.payload, string(snapshot.Event.Data))
			}
		})
	}
	t.Setenv("GITHUB_EVENT_PATH", "")
	snapshot, err := NewProvider().CollectPolicySnapshot(context.Background())
	require.NoError(t, err)
	require.Equal(t, policy.Unavailable, snapshot.Event.Status)
	t.Setenv("GITHUB_EVENT_PATH", t.TempDir())
	snapshot, err = NewProvider().CollectPolicySnapshot(context.Background())
	require.NoError(t, err)
	require.Equal(t, policy.Unavailable, snapshot.Event.Status)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = NewProvider().CollectPolicySnapshot(ctx)
	require.ErrorIs(t, err, context.Canceled)
}
