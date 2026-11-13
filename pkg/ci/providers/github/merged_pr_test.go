package github

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
)

const (
	mergedTestSHA  = "fbdaf104ec6e2ffeb329f5badb51fb220ec7dcea"
	mergedTestHead = "b1fb6745d01e6a3f4a7738e0f73de40f37f92617"
	mergedTestPath = "/api/v3/repos/owner/repo/commits/" + mergedTestSHA + "/pulls"
)

func mergedPRRun() *provider.Context {
	return &provider.Context{RepoOwner: "owner", RepoName: "repo", SHA: mergedTestSHA}
}

// mergedPRServer serves the "list pull requests associated with a commit" endpoint. Pages are
// JSON arrays; every page but the last links to the next one.
func mergedPRServer(t *testing.T, calls *atomic.Int32, pages ...string) *Provider {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, mergedTestPath, r.URL.Path)
		page := 1
		if r.URL.Query().Get("page") != "" {
			_, _ = fmt.Sscanf(r.URL.Query().Get("page"), "%d", &page)
		}
		if page < len(pages) {
			w.Header().Set("Link", fmt.Sprintf(`<http://%s%s?page=%d&per_page=100>; rel="next"`, r.Host, r.URL.Path, page+1))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, pages[page-1])
	}))
	t.Cleanup(s.Close)
	t.Setenv("ATMOS_CI_GITHUB_TOKEN", "merged-pr-test-token")
	t.Setenv("ATMOS_CI_GITHUB_API_URL", s.URL+"/api/v3/")
	return NewProvider()
}

func prJSON(number int, merged bool, mergeSHA, headSHA string) string {
	mergedAt := "null"
	if merged {
		mergedAt = `"2026-11-05T17:19:00Z"`
	}
	return fmt.Sprintf(`{"number":%d,"merged_at":%s,"merge_commit_sha":%q,"head":{"sha":%q}}`, number, mergedAt, mergeSHA, headSHA)
}

func TestProviderMergedPullRequest(t *testing.T) {
	tests := []struct {
		name string
		page string
		want *provider.PullRequestRevision
	}{
		{
			name: "squash merge resolves to the pull request head",
			page: "[" + prJSON(290, true, mergedTestSHA, mergedTestHead) + "]",
			want: &provider.PullRequestRevision{Number: 290, HeadSHA: mergedTestHead},
		},
		{
			name: "skips pull requests that merely contain the commit",
			page: "[" + prJSON(301, false, "0000000000000000000000000000000000000001", "aaaa") + "," +
				prJSON(299, true, "0000000000000000000000000000000000000002", "bbbb") + "," +
				prJSON(290, true, mergedTestSHA, mergedTestHead) + "]",
			want: &provider.PullRequestRevision{Number: 290, HeadSHA: mergedTestHead},
		},
		{
			name: "unmerged pull request with a test merge commit is ignored",
			page: "[" + prJSON(290, false, mergedTestSHA, mergedTestHead) + "]",
		},
		{
			name: "fast-forward merge where the head is the commit is ignored",
			page: "[" + prJSON(290, true, mergedTestSHA, mergedTestSHA) + "]",
		},
		{
			name: "merged pull request with deleted head metadata is ignored",
			page: `[{"number":290,"merged_at":"2026-11-05T17:19:00Z","merge_commit_sha":"` + mergedTestSHA + `","head":null}]`,
		},
		{
			name: "merged pull request without head sha is ignored",
			page: "[" + prJSON(290, true, mergedTestSHA, "") + "]",
		},
		{
			name: "earlier rebased commit is not the landed tip",
			page: "[" + prJSON(290, true, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", mergedTestHead) + "]",
		},
		{
			name: "commit not associated with any pull request",
			page: "[]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			p := mergedPRServer(t, &calls, tt.page)
			got, err := p.MergedPullRequest(context.Background(), mergedPRRun(), mergedTestSHA)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.EqualValues(t, 1, calls.Load())
		})
	}
}

func TestProviderMergedPullRequestPagination(t *testing.T) {
	var calls atomic.Int32
	p := mergedPRServer(
		t, &calls,
		"["+prJSON(301, false, "0000000000000000000000000000000000000001", "aaaa")+"]",
		"["+prJSON(290, true, mergedTestSHA, mergedTestHead)+"]",
	)
	got, err := p.MergedPullRequest(context.Background(), mergedPRRun(), mergedTestSHA)
	require.NoError(t, err)
	assert.Equal(t, &provider.PullRequestRevision{Number: 290, HeadSHA: mergedTestHead}, got)
	assert.EqualValues(t, 2, calls.Load())
}

func TestProviderMergedPullRequestAPIFailure(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprint(w, `{"message":"Resource not accessible by integration"}`)
	}))
	t.Cleanup(s.Close)
	t.Setenv("ATMOS_CI_GITHUB_TOKEN", "merged-pr-test-token")
	t.Setenv("ATMOS_CI_GITHUB_API_URL", s.URL+"/api/v3/")

	got, err := NewProvider().MergedPullRequest(context.Background(), mergedPRRun(), mergedTestSHA)
	require.ErrorIs(t, err, errUtils.ErrCIMergedPRLookupFailed)
	assert.Nil(t, got)
}

func TestProviderMergedPullRequestIncompleteInputSkipsAPI(t *testing.T) {
	var calls atomic.Int32
	p := mergedPRServer(t, &calls, "["+prJSON(290, true, mergedTestSHA, mergedTestHead)+"]")

	for name, tc := range map[string]struct {
		run *provider.Context
		sha string
	}{
		"nil run":       {nil, mergedTestSHA},
		"missing owner": {&provider.Context{RepoName: "repo"}, mergedTestSHA},
		"missing repo":  {&provider.Context{RepoOwner: "owner"}, mergedTestSHA},
		"missing sha":   {mergedPRRun(), ""},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := p.MergedPullRequest(context.Background(), tc.run, tc.sha)
			require.NoError(t, err)
			assert.Nil(t, got)
		})
	}
	assert.Zero(t, calls.Load())
}

func TestProviderMergedPullRequestRejectsAmbiguityAcrossPages(t *testing.T) {
	var calls atomic.Int32
	p := mergedPRServer(
		t, &calls,
		"["+prJSON(290, true, mergedTestSHA, mergedTestHead)+"]",
		"["+prJSON(291, true, mergedTestSHA, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")+"]",
	)
	got, err := p.MergedPullRequest(context.Background(), mergedPRRun(), mergedTestSHA)
	require.ErrorIs(t, err, errUtils.ErrCIMergedPRLookupFailed)
	assert.Nil(t, got)
	assert.EqualValues(t, 2, calls.Load())
}
