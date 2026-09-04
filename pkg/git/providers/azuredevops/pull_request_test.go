package azuredevops

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
)

func validOptions() *atmosgit.PullRequestOptions {
	return &atmosgit.PullRequestOptions{Owner: "acme", Namespace: []string{"proj"}, Repository: "repo", Base: "main", Head: "updates"}
}

func newTestProvider(server *httptest.Server) *Provider {
	return New(WithHTTPClient(server.Client()), WithBaseURL(server.URL), WithToken("s3cr3t-pat"))
}

func TestReconcilePullRequest(t *testing.T) {
	tests := []struct {
		name            string
		listResponse    string
		options         *atmosgit.PullRequestOptions
		wantNumber      int
		wantCreated     bool
		expectedMethods []string
	}{
		{
			name:         "updates existing pull request",
			listResponse: `{"value":[{"pullRequestId":7,"title":"old"}]}`,
			options: &atmosgit.PullRequestOptions{
				Owner: "acme", Namespace: []string{"proj"}, Repository: "repo", Base: "main", Head: "updates",
				Title: "title", Body: "body", Labels: []string{"component-update"},
			},
			wantNumber: 7,
			expectedMethods: []string{
				"GET /acme/proj/_apis/git/repositories/repo/pullrequests",
				"PATCH /acme/proj/_apis/git/repositories/repo/pullrequests/7",
				"POST /acme/proj/_apis/git/repositories/repo/pullrequests/7/labels",
			},
		},
		{
			name:         "creates pull request and applies metadata",
			listResponse: `{"value":[]}`,
			options: &atmosgit.PullRequestOptions{
				Owner: "acme", Namespace: []string{"proj"}, Repository: "repo", Base: "main", Head: "new-feature",
				Title: "title", Body: "body", Draft: true,
				Labels: []string{"component-update"}, Reviewers: []string{"reviewer-guid"},
			},
			wantNumber:  8,
			wantCreated: true,
			expectedMethods: []string{
				"GET /acme/proj/_apis/git/repositories/repo/pullrequests",
				"POST /acme/proj/_apis/git/repositories/repo/pullrequests",
				"POST /acme/proj/_apis/git/repositories/repo/pullrequests/8/labels",
				"PUT /acme/proj/_apis/git/repositories/repo/pullrequests/8/reviewers/reviewer-guid",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var methods []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				methods = append(methods, r.Method+" "+r.URL.Path)
				assertBasicAuth(t, r)
				w.Header().Set("Content-Type", "application/json")

				switch {
				case r.Method == http.MethodGet:
					_, _ = w.Write([]byte(tt.listResponse))
				case r.Method == http.MethodPost && r.URL.Path == "/acme/proj/_apis/git/repositories/repo/pullrequests":
					assertCreatedPullRequestPayload(t, r)
					_, _ = w.Write([]byte(`{"pullRequestId":8,"title":"title"}`))
				case r.Method == http.MethodPatch:
					_, _ = w.Write([]byte(`{"pullRequestId":7,"title":"title"}`))
				default:
					_, _ = w.Write([]byte(`{}`))
				}
			}))
			defer server.Close()

			p := newTestProvider(server)
			result, err := p.Reconcile(context.Background(), tt.options)
			require.NoError(t, err)
			assert.Equal(t, tt.wantNumber, result.Number)
			assert.Equal(t, tt.wantCreated, result.Created)
			assert.Equal(t, server.URL+"/acme/proj/_git/repo/pullrequest/"+strconv.Itoa(tt.wantNumber), result.URL)
			assert.Subset(t, methods, tt.expectedMethods)
		})
	}
}

func TestReconcileRejectsInvalidOptions(t *testing.T) {
	p := New(WithToken("s3cr3t-pat"))

	_, err := p.Reconcile(context.Background(), nil)
	assert.ErrorIs(t, err, errUtils.ErrComponentUpdaterConfig)

	_, err = p.Reconcile(context.Background(), &atmosgit.PullRequestOptions{Owner: "acme"})
	assert.ErrorIs(t, err, errUtils.ErrComponentUpdaterConfig)
}

func TestReconcileRejectsInvalidNamespace(t *testing.T) {
	p := New(WithToken("s3cr3t-pat"))

	tests := []struct {
		name      string
		namespace []string
	}{
		{name: "missing namespace", namespace: nil},
		{name: "empty project", namespace: []string{""}},
		{name: "too many segments", namespace: []string{"proj", "extra"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := &atmosgit.PullRequestOptions{Owner: "acme", Namespace: tt.namespace, Repository: "repo", Base: "main", Head: "updates"}
			_, err := p.Reconcile(context.Background(), options)
			assert.ErrorIs(t, err, errUtils.ErrAzureDevOpsNamespaceInvalid)
		})
	}
}

func TestReconcileMissingToken(t *testing.T) {
	t.Setenv("AZURE_DEVOPS_EXT_PAT", "")

	p := New()
	_, err := p.Reconcile(context.Background(), validOptions())
	assert.ErrorIs(t, err, errUtils.ErrAzureDevOpsAuthorization)
	assert.ErrorIs(t, err, errUtils.ErrAzureDevOpsTokenNotFound)
}

func TestReconcileRejectsAssignees(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"value":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"pullRequestId":9,"title":"title"}`))
	}))
	defer server.Close()

	p := newTestProvider(server)
	options := validOptions()
	options.Assignees = []string{"someone"}
	result, err := p.Reconcile(context.Background(), options)
	assert.ErrorIs(t, err, errUtils.ErrAzureDevOpsAssigneesUnsupported)
	// Assignees are rejected before reconcilePullRequest runs, so no pull request is created or
	// updated on invalid configuration -- the server must not see any request at all.
	assert.Nil(t, result, "invalid configuration must not report a pull request")
	assert.Empty(t, requests, "invalid configuration must not reach the Azure DevOps API")
}

func TestReconcileReturnsAuthorizationErrorOnUnauthorized(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{name: "unauthorized", status: http.StatusUnauthorized},
		{name: "forbidden", status: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
			}))
			defer server.Close()

			p := newTestProvider(server)
			_, err := p.Reconcile(context.Background(), validOptions())
			assert.ErrorIs(t, err, errUtils.ErrAzureDevOpsAuthorization)
		})
	}
}

func TestReconcileWrapsServerErrors(t *testing.T) {
	tests := []struct {
		name             string
		failMethod       string
		failPathContains string // "" matches any path for failMethod.
		listResponse     string
		options          *atmosgit.PullRequestOptions
		prAlreadyExisted bool
	}{
		{name: "list", failMethod: http.MethodGet, listResponse: `{"value":[]}`, options: validOptions()},
		{
			name: "create", failMethod: http.MethodPost, listResponse: `{"value":[]}`,
			options: &atmosgit.PullRequestOptions{Owner: "acme", Namespace: []string{"proj"}, Repository: "repo", Base: "main", Head: "new"},
		},
		{
			name: "update", failMethod: http.MethodPatch, listResponse: `{"value":[{"pullRequestId":7,"title":"title"}]}`, options: validOptions(),
			prAlreadyExisted: false, // no PR exists in Go terms yet since the update call itself fails.
		},
		{
			name: "labels", failMethod: http.MethodPost, failPathContains: "labels", listResponse: `{"value":[{"pullRequestId":7,"title":"title"}]}`,
			options: &atmosgit.PullRequestOptions{
				Owner: "acme", Namespace: []string{"proj"}, Repository: "repo", Base: "main", Head: "updates", Labels: []string{"label"},
			},
			prAlreadyExisted: true,
		},
		{
			name: "reviewers", failMethod: http.MethodPut, failPathContains: "reviewers", listResponse: `{"value":[{"pullRequestId":7,"title":"title"}]}`,
			options: &atmosgit.PullRequestOptions{
				Owner: "acme", Namespace: []string{"proj"}, Repository: "repo", Base: "main", Head: "updates", Reviewers: []string{"reviewer-guid"},
			},
			prAlreadyExisted: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == tt.failMethod && (tt.failPathContains == "" || strings.Contains(r.URL.Path, tt.failPathContains)) {
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"message":"server error"}`))
					return
				}
				if r.Method == http.MethodGet {
					_, _ = w.Write([]byte(tt.listResponse))
					return
				}
				_, _ = w.Write([]byte(`{"pullRequestId":7,"title":"title"}`))
			}))
			defer server.Close()

			p := newTestProvider(server)
			result, err := p.Reconcile(context.Background(), tt.options)
			assert.ErrorIs(t, err, errUtils.ErrPullRequestReconciliation)
			if tt.prAlreadyExisted {
				require.NotNil(t, result, "the already-created pull request must still be reported")
				assert.Equal(t, 7, result.Number)
			} else {
				assert.Nil(t, result)
			}
		})
	}
}

func TestSplitPathHelpers(t *testing.T) {
	repo := repositoryEndpoint{baseURL: "https://dev.azure.com", organization: "acme", project: "proj", repository: "repo"}
	assert.Equal(t, "https://dev.azure.com/acme/proj/_apis/git/repositories/repo", repo.apiBase())
	assert.Equal(t, "https://dev.azure.com/acme/proj/_git/repo/pullrequest/42", repo.pullRequestWebURL(42))
	assert.Equal(t, "refs/heads/main", refName("main"))
}

func assertBasicAuth(t *testing.T, r *http.Request) {
	t.Helper()
	username, password, ok := r.BasicAuth()
	require.True(t, ok, "expected Basic auth header")
	assert.Empty(t, username)
	assert.Equal(t, "s3cr3t-pat", password)
}

func assertCreatedPullRequestPayload(t *testing.T, r *http.Request) {
	t.Helper()
	var body createPullRequestBody
	require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
	assert.Equal(t, "refs/heads/new-feature", body.SourceRefName)
	assert.Equal(t, "refs/heads/main", body.TargetRefName)
	assert.Equal(t, "title", body.Title)
	assert.Equal(t, "body", body.Description)
	assert.True(t, body.IsDraft)
}
