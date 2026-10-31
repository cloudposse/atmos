package signature

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestPublicTrustFailsWhenCacheDirectoryCannotBeCreated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	t.Setenv("ATMOS_XDG_DATA_HOME", path)
	trusted, err := LoadPublicRoot(t.Context(), true)
	require.Error(t, err)
	require.Nil(t, trusted)
}

func TestTrustHTTPClientUsesVerificationContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/root.json", request.URL.Path)
		_, err := io.WriteString(w, "root")
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/root.json", nil)
	require.NoError(t, err)
	client := contextClient{ctx: t.Context(), client: server.Client()}
	response, err := client.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, "root", string(body))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	client.ctx = ctx
	response, err = client.Do(request)
	if response != nil {
		response.Body.Close()
	}
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, response)
	_, err = (offlineFetcher{}).DownloadFile(server.URL, 1024, 0)
	require.ErrorIs(t, err, errUtils.ErrSignatureOffline)
}

func TestOfflineTrustCannotFetch(t *testing.T) {
	t.Setenv("ATMOS_XDG_DATA_HOME", t.TempDir())
	_, err := LoadPublicRoot(t.Context(), true)
	require.Error(t, err)
}
