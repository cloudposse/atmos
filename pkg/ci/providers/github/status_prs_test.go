package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/go-github/v59/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ciprovider "github.com/cloudposse/atmos/pkg/ci/internal/provider"
)

// newPRSearchTestProvider serves the search-specific HTTP fixtures.
func newPRSearchTestProvider(t *testing.T, handler http.Handler) *Provider {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	serverURL, err := url.Parse(server.URL + "/")
	require.NoError(t, err)
	ghClient := github.NewClient(nil)
	ghClient.BaseURL = serverURL
	return NewProviderWithClient(&Client{client: ghClient})
}

// prSearchMux returns a mux that serves the authenticated user, a single-result
// issue search, the full PR, and a passing check-run for that PR's head SHA. It
// records the search query string into *gotQuery so tests can assert on the
// exact query each caller builds.
func prSearchMux(gotQuery *string) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"login": "octocat"})
	})

	mux.HandleFunc("/search/issues", func(w http.ResponseWriter, r *http.Request) {
		*gotQuery = r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total_count": 1,
			"items": []map[string]any{
				{
					"number":   7,
					"title":    "Add feature",
					"html_url": "https://github.com/owner/repo/pull/7",
				},
			},
		})
	})

	mux.HandleFunc("/repos/owner/repo/pulls/7", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"number": 7,
			"head":   map[string]any{"ref": "feature", "sha": "sha7"},
			"base":   map[string]any{"ref": "main"},
		})
	})

	mux.HandleFunc("/repos/owner/repo/commits/sha7/check-runs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total_count": 1,
			"check_runs": []map[string]any{
				{"id": 1, "name": "CI", "status": "completed", "conclusion": "success"},
			},
		})
	})

	return mux
}

// assertEnrichedPR7 asserts the fully-enriched PR #7 that prSearchMux returns:
// branch/base from the full PR fetch and a passing check from its head SHA.
func assertEnrichedPR7(t *testing.T, prs []*ciprovider.PRStatus) {
	t.Helper()
	require.Len(t, prs, 1)
	assert.Equal(t, 7, prs[0].Number)
	assert.Equal(t, "Add feature", prs[0].Title)
	assert.Equal(t, "https://github.com/owner/repo/pull/7", prs[0].URL)
	assert.Equal(t, "feature", prs[0].Branch)
	assert.Equal(t, "main", prs[0].BaseBranch)
	require.Len(t, prs[0].Checks, 1)
	assert.True(t, prs[0].AllPassed)
}

func TestProvider_SearchPRsWithQuery(t *testing.T) {
	var gotQuery string
	p := newPRSearchTestProvider(t, prSearchMux(&gotQuery))

	prs, err := p.searchPRsWithQuery(context.Background(), "owner", "repo", "repo:owner/repo is:pr is:open")
	require.NoError(t, err)

	assert.Equal(t, "repo:owner/repo is:pr is:open", gotQuery)
	assertEnrichedPR7(t, prs)
}

func TestProvider_GetPRsCreatedByUser(t *testing.T) {
	var gotQuery string
	p := newPRSearchTestProvider(t, prSearchMux(&gotQuery))

	prs, err := p.getPRsCreatedByUser(context.Background(), "owner", "repo")
	require.NoError(t, err)

	// The authenticated user's login must flow into an author: qualifier.
	assert.Equal(t, "repo:owner/repo is:pr is:open author:octocat", gotQuery)
	assertEnrichedPR7(t, prs)
}

func TestProvider_GetPRsRequestingReview(t *testing.T) {
	var gotQuery string
	p := newPRSearchTestProvider(t, prSearchMux(&gotQuery))

	prs, err := p.getPRsRequestingReview(context.Background(), "owner", "repo")
	require.NoError(t, err)

	// The authenticated user's login must flow into a review-requested: qualifier.
	assert.Equal(t, "repo:owner/repo is:pr is:open review-requested:octocat", gotQuery)
	assertEnrichedPR7(t, prs)
}

// When fetching the authenticated user fails, both user-scoped helpers return
// the error instead of issuing a search.
func TestProvider_GetPRsForUser_UserFetchError(t *testing.T) {
	mux := http.NewServeMux()
	searched := false
	mux.HandleFunc("/user", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	mux.HandleFunc("/search/issues", func(http.ResponseWriter, *http.Request) {
		searched = true
	})
	p := newPRSearchTestProvider(t, mux)

	_, err := p.getPRsCreatedByUser(context.Background(), "owner", "repo")
	require.Error(t, err)

	_, err = p.getPRsRequestingReview(context.Background(), "owner", "repo")
	require.Error(t, err)

	assert.False(t, searched, "search must not run when the user lookup fails")
}

// A failing search surfaces as an error from searchPRsWithQuery.
func TestProvider_SearchPRsWithQuery_SearchError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/search/issues", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	p := newPRSearchTestProvider(t, mux)

	_, err := p.searchPRsWithQuery(context.Background(), "owner", "repo", "repo:owner/repo is:pr")
	require.Error(t, err)
}

// A PR whose full fetch fails is still returned, but without the enriched
// branch/check details (the error is swallowed so one bad PR can't drop the
// rest of the search results).
func TestProvider_SearchPRsWithQuery_FullPRFetchErrorKeepsBarePR(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/search/issues", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total_count": 1,
			"items": []map[string]any{
				{"number": 9, "title": "Bare PR", "html_url": "https://github.com/owner/repo/pull/9"},
			},
		})
	})
	mux.HandleFunc("/repos/owner/repo/pulls/9", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	p := newPRSearchTestProvider(t, mux)

	prs, err := p.searchPRsWithQuery(context.Background(), "owner", "repo", "repo:owner/repo is:pr")
	require.NoError(t, err)
	require.Len(t, prs, 1)
	assert.Equal(t, 9, prs[0].Number)
	assert.Equal(t, "Bare PR", prs[0].Title)
	assert.Empty(t, prs[0].Branch, "branch stays unset when the full PR fetch fails")
	assert.Empty(t, prs[0].Checks)
}

// getStatus, with both opt-in flags set, enriches the status with the current
// branch plus the user's created and review-requested PRs.
func TestProvider_GetStatus_WithUserPRsAndReviewRequests(t *testing.T) {
	var gotQuery string
	mux := prSearchMux(&gotQuery)

	// getBranchStatus for the current branch: no open PR, empty checks/status.
	mux.HandleFunc("/repos/owner/repo/pulls", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	})
	mux.HandleFunc("/repos/owner/repo/commits/abc123/check-runs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"total_count": 0, "check_runs": []map[string]any{}})
	})
	mux.HandleFunc("/repos/owner/repo/commits/abc123/status", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"state": "success", "statuses": []map[string]any{}})
	})

	p := newPRSearchTestProvider(t, mux)

	status, err := p.getStatus(context.Background(), ciprovider.StatusOptions{
		Owner:                 "owner",
		Repo:                  "repo",
		Branch:                "main",
		SHA:                   "abc123",
		IncludeUserPRs:        true,
		IncludeReviewRequests: true,
	})
	require.NoError(t, err)
	require.NotNil(t, status)

	assert.Equal(t, "owner/repo", status.Repository)
	require.NotNil(t, status.CurrentBranch)
	assert.Equal(t, "main", status.CurrentBranch.Branch)
	assert.Nil(t, status.CurrentBranch.PullRequest)

	assertEnrichedPR7(t, status.CreatedByUser)
	assertEnrichedPR7(t, status.ReviewRequests)
}

// getStatus treats PR-enrichment failures as non-fatal: a failed user-PR search
// leaves those slices empty while the branch status still comes back.
func TestProvider_GetStatus_UserPRErrorsAreNonFatal(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/pulls", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	})
	mux.HandleFunc("/repos/owner/repo/commits/abc123/check-runs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"total_count": 0, "check_runs": []map[string]any{}})
	})
	mux.HandleFunc("/repos/owner/repo/commits/abc123/status", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"state": "success", "statuses": []map[string]any{}})
	})
	// The authenticated-user lookup fails, so both user-PR searches error out.
	mux.HandleFunc("/user", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})

	p := newPRSearchTestProvider(t, mux)

	status, err := p.getStatus(context.Background(), ciprovider.StatusOptions{
		Owner:                 "owner",
		Repo:                  "repo",
		Branch:                "main",
		SHA:                   "abc123",
		IncludeUserPRs:        true,
		IncludeReviewRequests: true,
	})
	require.NoError(t, err)
	require.NotNil(t, status.CurrentBranch)
	assert.Empty(t, status.CreatedByUser)
	assert.Empty(t, status.ReviewRequests)
}
