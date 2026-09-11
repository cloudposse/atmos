package gitmirror

import (
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// emptyGitConfigGlobal points GIT_CONFIG_GLOBAL at an empty temp file for the duration of a git
// command a test runs, so a developer's real ~/.gitconfig (credential helpers, insteadOf
// rewrites, etc.) can never interfere with a test asserting exact server behavior.
func emptyGitConfigGlobal(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gitconfig-empty")
	require.NoError(t, os.WriteFile(path, nil, filePerm))
	return path
}

// buildMirrorServer builds a mirror at a fresh temp root and serves it, registering a Close
// cleanup so callers don't have to.
func buildMirrorServer(t *testing.T, opts ...Option) *Server {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, Build(root))
	server, err := Serve(root, opts...)
	require.NoError(t, err)
	t.Cleanup(server.Close)
	return server
}

// refsDiscoveryURL returns the smart-HTTP discovery endpoint a `git clone` hits first.
func refsDiscoveryURL(server *Server) string {
	return server.URL() + "/" + Owner + "/" + Repo + ".git/info/refs?service=git-upload-pack"
}

// basicAuthResponse is the subset of an *http.Response a test needs, captured after the body has
// already been drained and closed so callers never have to manage its lifetime themselves.
type basicAuthResponse struct {
	StatusCode int
	Header     http.Header
}

// basicAuthGet issues a GET with a Basic-Auth header (user/pass may both be empty for an
// anonymous request), closing the response body itself before returning.
func basicAuthGet(t *testing.T, rawURL, user, pass string) basicAuthResponse {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	require.NoError(t, err)
	if user != "" || pass != "" {
		req.SetBasicAuth(user, pass)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	return basicAuthResponse{StatusCode: resp.StatusCode, Header: resp.Header.Clone()}
}

// TestServer_TokenAuth verifies a registered token is accepted (200) and an unregistered one is
// rejected (401) against the smart-HTTP discovery endpoint.
func TestServer_TokenAuth(t *testing.T) {
	server := buildMirrorServer(t)
	server.RegisterToken("good-token")

	resp := basicAuthGet(t, refsDiscoveryURL(server), defaultGitHubUsername, "good-token")
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp = basicAuthGet(t, refsDiscoveryURL(server), defaultGitHubUsername, "wrong-token")
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// TestServer_Anonymous verifies anonymous requests (no Authorization header) are rejected by
// default and accepted only when AllowAnonymous is set.
func TestServer_Anonymous(t *testing.T) {
	rejecting := buildMirrorServer(t)
	resp := basicAuthGet(t, refsDiscoveryURL(rejecting), "", "")
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	require.NotEmpty(t, resp.Header.Get("WWW-Authenticate"))

	allowing := buildMirrorServer(t, AllowAnonymous())
	resp = basicAuthGet(t, refsDiscoveryURL(allowing), "", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestServer_ReceivePackDisabled verifies a push (git-receive-pack) request is rejected even
// with a valid token, in both the discovery and RPC forms.
func TestServer_ReceivePackDisabled(t *testing.T) {
	server := buildMirrorServer(t)
	server.RegisterToken("push-token")

	discoveryURL := server.URL() + "/" + Owner + "/" + Repo + ".git/info/refs?service=git-receive-pack"
	resp := basicAuthGet(t, discoveryURL, defaultGitHubUsername, "push-token")
	require.Equal(t, http.StatusForbidden, resp.StatusCode)

	rpcURL := server.URL() + "/" + Owner + "/" + Repo + ".git/git-receive-pack"
	resp = basicAuthGet(t, rpcURL, defaultGitHubUsername, "push-token")
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// TestServer_RequestsRecordsAuthenticatedUser verifies Requests() records the authenticated
// username for a successful request.
func TestServer_RequestsRecordsAuthenticatedUser(t *testing.T) {
	server := buildMirrorServer(t)
	server.RegisterToken("logged-token")

	resp := basicAuthGet(t, refsDiscoveryURL(server), defaultGitHubUsername, "logged-token")
	require.Equal(t, http.StatusOK, resp.StatusCode)

	requests := server.Requests()
	require.NotEmpty(t, requests)
	last := requests[len(requests)-1]
	require.Equal(t, defaultGitHubUsername, last.User)
	require.Equal(t, http.MethodGet, last.Method)
	require.Contains(t, last.Path, "info/refs")
}

// withCredentials returns rawURL with user:pass embedded as Basic-Auth userinfo.
func withCredentials(t *testing.T, rawURL, user, pass string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	require.NoError(t, err)
	u.User = url.UserPassword(user, pass)
	return u.String()
}

// TestServer_CloneWithToken verifies a real `git clone` authenticated with a registered token
// succeeds end-to-end through git's smart-HTTP protocol, arrives authenticated as the expected
// username, and contains examples/demo-library, and that the same clone with the wrong token
// fails. The server here has no AllowAnonymous, so git's HTTP client is forced through its
// standard 401-challenge-then-retry-with-credentials dance -- git does NOT send URL-embedded
// Basic-Auth credentials preemptively, only in response to a 401 (unlike a plain `curl -u ...`,
// which sends them immediately). That is exactly what proves this mechanism authenticates real
// clones, not merely that a URL happens to carry a token.
func TestServer_CloneWithToken(t *testing.T) {
	server := buildMirrorServer(t)
	server.RegisterToken("clone-token")

	env := append(gitEnv(), "GIT_CONFIG_GLOBAL="+emptyGitConfigGlobal(t))
	repoURL := server.URL() + "/" + Owner + "/" + Repo + ".git"

	goodURL := withCredentials(t, repoURL, defaultGitHubUsername, "clone-token")
	dest := filepath.Join(t.TempDir(), "checkout")
	cmd := exec.Command("git", "clone", "--quiet", goodURL, dest)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git clone failed: %s", out)

	entries, err := os.ReadDir(filepath.Join(dest, "examples", "demo-library"))
	require.NoError(t, err)
	require.NotEmpty(t, entries, "examples/demo-library must not be empty in the clone")

	authenticated := false
	for _, req := range server.Requests() {
		if req.User == defaultGitHubUsername {
			authenticated = true
			break
		}
	}
	require.True(t, authenticated, "expected at least one request authenticated as %q, got: %+v", defaultGitHubUsername, server.Requests())

	badURL := withCredentials(t, repoURL, defaultGitHubUsername, "wrong-token")
	badDest := filepath.Join(t.TempDir(), "checkout-bad")
	badCmd := exec.Command("git", "clone", "--quiet", badURL, badDest)
	badCmd.Env = env
	badOut, err := badCmd.CombinedOutput()
	require.Error(t, err, "git clone with wrong token unexpectedly succeeded: %s", badOut)
}
