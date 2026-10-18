package downloader

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tarBytes builds a tar archive with one regular file and the given link entries.
func tarBytes(t *testing.T, links ...tar.Header) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	body := []byte("Resources: {}\n")
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "template.yaml", Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}))
	_, err := tw.Write(body)
	require.NoError(t, err)
	for i := range links {
		links[i].Mode = 0o777
		require.NoError(t, tw.WriteHeader(&links[i]))
	}
	require.NoError(t, tw.Close())
	return buf.Bytes()
}

func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, err := gz.Write(data)
	require.NoError(t, err)
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func zipBytes(t *testing.T, symlinks ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("template.yaml")
	require.NoError(t, err)
	_, err = w.Write([]byte("Resources: {}\n"))
	require.NoError(t, err)
	for _, name := range symlinks {
		header := &zip.FileHeader{Name: name}
		header.SetMode(os.ModeSymlink | 0o777)
		w, err := zw.CreateHeader(header)
		require.NoError(t, err)
		_, err = w.Write([]byte("template.yaml"))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func TestAuditArchiveLinks(t *testing.T) {
	symlink := tar.Header{Name: "link-a", Typeflag: tar.TypeSymlink, Linkname: "/etc/hosts"}
	hardlink := tar.Header{Name: "link-b", Typeflag: tar.TypeLink, Linkname: "template.yaml"}
	cases := []struct {
		name string
		ext  string
		data []byte
		want linkAudit
	}{
		{name: "tar without links", ext: "tar", data: tarBytes(t)},
		{name: "tar with links", ext: "tar", data: tarBytes(t, symlink, hardlink), want: linkAudit{Count: 2, First: "link-a"}},
		{name: "tar.gz with links", ext: "tar.gz", data: gzipBytes(t, tarBytes(t, symlink)), want: linkAudit{Count: 1, First: "link-a"}},
		{name: "tgz without links", ext: "tgz", data: gzipBytes(t, tarBytes(t))},
		{name: "zip without links", ext: "zip", data: zipBytes(t)},
		{name: "zip with symlinks", ext: "zip", data: zipBytes(t, "ln-1", "ln-2"), want: linkAudit{Count: 2, First: "ln-1"}},
		{name: "unreadable format is not an error", ext: "tar.xz", data: []byte("not scanned")},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			src := filepath.Join(t.TempDir(), "archive")
			require.NoError(t, os.WriteFile(src, tt.data, 0o600))

			audit, err := auditArchiveLinks(src, tt.ext)

			require.NoError(t, err)
			assert.Equal(t, tt.want, audit)
		})
	}

	t.Run("corrupt archive reports an error", func(t *testing.T) {
		src := filepath.Join(t.TempDir(), "archive")
		require.NoError(t, os.WriteFile(src, []byte("not a gzip stream"), 0o600))
		_, err := auditArchiveLinks(src, "tar.gz")
		require.Error(t, err)
	})
}

// TestGoGetterWarnsOncePerArchiveWithLinks drives the real downloader: links are warned about
// once per archive, and extraction behavior is unchanged (a link still becomes an empty file).
func TestGoGetterWarnsOncePerArchiveWithLinks(t *testing.T) {
	cases := []struct {
		name      string
		archive   []byte
		wantCalls int
	}{
		{name: "with links", archive: gzipBytes(t, tarBytes(t,
			tar.Header{Name: "hosts", Typeflag: tar.TypeSymlink, Linkname: "/etc/hosts"},
			tar.Header{Name: "other", Typeflag: tar.TypeSymlink, Linkname: "template.yaml"})), wantCalls: 1},
		{name: "without links", archive: gzipBytes(t, tarBytes(t)), wantCalls: 0},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			var audits []linkAudit
			original := warnArchiveLinks
			t.Cleanup(func() { warnArchiveLinks = original })
			warnArchiveLinks = func(audit linkAudit) { audits = append(audits, audit) }

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write(tt.archive)
			}))
			t.Cleanup(server.Close)

			dest := filepath.Join(t.TempDir(), "out")
			require.NoError(t, NewGoGetterDownloader(nil).Fetch(server.URL+"/module.tar.gz", dest, ClientModeDir, time.Minute))

			require.Len(t, audits, tt.wantCalls)
			content, err := os.ReadFile(filepath.Join(dest, "template.yaml"))
			require.NoError(t, err)
			assert.Equal(t, "Resources: {}\n", string(content))
			if tt.wantCalls > 0 {
				assert.Equal(t, linkAudit{Count: 2, First: "hosts"}, audits[0])
				info, err := os.Lstat(filepath.Join(dest, "hosts"))
				require.NoError(t, err)
				assert.True(t, info.Mode().IsRegular(), "behavior is unchanged: the link is still extracted as a regular file")
				assert.Zero(t, info.Size())
			}
		})
	}
}
