package downloader

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSourceAuthorizationStaysOnTheExactOrigin(t *testing.T) {
	var externalAuthorization string
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		externalAuthorization = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(external.Close)
	var firstAuthorization string
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstAuthorization = r.Header.Get("Authorization")
		http.Redirect(w, r, external.URL+"/archive", http.StatusFound)
	}))
	t.Cleanup(registry.Close)
	ctx := WithSourceAuthorization(context.Background(), &schema.SourceAuthorization{Origin: registry.URL, Header: "Bearer registry-secret"})
	client := authorizedHTTPClient(ctx, &http.Client{})
	response, err := client.Get(registry.URL + "/archive")
	require.NoError(t, err)
	defer response.Body.Close()
	_, err = io.Copy(io.Discard, response.Body)
	require.NoError(t, err)
	require.Equal(t, "Bearer registry-secret", firstAuthorization)
	require.Empty(t, externalAuthorization)
}
