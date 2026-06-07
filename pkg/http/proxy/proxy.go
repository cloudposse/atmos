// Package proxy is a generic, protocol-agnostic caching HTTP proxy. It binds an
// ephemeral loopback listener, routes each request through a pluggable Mirror to
// a cache key + upstream request, and either serves a cache hit or fetches the
// object upstream once (no retries — the caller owns retry), verifies it, stores
// it atomically, and serves it.
//
// The package deliberately knows nothing about Terraform: the Terraform provider
// and module registry mirrors are adapters that implement Mirror. It is named for
// proxying, not caching, so a future git mirror can reuse the same infrastructure.
package proxy

import (
	"context"
	"io"
	"net/http"

	"github.com/cloudposse/atmos/pkg/perf"
)

// ArtifactKind classifies a routed object for stats and freshness policy.
type ArtifactKind int

const (
	// KindMetadata is registry metadata (version listings, download resolution).
	// It honors metadata_ttl with stale-while-revalidate.
	KindMetadata ArtifactKind = iota
	// KindArtifact is immutable content (provider zips, module archives). Once
	// cached it never expires.
	KindArtifact
	// KindPassthrough is streamed straight through and never cached (e.g. a git::
	// module download that the HTTP proxy cannot cache).
	KindPassthrough
)

// String renders the kind for sidecar metadata and reporting.
func (k ArtifactKind) String() string {
	defer perf.Track(nil, "proxy.ArtifactKind.String")()

	switch k {
	case KindMetadata:
		return "metadata"
	case KindArtifact:
		return "artifact"
	case KindPassthrough:
		return "passthrough"
	default:
		return "unknown"
	}
}

// Doer performs an HTTP request. Implemented by pkg/http.Client and *http.Client;
// injectable so tests can stub upstream.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// UpstreamRequest describes the upstream fetch for a route. The proxy builds and
// executes the actual *http.Request, applying credential, header, and User-Agent
// propagation centrally (see propagation.go) so private registries and the Atmos
// User-Agent are handled uniformly across all mirrors.
type UpstreamRequest struct {
	// URL is the absolute upstream URL to fetch.
	URL string
	// Method defaults to GET when empty.
	Method string
	// Header carries mirror-supplied headers (e.g. Accept). The proxy merges
	// credential and User-Agent headers on top, never dropping the Atmos UA.
	Header http.Header
}

// Fetcher performs a propagated upstream fetch: it builds the request with the
// same credential/header/User-Agent propagation the proxy applies, then executes
// it. Mirrors use it inside Produce to compose multiple upstream calls (e.g.
// translating the provider registry protocol into a network-mirror response).
type Fetcher func(ctx context.Context, up UpstreamRequest) (*http.Response, error)

// Route is a Mirror's decision for a single inbound request.
type Route struct {
	// Key is the canonical cache key, which is also the backend object name. Empty
	// for KindPassthrough.
	Key string
	// Kind classifies the object for caching and freshness.
	Kind ArtifactKind
	// Upstream is the single upstream fetch performed on a miss (or always, for
	// passthrough). Ignored when Produce is set.
	Upstream UpstreamRequest
	// Produce, when non-nil, composes the (KindMetadata) response from one or more
	// upstream calls via the provided Fetcher — used when a single Upstream+Rewrite
	// cannot express the translation (e.g. building a provider <version>.json from
	// per-platform registry download calls). Returns the body and its content type.
	Produce func(ctx context.Context, fetch Fetcher, proxyBaseURL string) (body []byte, contentType string, err error)
	// Rewrite, when non-nil, post-processes a KindMetadata body before it is cached
	// and served — e.g. rewriting provider download URLs back through the proxy.
	// proxyBaseURL is the proxy's own base URL ("http://127.0.0.1:<port>/").
	Rewrite func(body []byte, proxyBaseURL string) ([]byte, error)
	// Verify, when non-nil, checks artifact integrity before commit. It receives the
	// hex-encoded SHA-256 of the downloaded bytes (sufficient to verify a provider
	// zip's zh: hash). Returning an error rejects the object.
	Verify func(sha256Hex string) error
	// HeaderRewrite, when non-nil on a KindPassthrough route, may modify the upstream
	// response headers before they are written back — e.g. rewriting a module's
	// HTTP-archive X-Terraform-Get to route the archive back through the proxy while
	// leaving git:: sources untouched.
	HeaderRewrite func(h http.Header, proxyBaseURL string)
	// ContentType for the served response. Falls back to the upstream Content-Type.
	ContentType string
}

// Mirror maps an inbound proxy request to a Route. Adapters (provider/module
// registry mirrors, a future git mirror) implement it.
type Mirror interface {
	// Handles reports whether this mirror owns the request path.
	Handles(r *http.Request) bool
	// Route computes the cache key, upstream request, kind, and rewrite/verify hooks.
	Route(r *http.Request) (Route, error)
}

// upstreamResult is the outcome of an upstream fetch, used internally by the flow.
type upstreamResult struct {
	body        io.ReadCloser
	statusCode  int
	header      http.Header
	contentType string
}
