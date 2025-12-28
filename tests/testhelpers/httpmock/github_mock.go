package httpmock

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/hashicorp/go-getter"
)

// GitHubMockServer intercepts GitHub requests and serves mock content.
// It provides a mock HTTP server that can be used to replace real GitHub
// raw content requests in tests, avoiding network dependencies and rate limits.
type GitHubMockServer struct {
	Server *httptest.Server
	files  map[string]string // path suffix -> content
}

// NewGitHubMockServer creates a mock server that intercepts GitHub requests.
// The server is automatically cleaned up when the test completes.
func NewGitHubMockServer(t *testing.T) *GitHubMockServer {
	t.Helper()

	mock := &GitHubMockServer{
		files: make(map[string]string),
	}

	mock.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Match path against registered files by suffix.
		for pathSuffix, content := range mock.files {
			if strings.HasSuffix(r.URL.Path, pathSuffix) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(content))
				return
			}
		}
		http.NotFound(w, r)
	}))

	t.Cleanup(func() { mock.Server.Close() })
	return mock
}

// RegisterFile registers content to be served for a given path suffix.
// The path suffix is matched against the end of incoming request paths.
// For example, RegisterFile("stacks/deploy/nonprod.yaml", content) will match
// requests to /cloudposse/atmos/main/tests/fixtures/scenarios/stack-templates-2/stacks/deploy/nonprod.yaml.
func (m *GitHubMockServer) RegisterFile(pathSuffix, content string) {
	m.files[pathSuffix] = content
}

// HTTPClient returns an http.Client that intercepts GitHub URLs.
// Use this with go-getter's HttpGetter or any HTTP client that needs
// to have GitHub requests redirected to the mock server.
func (m *GitHubMockServer) HTTPClient() *http.Client {
	return &http.Client{
		Transport: m.Transport(),
	}
}

// Transport returns an http.RoundTripper that intercepts GitHub URLs.
// Can be used to replace http.DefaultTransport in tests to intercept
// all HTTP requests to GitHub without modifying the code under test.
func (m *GitHubMockServer) Transport() http.RoundTripper {
	return &githubInterceptor{
		mockServerURL: m.Server.URL,
		base:          http.DefaultTransport,
	}
}

// HttpGetter returns a go-getter HttpGetter configured to use the mock.
// Use this when you need to inject a custom getter into go-getter's client.
func (m *GitHubMockServer) HttpGetter() *getter.HttpGetter {
	return &getter.HttpGetter{
		Client: m.HTTPClient(),
	}
}

// URL returns the mock server URL.
func (m *GitHubMockServer) URL() string {
	return m.Server.URL
}

// githubInterceptor rewrites GitHub URLs to mock server at transport layer.
type githubInterceptor struct {
	mockServerURL string
	base          http.RoundTripper
}

func (g *githubInterceptor) RoundTrip(req *http.Request) (*http.Response, error) {
	// Intercept GitHub raw content requests.
	if strings.Contains(req.URL.Host, "raw.githubusercontent.com") {
		mockURL, err := url.Parse(g.mockServerURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse mock server URL: %w", err)
		}
		// Clone the request to avoid modifying the original.
		newReq := req.Clone(req.Context())
		newReq.URL.Scheme = mockURL.Scheme
		newReq.URL.Host = mockURL.Host
		// Path is preserved - mock server matches by path suffix.
		return g.base.RoundTrip(newReq)
	}
	return g.base.RoundTrip(req)
}
