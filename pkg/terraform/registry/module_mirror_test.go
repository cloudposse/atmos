package registry

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/http/proxy"
)

// fakeResolver stands in for the go-getter downloader: it records each call and
// populates the destination dir with a fixed set of files, so module source caching
// is exercised end to end without touching the network.
type fakeResolver struct {
	mu      sync.Mutex
	calls   int
	sources []string
	files   map[string]string // relative path (slash) -> content
}

func (f *fakeResolver) Resolve(_ context.Context, source, destDir string) error {
	f.mu.Lock()
	f.calls++
	f.sources = append(f.sources, source)
	f.mu.Unlock()

	for rel, content := range f.files {
		p := filepath.Join(destDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeResolver) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// moduleHostRewriteClient rewrites upstream module-registry calls to the fake server.
type moduleHostRewriteClient struct {
	target string
}

func (c *moduleHostRewriteClient) Do(req *http.Request) (*http.Response, error) {
	if strings.HasPrefix(req.URL.Path, "/v1/modules/") {
		newURL := c.target + req.URL.Path
		if req.URL.RawQuery != "" {
			newURL += "?" + req.URL.RawQuery
		}
		rebuilt, err := http.NewRequestWithContext(req.Context(), req.Method, newURL, req.Body)
		if err != nil {
			return nil, err
		}
		rebuilt.Header = req.Header
		return http.DefaultClient.Do(rebuilt)
	}
	return http.DefaultClient.Do(req)
}

func startModuleProxy(t *testing.T, upstream *httptest.Server, resolver SourceResolver) *proxy.Server {
	t.Helper()
	client := &moduleHostRewriteClient{target: upstream.URL}
	srv := proxy.NewServer(proxy.Options{
		Mirrors: []proxy.Mirror{NewModuleMirror(resolver)},
		Store:   proxy.NewFileStore(t.TempDir()),
		Client:  client,
	})
	_, err := srv.Start(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Shutdown(t.Context()) })
	return srv
}

// downloadUpstream serves a module download resolution returning the given
// X-Terraform-Get source, counting how many times the download endpoint is hit.
func downloadUpstream(t *testing.T, xTerraformGet string, hits *int) *httptest.Server {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/download") {
			if hits != nil {
				*hits++
			}
			w.Header().Set("X-Terraform-Get", xTerraformGet)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(upstream.Close)
	return upstream
}

// downloadUpstreamJSON serves the OpenTofu-style download resolution: 200 + a JSON
// {"location": "..."} body and no X-Terraform-Get header.
func downloadUpstreamJSON(t *testing.T, location string, hits *int) *httptest.Server {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/download") {
			if hits != nil {
				*hits++
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte(`{"location":"` + location + `"}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(upstream.Close)
	return upstream
}

func TestModuleMirror_OpenTofuJSONDownloadRewrittenThroughProxy(t *testing.T) {
	// OpenTofu's static registry returns the source in a JSON body, not a header.
	const gitGet = "git::https://github.com/cloudposse/terraform-null-label?ref=488ab91e"
	srv := startModuleProxy(t, downloadUpstreamJSON(t, gitGet, nil), &fakeResolver{})

	resp, err := http.Get(srv.BaseURL() + "modules/registry.opentofu.org/cloudposse/label/null/0.25.0/download")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var payload struct {
		Location string `json:"location"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	// The JSON form is preserved, but location now routes back through the proxy.
	assert.True(t, strings.HasPrefix(payload.Location, srv.BaseURL()+"modules/"+moduleSourceSegment+"/"), "got %q", payload.Location)
	assert.Contains(t, payload.Location, sourceExt)
	assert.Empty(t, resp.Header.Get("X-Terraform-Get"), "JSON-form download must not invent a header")
}

func TestModuleMirror_GitDownloadRewrittenThroughProxy(t *testing.T) {
	const gitGet = "git::https://github.com/cloudposse/terraform-aws-vpc.git?ref=v2.1.0"
	srv := startModuleProxy(t, downloadUpstream(t, gitGet, nil), &fakeResolver{})

	resp, err := http.Get(srv.BaseURL() + "modules/registry.terraform.io/cloudposse/vpc/aws/2.1.0/download")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	// Every source — git:: included — is now rewritten back through the proxy's
	// _source route so it can be resolved and cached.
	got := resp.Header.Get("X-Terraform-Get")
	assert.True(t, strings.HasPrefix(got, srv.BaseURL()+"modules/"+moduleSourceSegment+"/"), "got %q", got)
	assert.Contains(t, got, sourceExt)
}

func TestModuleMirror_HTTPArchiveRewrittenThroughProxy(t *testing.T) {
	const archive = "https://archives.example.com/mod-1.0.0.tar.gz"
	srv := startModuleProxy(t, downloadUpstream(t, archive, nil), &fakeResolver{})

	resp, err := http.Get(srv.BaseURL() + "modules/registry.terraform.io/acme/mod/aws/1.0.0/download")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	got := resp.Header.Get("X-Terraform-Get")
	assert.True(t, strings.HasPrefix(got, srv.BaseURL()+"modules/"+moduleSourceSegment+"/"), "got %q", got)
}

func TestModuleMirror_DownloadResolutionCached(t *testing.T) {
	const gitGet = "git::https://github.com/cloudposse/terraform-aws-vpc.git?ref=v2.1.0"
	var hits int
	srv := startModuleProxy(t, downloadUpstream(t, gitGet, &hits), &fakeResolver{})

	url := srv.BaseURL() + "modules/registry.terraform.io/cloudposse/vpc/aws/2.1.0/download"
	first := mustGetXTerraformGet(t, url)
	second := mustGetXTerraformGet(t, url)

	assert.Equal(t, 1, hits, "download resolution must be cached after the first fetch")
	assert.Equal(t, first, second, "cached resolution must be served identically")
	assert.True(t, strings.HasPrefix(first, srv.BaseURL()+"modules/"+moduleSourceSegment+"/"), "got %q", first)
}

func TestModuleMirror_SubdirPreservedAndDeduped(t *testing.T) {
	// Two modules referencing different subdirs of the same mono-repo at the same ref.
	const repo = "git::https://github.com/org/monorepo.git"
	srv := startModuleProxy(t, downloadUpstream(t, repo+"//modules/foo?ref=v1", nil), &fakeResolver{})

	foo := mustGetXTerraformGet(t, srv.BaseURL()+"modules/registry.terraform.io/org/foo/aws/1.0.0/download")

	// The subdir is reattached client-side so go-getter extracts it after unpacking.
	assert.Contains(t, foo, "//modules/foo")
	assert.Contains(t, foo, sourceExt)

	// The base (without subdir) is what is encoded into the _source path, so a different
	// subdir of the same repo+ref keys to the same cached source.
	base, sub := splitModuleSource(repo + "//modules/foo?ref=v1")
	assert.Equal(t, "modules/foo", sub)
	otherBase, _ := splitModuleSource(repo + "//modules/bar?ref=v1")
	assert.Equal(t, base, otherBase, "different subdirs of the same repo must share a base source")
}

func TestModuleMirror_SourceFetchedTarredAndCached(t *testing.T) {
	const gitGet = "git::https://github.com/org/mod.git?ref=v1.2.3"
	resolver := &fakeResolver{files: map[string]string{
		"main.tf":             "# root module\n",
		"modules/sub/main.tf": "# submodule\n",
	}}
	srv := startModuleProxy(t, downloadUpstream(t, gitGet, nil), resolver)

	// Resolve the download to discover the _source URL.
	sourceURL := mustGetXTerraformGet(t, srv.BaseURL()+"modules/registry.terraform.io/org/mod/aws/1.2.3/download")

	// Fetch the source tar twice; the resolver must run exactly once (artifact cached).
	files1 := fetchTar(t, sourceURL)
	files2 := fetchTar(t, sourceURL)

	assert.Equal(t, 1, resolver.callCount(), "source must be resolved once and then served from cache")
	assert.Equal(t, []string{gitGet}, resolver.sources, "resolver must receive the base source string")
	assert.Equal(t, "# root module\n", files1["main.tf"])
	assert.Equal(t, "# submodule\n", files1["modules/sub/main.tf"])
	assert.Equal(t, files1, files2, "cached tar must be identical across fetches")
}

func TestModuleMirror_VersionsCached(t *testing.T) {
	var hits int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/versions") {
			hits++
			_, _ = w.Write([]byte(`{"modules":[{"versions":[{"version":"1.0.0"}]}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer upstream.Close()
	srv := startModuleProxy(t, upstream, &fakeResolver{})

	url := srv.BaseURL() + "modules/registry.terraform.io/acme/mod/aws/versions"
	_ = mustGet(t, url)
	_ = mustGet(t, url)
	assert.Equal(t, 1, hits, "versions listing must be cached after the first fetch")
}

// mustGetXTerraformGet issues a GET against a download endpoint and returns the
// (rewritten) X-Terraform-Get header.
func mustGetXTerraformGet(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.Header.Get("X-Terraform-Get")
}

// fetchTar GETs a _source URL and returns the gzipped tar's regular-file entries as a
// path->content map.
func fetchTar(t *testing.T, url string) map[string]string {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	gz, err := gzip.NewReader(resp.Body)
	require.NoError(t, err)
	defer gz.Close()

	out := map[string]string{}
	tr := tar.NewReader(gz)
	for {
		hdr, rerr := tr.Next()
		if errors.Is(rerr, io.EOF) {
			break
		}
		require.NoError(t, rerr)
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		b, rerr := io.ReadAll(tr)
		require.NoError(t, rerr)
		out[hdr.Name] = string(b)
	}
	return out
}
