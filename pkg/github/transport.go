package github

import (
	"net/http"
	"strings"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
)

// scopedTokenTransport is an http.RoundTripper that attaches a bearer token to a request only
// when the request's own URL -- evaluated fresh on every RoundTrip call, not once when the
// client was built -- is https and its host matches allowed's server, API, or upload host.
//
// This defends against the pattern golang.org/x/oauth2's Transport uses (re-adding
// Authorization unconditionally on every RoundTrip call, including one net/http built for a
// redirect it is automatically following): net/http's own default redirect header-forwarding
// policy already strips Authorization across a cross-host redirect, but explicitly *forwards*
// it across a same-host scheme downgrade (https to http) -- and an oauth2.Transport bypasses
// both behaviors by resetting the header itself, on every hop, regardless of destination.
// RoundTrip is invoked once per real network round-trip -- including each hop of an automatic
// redirect, since net/http's Client.send calls Transport.RoundTrip again for the redirected
// request -- so re-evaluating the request's URL here (rather than deciding once when the
// client is built) covers both cases.
type scopedTokenTransport struct {
	// base is the underlying RoundTripper that performs the actual network request. Defaults
	// to http.DefaultTransport when nil.
	base http.RoundTripper
	// token is the bearer token to attach when allowed, or "" to never attach one.
	token string
	// allowed is the set of hosts (server, API, upload) the token may be sent to.
	allowed Endpoints
}

// RoundTrip implements http.RoundTripper. It clones req (never mutating the caller's original,
// matching http.RoundTripper's documented contract), then sets or removes the Authorization
// header based on req's own URL before delegating to the base transport.
func (t *scopedTokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	defer perf.Track(nil, "github.scopedTokenTransport.RoundTrip")()

	req = req.Clone(req.Context())
	if t.token != "" && requestAllowsToken(req, t.allowed) {
		req.Header.Set("Authorization", "Bearer "+t.token)
	} else {
		req.Header.Del("Authorization")
	}

	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

// requestAllowsToken reports whether it is safe to attach a bearer token to req: only when its
// scheme is https and its host matches one of allowed's server, API, or upload hosts. A token
// scoped to one host must never ride along to another host, or over plain http, even when the
// second request is a redirect the underlying transport followed automatically.
func requestAllowsToken(req *http.Request, allowed Endpoints) bool {
	if !strings.EqualFold(req.URL.Scheme, "https") {
		return false
	}
	host := req.URL.Host
	return allowed.IsHost(host) || allowed.IsAPIHost(host) || allowed.IsUploadHost(host)
}

// IsApprovedGitHubDownloadHost reports whether host (case-insensitive, with port and trailing
// dot normalized) is a server, API, or upload host that RepoEndpoints() or ToolchainEndpoints()
// resolves to. Used to validate a GitHub-issued download or redirect URL (e.g. a PR-artifact
// archive download, which redirects to a pre-signed, unauthenticated blob URL) before deciding
// whether it is safe to attach a bearer token to a request against it.
func IsApprovedGitHubDownloadHost(host string) bool {
	defer perf.Track(nil, "github.IsApprovedGitHubDownloadHost")()

	repo := RepoEndpoints()
	toolchain := ToolchainEndpoints()
	return repo.IsHost(host) || repo.IsAPIHost(host) || repo.IsUploadHost(host) ||
		toolchain.IsHost(host) || toolchain.IsAPIHost(host) || toolchain.IsUploadHost(host)
}

// NewScopedTokenHTTPClient returns an *http.Client, honoring timeout, that attaches
// "Authorization: Bearer <token>" to a request only when -- re-evaluated on every hop of an
// automatic redirect, not decided once up front -- the request's own URL is https and its host
// matches allowed's server, API, or upload host. Token is passed through TokenForEndpoints
// first, so an empty result (allowed's own API host is not https) means no request ever
// carries a token, matching newGitHubClientForEndpoints' existing behavior for that case.
//
// Centralizes the fix for a class of bug where a token is decided once from an initial
// URL/host, then a redirect (cross-host, or the same host downgraded from https to http) sends
// it somewhere it was never scoped to: pkg/ci/artifact/github/store.go, pkg/ci/cache/github,
// and pkg/github's own GitHub API client all build their *http.Client this way.
func NewScopedTokenHTTPClient(token string, allowed Endpoints, timeout time.Duration) *http.Client {
	defer perf.Track(nil, "github.NewScopedTokenHTTPClient")()

	return &http.Client{
		Timeout: timeout,
		Transport: &scopedTokenTransport{
			base:    http.DefaultTransport,
			token:   TokenForEndpoints(allowed, token),
			allowed: allowed,
		},
	}
}
