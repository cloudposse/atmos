package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// artifactMirror is a minimal Mirror used for tests. It routes /obj/<name> to an
// upstream server and caches the response as an immutable artifact.
type artifactMirror struct {
	upstreamBase string
	verify       func(sha string) error
}

func (m *artifactMirror) Handles(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/obj/")
}

func (m *artifactMirror) Route(r *http.Request) (Route, error) {
	name := strings.TrimPrefix(r.URL.Path, "/obj/")
	return Route{
		Key:         "objects/" + name,
		Kind:        KindArtifact,
		Upstream:    UpstreamRequest{URL: m.upstreamBase + "/" + name},
		Verify:      m.verify,
		ContentType: "application/octet-stream",
	}, nil
}

func startProxy(t *testing.T, mirrors []Mirror, opts Options) *Server { //nolint:gocritic // test helper; Options passed by value for convenience.
	t.Helper()
	opts.Mirrors = mirrors
	if opts.Store == nil {
		opts.Store = NewFileStore(t.TempDir())
	}
	srv := NewServer(opts)
	_, err := srv.Start(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })
	return srv
}

func TestProxy_HitAndMiss(t *testing.T) {
	var upstreamHits int64
	payload := []byte("provider-zip-bytes")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&upstreamHits, 1)
		_, _ = w.Write(payload)
	}))
	defer upstream.Close()

	srv := startProxy(t, []Mirror{&artifactMirror{upstreamBase: upstream.URL}}, Options{})

	// First request: miss, fetched from upstream.
	body1 := httpGet(t, srv.BaseURL()+"obj/aws.zip")
	assert.Equal(t, payload, body1)
	assert.Equal(t, int64(1), atomic.LoadInt64(&upstreamHits))

	// Second request: hit, served from cache (no new upstream fetch).
	body2 := httpGet(t, srv.BaseURL()+"obj/aws.zip")
	assert.Equal(t, payload, body2)
	assert.Equal(t, int64(1), atomic.LoadInt64(&upstreamHits))

	snap := srv.Stats()
	assert.Equal(t, 1, snap.Hits)
	assert.Equal(t, int64(len(payload)), snap.BytesSaved)
	assert.Equal(t, 1, snap.Misses)
	// The first request committed the payload to cache, so the warmed counters
	// reflect the downloaded-and-cached bytes.
	assert.Equal(t, 1, snap.ObjectsCached)
	assert.Equal(t, int64(len(payload)), snap.BytesCached)
}

// faultyResponseWriter accepts the first limit bytes, then fails every subsequent
// Write. It simulates a client that disconnects mid-stream so the savings report
// counts only the bytes actually delivered.
type faultyResponseWriter struct {
	header  http.Header
	written int
	limit   int
}

func (w *faultyResponseWriter) Header() http.Header { return w.header }

func (w *faultyResponseWriter) WriteHeader(int) {}

func (w *faultyResponseWriter) Write(p []byte) (int, error) {
	remaining := w.limit - w.written
	if remaining <= 0 {
		return 0, fmt.Errorf("client disconnected")
	}
	if len(p) <= remaining {
		w.written += len(p)
		return len(p), nil
	}
	w.written += remaining
	return remaining, fmt.Errorf("client disconnected")
}

// TestProxy_HitPartialWriteCountsRealBytes verifies the savings report records the
// bytes actually streamed to the client on a hit, not the full on-disk object size,
// when the transfer is interrupted mid-stream.
func TestProxy_HitPartialWriteCountsRealBytes(t *testing.T) {
	payload := []byte("0123456789abcdefghijklmnopqrstuvwxyz") // 36 bytes.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer upstream.Close()

	mirror := &artifactMirror{upstreamBase: upstream.URL}
	srv := startProxy(t, []Mirror{mirror}, Options{})

	// First request: miss, commits the object to cache. Not counted as a hit.
	body := httpGet(t, srv.BaseURL()+"obj/partial.zip")
	assert.Equal(t, payload, body)
	require.Equal(t, 0, srv.Stats().Hits)

	// Serve the cached object through a writer that fails after the first k bytes.
	const k = 10
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.BaseURL()+"obj/partial.zip", nil)
	require.NoError(t, err)
	route, err := mirror.Route(req)
	require.NoError(t, err)

	fw := &faultyResponseWriter{header: make(http.Header), limit: k}
	require.True(t, srv.tryServeHit(fw, &route), "the cached object must be a servable hit")

	snap := srv.Stats()
	assert.Equal(t, 1, snap.Hits, "an interrupted transfer is still a hit")
	assert.Equal(t, int64(k), snap.BytesSaved, "only the bytes actually delivered are counted, not the full object size")
	assert.Less(t, snap.BytesSaved, int64(len(payload)), "a partial transfer must record fewer bytes than the object size")
}

func TestProxy_ConcurrentSingleDownloader(t *testing.T) {
	var upstreamHits int64
	payload := []byte("the-only-download")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&upstreamHits, 1)
		time.Sleep(20 * time.Millisecond) // widen the race window.
		_, _ = w.Write(payload)
	}))
	defer upstream.Close()

	srv := startProxy(t, []Mirror{&artifactMirror{upstreamBase: upstream.URL}}, Options{})

	const n = 12
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			body := httpGet(t, srv.BaseURL()+"obj/concurrent.zip")
			assert.Equal(t, payload, body)
		}()
	}
	wg.Wait()

	assert.Equal(t, int64(1), atomic.LoadInt64(&upstreamHits), "exactly one downloader on a cold key")
}

func TestProxy_VerifyFailureRejectsCommit(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("corrupt"))
	}))
	defer upstream.Close()

	mirror := &artifactMirror{
		upstreamBase: upstream.URL,
		verify:       func(sha string) error { return fmt.Errorf("hash mismatch") },
	}
	store := NewFileStore(t.TempDir())
	srv := startProxy(t, []Mirror{mirror}, Options{Store: store})

	resp, err := http.Get(srv.BaseURL() + "obj/bad.zip")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadGateway, resp.StatusCode)

	// Nothing was committed.
	_, ok, err := store.Stat("objects/bad.zip")
	require.NoError(t, err)
	assert.False(t, ok, "verify failure must not commit the object")
}

func TestProxy_PassthroughNotCached(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Terraform-Get", "git::https://github.com/org/repo.git")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	mirror := mirrorFunc{
		handles: func(r *http.Request) bool { return strings.HasPrefix(r.URL.Path, "/download") },
		route: func(r *http.Request) (Route, error) {
			return Route{Kind: KindPassthrough, Upstream: UpstreamRequest{URL: upstream.URL}}, nil
		},
	}
	store := NewFileStore(t.TempDir())
	srv := startProxy(t, []Mirror{mirror}, Options{Store: store})

	resp, err := http.Get(srv.BaseURL() + "download/mod")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, "git::https://github.com/org/repo.git", resp.Header.Get("X-Terraform-Get"))
}

func TestProxy_MetadataRewrite(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"url":"UPSTREAM/path/file.zip"}`))
	}))
	defer upstream.Close()

	mirror := mirrorFunc{
		handles: func(r *http.Request) bool { return strings.HasPrefix(r.URL.Path, "/meta") },
		route: func(r *http.Request) (Route, error) {
			return Route{
				Key:         "metadata/version.json",
				Kind:        KindMetadata,
				Upstream:    UpstreamRequest{URL: upstream.URL},
				ContentType: "application/json",
				Rewrite: func(body []byte, base string) ([]byte, error) {
					return []byte(strings.ReplaceAll(string(body), "UPSTREAM/", base)), nil
				},
			}, nil
		},
	}
	srv := startProxy(t, []Mirror{mirror}, Options{})

	body := httpGet(t, srv.BaseURL()+"meta/version.json")
	assert.Contains(t, string(body), srv.BaseURL()+"path/file.zip")
	assert.NotContains(t, string(body), "UPSTREAM/")
}

// TestProxy_ConcurrentSlowDownloader verifies the herd collapses for a fetch that
// outlasts the old fixed flock retry budget (~500ms): exactly one downloader runs
// and every waiter still succeeds. Before the singleflight + context-aware lock
// rework, the waiters exhausted the retry budget and received 502s.
func TestProxy_ConcurrentSlowDownloader(t *testing.T) {
	var upstreamHits int64
	payload := []byte("the-only-slow-download")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&upstreamHits, 1)
		time.Sleep(700 * time.Millisecond) // exceeds the old 500ms flock retry budget.
		_, _ = w.Write(payload)
	}))
	defer upstream.Close()

	srv := startProxy(t, []Mirror{&artifactMirror{upstreamBase: upstream.URL}}, Options{
		Client: &http.Client{Timeout: 10 * time.Second},
	})

	const n = 12
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			body := httpGet(t, srv.BaseURL()+"obj/slow.zip") // httpGet requires HTTP 200.
			assert.Equal(t, payload, body)
		}()
	}
	wg.Wait()

	assert.Equal(t, int64(1), atomic.LoadInt64(&upstreamHits), "exactly one downloader despite a fetch longer than the lock retry budget")
}

// TestProxy_ConcurrentNon2xxFanout verifies a non-cacheable upstream response (404)
// is fanned out to every waiter on the key and is never committed to the cache.
func TestProxy_ConcurrentNon2xxFanout(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond) // widen the race window.
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer upstream.Close()

	store := NewFileStore(t.TempDir())
	srv := startProxy(t, []Mirror{&artifactMirror{upstreamBase: upstream.URL}}, Options{
		Store:  store,
		Client: &http.Client{Timeout: 10 * time.Second},
	})

	const n = 8
	var wg sync.WaitGroup
	var got404 int64
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			resp, err := http.Get(srv.BaseURL() + "obj/missing.zip")
			if err != nil {
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusNotFound {
				atomic.AddInt64(&got404, 1)
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, int64(n), atomic.LoadInt64(&got404), "every waiter receives the non-cacheable 404")

	_, ok, err := store.Stat("objects/missing.zip")
	require.NoError(t, err)
	assert.False(t, ok, "a non-2xx upstream response must not be cached")
}

// TestProxy_ClientCancelDuringFill verifies that a client disconnecting while a
// shared fill is in flight does not abort that fill: it completes for the other
// requesters and the object ends up cached, fetched exactly once.
func TestProxy_ClientCancelDuringFill(t *testing.T) {
	var upstreamHits int64
	payload := []byte("eventually-cached")
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&upstreamHits, 1)
		<-release // block until the test lets the fill finish.
		_, _ = w.Write(payload)
	}))
	defer upstream.Close()

	srv := startProxy(t, []Mirror{&artifactMirror{upstreamBase: upstream.URL}}, Options{
		Client: &http.Client{Timeout: 10 * time.Second},
	})

	// First client starts the fill, then cancels while it is still in flight.
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.BaseURL()+"obj/cancel.zip", nil)
	require.NoError(t, err)
	errCh := make(chan error, 1)
	go func() {
		resp, gerr := http.DefaultClient.Do(req)
		if resp != nil {
			_ = resp.Body.Close()
		}
		errCh <- gerr
	}()

	waitForUpstreamHit(t, &upstreamHits) // the fill has started.
	cancel()
	<-errCh // the canceled client returns without wedging the server.

	close(release) // let the shared fill complete.

	body := httpGet(t, srv.BaseURL()+"obj/cancel.zip") // served from the completed fill / cache.
	assert.Equal(t, payload, body)
	assert.Equal(t, int64(1), atomic.LoadInt64(&upstreamHits), "the canceled client's fill completed and was reused")
}

// waitForUpstreamHit blocks until the upstream counter is at least one.
func waitForUpstreamHit(t *testing.T, counter *int64) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(counter) >= 1 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for upstream to be hit")
}

// mirrorFunc adapts closures to the Mirror interface for tests.
type mirrorFunc struct {
	handles func(r *http.Request) bool
	route   func(r *http.Request) (Route, error)
}

func (m mirrorFunc) Handles(r *http.Request) bool         { return m.handles(r) }
func (m mirrorFunc) Route(r *http.Request) (Route, error) { return m.route(r) }

func httpGet(t *testing.T, url string) []byte {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return body
}
