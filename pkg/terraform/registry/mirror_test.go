package registry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/http/proxy"
)

func TestPlatform_String(t *testing.T) {
	assert.Equal(t, "linux_amd64", Platform{OS: "linux", Arch: "amd64"}.String())
	assert.Equal(t, "darwin_arm64", Platform{OS: "darwin", Arch: "arm64"}.String())
}

func TestHostPlatform(t *testing.T) {
	p := HostPlatform()
	assert.Equal(t, runtime.GOOS, p.OS)
	assert.Equal(t, runtime.GOARCH, p.Arch)
}

func TestParsePlatform(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    Platform
		wantErr bool
	}{
		{name: "valid", in: "linux_amd64", want: Platform{OS: "linux", Arch: "amd64"}},
		{name: "valid with arch underscore", in: "windows_386", want: Platform{OS: "windows", Arch: "386"}},
		{name: "trims whitespace", in: "  darwin_arm64 ", want: Platform{OS: "darwin", Arch: "arm64"}},
		{name: "no separator", in: "linux", wantErr: true},
		{name: "empty os", in: "_amd64", wantErr: true},
		{name: "empty arch", in: "linux_", wantErr: true},
		{name: "empty", in: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePlatform(tt.in)
			if tt.wantErr {
				require.ErrorIs(t, err, ErrInvalidPlatform)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestProviderRef_Source(t *testing.T) {
	ref := ProviderRef{Host: "registry.terraform.io", Namespace: "hashicorp", Type: "aws", Version: "5.95.0"}
	assert.Equal(t, "registry.terraform.io/hashicorp/aws", ref.Source())
}

func TestParseSource(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		version string
		want    ProviderRef
		wantErr bool
	}{
		{
			name:    "valid",
			source:  "registry.terraform.io/hashicorp/aws",
			version: "5.95.0",
			want:    ProviderRef{Host: "registry.terraform.io", Namespace: "hashicorp", Type: "aws", Version: "5.95.0"},
		},
		{name: "too few parts", source: "hashicorp/aws", version: "1.0.0", wantErr: true},
		{name: "too many parts", source: "a/b/c/d", version: "1.0.0", wantErr: true},
		{name: "empty segment", source: "registry.terraform.io//aws", version: "1.0.0", wantErr: true},
		{name: "empty", source: "", version: "1.0.0", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSource(tt.source, tt.version)
			if tt.wantErr {
				require.ErrorIs(t, err, ErrInvalidProviderSource)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// awsRef is the provider the fake registry serves.
func awsRef() ProviderRef {
	return ProviderRef{Host: "registry.terraform.io", Namespace: "hashicorp", Type: "aws", Version: "5.95.0"}
}

func linuxAMD64() Platform { return Platform{OS: "linux", Arch: "amd64"} }

func TestProviderMirror_DownloadInto_SuccessAndCacheHit(t *testing.T) {
	fr := newFakeRegistry(t)
	client := &hostRewriteClient{target: fr.server.URL, host: "registry.terraform.io"}
	m := NewProviderMirror(client)
	store := proxy.NewFileStore(t.TempDir())

	meta, cached, err := m.DownloadInto(t.Context(), store, awsRef(), linuxAMD64())
	require.NoError(t, err)
	assert.False(t, cached, "first download is not a cache hit")
	assert.Equal(t, int64(len(fr.zip)), meta.Size)
	assert.Equal(t, fr.zipSum, meta.SHA256)
	assert.Equal(t, proxy.KindArtifact, meta.Kind)

	// The second call finds the committed object under the lock (cached=true) and so
	// never calls fetchAndCommit to re-download the zip body.
	meta2, cached2, err := m.DownloadInto(t.Context(), store, awsRef(), linuxAMD64())
	require.NoError(t, err)
	assert.True(t, cached2, "second download is served from the cache")
	assert.Equal(t, meta.SHA256, meta2.SHA256)
}

func TestProviderMirror_DownloadInto_MissingDownloadURL(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/providers/hashicorp/aws/5.95.0/download/linux/amd64", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(registryDownload{Filename: "x.zip", DownloadURL: ""})
	})
	_, _, err := downloadIntoServer(t, mux)
	require.ErrorIs(t, err, ErrInvalidProviderPath)
}

func TestProviderMirror_DownloadInto_NonSuccessZip(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/v1/providers/hashicorp/aws/5.95.0/download/linux/amd64", func(w http.ResponseWriter, _ *http.Request) {
		// Empty shasum skips verification so we test the non-2xx zip path itself.
		_ = json.NewEncoder(w).Encode(registryDownload{Filename: "x.zip", DownloadURL: srv.URL + "/zip", Shasum: ""})
	})
	mux.HandleFunc("/zip", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})

	client := &hostRewriteClient{target: srv.URL, host: "registry.terraform.io"}
	m := NewProviderMirror(client)
	_, _, err := m.DownloadInto(t.Context(), proxy.NewFileStore(t.TempDir()), awsRef(), linuxAMD64())
	require.ErrorIs(t, err, ErrUpstreamStatus)
}

func TestProviderMirror_DownloadInto_HashMismatch(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/v1/providers/hashicorp/aws/5.95.0/download/linux/amd64", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(registryDownload{Filename: "x.zip", DownloadURL: srv.URL + "/zip", Shasum: "deadbeef"})
	})
	mux.HandleFunc("/zip", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("PK\x03\x04 not matching the advertised shasum"))
	})

	client := &hostRewriteClient{target: srv.URL, host: "registry.terraform.io"}
	m := NewProviderMirror(client)
	_, _, err := m.DownloadInto(t.Context(), proxy.NewFileStore(t.TempDir()), awsRef(), linuxAMD64())
	require.ErrorIs(t, err, ErrUpstreamStatus)
}

// downloadIntoServer wires an inline registry mux to a ProviderMirror and runs
// DownloadInto for the aws/linux_amd64 target against a fresh store.
func downloadIntoServer(t *testing.T, mux *http.ServeMux) (proxy.Meta, bool, error) {
	t.Helper()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := &hostRewriteClient{target: srv.URL, host: "registry.terraform.io"}
	m := NewProviderMirror(client)
	return m.DownloadInto(context.Background(), proxy.NewFileStore(t.TempDir()), awsRef(), linuxAMD64())
}
