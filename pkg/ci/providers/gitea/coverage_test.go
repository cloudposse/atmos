package gitea

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
)

func TestStatusFailuresAndStateMapping(t *testing.T) {
	p := NewProvider()
	_, err := p.CreateCheckRun(context.Background(), nil)
	require.ErrorIs(t, err, errUtils.ErrCICheckRunCreateFailed)
	_, err = p.UpdateCheckRun(context.Background(), nil)
	require.ErrorIs(t, err, errUtils.ErrCICheckRunUpdateFailed)
	_, err = p.CreateCheckRun(context.Background(), &provider.CreateCheckRunOptions{})
	require.ErrorIs(t, err, errUtils.ErrCIStatusContextIncomplete)
	_, err = p.UpdateCheckRun(context.Background(), &provider.UpdateCheckRunOptions{})
	require.ErrorIs(t, err, errUtils.ErrCIStatusContextIncomplete)

	var states []string
	testAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var body struct {
				State string `json:"state"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			states = append(states, body.State)
			fmt.Fprint(w, `{"id":123}`)
			return
		}
		http.Error(w, "denied", http.StatusForbidden)
	})
	for _, state := range []provider.CheckRunState{provider.CheckRunStateFailure, provider.CheckRunStateError, provider.CheckRunStateCancelled} {
		_, err = p.CreateCheckRun(context.Background(), &provider.CreateCheckRunOptions{Owner: "org", Repo: "repo", SHA: "abc", Name: "plan", Status: state})
		require.NoError(t, err)
	}
	require.Equal(t, []string{"failure", "error", "error"}, states)
	_, err = p.GetStatus(context.Background(), provider.StatusOptions{Owner: "org", Repo: "repo", SHA: "abc"})
	require.ErrorIs(t, err, errUtils.ErrGiteaAPIFailed)
	_, err = p.GetStatus(context.Background(), provider.StatusOptions{SHA: "abc"})
	require.ErrorIs(t, err, errUtils.ErrCIStatusContextIncomplete)
}

func TestOutputWriterUsesGiteaFiles(t *testing.T) {
	output := filepath.Join(t.TempDir(), "output")
	summary := filepath.Join(t.TempDir(), "summary")
	t.Setenv("GITEA_OUTPUT", output)
	t.Setenv("GITEA_STEP_SUMMARY", summary)
	w := NewProvider().OutputWriter()
	require.NoError(t, w.WriteOutput("matrix", "one"))
	require.NoError(t, w.WriteSummary("summary"))
	b, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Contains(t, string(b), "matrix=one")
	b, err = os.ReadFile(summary)
	require.NoError(t, err)
	require.Contains(t, string(b), "summary")
}

func TestStatusReviewSelectionAndFailures(t *testing.T) {
	p := NewProvider()
	require.False(t, allChecksPassed(nil))
	require.False(t, allChecksPassed([]*provider.CheckStatus{{Conclusion: "failure"}}))
	require.True(t, allChecksPassed([]*provider.CheckStatus{{Conclusion: "success"}}))
	require.False(t, requestedReviewer(&pullRequest{}, 7))
	result := &provider.Status{}
	pr := &pullRequest{Number: 1}
	require.NoError(t, p.addPullRequest(context.Background(), provider.StatusOptions{SHA: "other"}, result, pr, 7))
	require.Empty(t, result.CreatedByUser)

	testAPI(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "denied", http.StatusForbidden) })
	err := p.addPullRequests(context.Background(), provider.StatusOptions{Owner: "org", Repo: "repo"}, result)
	require.ErrorIs(t, err, errUtils.ErrGiteaAPIFailed)
	pr.Head.SHA = "same"
	err = p.addPullRequest(context.Background(), provider.StatusOptions{Owner: "org", Repo: "repo", SHA: "same"}, result, pr, 7)
	require.ErrorIs(t, err, errUtils.ErrGiteaAPIFailed)
}

func TestReportingErrorsArePropagated(t *testing.T) {
	testAPI(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "denied", http.StatusForbidden) })
	p := NewProvider()
	create := &provider.CreateCheckRunOptions{Owner: "org", Repo: "repo", SHA: "abc", Name: "plan"}
	_, err := p.CreateCheckRun(context.Background(), create)
	require.ErrorIs(t, err, errUtils.ErrCICheckRunCreateFailed)
	require.ErrorIs(t, err, errUtils.ErrGiteaAPIFailed)
	_, err = p.UpdateCheckRun(context.Background(), &provider.UpdateCheckRunOptions{Owner: "org", Repo: "repo", SHA: "abc", Name: "plan"})
	require.ErrorIs(t, err, errUtils.ErrCICheckRunUpdateFailed)
	_, err = p.PostComment(context.Background(), &provider.PostCommentOptions{Owner: "org", Repo: "repo", PRNumber: 1, Body: "hello", Behavior: provider.CommentBehaviorCreate})
	require.ErrorIs(t, err, errUtils.ErrCICommentPostFailed)
	_, err = p.PostComment(context.Background(), &provider.PostCommentOptions{Owner: "org", Repo: "repo", PRNumber: 1, Body: "marker", Marker: "marker"})
	require.ErrorIs(t, err, errUtils.ErrGiteaAPIFailed)
	_, err = p.PostComment(context.Background(), &provider.PostCommentOptions{Owner: "org", Repo: "repo", PRNumber: 1, Body: "hello", Behavior: "invalid"})
	require.Error(t, err)
}

func TestStatusUsesContextAndReportsUserLookupError(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("GITEA_EVENT_NAME", "push")
	t.Setenv("GITEA_SHA", "abc")
	testAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/user" {
			http.Error(w, "denied", http.StatusForbidden)
			return
		}
		fmt.Fprint(w, `[]`)
	})
	p := NewProvider()
	status, err := p.GetStatus(context.Background(), provider.StatusOptions{Owner: "org", Repo: "repo"})
	require.NoError(t, err)
	require.Equal(t, "abc", status.CurrentBranch.CommitSHA)
	_, err = p.GetStatus(context.Background(), provider.StatusOptions{Owner: "org", Repo: "repo", IncludeUserPRs: true})
	require.ErrorIs(t, err, errUtils.ErrGiteaAPIFailed)
	t.Setenv("GITEA_EVENT_NAME", "workflow_run")
	_, err = p.GetStatus(context.Background(), provider.StatusOptions{Owner: "org", Repo: "repo"})
	require.ErrorIs(t, err, errUtils.ErrCIOperationNotSupported)
}

func TestEventValidationAndFallback(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, key := range []string{"GITEA_EVENT_PATH", "GITHUB_EVENT_PATH", "GITEA_BASE_REF", "GITHUB_BASE_REF"} {
		t.Setenv(key, "")
	}
	t.Setenv("GITEA_EVENT_NAME", "pull_request")
	p := NewProvider()
	_, err := p.Context()
	require.ErrorIs(t, err, errUtils.ErrCIPullRequestUnknown)
	t.Setenv("GITEA_EVENT_PATH", filepath.Join(t.TempDir(), "absent"))
	_, err = p.Context()
	require.Error(t, err)
	file := filepath.Join(t.TempDir(), "event.json")
	t.Setenv("GITEA_EVENT_PATH", file)
	require.NoError(t, os.WriteFile(file, []byte(`{"pull_request":null}`), 0o600))
	_, err = p.Context()
	require.ErrorIs(t, err, errUtils.ErrCIPullRequestUnknown)
	require.NoError(t, os.WriteFile(file, []byte(`{"pull_request":{"head":{},"base":{}}}`), 0o600))
	_, err = p.Context()
	require.ErrorIs(t, err, errUtils.ErrCIPullRequestUnknown)
	t.Setenv("GITEA_EVENT_NAME", "schedule")
	base, err := p.ResolveBase()
	require.NoError(t, err)
	require.Nil(t, base)
	require.False(t, validRepositoryName("invalid/name/extra"))
}

func TestClientValidatesAndDecodes(t *testing.T) {
	p := NewProvider()
	t.Setenv("ATMOS_CI_GITEA_API_URL", ":bad")
	err := p.request(context.Background(), http.MethodGet, "/user", nil, nil)
	require.ErrorIs(t, err, errUtils.ErrInvalidURL)
	testAPI(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `not-json`) })
	err = p.request(context.Background(), http.MethodPost, "/user", make(chan int), nil)
	require.Error(t, err)
	err = p.request(context.Background(), "bad\nmethod", "/user", nil, nil)
	require.Error(t, err)
	var result apiUser
	err = p.request(context.Background(), http.MethodGet, "/user", nil, &result)
	require.Error(t, err)
	t.Setenv("ATMOS_CI_GITEA_API_URL", "")
	t.Setenv("GITEA_API_URL", "")
	t.Setenv("GITHUB_API_URL", "")
	t.Setenv("GITEA_SERVER_URL", "https://git.example.com/")
	require.Equal(t, "https://git.example.com/api/v1", configuredAPIURL())
}

func TestCommentUpsertRejectsUnknownIdentity(t *testing.T) {
	testAPI(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"id":0}`) })
	opts := &provider.PostCommentOptions{Owner: "org", Repo: "repo", PRNumber: 1, Marker: "marker", Body: "marker"}
	_, err := NewProvider().PostComment(context.Background(), opts)
	require.ErrorIs(t, err, errUtils.ErrCICommentListFailed)
}

func TestStatusKeepsNewestContextWhenOlderStatusArrivesLater(t *testing.T) {
	testAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "1" {
			fmt.Fprint(w, `[{"id":3,"context":"plan","status":"success"},{"id":2,"context":"plan","status":"failure"}]`)
			return
		}
		fmt.Fprint(w, `[]`)
	})
	status, err := NewProvider().GetStatus(context.Background(), provider.StatusOptions{Owner: "org", Repo: "repo", SHA: "abc"})
	require.NoError(t, err)
	require.Len(t, status.CurrentBranch.Checks, 1)
	require.Equal(t, "success", status.CurrentBranch.Checks[0].Conclusion)
}

func TestPaginationPropagatesServerFailure(t *testing.T) {
	testAPI(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "denied", http.StatusForbidden) })
	pages := 0
	err := forEachPage(context.Background(), NewProvider(), listing[pagedItem]{path: "/items", id: func(i pagedItem) int64 { return i.ID }, limitErr: errUtils.ErrCIStatusFetchFailed}, func(_ []pagedItem) error { pages++; return nil })
	require.ErrorIs(t, err, errUtils.ErrGiteaAPIFailed)
	require.Zero(t, pages)
}
