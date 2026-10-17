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

func labelsRun() *provider.Context {
	return &provider.Context{RepoOwner: "owner", RepoName: "repo", PullRequest: &provider.PRInfo{Number: 42}}
}

func setupLabelsClient(t *testing.T, apiURL string) *Provider {
	t.Helper()
	t.Setenv("ATMOS_CI_GITHUB_TOKEN", "labels-test-token")
	t.Setenv("ATMOS_CI_GITHUB_API_URL", apiURL)
	return NewProvider()
}

func TestProviderLabelsPagination(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v3/repos/owner/repo/issues/42/labels", r.URL.Path)
		assert.Equal(t, "100", r.URL.Query().Get("per_page"))
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") == "2" {
			_, _ = fmt.Fprint(w, `[{"name":"deploy"},{"name":"needs review"}]`)
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<http://%s%s?page=2&per_page=100>; rel="next"`, r.Host, r.URL.Path))
		_, _ = fmt.Fprint(w, `[{"name":"preview"}]`)
	}))
	t.Cleanup(s.Close)
	p := setupLabelsClient(t, s.URL+"/api/v3/")
	got, err := p.Labels(context.Background(), labelsRun())
	require.NoError(t, err)
	assert.Equal(t, []string{"preview", "deploy", "needs review"}, got)
	assert.EqualValues(t, 2, calls.Load())
}

func TestProviderLabelsAPIFailure(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusTooManyRequests, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = fmt.Fprint(w, `{"message":"labels unavailable"}`)
			}))
			t.Cleanup(s.Close)
			p := setupLabelsClient(t, s.URL)
			got, err := p.Labels(context.Background(), labelsRun())
			require.ErrorIs(t, err, errUtils.ErrCILabelsFetchFailed)
			assert.Nil(t, got)
		})
	}
}

func TestProviderLabelsPartialPageFailure(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") == "2" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = fmt.Fprint(w, `{"message":"forbidden"}`)
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<http://%s%s?page=2>; rel="next"`, r.Host, r.URL.Path))
		_, _ = fmt.Fprint(w, `[{"name":"preview"}]`)
	}))
	t.Cleanup(s.Close)
	p := setupLabelsClient(t, s.URL)
	got, err := p.Labels(context.Background(), labelsRun())
	require.ErrorIs(t, err, errUtils.ErrCILabelsFetchFailed)
	assert.Nil(t, got, "partial label lists must not become a snapshot")
}

func TestProviderLabelsEmptyAndCancellation(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `[]`)
	}))
	t.Cleanup(s.Close)
	p := setupLabelsClient(t, s.URL)
	got, err := p.Labels(context.Background(), labelsRun())
	require.NoError(t, err)
	assert.Equal(t, []string{}, got)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = p.Labels(ctx, labelsRun())
	require.ErrorIs(t, err, context.Canceled)
}

func TestProviderLabelsMissingTokenAndContext(t *testing.T) {
	for _, name := range []string{"ATMOS_CI_GITHUB_TOKEN", "ATMOS_PRO_GITHUB_TOKEN", "GITHUB_TOKEN", "GH_TOKEN"} {
		t.Setenv(name, "")
	}
	p := NewProvider()
	_, err := p.Labels(context.Background(), labelsRun())
	require.ErrorIs(t, err, errUtils.ErrGitHubTokenNotFound)
	for _, run := range []*provider.Context{nil, {}, {PullRequest: &provider.PRInfo{Number: 0}}} {
		_, err = p.Labels(context.Background(), run)
		require.ErrorIs(t, err, errUtils.ErrInvalidArguments)
	}
}
