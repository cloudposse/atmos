package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	githubci "github.com/cloudposse/atmos/pkg/ci/providers/github"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
)

func TestReconcileExistingPullRequest(t *testing.T) {
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`[{"number":7,"html_url":"https://example.test/pr/7"}]`))
		case http.MethodPatch:
			_, _ = w.Write([]byte(`{"number":7,"html_url":"https://example.test/pr/7"}`))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	defer server.Close()

	p := NewWithClientFactory(func() (client, error) {
		c := githubci.NewClientWithHTTPClient(server.Client())
		c.GitHub().BaseURL, _ = c.GitHub().BaseURL.Parse(server.URL + "/")
		return c, nil
	})
	result, err := p.Reconcile(context.Background(), &atmosgit.PullRequestOptions{Owner: "acme", Repository: "repo", Base: "main", Head: "updates", Title: "title", Body: "body", Labels: []string{"component-update"}})
	require.NoError(t, err)
	assert.Equal(t, 7, result.Number)
	assert.False(t, result.Created)
	assert.Contains(t, methods, "GET /repos/acme/repo/pulls")
	assert.Contains(t, methods, "PATCH /repos/acme/repo/pulls/7")
	assert.Contains(t, methods, "POST /repos/acme/repo/issues/7/labels")
}
