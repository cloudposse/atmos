package github

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/cache"
)

// testBackend wires a Backend to a test server.
func testBackend(srv *httptest.Server) *Backend {
	return &Backend{
		client:     newTwirpClient(srv.URL+"/", "test-token"),
		blobClient: srv.Client(),
		restClient: srv.Client(),
		baseURL:    srv.URL,
		owner:      "o",
		repo:       "r",
		version:    "test-version",
	}
}

func TestBackend_SaveRoundTrip(t *testing.T) {
	var uploaded []byte
	var finalized bool

	mux := http.NewServeMux()
	mux.HandleFunc("/twirp/github.actions.results.api.v1.CacheService/CreateCacheEntry", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(createCacheEntryResponse{OK: true, SignedUploadURL: srvURL(r) + "/upload"})
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		uploaded, _ = io.ReadAll(r.Body)
		assert.Equal(t, "BlockBlob", r.Header.Get("x-ms-blob-type"))
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("/twirp/github.actions.results.api.v1.CacheService/FinalizeCacheEntryUpload", func(w http.ResponseWriter, _ *http.Request) {
		finalized = true
		_ = json.NewEncoder(w).Encode(finalizeCacheEntryResponse{OK: true, EntryID: "1"})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	b := testBackend(srv)
	payload := []byte("archive-bytes")
	err := b.Save(context.Background(), "k1", bytes.NewReader(payload), int64(len(payload)))
	require.NoError(t, err)
	assert.Equal(t, payload, uploaded)
	assert.True(t, finalized)
}

func TestBackend_SaveAlreadyExists(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/twirp/github.actions.results.api.v1.CacheService/CreateCacheEntry", func(w http.ResponseWriter, _ *http.Request) {
		// ok=false signals an existing entry for this key+version.
		_ = json.NewEncoder(w).Encode(createCacheEntryResponse{OK: false})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	b := testBackend(srv)
	err := b.Save(context.Background(), "k1", strings.NewReader("x"), 1)
	require.ErrorIs(t, err, errUtils.ErrCacheAlreadyExists)
}

func TestBackend_RestoreExact(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/twirp/github.actions.results.api.v1.CacheService/GetCacheEntryDownloadURL", func(w http.ResponseWriter, r *http.Request) {
		var req getCacheEntryDownloadURLRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		_ = json.NewEncoder(w).Encode(getCacheEntryDownloadURLResponse{
			OK:                true,
			SignedDownloadURL: srvURL(r) + "/download",
			MatchedKey:        req.Key,
		})
	})
	mux.HandleFunc("/download", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("cached-content"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	b := testBackend(srv)
	matched, rc, err := b.Restore(context.Background(), "k1", nil)
	require.NoError(t, err)
	defer rc.Close()
	assert.Equal(t, "k1", matched)
	body, _ := io.ReadAll(rc)
	assert.Equal(t, "cached-content", string(body))
}

func TestBackend_RestorePrefixFallback(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/twirp/github.actions.results.api.v1.CacheService/GetCacheEntryDownloadURL", func(w http.ResponseWriter, r *http.Request) {
		var req getCacheEntryDownloadURLRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		require.NotEmpty(t, req.RestoreKeys)
		_ = json.NewEncoder(w).Encode(getCacheEntryDownloadURLResponse{
			OK:                true,
			SignedDownloadURL: srvURL(r) + "/download",
			MatchedKey:        "k-old",
		})
	})
	mux.HandleFunc("/download", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("old"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	b := testBackend(srv)
	matched, rc, err := b.Restore(context.Background(), "k-new", []string{"k-"})
	require.NoError(t, err)
	defer rc.Close()
	assert.Equal(t, "k-old", matched)
}

func TestBackend_RestoreMiss(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/twirp/github.actions.results.api.v1.CacheService/GetCacheEntryDownloadURL", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(getCacheEntryDownloadURLResponse{OK: false})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	b := testBackend(srv)
	_, _, err := b.Restore(context.Background(), "missing", nil)
	require.ErrorIs(t, err, errUtils.ErrCacheNotFound)
}

func TestBackend_ListAndDelete(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/o/r/actions/caches", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(listCachesResponse{
				TotalCount: 2,
				ActionsCache: []githubCache{
					{ID: 1, Key: "atmos-cache-a", SizeInBytes: 10},
					{ID: 2, Key: "other-b", SizeInBytes: 20},
				},
			})
		case http.MethodDelete:
			assert.Equal(t, "atmos-cache-a", r.URL.Query().Get("key"))
			w.WriteHeader(http.StatusOK)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	b := testBackend(srv)

	entries, err := b.List(context.Background(), cache.ListOptions{KeyPrefix: "atmos-cache-"})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "atmos-cache-a", entries[0].Key)
	assert.Equal(t, int64(10), entries[0].Size)

	require.NoError(t, b.Delete(context.Background(), "atmos-cache-a"))
}

func TestNewBackend_UnavailableOutsideRunner(t *testing.T) {
	t.Setenv("ACTIONS_RUNTIME_TOKEN", "")
	t.Setenv("ACTIONS_RESULTS_URL", "")

	_, err := NewBackend(cache.Options{})
	require.ErrorIs(t, err, errUtils.ErrCacheUnavailable)
}

func TestBackend_SaveBlobUploadFails(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/twirp/github.actions.results.api.v1.CacheService/CreateCacheEntry", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(createCacheEntryResponse{OK: true, SignedUploadURL: srvURL(r) + "/upload"})
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	b := testBackend(srv)
	err := b.Save(context.Background(), "k1", strings.NewReader("x"), 1)
	require.ErrorIs(t, err, errUtils.ErrCacheSaveFailed)
}

func TestBackend_SaveFinalizeRejected(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/twirp/github.actions.results.api.v1.CacheService/CreateCacheEntry", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(createCacheEntryResponse{OK: true, SignedUploadURL: srvURL(r) + "/upload"})
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("/twirp/github.actions.results.api.v1.CacheService/FinalizeCacheEntryUpload", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(finalizeCacheEntryResponse{OK: false})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	b := testBackend(srv)
	err := b.Save(context.Background(), "k1", strings.NewReader("x"), 1)
	require.ErrorIs(t, err, errUtils.ErrCacheSaveFailed)
}

func TestBackend_SaveTwirpStatusError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/twirp/github.actions.results.api.v1.CacheService/CreateCacheEntry", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	b := testBackend(srv)
	err := b.Save(context.Background(), "k1", strings.NewReader("x"), 1)
	require.ErrorIs(t, err, errUtils.ErrCacheBackendRequest)
}

func TestBackend_RestoreDownloadFails(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/twirp/github.actions.results.api.v1.CacheService/GetCacheEntryDownloadURL", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(getCacheEntryDownloadURLResponse{
			OK:                true,
			SignedDownloadURL: srvURL(r) + "/download",
			MatchedKey:        "k1",
		})
	})
	mux.HandleFunc("/download", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("gone"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	b := testBackend(srv)
	_, _, err := b.Restore(context.Background(), "k1", nil)
	require.ErrorIs(t, err, errUtils.ErrCacheRestoreFailed)
}

func TestBackend_RestoreTwirpError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/twirp/github.actions.results.api.v1.CacheService/GetCacheEntryDownloadURL", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	b := testBackend(srv)
	_, _, err := b.Restore(context.Background(), "k1", nil)
	require.ErrorIs(t, err, errUtils.ErrCacheBackendRequest)
}

func TestBackend_ListPagination(t *testing.T) {
	var pagesSeen []string
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/o/r/actions/caches", func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		pagesSeen = append(pagesSeen, page)
		switch page {
		case "1":
			w.Header().Set("Link", `<`+srvURL(r)+`/repos/o/r/actions/caches?per_page=100&page=2>; rel="next"`)
			_ = json.NewEncoder(w).Encode(listCachesResponse{
				TotalCount:   2,
				ActionsCache: []githubCache{{ID: 1, Key: "atmos-a", SizeInBytes: 10}},
			})
		case "2":
			_ = json.NewEncoder(w).Encode(listCachesResponse{
				TotalCount:   2,
				ActionsCache: []githubCache{{ID: 2, Key: "atmos-b", SizeInBytes: 20}},
			})
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	b := testBackend(srv)
	entries, err := b.List(context.Background(), cache.ListOptions{KeyPrefix: "atmos-"})
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, []string{"1", "2"}, pagesSeen)
	// Newest first sort; both have zero CreatedAt so order is stable by insertion.
	keys := []string{entries[0].Key, entries[1].Key}
	assert.Contains(t, keys, "atmos-a")
	assert.Contains(t, keys, "atmos-b")
}

func TestBackend_ListStatusError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/o/r/actions/caches", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("forbidden"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	b := testBackend(srv)
	_, err := b.List(context.Background(), cache.ListOptions{})
	require.ErrorIs(t, err, errUtils.ErrCacheListFailed)
}

func TestBackend_ListRequiresOwnerRepo(t *testing.T) {
	b := &Backend{}
	_, err := b.List(context.Background(), cache.ListOptions{})
	require.ErrorIs(t, err, errUtils.ErrCacheListFailed)
}

func TestBackend_DeleteRequiresOwnerRepo(t *testing.T) {
	b := &Backend{}
	err := b.Delete(context.Background(), "k1")
	require.ErrorIs(t, err, errUtils.ErrCacheDeleteFailed)
}

func TestBackend_DeleteNotFoundIsNoOp(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/o/r/actions/caches", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	b := testBackend(srv)
	require.NoError(t, b.Delete(context.Background(), "missing"))
}

func TestBackend_DeleteStatusError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/o/r/actions/caches", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server error"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	b := testBackend(srv)
	err := b.Delete(context.Background(), "k1")
	require.ErrorIs(t, err, errUtils.ErrCacheDeleteFailed)
}

func TestTwirpCall_DecodeError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/twirp/github.actions.results.api.v1.CacheService/CreateCacheEntry", func(w http.ResponseWriter, _ *http.Request) {
		// Valid 200 but non-JSON body forces a decode error.
		_, _ = w.Write([]byte("not json"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTwirpClient(srv.URL+"/", "tok")
	_, err := c.CreateCacheEntry(context.Background(), &createCacheEntryRequest{Key: "k", Version: "v"})
	require.Error(t, err)
}

// srvURL reconstructs the base URL of the test server from a request.
func srvURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
