package ghtest_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/ci/providers/github"
	"github.com/cloudposse/atmos/pkg/ci/providers/github/ghtest"
)

func do(t *testing.T, s *ghtest.Server, method, path, body string) (int, string) {
	t.Helper()

	req, err := http.NewRequestWithContext(context.Background(), method, s.URL()+path, strings.NewReader(body))
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	var buf bytes.Buffer
	_, err = buf.ReadFrom(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, buf.String()
}

func TestServer_UnknownRouteIs404AndRecorded(t *testing.T) {
	s := ghtest.NewServer(t)

	status, body := do(t, s, http.MethodGet, "/user", "")
	assert.Equal(t, http.StatusNotFound, status)
	assert.Contains(t, body, "no route")

	reqs := s.Requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, "/user", reqs[0].Path)
	assert.Equal(t, http.StatusNotFound, reqs[0].Status)
}

func TestServer_CommentsCreateEditAndList(t *testing.T) {
	s := ghtest.NewServer(t)

	status, body := do(t, s, http.MethodPost, "/repos/o/r/issues/5/comments", `{"body":"first"}`)
	require.Equal(t, http.StatusCreated, status)
	var created struct {
		ID      int64  `json:"id"`
		HTMLURL string `json:"html_url"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &created))
	assert.NotZero(t, created.ID)
	assert.Contains(t, created.HTMLURL, "issuecomment-")

	status, _ = do(t, s, http.MethodPatch, "/repos/o/r/issues/comments/"+itoa(created.ID), `{"body":"second"}`)
	require.Equal(t, http.StatusOK, status)

	status, _ = do(t, s, http.MethodPatch, "/repos/o/r/issues/comments/424242", `{"body":"x"}`)
	assert.Equal(t, http.StatusNotFound, status)

	writes := s.Comments()
	require.Len(t, writes, 2)
	assert.False(t, writes[0].Edited)
	assert.Equal(t, "first", writes[0].Body)
	assert.True(t, writes[1].Edited)
	assert.Equal(t, "second", writes[1].Body)

	current := s.CommentsFor("o", "r", 5)
	require.Len(t, current, 1)
	assert.Equal(t, "second", current[0].Body)
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestServer_StatusesAndCombinedRollup(t *testing.T) {
	s := ghtest.NewServer(t)

	status, _ := do(t, s, http.MethodPost, "/repos/o/r/statuses/abc", `{"state":"pending","context":"plan","description":"d","target_url":"http://t"}`)
	require.Equal(t, http.StatusCreated, status)
	status, _ = do(t, s, http.MethodPost, "/repos/o/r/statuses/abc", `{"state":"bogus","context":"plan"}`)
	assert.Equal(t, http.StatusUnprocessableEntity, status)
	status, _ = do(t, s, http.MethodPost, "/repos/o/r/statuses/abc", `{"state":"failure","context":"plan"}`)
	require.Equal(t, http.StatusCreated, status)

	got := s.Statuses()
	require.Len(t, got, 2)
	assert.Equal(t, ghtest.Status{Owner: "o", Repo: "r", SHA: "abc", State: "pending", Context: "plan", Description: "d", TargetURL: "http://t"}, got[0])

	_, body := do(t, s, http.MethodGet, "/repos/o/r/commits/abc/status", "")
	var combined struct {
		State    string `json:"state"`
		Statuses []struct {
			Context string `json:"context"`
			State   string `json:"state"`
		} `json:"statuses"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &combined))
	assert.Equal(t, "failure", combined.State)
	require.Len(t, combined.Statuses, 1, "latest status per context")
	assert.Equal(t, "failure", combined.Statuses[0].State)
}

func TestServer_SARIFUploadDecoded(t *testing.T) {
	s := ghtest.NewServer(t)

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, err := gz.Write([]byte(`{"version":"2.1.0"}`))
	require.NoError(t, err)
	require.NoError(t, gz.Close())
	payload, err := json.Marshal(map[string]string{
		"commit_sha": "abc",
		"ref":        "refs/heads/main",
		"sarif":      base64.StdEncoding.EncodeToString(buf.Bytes()),
	})
	require.NoError(t, err)

	status, _ := do(t, s, http.MethodPost, "/repos/o/r/code-scanning/sarifs", string(payload))
	require.Equal(t, http.StatusAccepted, status)

	status, _ = do(t, s, http.MethodPost, "/repos/o/r/code-scanning/sarifs", `{"sarif":"not base64!"}`)
	assert.Equal(t, http.StatusBadRequest, status)

	uploads := s.SARIFUploads()
	require.Len(t, uploads, 1)
	assert.Equal(t, `{"version":"2.1.0"}`, uploads[0].SARIF)
	assert.Equal(t, "abc", uploads[0].CommitSHA)
	assert.Equal(t, "refs/heads/main", uploads[0].Ref)
}

func TestServer_WithFailureMatchesMethodAndPrefix(t *testing.T) {
	s := ghtest.NewServer(t, ghtest.WithFailure(http.MethodPost, "/repos/o/r/statuses", http.StatusForbidden, "nope"))

	status, body := do(t, s, http.MethodPost, "/repos/o/r/statuses/abc", `{"state":"success","context":"c"}`)
	assert.Equal(t, http.StatusForbidden, status)
	assert.Contains(t, body, "nope")
	assert.Empty(t, s.Statuses(), "failed writes are not recorded as statuses")

	status, _ = do(t, s, http.MethodGet, "/repos/o/r/commits/abc/status", "")
	assert.Equal(t, http.StatusOK, status, "other methods and paths are unaffected")
}

func TestSetEnv_PullRequestPopulatesProviderContext(t *testing.T) {
	s := ghtest.NewServer(t)
	env := ghtest.SetEnv(
		t, s,
		ghtest.WithRepository("acme/infra"),
		ghtest.WithPullRequest(42, "feature", "main"),
		ghtest.WithRunID("999"),
	)
	// Resolve the SHA from GITHUB_SHA rather than the surrounding git checkout.
	t.Chdir(t.TempDir())

	for _, p := range []string{env.Output, env.EnvFile, env.Path, env.Summary, env.EventPath} {
		_, err := os.Stat(p)
		require.NoError(t, err)
	}

	ghtest.RegisterProvider(t, github.NewProvider())
	p := ci.Detect()
	require.NotNil(t, p)
	assert.Equal(t, "github-actions", p.Name())

	ctx, err := p.Context()
	require.NoError(t, err)
	assert.Equal(t, "acme", ctx.RepoOwner)
	assert.Equal(t, "infra", ctx.RepoName)
	assert.Equal(t, "999", ctx.RunID)
	assert.Equal(t, "pull_request", ctx.EventName)
	assert.False(t, ctx.ElevatedEvent)
	require.NotNil(t, ctx.PullRequest)
	assert.Equal(t, 42, ctx.PullRequest.Number)
	assert.Equal(t, "feature", ctx.PullRequest.HeadRef)
	assert.Equal(t, "main", ctx.PullRequest.BaseRef)

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(ghtest.ReadFile(t, env.EventPath)), &payload))
	assert.Contains(t, payload, "pull_request")
}

func TestSetEnv_PullRequestTargetIsElevated(t *testing.T) {
	s := ghtest.NewServer(t)
	ghtest.SetEnv(
		t, s,
		ghtest.WithEvent("pull_request_target", map[string]any{"action": "synchronize"}),
		ghtest.WithPullRequest(7, "fork-branch", "main"),
	)

	ctx, err := github.NewProvider().Context()
	require.NoError(t, err)
	assert.Equal(t, "pull_request_target", ctx.EventName)
	assert.True(t, ctx.ElevatedEvent)
	require.NotNil(t, ctx.PullRequest)
	assert.Equal(t, 7, ctx.PullRequest.Number)
}

// TestEndToEnd_ProviderWritesAreRecorded drives the real provider, built by
// NewClient() from GITHUB_API_URL, against the fake server.
func TestEndToEnd_ProviderWritesAreRecorded(t *testing.T) {
	s := ghtest.NewServer(t)
	ghtest.SetEnv(t, s, ghtest.WithPullRequest(42, "feature", "main"))
	ghtest.RegisterProvider(t, github.NewProvider())

	p := ci.Detect()
	require.NotNil(t, p)

	marker := "<!-- atmos:ci:test -->"
	res, err := p.PostComment(context.Background(), &provider.PostCommentOptions{
		Owner: "owner", Repo: "repo", PRNumber: 42, Marker: marker, Body: marker + "\nhello",
		Behavior: provider.CommentBehaviorUpsert,
	})
	require.NoError(t, err)
	assert.True(t, res.Created)

	// A second upsert edits the first comment.
	res, err = p.PostComment(context.Background(), &provider.PostCommentOptions{
		Owner: "owner", Repo: "repo", PRNumber: 42, Marker: marker, Body: marker + "\nhello again",
		Behavior: provider.CommentBehaviorUpsert,
	})
	require.NoError(t, err)
	assert.False(t, res.Created)

	writes := s.Comments()
	require.Len(t, writes, 2)
	assert.Contains(t, writes[0].Body, "hello")
	assert.True(t, writes[1].Edited)
	assert.Contains(t, writes[1].Body, "hello again")
}
