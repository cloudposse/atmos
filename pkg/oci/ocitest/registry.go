// Package ocitest provides an in-process OCI registry for tests, so tests
// never depend on real network access or a real container registry.
package ocitest

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/perf"
)

// NewRegistry starts an in-process OCI registry serving a single image built
// from files (archive path -> content, one uncompressed-tar layer). It
// returns the bare image reference "127.0.0.1:PORT/repoTag" (no "oci://"
// prefix, no scheme) -- callers building a component source.uri prefix it
// with "oci://" themselves. The go-containerregistry name.ParseReference
// function treats "127.0.0.1:PORT" addresses as plain HTTP automatically (see
// go-containerregistry's pkg/name/registry.go reLoopback), so no Insecure
// option is needed. The registry's httptest.Server is closed automatically
// via t.Cleanup.
func NewRegistry(t *testing.T, repoTag string, files map[string]string) string {
	defer perf.Track(nil, "ocitest.NewRegistry")()

	t.Helper()

	tarBytes := buildTar(t, files)
	layer, err := tarball.LayerFromOpener(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(tarBytes)), nil
	})
	require.NoError(t, err)

	return pushImage(t, repoTag, layer)
}

// NewZipRegistry starts an in-process OCI registry serving a single image
// whose sole layer is a ZIP archive with media type "archive/zip" -- the
// format OpenTofu's native "install modules from OCI registries" feature uses
// (see https://opentofu.org, artifactType application/vnd.opentofu.modulepkg).
// Unlike NewRegistry's layer, this one is not gzip-wrapped: the blob is the
// raw zip bytes, matching what real OpenTofu module-package registries serve.
func NewZipRegistry(t *testing.T, repoTag string, files map[string]string) string {
	defer perf.Track(nil, "ocitest.NewZipRegistry")()

	t.Helper()

	zipBytes := buildZip(t, files)
	layer := static.NewLayer(zipBytes, "archive/zip")

	return pushImage(t, repoTag, layer)
}

// NewEmptyRegistry starts an in-process OCI registry serving an image with no
// layers at all. It exercises the "manifest resolves but has zero layers"
// path (distinct from a network/auth failure), which real registries can
// legitimately serve for an empty or config-only image.
func NewEmptyRegistry(t *testing.T, repoTag string) string {
	defer perf.Track(nil, "ocitest.NewEmptyRegistry")()

	t.Helper()

	return pushImage(t, repoTag)
}

// NewBrokenLayerRegistry starts an in-process OCI registry serving an image
// whose single layer declares the default tar+gzip media type but is not
// valid gzip data. The blob starts with the real gzip magic header
// (0x1f 0x8b) so go-containerregistry's compression sniffing selects the
// gzip decoder rather than treating the blob as an already-uncompressed tar
// (which would instead fail during tar parsing, not decompression); the
// remaining bytes are deliberately malformed so gzip.NewReader fails on the
// very first read, making the error deterministic and non-retryable.
func NewBrokenLayerRegistry(t *testing.T, repoTag string) string {
	defer perf.Track(nil, "ocitest.NewBrokenLayerRegistry")()

	t.Helper()

	brokenGzip := append([]byte{0x1f, 0x8b}, []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}...)
	layer := static.NewLayer(brokenGzip, "application/vnd.oci.image.layer.v1.tar+gzip")

	return pushImage(t, repoTag, layer)
}

// pushImage starts an in-process OCI registry and writes an image built from
// zero or more layers, returning the bare image reference
// "127.0.0.1:PORT/repoTag". Zero layers pushes empty.Image unmodified, giving
// callers a real, resolvable manifest with no layers.
func pushImage(t *testing.T, repoTag string, layers ...v1.Layer) string {
	t.Helper()

	srv := httptest.NewServer(registry.New())
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "http://")

	img, err := mutate.AppendLayers(empty.Image, layers...)
	require.NoError(t, err)

	imageRef := host + "/" + repoTag
	ref, err := name.ParseReference(imageRef)
	require.NoError(t, err)
	require.NoError(t, remote.Write(ref, img))

	return imageRef
}

// buildTar writes files into an in-memory uncompressed tar archive.
func buildTar(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for path, content := range files {
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name: path,
			Mode: 0o644,
			Size: int64(len(content)),
		}))
		_, err := tw.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	return buf.Bytes()
}

// buildZip writes files into an in-memory zip archive.
func buildZip(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for path, content := range files {
		w, err := zw.Create(path)
		require.NoError(t, err)
		_, err = w.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// Credential is the Basic-auth identity an authenticated test registry accepts.
type Credential struct{ Username, Password string }

// Server is an in-process registry that can be shut down independently of the test.
type Server struct {
	// Host is the "127.0.0.1:PORT" registry address.
	Host   string
	server *httptest.Server
}

// ServerOption customizes a test registry.
type ServerOption func(*serverConfig)

type serverConfig struct {
	tagsFollowManifestDelete bool
	referrers                bool
}

// WithTagsFollowManifestDelete makes deleting a manifest by digest also delete every tag that
// resolves to it, like a production registry, where a tag whose manifest is gone no longer
// resolves. The bare in-process registry keeps such tags, which would hide a delete that is not
// tag-safe.
//
//nolint:lintroller // Functional option only.
func WithTagsFollowManifestDelete() ServerOption {
	return func(config *serverConfig) { config.tagsFollowManifestDelete = true }
}

// WithoutReferrersAPI turns off the OCI 1.1 referrers API, so clients must use the referrers
// tag-schema fallback.
//
//nolint:lintroller // Functional option only.
func WithoutReferrersAPI() ServerOption {
	return func(config *serverConfig) { config.referrers = false }
}

// NewServer starts an empty in-process OCI registry (OCI 1.1 referrers included). When
// credential is non-nil, every request must carry exactly that Basic identity; any other
// request receives a 401 challenge, so a client that downgraded to anonymous or reused
// another registry's credentials fails visibly.
func NewServer(t *testing.T, credential *Credential) *Server {
	defer perf.Track(nil, "ocitest.NewServer")()

	t.Helper()

	return NewServerWithOptions(t, credential)
}

// NewServerWithOptions is NewServer with registry behavior options.
func NewServerWithOptions(t *testing.T, credential *Credential, options ...ServerOption) *Server {
	defer perf.Track(nil, "ocitest.NewServerWithOptions")()

	t.Helper()

	config := serverConfig{referrers: true}
	for _, option := range options {
		option(&config)
	}
	handler := registry.New(registry.Logger(log.New(io.Discard, "", 0)), registry.WithReferrersSupport(config.referrers))
	if config.tagsFollowManifestDelete {
		handler = tagsFollowDelete{next: handler}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if credential != nil {
			user, password, ok := r.BasicAuth()
			if !ok || user != credential.Username || password != credential.Password {
				w.Header().Set("WWW-Authenticate", `Basic realm="registry"`)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return &Server{Host: strings.TrimPrefix(srv.URL, "http://"), server: srv}
}

// tagsFollowDelete deletes the tags of a manifest together with the manifest itself.
type tagsFollowDelete struct{ next http.Handler }

//nolint:lintroller // Test-registry request wrapper.
func (d tagsFollowDelete) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	repository, reference, isManifest := splitManifestPath(r.URL.Path)
	if r.Method != http.MethodDelete || !isManifest || !strings.HasPrefix(reference, "sha256:") {
		d.next.ServeHTTP(w, r)
		return
	}
	tags := httptest.NewRecorder()
	d.next.ServeHTTP(tags, httptest.NewRequest(http.MethodGet, "/v2/"+repository+"/tags/list", nil))
	var listing struct {
		Tags []string `json:"tags"`
	}
	_ = json.Unmarshal(tags.Body.Bytes(), &listing)
	for _, tag := range listing.Tags {
		head := httptest.NewRecorder()
		d.next.ServeHTTP(head, httptest.NewRequest(http.MethodHead, "/v2/"+repository+"/manifests/"+tag, nil))
		if head.Header().Get("Docker-Content-Digest") == reference {
			d.next.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodDelete, "/v2/"+repository+"/manifests/"+tag, nil))
		}
	}
	d.next.ServeHTTP(w, r)
}

// splitManifestPath splits /v2/<repository>/manifests/<reference>.
func splitManifestPath(path string) (repository, reference string, ok bool) {
	trimmed := strings.TrimPrefix(path, "/v2/")
	index := strings.LastIndex(trimmed, "/manifests/")
	if trimmed == path || index < 0 {
		return "", "", false
	}
	return trimmed[:index], trimmed[index+len("/manifests/"):], true
}

// Close shuts the registry down now; calling it again is harmless.
func (s *Server) Close() {
	defer perf.Track(nil, "ocitest.Server.Close")()

	s.server.Close()
}
