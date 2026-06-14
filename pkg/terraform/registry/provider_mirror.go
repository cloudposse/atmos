package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/cloudposse/atmos/pkg/http/proxy"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// providerCoord identifies a provider by registry host, namespace, and type.
type providerCoord struct {
	host      string
	namespace string
	typ       string
}

// ErrInvalidProviderPath indicates a provider mirror request path that does not
// match the network-mirror protocol shape.
var ErrInvalidProviderPath = errors.New("invalid provider mirror request path")

const (
	contentTypeJSON = "application/json"
	contentTypeZip  = "application/zip"
)

// ProviderMirror implements proxy.Mirror for the Provider Network Mirror Protocol,
// translating it into the upstream Provider Registry Protocol.
type ProviderMirror struct {
	client proxy.Doer
	disc   *discovery
}

// NewProviderMirror constructs a provider mirror that uses client for upstream
// pre-flight calls (discovery, download resolution).
func NewProviderMirror(client proxy.Doer) *ProviderMirror {
	defer perf.Track(nil, "registry.NewProviderMirror")()

	return &ProviderMirror{client: client, disc: newDiscovery()}
}

// Handles reports whether the request is a provider network-mirror request.
func (m *ProviderMirror) Handles(r *http.Request) bool {
	defer perf.Track(nil, "registry.ProviderMirror.Handles")()

	return strings.HasPrefix(r.URL.Path, providerPrefix)
}

// registry protocol response shapes.
type registryVersions struct {
	Versions []struct {
		Version   string `json:"version"`
		Platforms []struct {
			OS   string `json:"os"`
			Arch string `json:"arch"`
		} `json:"platforms"`
	} `json:"versions"`
}

type registryDownload struct {
	Filename    string `json:"filename"`
	DownloadURL string `json:"download_url"`
	Shasum      string `json:"shasum"`
}

// network-mirror response shapes.
type mirrorIndex struct {
	Versions map[string]struct{} `json:"versions"`
}

type mirrorArchive struct {
	URL    string   `json:"url"`
	Hashes []string `json:"hashes,omitempty"`
}

type mirrorVersion struct {
	Archives map[string]mirrorArchive `json:"archives"`
}

// Route maps a provider network-mirror request to a Route.
func (m *ProviderMirror) Route(r *http.Request) (proxy.Route, error) {
	defer perf.Track(nil, "registry.ProviderMirror.Route")()

	coord, leaf, err := splitProviderPath(r.URL.Path)
	if err != nil {
		return proxy.Route{}, err
	}

	fetch := httpFetcher(m.client, r)

	switch {
	case leaf == "index.json":
		return m.routeIndex(coord), nil
	case strings.HasSuffix(leaf, ".zip"):
		return m.routeArchive(r.Context(), fetch, coord, leaf)
	case strings.HasSuffix(leaf, ".json"):
		version := strings.TrimSuffix(leaf, ".json")
		return m.routeVersion(coord, version), nil
	default:
		return proxy.Route{}, fmt.Errorf("%w: %s", ErrInvalidProviderPath, r.URL.Path)
	}
}

// routeIndex fetches the registry versions list and rewrites it into the
// network-mirror index.json shape.
func (m *ProviderMirror) routeIndex(c providerCoord) proxy.Route {
	return proxy.Route{
		Key:         providerIndexKey(c.host, c.namespace, c.typ),
		Kind:        proxy.KindMetadata,
		ContentType: contentTypeJSON,
		Produce: func(ctx context.Context, fetch proxy.Fetcher, _ string) ([]byte, string, error) {
			svc := m.disc.resolve(ctx, fetch, c.host)
			versions, err := fetchJSON[registryVersions](ctx, fetch, svc.providersV1+c.namespace+"/"+c.typ+"/versions")
			if err != nil {
				return nil, "", err
			}
			idx := mirrorIndex{Versions: make(map[string]struct{}, len(versions.Versions))}
			for _, v := range versions.Versions {
				idx.Versions[v.Version] = struct{}{}
			}
			b, err := json.Marshal(idx)
			return b, contentTypeJSON, err
		},
	}
}

// routeVersion builds the network-mirror <version>.json by resolving every
// platform's download metadata (filename + zh: hash).
func (m *ProviderMirror) routeVersion(c providerCoord, version string) proxy.Route {
	return proxy.Route{
		Key:         providerVersionKey(c.host, c.namespace, c.typ, version),
		Kind:        proxy.KindMetadata,
		ContentType: contentTypeJSON,
		Produce: func(ctx context.Context, fetch proxy.Fetcher, _ string) ([]byte, string, error) {
			svc := m.disc.resolve(ctx, fetch, c.host)
			versions, err := fetchJSON[registryVersions](ctx, fetch, svc.providersV1+c.namespace+"/"+c.typ+"/versions")
			if err != nil {
				return nil, "", err
			}
			platforms := platformsForVersion(versions, version)
			archives := map[string]mirrorArchive{}
			for _, p := range platforms {
				dlURL := fmt.Sprintf("%s%s/%s/%s/download/%s/%s", svc.providersV1, c.namespace, c.typ, version, p.OS, p.Arch)
				dl, derr := fetchJSON[registryDownload](ctx, fetch, dlURL)
				if derr != nil {
					// Skip a platform that fails to resolve; the one Terraform needs surfaces in its own request.
					log.Debug("Provider mirror: skipping platform", "provider", c.typ, "version", version, "os", p.OS, "arch", p.Arch, "error", derr)
					continue
				}
				entry := mirrorArchive{URL: dl.Filename}
				if dl.Shasum != "" {
					entry.Hashes = []string{"zh:" + dl.Shasum}
				}
				archives[p.OS+"_"+p.Arch] = entry
			}
			b, err := json.Marshal(mirrorVersion{Archives: archives})
			return b, contentTypeJSON, err
		},
	}
}

// registryPlatform is a single os/arch a provider version supports.
type registryPlatform struct {
	OS   string
	Arch string
}

// platformsForVersion returns the platforms a specific version advertises.
func platformsForVersion(versions registryVersions, version string) []registryPlatform {
	for _, v := range versions.Versions {
		if v.Version != version {
			continue
		}
		out := make([]registryPlatform, 0, len(v.Platforms))
		for _, p := range v.Platforms {
			out = append(out, registryPlatform{OS: p.OS, Arch: p.Arch})
		}
		return out
	}
	return nil
}

// routeArchive resolves the real provider download URL (and shasum) and streams the
// zip through the cache, verifying its zh: hash.
func (m *ProviderMirror) routeArchive(ctx context.Context, fetch proxy.Fetcher, c providerCoord, filename string) (proxy.Route, error) {
	ref, ok := parseProviderArchive(filename)
	if !ok {
		return proxy.Route{}, fmt.Errorf("%w: unparseable provider archive %q", ErrInvalidProviderPath, filename)
	}

	svc := m.disc.resolve(ctx, fetch, c.host)
	dlURL := fmt.Sprintf("%s%s/%s/%s/download/%s/%s", svc.providersV1, c.namespace, c.typ, ref.version, ref.os, ref.arch)
	dl, err := fetchJSON[registryDownload](ctx, fetch, dlURL)
	if err != nil {
		return proxy.Route{}, err
	}
	if dl.DownloadURL == "" {
		return proxy.Route{}, fmt.Errorf("%w: no download_url for %s/%s %s %s_%s", ErrInvalidProviderPath, c.namespace, c.typ, ref.version, ref.os, ref.arch)
	}

	return proxy.Route{
		Key:         providerArchiveKey(c.host, c.namespace, c.typ, filename),
		Kind:        proxy.KindArtifact,
		Upstream:    proxy.UpstreamRequest{URL: dl.DownloadURL},
		ContentType: contentTypeZip,
		Verify:      zhVerify(dl.Shasum),
	}, nil
}

// zhVerify returns a verifier that checks the downloaded zip's SHA-256 against the
// registry-provided shasum (the zh: hash). When shasum is empty, verification is
// skipped.
func zhVerify(shasum string) func(sha256Hex string) error {
	if shasum == "" {
		return nil
	}
	return func(sha256Hex string) error {
		if !strings.EqualFold(sha256Hex, shasum) {
			return fmt.Errorf("%w: provider zip sha256 %s does not match registry shasum %s", ErrUpstreamStatus, sha256Hex, shasum)
		}
		return nil
	}
}

// splitProviderPath parses /providers/<host>/<namespace>/<type>/<leaf>.
func splitProviderPath(p string) (providerCoord, string, error) {
	rest := strings.TrimPrefix(p, providerPrefix)
	parts := strings.Split(rest, "/")
	if len(parts) != providerPathParts || parts[0] == "" || parts[1] == "" || parts[2] == "" || parts[3] == "" {
		return providerCoord{}, "", fmt.Errorf("%w: %s", ErrInvalidProviderPath, p)
	}
	return providerCoord{host: parts[0], namespace: parts[1], typ: parts[2]}, parts[3], nil
}

// providerPathParts is the number of slash-separated segments in a provider
// network-mirror path: <host>/<namespace>/<type>/<leaf>.
const providerPathParts = 4
