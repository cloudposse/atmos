package github

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
)

func TestPlanSourceMissingCredentials(t *testing.T) {
	for _, name := range []string{"ATMOS_CI_GITHUB_TOKEN", "ATMOS_PRO_GITHUB_TOKEN", "GITHUB_TOKEN", "GH_TOKEN"} {
		t.Setenv(name, "")
	}
	run := mergedPRRun()
	run.MergeQueue = &provider.MergeQueueInfo{PullRequestNumber: 290}
	p := NewProvider()
	tree, err := p.CommitTree(context.Background(), run)
	require.ErrorIs(t, err, errUtils.ErrGitHubTokenNotFound)
	assert.Empty(t, tree)
	revision, err := p.ReviewPullRequest(context.Background(), run)
	require.ErrorIs(t, err, errUtils.ErrGitHubTokenNotFound)
	assert.Nil(t, revision)
	revision, err = p.MergedPullRequest(context.Background(), run, run.SHA)
	require.ErrorIs(t, err, errUtils.ErrCIMergedPRLookupFailed)
	require.ErrorIs(t, err, errUtils.ErrGitHubTokenNotFound)
	assert.Nil(t, revision)
}

func TestPlanSourceAPIFailure(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				fmt.Fprint(w, `{"message":"source lookup failed"}`)
			}))
			defer server.Close()
			t.Setenv("ATMOS_CI_GITHUB_TOKEN", "test-token")
			t.Setenv("ATMOS_CI_GITHUB_API_URL", server.URL+"/")
			run := mergedPRRun()
			run.MergeQueue = &provider.MergeQueueInfo{PullRequestNumber: 290}
			p := NewProvider()
			tree, err := p.CommitTree(context.Background(), run)
			require.Error(t, err)
			assert.Empty(t, tree)
			revision, err := p.ReviewPullRequest(context.Background(), run)
			require.Error(t, err)
			assert.Nil(t, revision)
			assert.EqualValues(t, 2, requests.Load(), "failed lookups must not retry with a different revision")
		})
	}
}

func TestPlanSourceIncompleteContextDoesNotInitializeClient(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  *provider.Context
	}{
		{"nil", nil},
		{"empty", &provider.Context{}},
		{"missing owner", &provider.Context{SHA: mergedTestSHA, RepoName: "repo"}},
		{"missing repository", &provider.Context{SHA: mergedTestSHA, RepoOwner: "owner"}},
		{"missing sha", &provider.Context{RepoOwner: "owner", RepoName: "repo"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := NewProvider()
			tree, err := p.CommitTree(context.Background(), tc.run)
			require.NoError(t, err)
			assert.Empty(t, tree)
			revision, err := p.QueuedPullRequest(context.Background(), tc.run)
			require.NoError(t, err)
			assert.Nil(t, revision)
			assert.Nil(t, p.client)
		})
	}
	for _, run := range []*provider.Context{nil, {}, mergedPRRun(), {PullRequest: &provider.PRInfo{Number: 290}, EventName: "workflow_run"}} {
		p := NewProvider()
		revision, err := p.ReviewPullRequest(context.Background(), run)
		require.NoError(t, err)
		assert.Nil(t, revision)
		assert.Nil(t, p.client)
	}
}

func TestQueuedPullRequestInvalidClientEndpoint(t *testing.T) {
	p := NewProvider()
	p.client = NewClientWithHTTPClient(http.DefaultClient)
	// The injected client has not been given a valid slash-terminated REST URL.
	p.client.GitHub().BaseURL.Path = "/api/v3"
	run := mergedPRRun()
	run.MergeQueue = &provider.MergeQueueInfo{PullRequestNumber: 290}
	revision, err := p.QueuedPullRequest(context.Background(), run)
	require.Error(t, err)
	assert.Nil(t, revision)
}

func TestReviewPullRequestMissingEventDoesNotGuessLatestHead(t *testing.T) {
	t.Setenv("GITHUB_EVENT_PATH", filepath.Join(t.TempDir(), "missing-event.json"))
	run := mergedPRRun()
	run.EventName = "pull_request"
	run.PullRequest = &provider.PRInfo{Number: 290}
	p := NewProvider()
	revision, err := p.ReviewPullRequest(context.Background(), run)
	require.Error(t, err)
	assert.Nil(t, revision)
	assert.Nil(t, p.client, "a missing event must not become an API lookup of a mutable head")
}
