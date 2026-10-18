package github

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	githubci "github.com/cloudposse/atmos/pkg/ci/providers/github"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	ghendpoints "github.com/cloudposse/atmos/pkg/github"
)

// TestPullRequestBodyBadge proves *Provider implements atmosgit.PullRequestBodyBadger (so
// updater.RenderPRTemplates picks up GitHub's own badge instead of the static fallback) and that
// the badge it returns actually carries the raw HTML light/dark <picture> markup this provider's
// pull request rendering relies on.
func TestPullRequestBodyBadge(t *testing.T) {
	var badger atmosgit.PullRequestBodyBadger = New()
	badge := badger.PullRequestBodyBadge()
	assert.Contains(t, badge, "<picture>")
	assert.Contains(t, badge, `prefers-color-scheme: dark`)
	assert.Contains(t, badge, "https://atmos.tools/ci")
}

func TestReconcilePullRequest(t *testing.T) {
	tests := []struct {
		name            string
		listStatus      int
		listResponse    string
		options         *atmosgit.PullRequestOptions
		wantNumber      int
		wantCreated     bool
		wantError       error
		expectedMethods []string
	}{
		{
			name:         "updates existing pull request",
			listResponse: `[{"number":7,"html_url":"https://example.test/pr/7"}]`,
			options:      &atmosgit.PullRequestOptions{Owner: "acme", Repository: "repo", Base: "main", Head: "updates", Title: "title", Body: "body", Labels: []string{"component-update"}},
			wantNumber:   7,
			expectedMethods: []string{
				"GET /repos/acme/repo/pulls",
				"PATCH /repos/acme/repo/pulls/7",
				"POST /repos/acme/repo/issues/7/labels",
			},
		},
		{
			name:         "creates pull request and applies metadata",
			listResponse: `[]`,
			options: &atmosgit.PullRequestOptions{
				Owner: "acme", Repository: "repo", Base: "main", Head: "updates", Title: "title", Body: "body", Draft: true,
				Labels: []string{"component-update"}, Assignees: []string{"maintainer"}, Reviewers: []string{"reviewer"},
			},
			wantNumber:  8,
			wantCreated: true,
			expectedMethods: []string{
				"GET /repos/acme/repo/pulls",
				"POST /repos/acme/repo/pulls",
				"POST /repos/acme/repo/issues/8/labels",
				"POST /repos/acme/repo/issues/8/assignees",
				"POST /repos/acme/repo/pulls/8/requested_reviewers",
			},
		},
		{name: "returns unauthorized list error", listStatus: http.StatusUnauthorized, options: validPullRequestOptions(), wantError: errUtils.ErrGitHubAuthorization},
		{name: "returns forbidden list error", listStatus: http.StatusForbidden, options: validPullRequestOptions(), wantError: errUtils.ErrGitHubAuthorization},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var methods []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				methods = append(methods, r.Method+" "+r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					if tt.listStatus != 0 {
						w.WriteHeader(tt.listStatus)
						_, _ = w.Write([]byte(`{"message":"forbidden"}`))
						return
					}
					_, _ = w.Write([]byte(tt.listResponse))
					return
				}
				if tt.wantCreated {
					assertCreatedPullRequestPayload(t, r)
				}
				writePullRequestResponse(w, r.URL.Path, tt.wantCreated)
			}))
			defer server.Close()

			p := newTestProvider(server)
			result, err := p.Reconcile(context.Background(), tt.options)
			if tt.wantError != nil {
				assert.ErrorIs(t, err, tt.wantError)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantNumber, result.Number)
			assert.Equal(t, tt.wantCreated, result.Created)
			assert.Subset(t, methods, tt.expectedMethods)
		})
	}
}

func TestReconcileReturnsActionableErrors(t *testing.T) {
	provider, err := atmosgit.NewPullRequestPublisher(ProviderName)
	require.NoError(t, err)
	assert.IsType(t, &Provider{}, provider)
	p := New()
	_, _ = p.newClient()
	p = NewWithClientFactory(func() (client, error) { return nil, assert.AnError })
	_, err = p.Reconcile(context.Background(), validPullRequestOptions())
	assert.ErrorIs(t, err, errUtils.ErrGitHubAuthorization)

	_, err = p.Reconcile(context.Background(), nil)
	assert.ErrorIs(t, err, errUtils.ErrComponentUpdaterConfig)

	_, err = p.Reconcile(context.Background(), &atmosgit.PullRequestOptions{Owner: "acme"})
	assert.ErrorIs(t, err, errUtils.ErrComponentUpdaterConfig)
}

// TestReconcileRejectsNamespace proves a non-empty Namespace (meaningful only to a three-segment
// forge like Azure DevOps) is rejected outright rather than silently ignored -- GitHub addresses
// repositories with exactly two segments, so silently dropping Namespace would resolve to the
// wrong repository instead of surfacing the misconfiguration.
func TestReconcileRejectsNamespace(t *testing.T) {
	p := New()
	options := validPullRequestOptions()
	options.Namespace = []string{"proj"}

	_, err := p.Reconcile(context.Background(), options)

	assert.ErrorIs(t, err, errUtils.ErrComponentUpdaterConfig)
}

func TestGitHubError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want error
	}{
		{name: "bad credentials", err: errors.New("bad credentials"), want: errUtils.ErrGitHubAuthorization},
		{name: "unexpected error", err: errors.New("unexpected"), want: errUtils.ErrPullRequestReconciliation},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.ErrorIs(t, githubError(tt.err, nil), tt.want)
		})
	}
}

func validPullRequestOptions() *atmosgit.PullRequestOptions {
	return &atmosgit.PullRequestOptions{Owner: "acme", Repository: "repo", Base: "main", Head: "updates"}
}

func newTestProvider(server *httptest.Server) *Provider {
	return NewWithClientFactory(func() (client, error) {
		c := githubci.NewClientWithHTTPClient(server.Client())
		c.GitHub().BaseURL, _ = c.GitHub().BaseURL.Parse(server.URL + "/")
		return c, nil
	})
}

func assertCreatedPullRequestPayload(t *testing.T, r *http.Request) {
	t.Helper()
	switch r.URL.Path {
	case "/repos/acme/repo/pulls":
		var body struct {
			Title string `json:"title"`
			Body  string `json:"body"`
			Base  string `json:"base"`
			Head  string `json:"head"`
			Draft bool   `json:"draft"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "title", body.Title)
		assert.Equal(t, "body", body.Body)
		assert.Equal(t, "main", body.Base)
		assert.Equal(t, "updates", body.Head)
		assert.True(t, body.Draft)
	case "/repos/acme/repo/issues/8/labels":
		var labels []string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&labels))
		assert.Equal(t, []string{"component-update"}, labels)
	case "/repos/acme/repo/issues/8/assignees":
		var body struct {
			Assignees []string `json:"assignees"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, []string{"maintainer"}, body.Assignees)
	case "/repos/acme/repo/pulls/8/requested_reviewers":
		var body struct {
			Reviewers []string `json:"reviewers"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, []string{"reviewer"}, body.Reviewers)
	}
}

func writePullRequestResponse(w http.ResponseWriter, requestPath string, created bool) {
	if created {
		switch requestPath {
		case "/repos/acme/repo/pulls":
			_, _ = w.Write([]byte(`{"number":8,"html_url":"https://example.test/pr/8"}`))
		case "/repos/acme/repo/issues/8/labels":
			_, _ = w.Write([]byte(`[]`))
		default:
			_, _ = w.Write([]byte(`{"number":8,"html_url":"https://example.test/pr/8"}`))
		}
		return
	}
	if requestPath == "/repos/acme/repo/issues/7/labels" {
		_, _ = w.Write([]byte(`[]`))
		return
	}
	_, _ = w.Write([]byte(`{"number":7,"html_url":"https://example.test/pr/7"}`))
}

func TestReconcileWrapsGitHubMutationErrors(t *testing.T) {
	tests := []struct {
		name             string
		options          *atmosgit.PullRequestOptions
		path             string
		prAlreadyExisted bool // whether the PR itself was already created/edited before this step fails
	}{
		{name: "edit", options: &atmosgit.PullRequestOptions{Owner: "acme", Repository: "repo", Base: "main", Head: "updates"}, path: "/repos/acme/repo/pulls/7"},
		{name: "create", options: &atmosgit.PullRequestOptions{Owner: "acme", Repository: "repo", Base: "main", Head: "new"}, path: "/repos/acme/repo/pulls"},
		{name: "labels", options: &atmosgit.PullRequestOptions{Owner: "acme", Repository: "repo", Base: "main", Head: "updates", Labels: []string{"label"}}, path: "/repos/acme/repo/issues/7/labels", prAlreadyExisted: true},
		{name: "assignees", options: &atmosgit.PullRequestOptions{Owner: "acme", Repository: "repo", Base: "main", Head: "updates", Assignees: []string{"maintainer"}}, path: "/repos/acme/repo/issues/7/assignees", prAlreadyExisted: true},
		{name: "reviewers", options: &atmosgit.PullRequestOptions{Owner: "acme", Repository: "repo", Base: "main", Head: "updates", Reviewers: []string{"reviewer"}}, path: "/repos/acme/repo/pulls/7/requested_reviewers", prAlreadyExisted: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == tt.path && r.Method != http.MethodGet {
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
			result, err := p.Reconcile(context.Background(), tt.options)
			assert.ErrorIs(t, err, errUtils.ErrPullRequestReconciliation)

			if tt.prAlreadyExisted {
				// A pull request was already created/edited before this step failed -- its
				// number/URL must still be reported, not discarded alongside the error.
				require.NotNil(t, result, "the already-created pull request must still be reported")
				assert.Equal(t, 7, result.Number)
				assert.Equal(t, "https://example.test/pr/7", result.URL)
			} else {
				assert.Nil(t, result, "no pull request exists yet when create/edit itself fails")
			}
		})
	}
}

// TestRepositoryFromURL verifies the GitHub provider claims only the host its API client targets
// (github.com by default), over HTTPS and SSH, and nothing else.
func TestRepositoryFromURL(t *testing.T) {
	want := atmosgit.PullRequestRepository{Owner: "acme", Repository: "deployments"}
	tests := []struct {
		name      string
		serverURL string
		uri       string
		ok        bool
	}{
		{"github.com https", "", "https://github.com/acme/deployments.git", true},
		{"github.com ssh", "", "git@github.com:acme/deployments.git", true},
		{"github.com without .git", "", "https://github.com/acme/deployments", true},
		{"enterprise host is not claimed by default", "", "https://ghe.example.com/acme/deployments.git", false},
		{"enterprise https", "https://ghe.example.com", "https://ghe.example.com/acme/deployments.git", true},
		{"enterprise ssh", "https://ghe.example.com", "git@ghe.example.com:acme/deployments.git", true},
		{"enterprise host is case-insensitive", "https://ghe.example.com", "https://GHE.example.com/acme/deployments", true},
		{"github.com is not claimed under an enterprise server", "https://ghe.example.com", "https://github.com/acme/deployments.git", false},
		{"other forge https", "", "https://gitlab.com/acme/deployments.git", false},
		{"other forge ssh", "", "git@gitlab.com:acme/deployments.git", false},
		{"azure devops", "", "https://dev.azure.com/acme/platform/_git/deployments", false},
		{"nested path is not owner/repository", "", "https://github.com/acme/group/deployments.git", false},
		{"local path", "", "/tmp/origin.git", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GITHUB_SERVER_URL", tt.serverURL)
			t.Setenv("GITHUB_API_URL", "")

			got, ok := New().RepositoryFromURL(tt.uri)

			assert.Equal(t, tt.ok, ok)
			if tt.ok {
				assert.Equal(t, want, got)
			} else {
				assert.Equal(t, atmosgit.PullRequestRepository{}, got)
			}
		})
	}
}

// TestNewRepoEndpointsClient verifies the API client targets public GitHub by default and the
// Enterprise Server API when GITHUB_SERVER_URL names one, so a resolved repository's pull request
// requests go to the host that repository lives on.
func TestNewRepoEndpointsClient(t *testing.T) {
	t.Run("public GitHub by default", func(t *testing.T) {
		t.Setenv("ATMOS_CI_GITHUB_TOKEN", "token")
		t.Setenv("GITHUB_SERVER_URL", "")
		t.Setenv("GITHUB_API_URL", "")

		c, err := newRepoEndpointsClient()

		require.NoError(t, err)
		assert.Equal(t, "https://api.github.com/", c.GitHub().BaseURL.String())
	})

	t.Run("enterprise server", func(t *testing.T) {
		t.Setenv("ATMOS_CI_GITHUB_TOKEN", "token")
		t.Setenv("GITHUB_SERVER_URL", "https://ghe.example.com")
		t.Setenv("GITHUB_API_URL", "")

		c, err := newRepoEndpointsClient()

		require.NoError(t, err)
		assert.Equal(t, "ghe.example.com", c.GitHub().BaseURL.Host)
		assert.Equal(t, "/api/v3/", c.GitHub().BaseURL.Path)
	})

	t.Run("enterprise server with an explicit API URL", func(t *testing.T) {
		t.Setenv("ATMOS_CI_GITHUB_TOKEN", "token")
		t.Setenv("GITHUB_SERVER_URL", "https://ghe.example.com")
		t.Setenv("GITHUB_API_URL", "https://ghe.example.com/api/v3")

		c, err := newRepoEndpointsClient()

		require.NoError(t, err)
		assert.Equal(t, "https://ghe.example.com/api/v3/", c.GitHub().BaseURL.String())
	})

	t.Run("missing token", func(t *testing.T) {
		for _, name := range []string{"ATMOS_CI_GITHUB_TOKEN", "ATMOS_PRO_GITHUB_TOKEN", "GITHUB_TOKEN", "GH_TOKEN"} {
			t.Setenv(name, "")
		}

		_, err := newRepoEndpointsClient()

		assert.ErrorIs(t, err, errUtils.ErrGitHubTokenNotFound)
	})
}

// TestClientForEndpointsRejectsInvalidEnterpriseURL verifies an Enterprise Server API URL the
// GitHub client cannot parse is reported as an authorization configuration error.
func TestClientForEndpointsRejectsInvalidEnterpriseURL(t *testing.T) {
	_, err := clientForEndpoints(githubci.NewClientWithToken("token"), ghendpoints.Endpoints{
		APIURL:    "https://ghe.example.com/%zz",
		UploadURL: "https://ghe.example.com/api/uploads",
	})

	require.ErrorIs(t, err, errUtils.ErrGitHubAuthorization)
	assert.Contains(t, err.Error(), "https://ghe.example.com/%zz")
}
