package registry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/http/proxy"
)

// fakeRegistry serves the provider registry protocol for one provider, rewriting
// service-discovery and download URLs to point at itself.
type fakeRegistry struct {
	server  *httptest.Server
	zip     []byte
	zipSum  string
	dlHits  int
	verHits int
}

func newFakeRegistry(t *testing.T) *fakeRegistry {
	t.Helper()
	zip := []byte("PK\x03\x04 fake provider zip")
	sum := sha256.Sum256(zip)
	fr := &fakeRegistry{zip: zip, zipSum: hex.EncodeToString(sum[:])}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/terraform.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"providers.v1":"/v1/providers/","modules.v1":"/v1/modules/"}`))
	})
	mux.HandleFunc("/v1/providers/hashicorp/aws/versions", func(w http.ResponseWriter, r *http.Request) {
		fr.verHits++
		_, _ = w.Write([]byte(`{"versions":[{"version":"5.95.0","platforms":[{"os":"linux","arch":"amd64"},{"os":"darwin","arch":"arm64"}]}]}`))
	})
	mux.HandleFunc("/v1/providers/hashicorp/aws/5.95.0/download/", func(w http.ResponseWriter, r *http.Request) {
		fr.dlHits++
		// .../download/<os>/<arch>.
		seg := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/providers/hashicorp/aws/5.95.0/download/"), "/")
		osName, arch := seg[0], seg[1]
		resp := registryDownload{
			Filename:    "terraform-provider-aws_5.95.0_" + osName + "_" + arch + ".zip",
			DownloadURL: fr.server.URL + "/zip/" + osName + "_" + arch,
			Shasum:      fr.zipSum,
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/zip/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(fr.zip)
	})

	fr.server = httptest.NewServer(mux)
	t.Cleanup(fr.server.Close)
	return fr
}

// hostRewriteClient rewrites requests targeting the provider host to the fake
// registry, so discovery URLs targeting registry.terraform.io reach the test server.
type hostRewriteClient struct {
	target string // fake server base URL.
	host   string // host to intercept.
}

func (c *hostRewriteClient) Do(req *http.Request) (*http.Response, error) {
	if req.URL.Host == c.host {
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

func startProviderProxy(t *testing.T, fr *fakeRegistry) *proxy.Server {
	t.Helper()
	client := &hostRewriteClient{target: fr.server.URL, host: "registry.terraform.io"}
	mirror := NewProviderMirror(client)
	srv := proxy.NewServer(proxy.Options{
		Mirrors: []proxy.Mirror{mirror},
		Store:   proxy.NewFileStore(t.TempDir()),
		Client:  client,
	})
	_, err := srv.Start(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Shutdown(t.Context()) })
	return srv
}

func TestProviderMirror_Index(t *testing.T) {
	fr := newFakeRegistry(t)
	srv := startProviderProxy(t, fr)

	body := mustGet(t, srv.BaseURL()+"providers/registry.terraform.io/hashicorp/aws/index.json")
	var idx mirrorIndex
	require.NoError(t, json.Unmarshal(body, &idx))
	_, ok := idx.Versions["5.95.0"]
	assert.True(t, ok, "index.json must list version 5.95.0")
}

func TestProviderMirror_VersionListsAllPlatforms(t *testing.T) {
	fr := newFakeRegistry(t)
	srv := startProviderProxy(t, fr)

	body := mustGet(t, srv.BaseURL()+"providers/registry.terraform.io/hashicorp/aws/5.95.0.json")
	var ver mirrorVersion
	require.NoError(t, json.Unmarshal(body, &ver))

	require.Contains(t, ver.Archives, "linux_amd64")
	require.Contains(t, ver.Archives, "darwin_arm64")
	assert.Equal(t, "terraform-provider-aws_5.95.0_linux_amd64.zip", ver.Archives["linux_amd64"].URL)
	assert.Equal(t, []string{"zh:" + fr.zipSum}, ver.Archives["linux_amd64"].Hashes)
}

// TestProviderMirror_VersionResolvesPlatformsConcurrently guards against a
// regression to serial per-platform resolution (the root cause of a CI
// screengrab failure: OpenTofu's own mirror-request deadline was exceeded
// because a cold-cache version fetch resolved a dozen-plus platforms one
// upstream round-trip at a time). With N platforms each taking `delay` to
// resolve, a serial implementation takes at least N*delay; a concurrent one
// takes roughly one delay regardless of N.
func TestProviderMirror_VersionResolvesPlatformsConcurrently(t *testing.T) {
	const (
		platformCount = 10
		delay         = 150 * time.Millisecond
	)

	platforms := make([]struct {
		OS   string `json:"os"`
		Arch string `json:"arch"`
	}, platformCount)
	for i := range platforms {
		platforms[i].OS = fmt.Sprintf("os%d", i)
		platforms[i].Arch = "amd64"
	}
	platformsJSON, err := json.Marshal(platforms)
	require.NoError(t, err)

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/terraform.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"providers.v1":"/v1/providers/","modules.v1":"/v1/modules/"}`))
	})
	mux.HandleFunc("/v1/providers/hashicorp/aws/versions", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `{"versions":[{"version":"5.95.0","platforms":%s}]}`, platformsJSON)
	})
	mux.HandleFunc("/v1/providers/hashicorp/aws/5.95.0/download/", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		seg := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/providers/hashicorp/aws/5.95.0/download/"), "/")
		osName, arch := seg[0], seg[1]
		resp := registryDownload{
			Filename: "terraform-provider-aws_5.95.0_" + osName + "_" + arch + ".zip",
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	client := &hostRewriteClient{target: server.URL, host: "registry.terraform.io"}
	srv := proxy.NewServer(proxy.Options{
		Mirrors: []proxy.Mirror{NewProviderMirror(client)},
		Store:   proxy.NewFileStore(t.TempDir()),
		Client:  client,
	})
	_, err = srv.Start(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Shutdown(t.Context()) })

	start := time.Now()
	body := mustGet(t, srv.BaseURL()+"providers/registry.terraform.io/hashicorp/aws/5.95.0.json")
	elapsed := time.Since(start)

	var ver mirrorVersion
	require.NoError(t, json.Unmarshal(body, &ver))
	assert.Len(t, ver.Archives, platformCount, "all platforms should resolve")

	// Bounded at 4/5 of the serial floor (not 1/2): a concurrent run should
	// complete in roughly one delay plus scheduling/HTTP overhead, but CI
	// runners -- Windows in particular -- occasionally add several hundred ms
	// of that overhead under load. A tighter bound flaked at 784ms against a
	// 750ms threshold despite being nowhere near the 1.5s serial floor it
	// exists to catch; this keeps a wide, unambiguous gap from a true serial
	// regression while tolerating realistic CI variance.
	serialFloor := time.Duration(platformCount) * delay
	assert.Less(t, elapsed, serialFloor*4/5,
		"resolving %d platforms took %s - expected well under the %s serial floor, indicating platforms are fetched concurrently, not one at a time",
		platformCount, elapsed, serialFloor)
}

// TestFetchPlatformArchives_BoundsConcurrency guards against a malformed or
// malicious registry response advertising an excessive platform list from
// spawning unbounded goroutines and outbound requests: it asserts the peak
// number of concurrent platform lookups never exceeds maxConcurrentPlatformFetches.
func TestFetchPlatformArchives_BoundsConcurrency(t *testing.T) {
	const platformCount = maxConcurrentPlatformFetches * 3

	var current, peak int64
	fetch := func(_ context.Context, _ proxy.UpstreamRequest) (*http.Response, error) {
		n := atomic.AddInt64(&current, 1)
		for {
			p := atomic.LoadInt64(&peak)
			if n <= p || atomic.CompareAndSwapInt64(&peak, p, n) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		atomic.AddInt64(&current, -1)

		body, err := json.Marshal(registryDownload{Filename: "terraform-provider-aws.zip"})
		require.NoError(t, err)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))}, nil
	}

	platforms := make([]registryPlatform, platformCount)
	for i := range platforms {
		platforms[i] = registryPlatform{OS: fmt.Sprintf("os%d", i), Arch: "amd64"}
	}

	req := &platformArchiveRequest{
		svc:     services{providersV1: "/v1/providers/"},
		coord:   providerCoord{host: "registry.terraform.io", namespace: "hashicorp", typ: "aws"},
		version: "5.95.0",
	}

	archives := fetchPlatformArchives(context.Background(), fetch, req, platforms)

	assert.Len(t, archives, platformCount, "all platforms should still resolve")
	assert.LessOrEqual(t, int(atomic.LoadInt64(&peak)), maxConcurrentPlatformFetches,
		"peak concurrent platform fetches (%d) exceeded the bound (%d)", peak, maxConcurrentPlatformFetches)
}

func TestProviderMirror_ArchiveDownloadAndVerify(t *testing.T) {
	fr := newFakeRegistry(t)
	srv := startProviderProxy(t, fr)

	url := srv.BaseURL() + "providers/registry.terraform.io/hashicorp/aws/terraform-provider-aws_5.95.0_linux_amd64.zip"
	body := mustGet(t, url)
	assert.Equal(t, fr.zip, body)

	// Second fetch is a cache hit (no new upstream zip request is required; download
	// resolution may re-run but the served bytes match and stats record a hit).
	body2 := mustGet(t, url)
	assert.Equal(t, fr.zip, body2)
	assert.Positive(t, srv.Stats().Hits)
}

func mustGet(t *testing.T, url string) []byte {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body := make([]byte, 0)
	buf := make([]byte, 4096)
	for {
		n, rerr := resp.Body.Read(buf)
		body = append(body, buf[:n]...)
		if rerr != nil {
			break
		}
	}
	return body
}
