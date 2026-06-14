package registry

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/http/proxy"
)

// errResolver always fails, to exercise produceSource's resolve-error path.
type errResolver struct{}

func (errResolver) Resolve(context.Context, string, string) error {
	return errors.New("resolve boom")
}

// errReader fails on the first Read, to exercise extractModuleSource's read-error path.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read boom") }

func TestModuleMirror_Route(t *testing.T) {
	m := NewModuleMirror(&fakeResolver{})

	t.Run("passthrough for unrecognized endpoint", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/modules/registry.terraform.io/ns/name/aws/extra/endpoint", nil)
		route, err := m.Route(r)
		require.NoError(t, err)
		assert.Equal(t, proxy.KindPassthrough, route.Kind)
		assert.Equal(t, "https://registry.terraform.io/v1/modules/ns/name/aws/extra/endpoint", route.Upstream.URL)
	})

	t.Run("invalid path with too few segments", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/modules/too/short", nil)
		_, err := m.Route(r)
		require.ErrorIs(t, err, ErrInvalidModulePath)
	})

	t.Run("source sub-route with undecodable encoding", func(t *testing.T) {
		// "@@@" is URL-legal but not valid base64url, so decoding the source fails.
		_, err := m.routeSource("@@@")
		require.Error(t, err)
	})
}

func TestExtractModuleSource(t *testing.T) {
	t.Run("X-Terraform-Get header form", func(t *testing.T) {
		resp := &http.Response{
			Header: http.Header{xTerraformGetHeader: {"git::https://example.com/repo.git?ref=v1"}},
			Body:   io.NopCloser(strings.NewReader("body ignored")),
		}
		src, viaHeader, err := extractModuleSource(resp)
		require.NoError(t, err)
		assert.True(t, viaHeader)
		assert.Equal(t, "git::https://example.com/repo.git?ref=v1", src)
	})

	t.Run("JSON location body form", func(t *testing.T) {
		resp := &http.Response{
			Header: http.Header{},
			Body:   io.NopCloser(strings.NewReader(`{"location":"https://archives.example.com/m.tar.gz"}`)),
		}
		src, viaHeader, err := extractModuleSource(resp)
		require.NoError(t, err)
		assert.False(t, viaHeader)
		assert.Equal(t, "https://archives.example.com/m.tar.gz", src)
	})

	t.Run("neither header nor location is an error", func(t *testing.T) {
		resp := &http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}
		_, _, err := extractModuleSource(resp)
		require.ErrorIs(t, err, ErrInvalidModulePath)
	})

	t.Run("body read failure", func(t *testing.T) {
		resp := &http.Response{Header: http.Header{}, Body: io.NopCloser(errReader{})}
		_, _, err := extractModuleSource(resp)
		require.Error(t, err)
	})
}

func TestServeDownloadResolution_DecodeError(t *testing.T) {
	rec := httptest.NewRecorder()
	err := serveDownloadResolution(rec, strings.NewReader("not json"), "https://proxy.local/")
	require.Error(t, err)
}

func TestProduceSource_ResolveError(t *testing.T) {
	m := NewModuleMirror(errResolver{})
	_, _, err := m.produceSource(context.Background(), "git::https://example.com/repo.git")
	require.ErrorIs(t, err, ErrModuleSourceFetch)
}

func TestProduceSource_TarsAndCleansUp(t *testing.T) {
	resolver := &fakeResolver{files: map[string]string{
		"main.tf":             "# root\n",
		"modules/sub/main.tf": "# sub\n",
	}}
	m := NewModuleMirror(resolver)

	rc, contentType, err := m.produceSource(context.Background(), "git::https://example.com/repo.git?ref=v1")
	require.NoError(t, err)
	assert.Equal(t, contentTypeTarGz, contentType)

	files := readGzTar(t, rc) // exercises cleanupReadCloser.Read + tarGzDir + writeTarEntry.
	require.NoError(t, rc.Close())

	assert.Equal(t, "# root\n", files["main.tf"])
	assert.Equal(t, "# sub\n", files["modules/sub/main.tf"])
}

func TestTarGzDir_WalkError(t *testing.T) {
	// A non-existent root makes WalkDir return an error that tarGzDir surfaces.
	err := tarGzDir(filepath.Join(t.TempDir(), "does-not-exist"), io.Discard)
	require.Error(t, err)
}

func TestTarGzDir_PacksDirsFilesAndSymlinks(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "nested"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "nested", "a.txt"), []byte("alpha"), 0o644))

	wantSymlink := runtime.GOOS != "windows"
	if wantSymlink {
		if err := os.Symlink("nested/a.txt", filepath.Join(root, "link")); err != nil {
			wantSymlink = false // some CI filesystems disallow symlinks.
		}
	}

	var buf strings.Builder
	require.NoError(t, tarGzDir(root, &stringWriter{&buf}))

	names, contents := readGzTarRaw(t, strings.NewReader(buf.String()))
	assert.Contains(t, names, "nested/")
	assert.Contains(t, names, "nested/a.txt")
	assert.Equal(t, "alpha", contents["nested/a.txt"])
	if wantSymlink {
		assert.Contains(t, names, "link")
	}
}

// stringWriter adapts a strings.Builder to io.Writer for tarGzDir.
type stringWriter struct{ b *strings.Builder }

func (w *stringWriter) Write(p []byte) (int, error) { return w.b.Write(p) }

// readGzTar reads a gzip-tar stream's regular files into a path->content map.
func readGzTar(t *testing.T, r io.Reader) map[string]string {
	t.Helper()
	_, contents := readGzTarRaw(t, r)
	return contents
}

// readGzTarRaw returns all entry names plus the regular-file contents of a gzip-tar.
func readGzTarRaw(t *testing.T, r io.Reader) (names []string, contents map[string]string) {
	t.Helper()
	gz, err := gzip.NewReader(r)
	require.NoError(t, err)
	defer gz.Close()

	contents = map[string]string{}
	tr := tar.NewReader(gz)
	for {
		hdr, rerr := tr.Next()
		if errors.Is(rerr, io.EOF) {
			break
		}
		require.NoError(t, rerr)
		names = append(names, hdr.Name)
		if hdr.Typeflag == tar.TypeReg {
			b, rerr := io.ReadAll(tr)
			require.NoError(t, rerr)
			contents[hdr.Name] = string(b)
		}
	}
	return names, contents
}
