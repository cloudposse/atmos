package gitmirror

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/cgi" //nolint:gosec // G504: loopback-only git-http-backend front end for the acceptance suite, no untrusted-network exposure; cgi.Handler already strips the "Proxy" header (net/http/cgi/host.go), which is what CVE-2016-5386 (httpoxy) exploits.
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Request records one HTTP request the mirror Server handled, for test assertions that a git
// client actually authenticated (or didn't) the way a scenario expects.
type Request struct {
	Method string // HTTP method, e.g. "GET" or "POST".
	Path   string // URL path, e.g. "/cloudposse/atmos.git/info/refs".
	User   string // Basic-Auth username the request authenticated as, or "" if anonymous/rejected.
}

// Server is a local git-over-HTTP server backed by git-http-backend -- the same CGI program a
// real git host runs behind its HTTP frontend -- so a clone against it exercises git's actual
// smart-HTTP transport instead of a synthetic file:// mirror, and atmos's own credential handling
// (token injection, GIT_CONFIG insteadOf detection) runs the same code path it would against a
// real host. It validates a Basic-Auth token against a registry the test controls (see
// RegisterToken) instead of trusting every request, so a test can assert atmos authenticated (or
// didn't) exactly the way a real broker-fronted clone would.
//
// Only fetches (git-upload-pack) are served; receive-pack (push) is always rejected -- this
// mirror only ever needs to answer clones/fetches of the read-only snapshot Build publishes.
type Server struct {
	httpServer *httptest.Server

	mu             sync.Mutex
	tokens         map[string]struct{}
	allowAnonymous bool
	requests       []Request
}

// Option configures a Server at construction time.
type Option func(*Server)

// AllowAnonymous accepts requests carrying no Authorization header at all, instead of the
// default 401. Needed for cases that fetch without any token, e.g. the ssh insteadOf form, or a
// test case that deliberately scrubs all credentials.
func AllowAnonymous() Option {
	return func(s *Server) { s.allowAnonymous = true }
}

// Serve starts a local HTTP server exposing every bare repository under root (see Build) via
// git's smart-HTTP protocol, fetches only. Callers register acceptable Basic-Auth tokens with
// RegisterToken before pointing a git client at Server.URL(); the caller must Close the server
// when done.
func Serve(root string, opts ...Option) (*Server, error) {
	backend, err := gitHTTPBackendPath()
	if err != nil {
		return nil, err
	}

	s := &Server{
		tokens: make(map[string]struct{}),
	}
	for _, opt := range opts {
		opt(s)
	}

	cgiHandler := &cgi.Handler{
		Path: backend,
		Dir:  root,
		Env: []string{
			"GIT_PROJECT_ROOT=" + root,
			"GIT_HTTP_EXPORT_ALL=1",
		},
	}

	s.httpServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.handle(w, r, cgiHandler)
	}))

	return s, nil
}

// gitHTTPBackendPath locates the git-http-backend executable that ships with every supported git
// installation via `git --exec-path`, rather than hardcoding a path that varies by OS and package
// manager. On Windows every helper binary in that directory (including this one) carries an
// ".exe" suffix; everywhere else it does not.
func gitHTTPBackendPath() (string, error) {
	out, err := exec.Command("git", "--exec-path").Output()
	if err != nil {
		return "", fmt.Errorf("gitmirror: git --exec-path: %w", err)
	}

	name := "git-http-backend"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(strings.TrimSpace(string(out)), name)

	if _, statErr := os.Stat(path); statErr != nil {
		return "", fmt.Errorf("gitmirror: git-http-backend not found at %s: %w", path, statErr)
	}
	return path, nil
}

// handle enforces the fetch-only, token-gated policy in front of the CGI backend: it rejects
// push (receive-pack) outright, authenticates every other request, and always records the
// attempt (see Requests) before delegating an authenticated one to git-http-backend.
func (s *Server) handle(w http.ResponseWriter, r *http.Request, backend http.Handler) {
	if isReceivePack(r) {
		s.record(r, "")
		http.Error(w, "gitmirror: receive-pack is disabled; this mirror serves fetches only", http.StatusForbidden)
		return
	}

	user, ok := s.authenticate(r)
	s.record(r, user)
	if !ok {
		w.Header().Set("WWW-Authenticate", `Basic realm="atmos-git-mirror"`)
		http.Error(w, "gitmirror: unauthorized", http.StatusUnauthorized)
		return
	}

	backend.ServeHTTP(w, r)
}

// isReceivePack reports whether r is a push request (git-receive-pack), in either the
// smart-HTTP discovery form (?service=git-receive-pack) or the RPC form (a path ending in
// git-receive-pack).
func isReceivePack(r *http.Request) bool {
	if strings.HasSuffix(r.URL.Path, "git-receive-pack") {
		return true
	}
	return r.URL.Query().Get("service") == "git-receive-pack"
}

// authenticate validates the request's Basic-Auth header against the registered token set. It
// returns the authenticated username and true on success; on a missing Authorization header it
// succeeds with an empty username only when AllowAnonymous was set.
func (s *Server) authenticate(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", s.allowAnonymous
	}

	const prefix = "Basic "
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(header, prefix))
	if err != nil {
		return "", false
	}

	user, token, ok := strings.Cut(string(decoded), ":")
	if !ok {
		return "", false
	}

	s.mu.Lock()
	_, known := s.tokens[token]
	s.mu.Unlock()
	if !known {
		return "", false
	}
	return user, true
}

// record appends one entry to the request log (see Requests).
func (s *Server) record(r *http.Request, user string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, Request{Method: r.Method, Path: r.URL.Path, User: user})
}

// RegisterToken adds t to the set of Basic-Auth passwords the server accepts. The username half
// of the credential is never checked -- only the token matters, matching how a real token-based
// git host authenticates (the username is a convention, e.g. GitHub's "x-access-token", not a
// secret).
func (s *Server) RegisterToken(t string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[t] = struct{}{}
}

// URL returns the server's base URL (e.g. "http://127.0.0.1:54321"), with no trailing slash.
func (s *Server) URL() string {
	return s.httpServer.URL
}

// Requests returns a snapshot of every request the server has handled so far, for assertions
// like "the clone authenticated as x-access-token".
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Request, len(s.requests))
	copy(out, s.requests)
	return out
}

// Close shuts down the underlying HTTP server. Safe to call once, matching http.Server.Close.
func (s *Server) Close() {
	s.httpServer.Close()
}
