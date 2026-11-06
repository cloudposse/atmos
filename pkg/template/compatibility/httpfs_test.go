package compatibility

import (
	"context"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requestLog records the method and path of every request a test server sees.
type requestLog struct {
	mu       sync.Mutex
	requests []string
	headers  []http.Header
}

func (l *requestLog) record(r *http.Request) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.requests = append(l.requests, r.Method+" "+r.URL.RequestURI())
	l.headers = append(l.headers, r.Header.Clone())
}

// newServer serves /config.json and rejects HEAD with 501, like the
// compatibility corpus's no-head fixture.
func newServer(t *testing.T, log *requestLog) *url.URL {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.record(r)
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusNotImplemented)
			return
		}
		if r.URL.Path != "/data/config.json" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"name":"fixture"}`)
	}))
	t.Cleanup(server.Close)
	base, err := url.Parse(server.URL + "/data/")
	require.NoError(t, err)
	return base
}

func TestHTTPFSStatAndReadMakeOneGet(t *testing.T) {
	log := &requestLog{}
	fsys, err := NewHTTPFS(newServer(t, log))
	require.NoError(t, err)

	file, err := fsys.Open("config.json")
	require.NoError(t, err)
	info, err := file.Stat()
	require.NoError(t, err, "a server that rejects HEAD must still be readable")
	data, err := io.ReadAll(file)
	require.NoError(t, err)
	require.NoError(t, file.Close())

	assert.JSONEq(t, `{"name":"fixture"}`, string(data))
	assert.Equal(t, "config.json", info.Name())
	assert.False(t, info.IsDir())
	assert.Equal(t, fs.FileMode(0o444), info.Mode())
	assert.Nil(t, info.Sys())
	assert.Equal(t, "application/json", info.(interface{ ContentType() string }).ContentType())
	assert.Equal(t, []string{"GET /data/config.json"}, log.requests, "exactly one GET, no HEAD")
}

func TestHTTPFSReadFileSubAndHeaders(t *testing.T) {
	log := &requestLog{}
	base := newServer(t, log)
	fsys, err := NewHTTPFS(&url.URL{Scheme: base.Scheme, Host: base.Host, Path: "/", RawQuery: "token=base"})
	require.NoError(t, err)

	type withHeader interface{ WithHeader(http.Header) fs.FS }
	type withContext interface{ WithContext(context.Context) fs.FS }
	type withClient interface{ WithHTTPClient(*http.Client) fs.FS }
	fsys = fsys.(withHeader).WithHeader(http.Header{"X-Fixture": {"compatibility"}})
	fsys = fsys.(withContext).WithContext(context.Background())
	fsys = fsys.(withClient).WithHTTPClient(http.DefaultClient)

	sub, err := fsys.(fs.SubFS).Sub("data/")
	require.NoError(t, err)
	data, err := sub.(fs.ReadFileFS).ReadFile("config.json")
	require.NoError(t, err)
	assert.JSONEq(t, `{"name":"fixture"}`, string(data))

	require.Len(t, log.requests, 1)
	assert.Equal(t, "GET /data/config.json?token=base", log.requests[0], "base query parameters are kept")
	assert.Equal(t, "compatibility", log.headers[0].Get("X-Fixture"))
}

func TestHTTPFSNilOptionsKeepTheFilesystem(t *testing.T) {
	fsys, err := NewHTTPFS(&url.URL{Scheme: "https", Host: "example.invalid", Path: "/"})
	require.NoError(t, err)
	assert.Same(t, fsys, fsys.(interface{ WithHeader(http.Header) fs.FS }).WithHeader(nil))
	assert.Same(t, fsys, fsys.(interface {
		WithContext(context.Context) fs.FS
	}).WithContext(nil)) //nolint:staticcheck // A nil context must leave the filesystem unchanged.
	assert.Same(t, fsys, fsys.(interface{ WithHTTPClient(*http.Client) fs.FS }).WithHTTPClient(nil))
}

func TestHTTPFSErrors(t *testing.T) {
	log := &requestLog{}
	fsys, err := NewHTTPFS(newServer(t, log))
	require.NoError(t, err)

	_, err = fsys.Open("../escape")
	require.ErrorIs(t, err, fs.ErrInvalid)

	missing, err := fsys.Open("missing.json")
	require.NoError(t, err)
	_, err = missing.Stat()
	require.ErrorIs(t, err, errHTTPStatus)
	_, err = missing.Read(make([]byte, 1))
	require.ErrorIs(t, err, errHTTPStatus, "the failed GET is not retried")
	require.NoError(t, missing.Close())
	assert.Equal(t, []string{"GET /data/missing.json"}, log.requests)

	unreachable, err := NewHTTPFS(&url.URL{Scheme: "http", Host: "127.0.0.1:1", Path: "/"})
	require.NoError(t, err)
	_, err = unreachable.(fs.ReadFileFS).ReadFile("config.json")
	require.Error(t, err)
}
