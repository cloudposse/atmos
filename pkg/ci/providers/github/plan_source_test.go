package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueuedPullRequestBoundToQueueCommit(t *testing.T) {
	for _, tc := range []struct {
		name, entry, head string
		want              bool
	}{
		{"active queue", `{"headCommit":{"oid":"` + mergedTestSHA + `"}}`, mergedTestHead, true},
		{"rebuilt queue", `{"headCommit":{"oid":"new-queue-sha"}}`, "new-pr-head", false},
		{"removed queue", `null`, mergedTestHead, false},
		{"queue not built", `{"headCommit":null}`, mergedTestHead, false},
		{"missing PR head", `{"headCommit":{"oid":"` + mergedTestSHA + `"}}`, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/api/graphql", r.URL.Path)
				assert.Equal(t, http.MethodPost, r.Method)
				var body struct {
					Variables struct {
						Owner, Repo string
						Number      int
					}
				}
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				assert.Equal(t, "owner", body.Variables.Owner)
				assert.Equal(t, "repo", body.Variables.Repo)
				assert.Equal(t, 290, body.Variables.Number)
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"data":{"repository":{"pullRequest":{"number":290,"headRefOid":%q,"mergeQueueEntry":%s}}}}`, tc.head, tc.entry)
			}))
			defer server.Close()
			t.Setenv("ATMOS_CI_GITHUB_API_URL", server.URL+"/api/v3/")
			t.Setenv("ATMOS_CI_GITHUB_TOKEN", "test-token")
			run := mergedPRRun()
			run.MergeQueue = &provider.MergeQueueInfo{PullRequestNumber: 290}
			got, err := NewProvider().QueuedPullRequest(context.Background(), run)
			require.NoError(t, err)
			if tc.want {
				assert.Equal(t, &provider.PullRequestRevision{Number: 290, HeadSHA: mergedTestHead}, got)
			} else {
				assert.Nil(t, got)
			}
		})
	}
}

func TestQueuedPullRequestGraphQLError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/graphql", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"errors":[{"message":"permission denied"}]}`)
	}))
	defer server.Close()
	t.Setenv("ATMOS_CI_GITHUB_API_URL", server.URL+"/")
	t.Setenv("ATMOS_CI_GITHUB_TOKEN", "test-token")
	run := mergedPRRun()
	run.MergeQueue = &provider.MergeQueueInfo{PullRequestNumber: 290}
	got, err := NewProvider().QueuedPullRequest(context.Background(), run)
	require.Error(t, err)
	assert.Nil(t, got)
}

func TestCommitTreeUsesCheckoutSHA(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/git/commits/"+mergedTestSHA, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"tree":{"sha":"tree-sha"}}`)
	}))
	defer server.Close()
	t.Setenv("ATMOS_CI_GITHUB_API_URL", server.URL+"/")
	t.Setenv("ATMOS_CI_GITHUB_TOKEN", "test-token")
	tree, err := NewProvider().CommitTree(context.Background(), mergedPRRun())
	require.NoError(t, err)
	assert.Equal(t, "tree-sha", tree)
}

func TestReviewPullRequestUsesEventRevision(t *testing.T) {
	t.Setenv("GITHUB_EVENT_PATH", writeEventPayload(t, map[string]any{
		"pull_request": map[string]any{"number": 290, "head": map[string]any{"sha": mergedTestHead}, "merge_commit_sha": mergedTestSHA},
	}))
	for _, sha := range []string{mergedTestHead, mergedTestSHA, "another-checkout"} {
		t.Run(sha, func(t *testing.T) {
			run := mergedPRRun()
			run.SHA = sha
			run.EventName = "pull_request"
			run.PullRequest = &provider.PRInfo{Number: 290}
			p := NewProvider()
			revision, err := p.ReviewPullRequest(context.Background(), run)
			require.NoError(t, err)
			if sha == "another-checkout" {
				assert.Nil(t, revision)
			} else {
				assert.Equal(t, &provider.PullRequestRevision{Number: 290, HeadSHA: mergedTestHead}, revision)
			}
			assert.Nil(t, p.client, "event resolution does not need an API token")
		})
	}
}
