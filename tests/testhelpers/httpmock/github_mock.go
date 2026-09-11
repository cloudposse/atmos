// Package httpmock provides small, in-process mock HTTP servers standing in for GitHub (and
// GitHub-adjacent services like Artifactory) in tests, so the acceptance and unit-test suites
// never depend on live network access.
package httpmock

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/go-getter"

	atmosErrors "github.com/cloudposse/atmos/errors"
)

// defaultAquaPrefix is the path prefix under which the mock serves the aqua-registry raw
// content endpoints (registry.yaml and pkgs/<path>/registry.yaml), mirroring how
// ATMOS_TOOLCHAIN_AQUA_REGISTRY_URL points at "<base>/aqua" in the tests that use it.
const defaultAquaPrefix = "/aqua"

// GitHubMockServer is a small GitHub HTTP façade for tests. It started as a raw-content-only
// mock (suffix-matched files served over what looked like raw.githubusercontent.com) and has
// grown path-prefix handlers for the toolchain/registry/raw-fetch surface that
// pkg/github.Endpoints, pkg/toolchain/registry/aqua, and pkg/toolchain/installer talk to:
//
//   - GET  /api/v3/repos/{owner}/{repo}/releases[?page=&per_page=]
//   - GET  /api/v3/repos/{owner}/{repo}/releases/latest
//   - GET  /api/v3/repos/{owner}/{repo}/tags[?per_page=]
//   - GET  /{owner}/{repo}/releases/download/{tag}/{asset}
//   - GET  /{owner}/{repo}/archive/refs/tags/{tag}.tar.gz
//   - GET  /raw/{owner}/{repo}/{ref}/{path}           (GitHub Enterprise Server raw-content shape)
//   - GET  {aquaPrefix}/registry.yaml                  (aqua-registry package index)
//   - GET  {aquaPrefix}/pkgs/{path}/registry.yaml      (aqua-registry per-package file)
//
// Legacy suffix-matched raw files (RegisterFile, Transport, HTTPClient, HttpGetter) keep
// working unchanged: the new path-prefix routes are tried first, and any request that misses
// all of them falls through to the original suffix-match loop.
type GitHubMockServer struct {
	Server *httptest.Server
	files  map[string]string // path suffix -> content (legacy raw.githubusercontent.com stand-in).

	mu         sync.Mutex
	aquaPrefix string
	aquaTools  map[string]*AquaTool // registry path (e.g. "jqlang/jq") -> package spec.
	releases   map[string][]ReleaseSpec
	tags       map[string][]string
	assets     map[string][]byte // "owner/repo/tag/asset" -> bytes.
	archives   map[string][]byte // "owner/repo/tag" -> tar.gz bytes.
	rawFiles   map[string]string // "owner/repo/ref/path" -> content (GHES raw shape, exact match).
	rateLimit  *rateLimitState   // nil = default full, unthrottled budget. See SetRateLimit.
	failures   []*failureRule
	requests   []RequestLogEntry
}

// RequestLogEntry records one request the mock received, for egress/routing assertions.
type RequestLogEntry struct {
	Method string
	Path   string
}

// failureRule is one FailWith/FailWithTimes registration: any request whose path has this
// prefix is answered with status instead of being routed normally. Remaining < 0 means
// unlimited (persistent failure); remaining == 0 means exhausted (no longer applied).
type failureRule struct {
	prefix    string
	status    int
	remaining int
	headers   map[string]string // Extra response headers to set (e.g. Retry-After) before status.
}

// NewGitHubMockServer creates a mock server that intercepts GitHub requests.
// The server is automatically cleaned up when the test completes.
func NewGitHubMockServer(t *testing.T) *GitHubMockServer {
	t.Helper()

	mock := &GitHubMockServer{
		files:      make(map[string]string),
		aquaPrefix: defaultAquaPrefix,
		aquaTools:  make(map[string]*AquaTool),
		releases:   make(map[string][]ReleaseSpec),
		tags:       make(map[string][]string),
		assets:     make(map[string][]byte),
		archives:   make(map[string][]byte),
		rawFiles:   make(map[string]string),
	}

	mock.Server = httptest.NewServer(http.HandlerFunc(mock.handle))

	t.Cleanup(func() { mock.Server.Close() })
	return mock
}

// handle is the mock's single entry point: log the request, apply any registered failure
// injection, then try each path-prefix route in turn before falling back to the legacy
// suffix-matched file map.
func (m *GitHubMockServer) handle(w http.ResponseWriter, r *http.Request) {
	m.logRequest(r)

	const apiPrefix = "/api/v3/"
	if strings.HasPrefix(r.URL.Path, apiPrefix) {
		// Every real GitHub REST API response (not just /rate_limit) carries these headers.
		m.stampRateLimitHeaders(w)
	}

	if m.applyFailureInjection(w, r) {
		return
	}
	if m.route(w, r) {
		return
	}

	// Legacy suffix-matched raw file registrations (backward compatible).
	for pathSuffix, content := range m.files {
		if strings.HasSuffix(r.URL.Path, pathSuffix) {
			writeBytes(w, []byte(content), "")
			return
		}
	}
	http.NotFound(w, r)
}

// applyFailureInjection answers the request with a registered FailWith/FailWithTimes/
// FailWithHeaders status (and any extra headers) when path matches, reporting whether it did.
func (m *GitHubMockServer) applyFailureInjection(w http.ResponseWriter, r *http.Request) bool {
	status, headers, ok := m.matchFailure(r.URL.Path)
	if !ok {
		return false
	}
	for k, v := range headers {
		w.Header().Set(k, v)
	}
	http.Error(w, http.StatusText(status), status)
	return true
}

// route tries each path-prefix handler in turn, reporting whether one of them handled the
// request.
func (m *GitHubMockServer) route(w http.ResponseWriter, r *http.Request) bool {
	routes := []func(http.ResponseWriter, *http.Request) bool{
		m.tryRateLimit,
		m.tryAPIRepos,
		m.tryRaw,
		m.tryAqua,
		m.tryReleaseDownload,
	}
	for _, try := range routes {
		if try(w, r) {
			return true
		}
	}
	return false
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

// SetAquaPrefix overrides the path prefix used for the aqua-registry endpoints (default
// "/aqua"). Must be called before any RegisterAquaTool/aqua request.
func (m *GitHubMockServer) SetAquaPrefix(prefix string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.aquaPrefix = strings.TrimRight(prefix, "/")
}

// EnvForSubprocess returns the environment variables a subprocess needs to route GitHub,
// toolchain, and aqua-registry traffic at this mock: GITHUB_SERVER_URL/GITHUB_API_URL (the
// repo endpoints resolved by pkg/github.RepoEndpoints), ATMOS_TOOLCHAIN_GITHUB_URL/
// ATMOS_TOOLCHAIN_GITHUB_API_URL (pkg/github.ToolchainEndpoints), and
// ATMOS_TOOLCHAIN_AQUA_REGISTRY_URL (pkg/github.AquaRegistryURL).
func (m *GitHubMockServer) EnvForSubprocess() map[string]string {
	base := m.URL()
	m.mu.Lock()
	aquaPrefix := m.aquaPrefix
	m.mu.Unlock()

	return map[string]string{
		"GITHUB_SERVER_URL":                 base,
		"GITHUB_API_URL":                    base + "/api/v3",
		"ATMOS_TOOLCHAIN_GITHUB_URL":        base,
		"ATMOS_TOOLCHAIN_GITHUB_API_URL":    base + "/api/v3",
		"ATMOS_TOOLCHAIN_AQUA_REGISTRY_URL": base + aquaPrefix,
	}
}

// Setenv applies EnvForSubprocess via t.Setenv, for in-process tests (or tests that build the
// atmos binary in-process via exec.Command inheriting os.Environ()).
func (m *GitHubMockServer) Setenv(t *testing.T) {
	t.Helper()
	for k, v := range m.EnvForSubprocess() {
		t.Setenv(k, v)
	}
}

// FailWith makes every request whose path starts with pathPrefix fail with status,
// indefinitely, instead of being routed normally. Use FailWithTimes for a failure that
// recovers after N hits (e.g. to test an unauthenticated-retry path).
func (m *GitHubMockServer) FailWith(pathPrefix string, status int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failures = append(m.failures, &failureRule{prefix: pathPrefix, status: status, remaining: -1})
}

// FailWithTimes makes the first `times` requests whose path starts with pathPrefix fail with
// status; subsequent requests are routed normally. Useful for testing a retry-without-auth
// path: the first hit returns 403, the retry (a fresh, unauthenticated request) succeeds.
func (m *GitHubMockServer) FailWithTimes(pathPrefix string, status, times int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failures = append(m.failures, &failureRule{prefix: pathPrefix, status: status, remaining: times})
}

// FailWithHeaders makes every request whose path starts with pathPrefix fail with status,
// indefinitely, carrying the given extra response headers -- e.g. a secondary GitHub rate
// limit, which signals via `Retry-After` rather than `X-RateLimit-Remaining: 0`:
//
//	mock.FailWithHeaders("/api/v3/repos/owner/repo", http.StatusForbidden, map[string]string{"Retry-After": "30"})
func (m *GitHubMockServer) FailWithHeaders(pathPrefix string, status int, headers map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failures = append(m.failures, &failureRule{prefix: pathPrefix, status: status, remaining: -1, headers: headers})
}

// matchFailure returns the status and extra headers of the longest-prefix-matching,
// still-active failure rule for path, decrementing its remaining-hits counter (if bounded) as
// a side effect.
func (m *GitHubMockServer) matchFailure(path string) (int, map[string]string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var best *failureRule
	for _, f := range m.failures {
		if f.remaining == 0 {
			continue
		}
		if !strings.HasPrefix(path, f.prefix) {
			continue
		}
		if best == nil || len(f.prefix) > len(best.prefix) {
			best = f
		}
	}
	if best == nil {
		return 0, nil, false
	}
	if best.remaining > 0 {
		best.remaining--
	}
	return best.status, best.headers, true
}

// logRequest appends an entry to the mock's request log for egress/routing assertions.
func (m *GitHubMockServer) logRequest(r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = append(m.requests, RequestLogEntry{Method: r.Method, Path: r.URL.Path})
}

// Requests returns a copy of every request the mock has received so far, in order.
func (m *GitHubMockServer) Requests() []RequestLogEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]RequestLogEntry, len(m.requests))
	copy(out, m.requests)
	return out
}

// RequestCount returns how many logged requests have a path starting with pathPrefix.
func (m *GitHubMockServer) RequestCount(pathPrefix string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, entry := range m.requests {
		if strings.HasPrefix(entry.Path, pathPrefix) {
			count++
		}
	}
	return count
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
			return nil, errors.Join(atmosErrors.ErrParseURL, err)
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
