package terraform_backend

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Azurite's well-known development connection string (public, fixed test values).
// AccountKey "a2V5" is the base64 encoding of "key" — enough for offline client construction.
const testAzuriteConnString = "DefaultEndpointsProtocol=http;AccountName=devstoreaccount1;" +
	"AccountKey=a2V5;BlobEndpoint=http://localhost:10000/devstoreaccount1;"

// TestIdentityGCSEndpoint verifies the GCS endpoint helper is nil-safe and reads
// StorageEmulatorHost from the active GCP identity.
func TestIdentityGCSEndpoint(t *testing.T) {
	assert.Empty(t, identityGCSEndpoint(nil))
	assert.Empty(t, identityGCSEndpoint(&schema.AuthContext{}))
	assert.Empty(t, identityGCSEndpoint(&schema.AuthContext{GCP: &schema.GCPAuthContext{}}))
	assert.Equal(t, "http://localhost:34568",
		identityGCSEndpoint(&schema.AuthContext{GCP: &schema.GCPAuthContext{StorageEmulatorHost: "http://localhost:34568"}}))
}

// TestIdentityAzureConnectionString verifies the Azure connection-string helper is
// nil-safe and reads StorageConnectionString from the active Azure identity.
func TestIdentityAzureConnectionString(t *testing.T) {
	assert.Empty(t, identityAzureConnectionString(nil))
	assert.Empty(t, identityAzureConnectionString(&schema.AuthContext{}))
	assert.Empty(t, identityAzureConnectionString(&schema.AuthContext{Azure: &schema.AzureAuthContext{}}))
	assert.Equal(t, "conn",
		identityAzureConnectionString(&schema.AuthContext{Azure: &schema.AzureAuthContext{StorageConnectionString: "conn"}}))
}

// TestCreateGCSClient_HonorsIdentityEndpoint verifies the GCS client built for the
// emulator actually routes requests to the identity-provided endpoint rather than
// falling back to ADC / real GCS. A local httptest server stands in for the emulator;
// the client is expected to reach it when a read is attempted.
func TestCreateGCSClient_HonorsIdentityEndpoint(t *testing.T) {
	// Start a local HTTP server that records whether it received any request.
	requestReceived := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestReceived = true
		// Return a minimal JSON that the GCS storage client will accept as an
		// error response rather than a silent empty body.
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	backend := map[string]any{}
	ac := &schema.AuthContext{GCP: &schema.GCPAuthContext{
		// Use the httptest server address — no pre-existing scheme so the normalizer
		// must add "http://" to make this a valid URL.
		StorageEmulatorHost:   strings.TrimPrefix(srv.URL, "http://"),
		WithoutAuthentication: true,
	}}

	client, err := createGCSClient(context.Background(), &backend, ac)
	require.NoError(t, err)
	require.NotNil(t, client)

	// Drive one read against the fake server.  The call will fail (no real bucket),
	// but the important thing is that requestReceived flips to true, which proves
	// the emulator transport was wired and the client did NOT fall back to ADC.
	_, _ = ReadTerraformBackendGCSInternal(client, &map[string]any{}, &map[string]any{"bucket": "test-bucket"})
	assert.True(t, requestReceived, "GCS client must route requests to the identity-provided emulator endpoint, not ADC")
}

// TestNewAzureBlobClient_FromConnectionString verifies the azurerm reader builds a blob
// client from an identity-provided connection string (the Azurite emulator path),
// bypassing DefaultAzureCredential, and that the client URL targets the Azurite endpoint.
func TestNewAzureBlobClient_FromConnectionString(t *testing.T) {
	client, err := newAzureBlobClient("ignored-account", "blob.core.windows.net", testAzuriteConnString, &azblob.ClientOptions{})
	require.NoError(t, err)
	require.NotNil(t, client)

	// Assert the client actually targets the Azurite endpoint declared in the connection
	// string, not a real Azure storage URL.  This proves the connection-string path ran
	// (not the DefaultAzureCredential path) even when Azure creds are present in CI.
	clientURL := client.URL()
	assert.True(
		t,
		strings.HasPrefix(clientURL, "http://localhost:10000/"),
		"client URL must target Azurite endpoint, got: %s", clientURL,
	)
}

// TestGetCachedAzureBlobClient_ConnectionStringIsolatesCache verifies that an
// identity-provided connection string yields a distinct cache entry, so an
// emulator-backed read never aliases a real-Azure cached client (and vice versa).
// It also asserts that repeated calls with the same connection string return the
// same (pointer-equal) cached client, confirming identity-keyed caching is active.
func TestGetCachedAzureBlobClient_ConnectionStringIsolatesCache(t *testing.T) {
	const account = "isoaccount"
	baseKey := account + ":blob.core.windows.net"

	mock := &mockAzureBlobClient{
		downloadStreamFunc: func(_ context.Context, _, _ string, _ *azblob.DownloadStreamOptions) (AzureBlobDownloadResponse, error) {
			return createMockDownloadResponse(`{"cached":true}`), nil
		},
	}
	azureBlobClientCache.Store(baseKey, mock)
	defer azureBlobClientCache.Delete(baseKey)

	backend := map[string]any{"storage_account_name": account}

	// No identity connection string -> base key -> the seeded mock is returned.
	got, err := getCachedAzureBlobClient(&backend, nil)
	require.NoError(t, err)
	_, isMock := got.(*mockAzureBlobClient)
	assert.True(t, isMock, "real-cloud read uses the base cache key")

	// With an Azurite connection string -> distinct key -> NOT the seeded mock.
	h := sha256.Sum256([]byte(testAzuriteConnString))
	connKey := baseKey + ":conn=" + hex.EncodeToString(h[:8])
	defer azureBlobClientCache.Delete(connKey)

	ac := &schema.AuthContext{Azure: &schema.AzureAuthContext{StorageConnectionString: testAzuriteConnString}}
	got2, err := getCachedAzureBlobClient(&backend, ac)
	require.NoError(t, err)
	_, isMock2 := got2.(*mockAzureBlobClient)
	assert.False(t, isMock2, "emulator-backed read must not alias the base-key cached client")

	// Assert the identity-keyed client is actually stored in the cache under the derived
	// key (not just that a different object came back).  A second call must return the
	// exact same pointer, proving caching — not construction — is serving the result.
	got3, err := getCachedAzureBlobClient(&backend, ac)
	require.NoError(t, err)
	assert.Same(t, got2, got3,
		"repeated calls with the same connection string must return the same cached client (pointer reuse)")

	// Also verify the cache entry itself is present at the expected key.
	cachedVal, ok := azureBlobClientCache.Load(connKey)
	assert.True(t, ok, "identity-keyed cache entry must exist at key %q", connKey)
	assert.Same(t, got2, cachedVal.(AzureBlobAPI),
		"value stored in cache must be the same object returned by getCachedAzureBlobClient")
}
