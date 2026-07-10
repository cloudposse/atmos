// Package ocitest provides an in-process OCI registry for tests, so tests
// never depend on real network access or a real container registry.
package ocitest

import (
	"archive/tar"
	"bytes"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
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

	srv := httptest.NewServer(registry.New())
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "http://")

	tarBytes := buildTar(t, files)
	layer, err := tarball.LayerFromOpener(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(tarBytes)), nil
	})
	require.NoError(t, err)

	img, err := mutate.AppendLayers(empty.Image, layer)
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
