package httpmock

import (
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitHubMockServer_RegisterFile(t *testing.T) {
	mock := NewGitHubMockServer(t)
	mock.RegisterFile("test/file.yaml", "key: value")

	// Direct request to mock server should work.
	resp, err := http.Get(mock.URL() + "/test/file.yaml")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "key: value", string(body))
}

func TestGitHubMockServer_NotFound(t *testing.T) {
	mock := NewGitHubMockServer(t)
	mock.RegisterFile("existing.yaml", "content")

	// Request for non-registered file should 404.
	resp, err := http.Get(mock.URL() + "/nonexistent.yaml")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestGitHubMockServer_Transport_InterceptsGitHub(t *testing.T) {
	mock := NewGitHubMockServer(t)
	mock.RegisterFile("stacks/deploy/nonprod.yaml", "intercepted: true")

	// Create client with intercepting transport.
	client := mock.HTTPClient()

	// Request to GitHub should be intercepted.
	resp, err := client.Get("https://raw.githubusercontent.com/cloudposse/atmos/main/tests/fixtures/scenarios/stack-templates-2/stacks/deploy/nonprod.yaml")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "intercepted: true", string(body))
}

func TestGitHubMockServer_Transport_PassesThroughNonGitHub(t *testing.T) {
	mock := NewGitHubMockServer(t)

	// Create client with intercepting transport.
	client := mock.HTTPClient()

	// Request to non-GitHub URL should pass through (and likely fail, but not be intercepted).
	// We use the mock server URL directly to verify non-GitHub URLs are not modified.
	mock.RegisterFile("direct.yaml", "direct: content")
	resp, err := client.Get(mock.URL() + "/direct.yaml")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestGitHubMockServer_PathSuffixMatching(t *testing.T) {
	mock := NewGitHubMockServer(t)

	// Register with partial path suffix.
	mock.RegisterFile("deploy/nonprod.yaml", "content: matched")

	client := mock.HTTPClient()

	// Full GitHub path should match by suffix.
	resp, err := client.Get("https://raw.githubusercontent.com/cloudposse/atmos/main/tests/fixtures/scenarios/stack-templates-2/stacks/deploy/nonprod.yaml")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "content: matched", string(body))
}
