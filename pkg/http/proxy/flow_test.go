package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer_IsFresh(t *testing.T) {
	t.Run("artifacts are always fresh", func(t *testing.T) {
		s := NewServer(Options{MetadataTTL: time.Hour})
		assert.True(t, s.isFresh(KindArtifact, Meta{FetchedAt: time.Now().Add(-100 * time.Hour)}))
	})
	t.Run("no TTL means metadata is fresh forever", func(t *testing.T) {
		s := NewServer(Options{MetadataTTL: 0})
		assert.True(t, s.isFresh(KindMetadata, Meta{FetchedAt: time.Now().Add(-100 * time.Hour)}))
	})
	t.Run("metadata within TTL is fresh", func(t *testing.T) {
		s := NewServer(Options{MetadataTTL: time.Hour})
		assert.True(t, s.isFresh(KindMetadata, Meta{FetchedAt: time.Now().Add(-30 * time.Minute)}))
	})
	t.Run("metadata past TTL is stale", func(t *testing.T) {
		s := NewServer(Options{MetadataTTL: time.Hour})
		assert.False(t, s.isFresh(KindMetadata, Meta{FetchedAt: time.Now().Add(-2 * time.Hour)}))
	})
}

func TestServer_WithinSWR(t *testing.T) {
	t.Run("zero SWR is never within window", func(t *testing.T) {
		s := NewServer(Options{MetadataTTL: time.Hour, StaleWhileRevalidate: 0})
		assert.False(t, s.withinSWR(Meta{FetchedAt: time.Now()}))
	})
	t.Run("within TTL+SWR window", func(t *testing.T) {
		s := NewServer(Options{MetadataTTL: time.Hour, StaleWhileRevalidate: time.Hour})
		assert.True(t, s.withinSWR(Meta{FetchedAt: time.Now().Add(-90 * time.Minute)}))
	})
	t.Run("beyond TTL+SWR window", func(t *testing.T) {
		s := NewServer(Options{MetadataTTL: time.Hour, StaleWhileRevalidate: time.Hour})
		assert.False(t, s.withinSWR(Meta{FetchedAt: time.Now().Add(-3 * time.Hour)}))
	})
}

func TestServer_Servable(t *testing.T) {
	s := NewServer(Options{MetadataTTL: time.Hour, StaleWhileRevalidate: time.Hour})

	// Artifacts are always servable regardless of age.
	assert.True(t, s.servable(KindArtifact, Meta{FetchedAt: time.Now().Add(-100 * time.Hour)}))
	// Fresh metadata is servable.
	assert.True(t, s.servable(KindMetadata, Meta{FetchedAt: time.Now()}))
	// Stale metadata inside the SWR window is still servable.
	assert.True(t, s.servable(KindMetadata, Meta{FetchedAt: time.Now().Add(-90 * time.Minute)}))
	// Stale metadata past the SWR window is not servable.
	assert.False(t, s.servable(KindMetadata, Meta{FetchedAt: time.Now().Add(-3 * time.Hour)}))
}

func TestProxy_ProduceMetadata(t *testing.T) {
	// A Produce route composes the body from upstream calls and caches it as metadata.
	var produceCalls int
	mirror := mirrorFunc{
		handles: func(r *http.Request) bool { return strings.HasPrefix(r.URL.Path, "/produce") },
		route: func(r *http.Request) (Route, error) {
			return Route{
				Key:         "metadata/produced.json",
				Kind:        KindMetadata,
				ContentType: "application/json",
				Produce: func(_ context.Context, _ Fetcher, _ string) ([]byte, string, error) {
					produceCalls++
					return []byte(`{"produced":true}`), "application/json", nil
				},
			}, nil
		},
	}
	srv := startProxy(t, []Mirror{mirror}, Options{})

	body := httpGet(t, srv.BaseURL()+"produce/x")
	assert.JSONEq(t, `{"produced":true}`, string(body))

	// Second request is served from cache; Produce is not called again.
	body2 := httpGet(t, srv.BaseURL()+"produce/x")
	assert.JSONEq(t, `{"produced":true}`, string(body2))
	assert.Equal(t, 1, produceCalls, "produced metadata is cached after the first request")
}

func TestProxy_ProduceUsesBoundFetcher(t *testing.T) {
	// The Produce hook composes its body from a propagated upstream call via the bound
	// Fetcher, exercising boundFetcher + buildUpstreamRequest.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("UPSTREAM-PIECE"))
	}))
	defer upstream.Close()

	mirror := mirrorFunc{
		handles: func(r *http.Request) bool { return strings.HasPrefix(r.URL.Path, "/compose") },
		route: func(r *http.Request) (Route, error) {
			return Route{
				Key:  "metadata/composed.json",
				Kind: KindMetadata,
				Produce: func(ctx context.Context, fetch Fetcher, _ string) ([]byte, string, error) {
					resp, err := fetch(ctx, UpstreamRequest{URL: upstream.URL})
					if err != nil {
						return nil, "", err
					}
					defer resp.Body.Close()
					body, err := io.ReadAll(resp.Body)
					return body, "text/plain", err
				},
			}, nil
		},
	}
	srv := startProxy(t, []Mirror{mirror}, Options{})

	body := httpGet(t, srv.BaseURL()+"compose/x")
	assert.Equal(t, "UPSTREAM-PIECE", string(body))
}

func TestProxy_ProduceArtifact(t *testing.T) {
	artifact := []byte("generated-artifact-tarball")
	var produceCalls int
	mirror := mirrorFunc{
		handles: func(r *http.Request) bool { return strings.HasPrefix(r.URL.Path, "/artifact") },
		route: func(r *http.Request) (Route, error) {
			return Route{
				Key:         "objects/generated.tar.gz",
				Kind:        KindArtifact,
				ContentType: "application/gzip",
				ProduceArtifact: func(_ context.Context) (io.ReadCloser, string, error) {
					produceCalls++
					return io.NopCloser(strings.NewReader(string(artifact))), "application/gzip", nil
				},
			}, nil
		},
	}
	srv := startProxy(t, []Mirror{mirror}, Options{})

	body := httpGet(t, srv.BaseURL()+"artifact/x")
	assert.Equal(t, artifact, body)

	body2 := httpGet(t, srv.BaseURL()+"artifact/x")
	assert.Equal(t, artifact, body2)
	assert.Equal(t, 1, produceCalls, "generated artifacts are cached after the first request")
}

func TestProxy_MetadataNoRewriteCachedVerbatim(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"verbatim":"value"}`))
	}))
	defer upstream.Close()

	mirror := mirrorFunc{
		handles: func(r *http.Request) bool { return strings.HasPrefix(r.URL.Path, "/raw") },
		route: func(r *http.Request) (Route, error) {
			return Route{
				Key:         "metadata/raw.json",
				Kind:        KindMetadata,
				Upstream:    UpstreamRequest{URL: upstream.URL},
				ContentType: "application/json",
			}, nil
		},
	}
	srv := startProxy(t, []Mirror{mirror}, Options{})

	body := httpGet(t, srv.BaseURL()+"raw/x")
	assert.JSONEq(t, `{"verbatim":"value"}`, string(body))
}

func TestProxy_PassthroughHeaderRewrite(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Terraform-Get", "https://upstream/archive.tar.gz")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	mirror := mirrorFunc{
		handles: func(r *http.Request) bool { return strings.HasPrefix(r.URL.Path, "/dl") },
		route: func(r *http.Request) (Route, error) {
			return Route{
				Kind:     KindPassthrough,
				Upstream: UpstreamRequest{URL: upstream.URL},
				HeaderRewrite: func(h http.Header, base string) {
					h.Set("X-Terraform-Get", base+"rewritten")
				},
			}, nil
		},
	}
	srv := startProxy(t, []Mirror{mirror}, Options{})

	resp, err := http.Get(srv.BaseURL() + "dl/mod")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, srv.BaseURL()+"rewritten", resp.Header.Get("X-Terraform-Get"))
}

func TestProxy_PassthroughUpstreamErrorIs502(t *testing.T) {
	// Bind then immediately close an upstream so the connection is refused.
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	deadURL := dead.URL
	dead.Close()

	mirror := mirrorFunc{
		handles: func(r *http.Request) bool { return strings.HasPrefix(r.URL.Path, "/dl") },
		route: func(r *http.Request) (Route, error) {
			return Route{Kind: KindPassthrough, Upstream: UpstreamRequest{URL: deadURL}}, nil
		},
	}
	srv := startProxy(t, []Mirror{mirror}, Options{Client: &http.Client{Timeout: 2 * time.Second}})

	resp, err := http.Get(srv.BaseURL() + "dl/mod")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadGateway, resp.StatusCode)
}
