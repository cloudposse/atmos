package gitea

import (
	"context"
	"os"
	"strconv"
	"testing"

	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/stretchr/testify/require"
)

// TestLiveGitea is an opt-in integration test against a disposable repository.
// It writes a commit status and one marker-based PR comment (then updates it).
func TestLiveGitea(t *testing.T) {
	if os.Getenv("ATMOS_TEST_GITEA_LIVE") != "true" {
		t.Skip("requires disposable Gitea repository and explicit opt-in")
	}
	n, err := strconv.Atoi(os.Getenv("ATMOS_TEST_GITEA_PR"))
	require.NoError(t, err)
	require.Positive(t, n)
	owner, repo, sha := os.Getenv("ATMOS_TEST_GITEA_OWNER"), os.Getenv("ATMOS_TEST_GITEA_REPO"), os.Getenv("ATMOS_TEST_GITEA_SHA")
	p := NewProvider()
	ctx := context.Background()
	created, err := p.CreateCheckRun(ctx, &provider.CreateCheckRunOptions{Owner: owner, Repo: repo, SHA: sha, Name: "atmos/provider-integration", Status: provider.CheckRunStateInProgress, Title: "Testing native Gitea provider"})
	require.NoError(t, err)
	require.Positive(t, created.ID)
	_, err = p.UpdateCheckRun(ctx, &provider.UpdateCheckRunOptions{Owner: owner, Repo: repo, SHA: sha, Name: created.Name, Status: provider.CheckRunStateSuccess, Title: "Native provider API integration passed"})
	require.NoError(t, err)
	opts := &provider.PostCommentOptions{Owner: owner, Repo: repo, PRNumber: n, Marker: "<!-- atmos:gitea-provider-integration -->", Body: "<!-- atmos:gitea-provider-integration -->\nNative Atmos Gitea provider integration test: commit status published."}
	first, err := p.PostComment(ctx, opts)
	require.NoError(t, err)
	opts.Body += " PR comment upsert verified."
	second, err := p.PostComment(ctx, opts)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
	require.False(t, second.Created)
	status, err := p.GetStatus(ctx, provider.StatusOptions{Owner: owner, Repo: repo, SHA: sha, IncludeUserPRs: true, IncludeReviewRequests: true})
	require.NoError(t, err)
	found := false
	for _, check := range status.CurrentBranch.Checks {
		if check.Name == created.Name {
			found = true
			require.Equal(t, "success", check.Conclusion)
		}
	}
	require.True(t, found)
}
