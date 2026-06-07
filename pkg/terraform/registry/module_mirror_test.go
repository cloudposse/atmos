package registry

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/http/proxy"
)

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

func startModuleProxy(t *testing.T, upstream *httptest.Server) *proxy.Server {
	t.Helper()
	client := &moduleHostRewriteClient{target: upstream.URL}
	srv := proxy.NewServer(proxy.Options{
		Mirrors: []proxy.Mirror{NewModuleMirror()},
		Store:   proxy.NewFileStore(t.TempDir()),
		Client:  client,
	})
	_, err := srv.Start(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Shutdown(t.Context()) })
	return srv
}

func TestModuleMirror_GitDownloadPassesThroughVerbatim(t *testing.T) {
	const gitGet = "git::https://github.com/cloudposse/terraform-aws-vpc.git?ref=v2.1.0"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/download") {
			w.Header().Set("X-Terraform-Get", gitGet)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer upstream.Close()

	srv := startModuleProxy(t, upstream)

	resp, err := http.Get(srv.BaseURL() + "modules/registry.terraform.io/cloudposse/vpc/aws/2.1.0/download")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	// git:: source must pass through unchanged (future git mirror completes it).
	assert.Equal(t, gitGet, resp.Header.Get("X-Terraform-Get"))
}

func TestModuleMirror_HTTPArchiveRewrittenThroughProxy(t *testing.T) {
	srv := newModuleArchiveProxy(t)

	resp, err := http.Get(srv.BaseURL() + "modules/registry.terraform.io/acme/mod/aws/1.0.0/download")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	got := resp.Header.Get("X-Terraform-Get")
	// HTTP-archive X-Terraform-Get is rewritten to route back through the proxy.
	assert.True(t, strings.HasPrefix(got, srv.BaseURL()+"modules/"+moduleArchiveSegment+"/"), "got %q", got)
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
	srv := startModuleProxy(t, upstream)

	url := srv.BaseURL() + "modules/registry.terraform.io/acme/mod/aws/versions"
	_ = mustGet(t, url)
	_ = mustGet(t, url)
	assert.Equal(t, 1, hits, "versions listing must be cached after the first fetch")
}

// newModuleArchiveProxy wires an upstream whose download returns an HTTP archive.
func newModuleArchiveProxy(t *testing.T) *proxy.Server {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/download") {
			w.Header().Set("X-Terraform-Get", "https://archives.example.com/mod-1.0.0.tar.gz")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(upstream.Close)
	return startModuleProxy(t, upstream)
}
