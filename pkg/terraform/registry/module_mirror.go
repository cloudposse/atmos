package registry

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/cloudposse/atmos/pkg/http/proxy"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ErrInvalidModulePath indicates a module mirror request path that does not match
// the module registry protocol shape.
var ErrInvalidModulePath = errors.New("invalid module mirror request path")

const (
	xTerraformGetHeader = "X-Terraform-Get"
	// The moduleArchiveParts is the segment count of the cached-archive sub-route
	// /modules/_archive/<encoded>.
	moduleArchiveParts = 2
	// The minModulePathParts is the minimum segment count of a module registry path
	// (<host>/<ns>/<name>/<provider>/<endpoint>).
	minModulePathParts = 5
	// The moduleArchiveDigestLen is the hex-digest length used to key cached module archives.
	moduleArchiveDigestLen = 32
)

// ModuleMirror implements proxy.Mirror for the module registry protocol. It caches
// version listings, redirects HTTP-archive module downloads back through the proxy
// for caching, and passes git:: sources through unchanged.
type ModuleMirror struct{}

// NewModuleMirror constructs a module mirror.
func NewModuleMirror() *ModuleMirror {
	defer perf.Track(nil, "registry.NewModuleMirror")()

	return &ModuleMirror{}
}

// Handles reports whether the request is a module registry request.
func (m *ModuleMirror) Handles(r *http.Request) bool {
	defer perf.Track(nil, "registry.ModuleMirror.Handles")()

	return strings.HasPrefix(r.URL.Path, modulePrefix)
}

// Route maps a module registry request to a Route.
func (m *ModuleMirror) Route(r *http.Request) (proxy.Route, error) {
	defer perf.Track(nil, "registry.ModuleMirror.Route")()

	rest := strings.TrimPrefix(r.URL.Path, modulePrefix)
	parts := strings.Split(rest, "/")

	// Cached HTTP-archive sub-route: /modules/_archive/<base64url(originalURL)>.
	if len(parts) == moduleArchiveParts && parts[0] == moduleArchiveSegment {
		return m.routeArchive(parts[1])
	}

	if len(parts) < minModulePathParts {
		return proxy.Route{}, fmt.Errorf("%w: %s", ErrInvalidModulePath, r.URL.Path)
	}
	host, namespace, name, provider := parts[0], parts[1], parts[2], parts[3]
	tail := parts[4:]

	// Version listing: /modules/<host>/<ns>/<name>/<provider>/versions.
	if len(tail) == 1 && tail[0] == "versions" {
		return m.routeVersions(host, namespace, name, provider), nil
	}

	// Download resolution: /modules/<host>/<ns>/<name>/<provider>/<version>/download.
	if len(tail) == 2 && tail[1] == "download" {
		return m.routeDownload(host, namespace, name, provider, tail[0]), nil
	}

	// Any other module endpoint passes through to the upstream registry verbatim.
	return m.routePassthrough(host, namespace, name, provider, tail), nil
}

// routeVersions caches the module version listing verbatim.
func (m *ModuleMirror) routeVersions(host, namespace, name, provider string) proxy.Route {
	return proxy.Route{
		Key:         moduleVersionsKey(host, namespace, name, provider),
		Kind:        proxy.KindMetadata,
		ContentType: contentTypeJSON,
		Upstream: proxy.UpstreamRequest{
			URL: m.endpoint(host, namespace, name, provider, "versions"),
		},
	}
}

// routeDownload resolves a module download. The upstream returns 204 + X-Terraform-Get;
// HTTP-archive sources are rewritten to route the archive back through the proxy
// (and get cached), while git:: sources pass through unchanged.
func (m *ModuleMirror) routeDownload(host, namespace, name, provider, version string) proxy.Route {
	return proxy.Route{
		Kind: proxy.KindPassthrough,
		Upstream: proxy.UpstreamRequest{
			URL: m.endpoint(host, namespace, name, provider, version, "download"),
		},
		HeaderRewrite: func(h http.Header, proxyBase string) {
			got := h.Get(xTerraformGetHeader)
			if archiveURL, ok := classifyXTerraformGet(got); ok {
				encoded := base64.RawURLEncoding.EncodeToString([]byte(archiveURL))
				h.Set(xTerraformGetHeader, proxyBase+strings.TrimPrefix(modulePrefix, "/")+moduleArchiveSegment+"/"+encoded)
			}
		},
	}
}

// routeArchive serves and caches an HTTP-archive module whose original URL is
// carried (base64url-encoded) in the request path.
func (m *ModuleMirror) routeArchive(encoded string) (proxy.Route, error) {
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return proxy.Route{}, fmt.Errorf("%w: undecodable archive ref: %w", ErrInvalidModulePath, err)
	}
	originalURL := string(raw)
	sum := sha256.Sum256([]byte(originalURL))
	digest := hex.EncodeToString(sum[:])[:moduleArchiveDigestLen]

	return proxy.Route{
		Key:      moduleArchiveKey(digest),
		Kind:     proxy.KindArtifact,
		Upstream: proxy.UpstreamRequest{URL: originalURL},
	}, nil
}

// routePassthrough forwards any other module endpoint to the upstream registry.
func (m *ModuleMirror) routePassthrough(host, namespace, name, provider string, tail []string) proxy.Route {
	segments := append([]string{namespace, name, provider}, tail...)
	return proxy.Route{
		Kind: proxy.KindPassthrough,
		Upstream: proxy.UpstreamRequest{
			URL: fmt.Sprintf("https://%s/v1/modules/%s", host, strings.Join(segments, "/")),
		},
	}
}

// endpoint builds an upstream modules.v1 URL. Discovery is intentionally skipped for
// the standard path (modules.v1 is conventionally /v1/modules/); the conventional
// host base is used directly to avoid a discovery round-trip per request.
func (m *ModuleMirror) endpoint(host, namespace, name, provider string, parts ...string) string {
	segments := append([]string{namespace, name, provider}, parts...)
	return fmt.Sprintf("https://%s/v1/modules/%s", host, strings.Join(segments, "/"))
}
