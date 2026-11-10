package gitea

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
)

func testAPI(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	s := httptest.NewServer(handler)
	t.Cleanup(s.Close)
	t.Setenv("ATMOS_CI_GITEA_API_URL", s.URL+"/api/v1")
	t.Setenv("ATMOS_CI_GITEA_ALLOW_INSECURE", "true")
	t.Setenv("ATMOS_CI_GITEA_TOKEN", "test-token")
}

func TestDetectionAndContext(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITEA_ACTIONS", "true")
	t.Setenv("GITHUB_REPOSITORY", "wrong/repo")
	t.Setenv("GITEA_REPOSITORY", "atmos/demo")
	t.Setenv("GITEA_SERVER_URL", "https://git.example.com")
	t.Setenv("GITEA_RUN_ID", "42")
	t.Setenv("GITEA_EVENT_NAME", "pull_request")
	t.Setenv("GITEA_SHA", "head")
	t.Chdir(t.TempDir())
	file := filepath.Join(t.TempDir(), "event.json")
	t.Setenv("GITEA_EVENT_PATH", file)
	require.NoError(t, os.WriteFile(file, []byte(`{"number":3,"pull_request":{"head":{"sha":"head","ref":"feature","repo":{"full_name":"atmos/demo"}},"base":{"sha":"base","ref":"main","repo":{"full_name":"atmos/demo"}}}}`), 0o600))
	p := NewProvider()
	require.True(t, p.Detect())
	c, err := p.Context()
	require.NoError(t, err)
	require.Equal(t, "atmos/demo", c.Repository)
	require.Equal(t, "https://git.example.com/atmos/demo/actions/runs/42", c.RunURL)
	require.Equal(t, 3, c.PullRequest.Number)
	require.False(t, c.PullRequest.Fork)
	base, err := p.ResolveBase()
	require.NoError(t, err)
	require.Equal(t, "base", base.SHA)
	t.Setenv("GITEA_SHA", "unknown")
	base, err = p.ResolveBase()
	require.NoError(t, err)
	require.Nil(t, base)
	require.NoError(t, os.WriteFile(file, []byte(`{"number":3,"pull_request":{"head":{},"base":{}}}`), 0o600))
	c, err = p.Context()
	require.NoError(t, err)
	require.True(t, c.PullRequest.Fork)
	require.NoError(t, os.WriteFile(file, []byte(`invalid`), 0o600))
	_, err = p.Context()
	require.Error(t, err)
}

func TestPushBase(t *testing.T) {
	t.Setenv("GITEA_EVENT_NAME", "push")
	file := filepath.Join(t.TempDir(), "event.json")
	t.Setenv("GITEA_EVENT_PATH", file)
	for _, before := range []string{"000000", "", "base"} {
		require.NoError(t, os.WriteFile(file, []byte(fmt.Sprintf(`{"before":%q}`, before)), 0o600))
		base, err := NewProvider().ResolveBase()
		require.NoError(t, err)
		if before == "base" {
			require.Equal(t, before, base.SHA)
		} else {
			require.Nil(t, base)
		}
	}
}

func TestCommitStatusLifecycle(t *testing.T) {
	states := []string{}
	testAPI(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "token test-token", r.Header.Get("Authorization"))
		if r.Method == http.MethodPost {
			require.Equal(t, "/api/v1/repos/org/repo/statuses/abc", r.URL.Path)
			var body map[string]string
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			states = append(states, body["state"])
			require.LessOrEqual(t, len([]rune(body["description"])), 140)
			fmt.Fprint(w, `{"id":1}`)
		} else {
			fmt.Fprint(w, `[{"id":1,"context":"plan","status":"pending"},{"id":2,"context":"plan","status":"success"},{"id":3,"context":"apply","status":"error"}]`)
		}
	})
	p := NewProvider()
	_, err := p.CreateCheckRun(context.Background(), &provider.CreateCheckRunOptions{Owner: "org", Repo: "repo", SHA: "abc", Name: "plan", Title: strings.Repeat("好", 150), Status: provider.CheckRunStateInProgress})
	require.NoError(t, err)
	_, err = p.UpdateCheckRun(context.Background(), &provider.UpdateCheckRunOptions{Owner: "org", Repo: "repo", SHA: "abc", Name: "plan", Status: provider.CheckRunStateSuccess})
	require.NoError(t, err)
	require.Equal(t, []string{"pending", "success"}, states)
	result, err := p.GetStatus(context.Background(), provider.StatusOptions{Owner: "org", Repo: "repo", SHA: "abc"})
	require.NoError(t, err)
	require.Len(t, result.CurrentBranch.Checks, 2)
	require.Equal(t, "success", result.CurrentBranch.Checks[0].Conclusion)
	require.Equal(t, "failure", result.CurrentBranch.Checks[1].Conclusion)
}

func TestCommentUpsertPaginationAndOwnership(t *testing.T) {
	testAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/user":
			fmt.Fprint(w, `{"id":7}`)
		case r.Method == http.MethodGet && r.URL.Query().Get("page") == "1":
			comments := make([]comment, 50)
			for i := range comments {
				comments[i].ID = int64(i + 1)
				comments[i].Body = "<!-- marker -->"
				comments[i].User.ID = 99
			}
			require.NoError(t, json.NewEncoder(w).Encode(comments))
		case r.Method == http.MethodGet:
			fmt.Fprint(w, `[{"id":51,"body":"<!-- marker --> old","user":{"id":7}}]`)
		default:
			require.Equal(t, http.MethodPatch, r.Method)
			require.Equal(t, "/api/v1/repos/org/repo/issues/comments/51", r.URL.Path)
			fmt.Fprint(w, `{"id":51,"body":"<!-- marker --> new","html_url":"https://git.example/comment/51"}`)
		}
	})
	c, err := NewProvider().PostComment(context.Background(), &provider.PostCommentOptions{Owner: "org", Repo: "repo", PRNumber: 1, Marker: "<!-- marker -->", Body: "<!-- marker --> new"})
	require.NoError(t, err)
	require.False(t, c.Created)
	require.EqualValues(t, 51, c.ID)
}

func fullCommentPage(firstID int64, userID int64) []comment {
	comments := make([]comment, pageSize)
	for i := range comments {
		comments[i].ID = firstID + int64(i)
		comments[i].Body = "<!-- marker -->"
		comments[i].User.ID = userID
	}
	return comments
}

func TestCommentPaginationStopsOnRepeatedPage(t *testing.T) {
	pages := 0
	testAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/user" {
			fmt.Fprint(w, `{"id":7}`)
			return
		}
		pages++
		// A server that ignores the page parameter returns the same full page forever.
		require.NoError(t, json.NewEncoder(w).Encode(fullCommentPage(1, 7)))
	})
	match, err := NewProvider().findComment(context.Background(), "/repos/org/repo/issues/1/comments", "<!-- marker -->")
	require.NoError(t, err)
	require.NotNil(t, match)
	require.EqualValues(t, pageSize, match.ID)
	require.Equal(t, 2, pages)
}

func TestCommentPaginationIsCapped(t *testing.T) {
	pages := 0
	testAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/user" {
			fmt.Fprint(w, `{"id":7}`)
			return
		}
		pages++
		require.NoError(t, json.NewEncoder(w).Encode(fullCommentPage(int64(pages*pageSize), 99)))
	})
	_, err := NewProvider().findComment(context.Background(), "/repos/org/repo/issues/1/comments", "<!-- marker -->")
	require.ErrorIs(t, err, errUtils.ErrCICommentListFailed)
	require.Equal(t, maxCommentPages, pages)
}

func TestAPIRejectsUnsafeConfigAndRedirects(t *testing.T) {
	p := NewProvider()
	for _, endpoint := range []string{"", "http://git.example/api/v1", "https://user:pass@git.example/api/v1", "https://git.example/api/v1?token=secret"} {
		t.Setenv("ATMOS_CI_GITEA_API_URL", endpoint)
		t.Setenv("GITEA_API_URL", "")
		t.Setenv("GITHUB_API_URL", "")
		t.Setenv("GITEA_SERVER_URL", "")
		t.Setenv("GITHUB_SERVER_URL", "")
		t.Setenv("ATMOS_CI_GITEA_ALLOW_INSECURE", "")
		require.Error(t, p.ValidateConfig())
	}
	redirected := false
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected = true }))
	defer destination.Close()
	testAPI(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, http.StatusFound) })
	err := p.request(context.Background(), http.MethodGet, "/user", nil, nil)
	require.ErrorContains(t, err, "HTTP 302")
	require.False(t, redirected)
	t.Setenv("ATMOS_CI_GITEA_TOKEN", "")
	t.Setenv("GITEA_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "must-not-use")
	require.ErrorContains(t, p.request(context.Background(), http.MethodGet, "/user", nil, nil), "requires ATMOS_CI_GITEA_TOKEN")
}

func TestEnvironmentFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "env")
	t.Setenv("GITHUB_ENV", path)
	t.Setenv("GITEA_ENV", "")
	p := NewProvider()
	require.NoError(t, p.WriteEnv("TEST", "one\ntwo"))
	require.Error(t, p.WriteEnv("BAD\nKEY", "value"))
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(content), "one\ntwo")
	t.Setenv("GITHUB_PATH", path)
	require.Error(t, p.AddPath("one\ntwo"))
	require.NoError(t, p.AddPath("/bin"))
}

func TestStatusIncludesPullRequests(t *testing.T) {
	testAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/user":
			fmt.Fprint(w, `{"id":7}`)
		case "/api/v1/repos/org/repo/pulls":
			fmt.Fprint(w, `[{"number":1,"title":"Infrastructure","head":{"sha":"abc","ref":"feature"},"base":{"ref":"main"},"user":{"id":7},"requested_reviewers":[{"id":7}]}]`)
		default:
			fmt.Fprint(w, `[{"id":1,"context":"plan","status":"success"}]`)
		}
	})
	result, err := NewProvider().GetStatus(context.Background(), provider.StatusOptions{Owner: "org", Repo: "repo", SHA: "abc", IncludeUserPRs: true, IncludeReviewRequests: true})
	require.NoError(t, err)
	require.Equal(t, 1, result.CurrentBranch.PullRequest.Number)
	require.Len(t, result.CreatedByUser, 1)
	require.Len(t, result.ReviewRequests, 1)
	require.True(t, result.CreatedByUser[0].AllPassed)
}

func TestCommentCreateAndMissingUpdate(t *testing.T) {
	posts := 0
	testAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/user":
			fmt.Fprint(w, `{"id":7}`)
		case r.Method == http.MethodGet:
			fmt.Fprint(w, `[]`)
		default:
			posts++
			fmt.Fprint(w, `{"id":1,"body":"<!-- marker -->"}`)
		}
	})
	opts := &provider.PostCommentOptions{Owner: "org", Repo: "repo", PRNumber: 1, Marker: "<!-- marker -->", Body: "<!-- marker -->", Behavior: provider.CommentBehaviorUpdate}
	p := NewProvider()
	_, err := p.PostComment(context.Background(), opts)
	require.ErrorIs(t, err, errUtils.ErrCICommentNotFound)
	require.Zero(t, posts)
	opts.Behavior = provider.CommentBehaviorCreate
	result, err := p.PostComment(context.Background(), opts)
	require.NoError(t, err)
	require.True(t, result.Created)
	require.Equal(t, 1, posts)
	opts.Body = "marker missing"
	_, err = p.PostComment(context.Background(), opts)
	require.Error(t, err)
	require.Equal(t, 1, posts)
}

func TestAPIErrorAndCancellation(t *testing.T) {
	testAPI(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "test-token should never appear in returned error", http.StatusForbidden)
	})
	p := NewProvider()
	err := p.request(context.Background(), http.MethodPost, "/test", nil, nil)
	require.ErrorContains(t, err, "HTTP 403")
	require.NotContains(t, err.Error(), "test-token")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, p.request(ctx, http.MethodGet, "/test", nil, nil), context.Canceled)
}
