package registry

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/http/proxy"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ErrInvalidModulePath indicates a module mirror request path that does not match
// the module registry protocol shape.
var ErrInvalidModulePath = errors.New("invalid module mirror request path")

// ErrModuleSourceFetch indicates a module source could not be resolved and packed.
var ErrModuleSourceFetch = errors.New("module source fetch failed")

const (
	xTerraformGetHeader = "X-Terraform-Get"
	contentTypeTarGz    = "application/gzip"
	// The moduleSourceParts is the segment count of the cached-source sub-route
	// /modules/_source/<encoded>.
	moduleSourceParts = 2
	// The minModulePathParts is the minimum segment count of a module registry path
	// (<host>/<ns>/<name>/<provider>/<endpoint>).
	minModulePathParts = 5
)

// SourceResolver resolves a go-getter source string (e.g.
// "git::https://github.com/org/repo.git?ref=v1") into destDir, populating it with the
// module's working tree. The production implementation wraps pkg/downloader's
// go-getter client (with its insteadOf/credential-broker plumbing); tests inject a
// fake so no network is touched.
type SourceResolver interface {
	Resolve(ctx context.Context, source, destDir string) error
}

// ModuleMirror implements proxy.Mirror for the module registry protocol. It caches
// version listings and download resolutions, and routes every module download back
// through the proxy: the source is resolved with go-getter and cached as a single tar
// artifact, so git:: sources (the common case for the public registry and mono-repos)
// are cached just like HTTP archives.
type ModuleMirror struct {
	resolver SourceResolver
}

// NewModuleMirror constructs a module mirror that resolves module sources via resolver.
func NewModuleMirror(resolver SourceResolver) *ModuleMirror {
	defer perf.Track(nil, "registry.NewModuleMirror")()

	return &ModuleMirror{resolver: resolver}
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

	// Cached source sub-route: /modules/_source/<base64url(base-source)>.tar. The
	// Terraform-side go-getter strips any //subdir before this GET (it extracts the
	// subdir locally after unpacking) and detects the tar from the .tar suffix, so only
	// the two-segment, extension-tagged form reaches the proxy.
	if len(parts) == moduleSourceParts && parts[0] == moduleSourceSegment {
		return m.routeSource(strings.TrimSuffix(parts[1], sourceExt))
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

// downloadResolution is the cached representation of a module download resolution: the
// upstream source string and which protocol form carried it. The source-to-proxy
// rewrite is deferred to serve time (see routeDownload), so the cached value stays
// valid across runs even though the proxy binds a different ephemeral port each run.
type downloadResolution struct {
	Source    string `json:"source"`
	ViaHeader bool   `json:"via_header"`
}

// routeDownload caches a module's download resolution and serves it rewritten to route
// the source through the proxy's _source sub-route. The download protocol has two forms
// in the wild: HashiCorp's registry returns 204 + X-Terraform-Get, while OpenTofu's
// static registry returns 200 + a JSON {"location": "..."} body. The mirror extracts
// the source from whichever form the upstream used, caches it (immutable for a released
// version, so a warm cache resolves with no upstream call), and re-emits the same form
// at serve time with the source rewritten to the current run's _source URL.
func (m *ModuleMirror) routeDownload(host, namespace, name, provider, version string) proxy.Route {
	endpoint := m.endpoint(host, namespace, name, provider, version, "download")
	return proxy.Route{
		Key:         moduleDownloadKey(host, namespace, name, provider, version),
		Kind:        proxy.KindMetadata,
		ContentType: contentTypeJSON,
		Produce: func(ctx context.Context, fetch proxy.Fetcher, _ string) ([]byte, string, error) {
			resp, err := fetch(ctx, proxy.UpstreamRequest{URL: endpoint})
			if err != nil {
				return nil, "", err
			}
			defer resp.Body.Close()

			source, viaHeader, err := extractModuleSource(resp)
			if err != nil {
				return nil, "", err
			}
			payload, err := json.Marshal(downloadResolution{Source: source, ViaHeader: viaHeader})
			if err != nil {
				return nil, "", err
			}
			return payload, contentTypeJSON, nil
		},
		Serve: serveDownloadResolution,
	}
}

// extractModuleSource reads the module source from a download response, supporting both
// the X-Terraform-Get header form (HashiCorp) and the JSON {"location": "..."} body form
// (OpenTofu's static registry).
func extractModuleSource(resp *http.Response) (source string, viaHeader bool, err error) {
	if h := resp.Header.Get(xTerraformGetHeader); h != "" {
		_, _ = io.Copy(io.Discard, resp.Body)
		return h, true, nil
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false, err
	}
	var payload struct {
		Location string `json:"location"`
	}
	if jerr := json.Unmarshal(raw, &payload); jerr == nil && payload.Location != "" {
		return payload.Location, false, nil
	}
	return "", false, fmt.Errorf("%w: download response carried neither an %s header nor a JSON location", ErrInvalidModulePath, xTerraformGetHeader)
}

// serveDownloadResolution renders a cached download resolution, rewriting the source to
// the current run's _source URL and re-emitting it in the same protocol form the
// upstream used.
func serveDownloadResolution(w http.ResponseWriter, body io.Reader, proxyBaseURL string) error {
	var res downloadResolution
	if err := json.NewDecoder(body).Decode(&res); err != nil {
		return err
	}

	base, subdir := splitModuleSource(res.Source)
	rewritten := moduleSourceProxyURL(proxyBaseURL, base, subdir)

	if res.ViaHeader {
		w.Header().Set(xTerraformGetHeader, rewritten)
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(map[string]string{"location": rewritten})
}

// routeSource resolves and caches a module source whose base go-getter string is
// carried (base64url-encoded) in the request path. The resolved working tree is packed
// into a single uncompressed tar and stored as an immutable artifact.
func (m *ModuleMirror) routeSource(encoded string) (proxy.Route, error) {
	base, err := decodeModuleSource(encoded)
	if err != nil {
		return proxy.Route{}, err
	}
	return proxy.Route{
		Key:         moduleSourceKey(moduleSourceDigest(base)),
		Kind:        proxy.KindArtifact,
		ContentType: contentTypeTarGz,
		ProduceArtifact: func(ctx context.Context) (io.ReadCloser, string, error) {
			return m.produceSource(ctx, base)
		},
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

// produceSource resolves source into a temp dir with go-getter, then streams a tar of
// that tree. The tar is produced on a pipe (no full-archive buffering); the temp dir is
// removed when the proxy closes the returned reader after the commit completes.
func (m *ModuleMirror) produceSource(ctx context.Context, source string) (io.ReadCloser, string, error) {
	defer perf.Track(nil, "registry.ModuleMirror.produceSource")()

	dir, err := os.MkdirTemp("", "atmos-tf-modsrc-")
	if err != nil {
		return nil, "", fmt.Errorf("%w: creating temp dir: %w", ErrModuleSourceFetch, err)
	}
	if rerr := m.resolver.Resolve(ctx, source, dir); rerr != nil {
		_ = os.RemoveAll(dir)
		return nil, "", fmt.Errorf("%w: %s: %w", ErrModuleSourceFetch, source, rerr)
	}

	pr, pw := io.Pipe()
	go func() {
		pw.CloseWithError(tarGzDir(dir, pw))
	}()

	return &cleanupReadCloser{
		reader: pr,
		closeFn: func() error {
			_ = pr.Close()
			return os.RemoveAll(dir)
		},
	}, contentTypeTarGz, nil
}

// endpoint builds an upstream modules.v1 URL. Discovery is intentionally skipped for
// the standard path (modules.v1 is conventionally /v1/modules/); the conventional
// host base is used directly to avoid a discovery round-trip per request.
func (m *ModuleMirror) endpoint(host, namespace, name, provider string, parts ...string) string {
	segments := append([]string{namespace, name, provider}, parts...)
	return fmt.Sprintf("https://%s/v1/modules/%s", host, strings.Join(segments, "/"))
}

// tarGzDir writes a gzip-compressed tar of root's contents (entries relative to root) to
// w, preserving file modes and symlinks so the module extracts faithfully. Gzip (rather
// than a bare tar) is required: the Terraform-side go-getter only recognizes the
// compressed forms (.tar.gz/.tgz/.zip), not a plain .tar.
func tarGzDir(root string, w io.Writer) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		return writeTarEntry(tw, root, path, d)
	})
	if walkErr != nil {
		_ = tw.Close()
		_ = gz.Close()
		return walkErr
	}
	if err := tw.Close(); err != nil {
		_ = gz.Close()
		return err
	}
	return gz.Close()
}

// writeTarEntry writes a single filesystem entry (file, dir, or symlink) to tw, keyed
// by its path relative to root.
func writeTarEntry(tw *tar.Writer, root, path string, d fs.DirEntry) error {
	info, err := d.Info()
	if err != nil {
		return err
	}

	link := ""
	if info.Mode()&fs.ModeSymlink != 0 {
		if link, err = os.Readlink(path); err != nil {
			return err
		}
	}

	hdr, err := tar.FileInfoHeader(info, link)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	hdr.Name = filepath.ToSlash(rel)
	if d.IsDir() {
		hdr.Name += "/"
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}

	if !info.Mode().IsRegular() {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(tw, f)
	return err
}

// cleanupReadCloser wraps a reader with a Close that also runs cleanup (removing the
// temp source dir once the tar has been fully read and committed).
type cleanupReadCloser struct {
	reader  io.Reader
	closeFn func() error
}

//nolint:lintroller // Trivial io.Reader delegator on a hot copy path - no perf tracking.
func (c *cleanupReadCloser) Read(p []byte) (int, error) { return c.reader.Read(p) }

//nolint:lintroller // Trivial io.Closer delegator - no perf tracking.
func (c *cleanupReadCloser) Close() error { return c.closeFn() }
