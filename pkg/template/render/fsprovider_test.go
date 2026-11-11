package render

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHTTPDatasourceUsesOneGet verifies an http datasource is read with a
// single GET, as gomplate v3 did, so a server that rejects HEAD still works.
func TestHTTPDatasourceUsesOneGet(t *testing.T) {
	var mu sync.Mutex
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		methods = append(methods, r.Method)
		mu.Unlock()
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusNotImplemented)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"name":"fixture"}`)
	}))
	defer server.Close()

	out, err := New().Render(context.Background(), &Request{
		Name:        "t",
		Text:        `{{ (ds "cfg").name }}`,
		Datasources: map[string]Datasource{"cfg": {URL: server.URL + "/config.json"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "fixture", out)
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{http.MethodGet}, methods)
}

// TestDatasourceFSProviderDelegatesOtherSchemes verifies only http and https
// are replaced.
func TestDatasourceFSProviderDelegatesOtherSchemes(t *testing.T) {
	provider, ok := gomplateProvider().(datasourceFSProvider)
	require.True(t, ok, "gomplate.DefaultFSProvider is wrapped at init")
	assert.Contains(t, provider.Schemes(), "file")
	assert.Contains(t, provider.Schemes(), "https")
}
