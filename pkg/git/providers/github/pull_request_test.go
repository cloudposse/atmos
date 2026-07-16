package github

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
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

func TestReconcileCreatesPullRequestAndAppliesMetadata(t *testing.T) {
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/acme/repo/pulls":
			_, _ = w.Write([]byte(`{"number":8,"html_url":"https://example.test/pr/8"}`))
		case r.URL.Path == "/repos/acme/repo/issues/8/labels":
			_, _ = w.Write([]byte(`[]`))
		default:
			_, _ = w.Write([]byte(`{"number":8,"html_url":"https://example.test/pr/8"}`))
		}
	}))
	defer server.Close()

	p := NewWithClientFactory(func() (client, error) {
		c := githubci.NewClientWithHTTPClient(server.Client())
		c.GitHub().BaseURL, _ = c.GitHub().BaseURL.Parse(server.URL + "/")
		return c, nil
	})
	result, err := p.Reconcile(context.Background(), &atmosgit.PullRequestOptions{
		Owner: "acme", Repository: "repo", Base: "main", Head: "updates", Title: "title", Body: "body", Draft: true,
		Labels: []string{"component-update"}, Assignees: []string{"maintainer"}, Reviewers: []string{"reviewer"},
	})
	require.NoError(t, err)
	assert.Equal(t, 8, result.Number)
	assert.True(t, result.Created)
	assert.Contains(t, methods, "POST /repos/acme/repo/pulls")
	assert.Contains(t, methods, "POST /repos/acme/repo/issues/8/labels")
	assert.Contains(t, methods, "POST /repos/acme/repo/issues/8/assignees")
	assert.Contains(t, methods, "POST /repos/acme/repo/pulls/8/requested_reviewers")
}

func TestReconcileReturnsActionableErrors(t *testing.T) {
	provider, err := atmosgit.NewPullRequestPublisher(ProviderName)
	require.NoError(t, err)
	assert.IsType(t, &Provider{}, provider)
	p := New()
	_, _ = p.newClient()
	p = NewWithClientFactory(func() (client, error) { return nil, assert.AnError })
	_, err = p.Reconcile(context.Background(), &atmosgit.PullRequestOptions{Owner: "acme", Repository: "repo", Base: "main", Head: "updates"})
	assert.ErrorIs(t, err, errUtils.ErrGitHubAuthorization)

	_, err = p.Reconcile(context.Background(), nil)
	assert.ErrorIs(t, err, errUtils.ErrComponentUpdaterConfig)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"forbidden"}`))
	}))
	defer server.Close()
	p = NewWithClientFactory(func() (client, error) {
		c := githubci.NewClientWithHTTPClient(server.Client())
		c.GitHub().BaseURL, _ = c.GitHub().BaseURL.Parse(server.URL + "/")
		return c, nil
	})
	_, err = p.Reconcile(context.Background(), &atmosgit.PullRequestOptions{Owner: "acme", Repository: "repo", Base: "main", Head: "updates"})
	assert.ErrorIs(t, err, errUtils.ErrGitHubAuthorization)

	_, err = p.Reconcile(context.Background(), &atmosgit.PullRequestOptions{Owner: "acme"})
	assert.ErrorIs(t, err, errUtils.ErrComponentUpdaterConfig)
	assert.ErrorIs(t, githubError(errors.New("bad credentials"), nil), errUtils.ErrGitHubAuthorization)
	assert.ErrorIs(t, githubError(errors.New("unexpected"), nil), errUtils.ErrPullRequestReconciliation)
}

func TestReconcileWrapsGitHubMutationErrors(t *testing.T) {
	tests := []struct {
		name    string
		options *atmosgit.PullRequestOptions
		path    string
	}{
		{name: "edit", options: &atmosgit.PullRequestOptions{Owner: "acme", Repository: "repo", Base: "main", Head: "updates"}, path: "/repos/acme/repo/pulls/7"},
		{name: "create", options: &atmosgit.PullRequestOptions{Owner: "acme", Repository: "repo", Base: "main", Head: "new"}, path: "/repos/acme/repo/pulls"},
		{name: "labels", options: &atmosgit.PullRequestOptions{Owner: "acme", Repository: "repo", Base: "main", Head: "updates", Labels: []string{"label"}}, path: "/repos/acme/repo/issues/7/labels"},
		{name: "assignees", options: &atmosgit.PullRequestOptions{Owner: "acme", Repository: "repo", Base: "main", Head: "updates", Assignees: []string{"maintainer"}}, path: "/repos/acme/repo/issues/7/assignees"},
		{name: "reviewers", options: &atmosgit.PullRequestOptions{Owner: "acme", Repository: "repo", Base: "main", Head: "updates", Reviewers: []string{"reviewer"}}, path: "/repos/acme/repo/pulls/7/requested_reviewers"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == tt.path {
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"message":"server error"}`))
					return
				}
				if r.Method == http.MethodGet {
					if tt.name == "create" {
						_, _ = w.Write([]byte(`[]`))
						return
					}
					_, _ = w.Write([]byte(`[{"number":7,"html_url":"https://example.test/pr/7"}]`))
					return
				}
				_, _ = w.Write([]byte(`{"number":7,"html_url":"https://example.test/pr/7"}`))
			}))
			defer server.Close()
			p := NewWithClientFactory(func() (client, error) {
				c := githubci.NewClientWithHTTPClient(server.Client())
				c.GitHub().BaseURL, _ = c.GitHub().BaseURL.Parse(server.URL + "/")
				return c, nil
			})
			_, err := p.Reconcile(context.Background(), tt.options)
			assert.ErrorIs(t, err, errUtils.ErrPullRequestReconciliation)
		})
	}
}
