package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchRepoDescription(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "application/vnd.github+json", r.Header.Get("Accept"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"description": "Atmos is the open-source runtime for infrastructure."}`))
	}))
	defer server.Close()

	got, err := fetchRepoDescription(server.URL)

	require.NoError(t, err)
	assert.Equal(t, "Atmos is the open-source runtime for infrastructure.", got)
}

func TestFetchRepoDescriptionSetsAuthorizationHeaderFromToken(t *testing.T) {
	t.Setenv("GH_TOKEN", "test-token")
	t.Setenv("GITHUB_TOKEN", "")

	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"description": "x"}`))
	}))
	defer server.Close()

	_, err := fetchRepoDescription(server.URL)

	require.NoError(t, err)
	assert.Equal(t, "Bearer test-token", gotAuth)
}

func TestFetchRepoDescriptionNonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	_, err := fetchRepoDescription(server.URL)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestFetchRepoDescriptionMalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer server.Close()

	_, err := fetchRepoDescription(server.URL)

	require.Error(t, err)
}

func TestFetchRepoDescriptionEmptyDescription(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"description": ""}`))
	}))
	defer server.Close()

	_, err := fetchRepoDescription(server.URL)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no description")
}

func TestGithubTokenPrefersGHToken(t *testing.T) {
	t.Setenv("GH_TOKEN", "gh-token")
	t.Setenv("GITHUB_TOKEN", "github-token")

	assert.Equal(t, "gh-token", githubToken())
}

func TestGithubTokenFallsBackToGitHubToken(t *testing.T) {
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "github-token")

	assert.Equal(t, "github-token", githubToken())
}

func TestGithubTokenEmptyWhenNeitherSet(t *testing.T) {
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")

	assert.Empty(t, githubToken())
}
