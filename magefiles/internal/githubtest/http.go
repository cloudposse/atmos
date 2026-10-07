//go:build mage

package githubtest

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// Server exercises the real go-gh client against a local HTTP server. No
// request can escape to GitHub or use the developer's stored credentials.
func Server(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	t.Setenv("GH_HOST", "github.localhost")
	t.Setenv("GH_TOKEN", "test-token")
	t.Setenv("GH_ENTERPRISE_TOKEN", "test-token")
	t.Setenv("GH_DEBUG", "")
	original := http.DefaultTransport
	transport := &http.Transport{DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		require.Equal(t, "api.github.localhost:80", addr)
		var dialer net.Dialer
		return dialer.DialContext(ctx, network, server.Listener.Addr().String())
	}}
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = original; transport.CloseIdleConnections() })
}

func JSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(value))
}
