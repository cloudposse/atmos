package terraform_backend

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

// TestCreateGCSClient_HonorsIdentityEndpoint verifies the GCS client builds against the
// active identity's emulator endpoint (WithoutAuthentication) instead of requiring real
// GCP credentials.
func TestCreateGCSClient_HonorsIdentityEndpoint(t *testing.T) {
	backend := map[string]any{}
	ac := &schema.AuthContext{GCP: &schema.GCPAuthContext{
		StorageEmulatorHost:   "http://localhost:34568",
		WithoutAuthentication: true,
	}}

	client, err := createGCSClient(context.Background(), &backend, ac)
	require.NoError(t, err)
	require.NotNil(t, client)
}

// TestNewAzureBlobClient_FromConnectionString verifies the azurerm reader builds a blob
// client from an identity-provided connection string (the Azurite emulator path),
// bypassing DefaultAzureCredential.
func TestNewAzureBlobClient_FromConnectionString(t *testing.T) {
	client, err := newAzureBlobClient("ignored-account", "blob.core.windows.net", testAzuriteConnString, &azblob.ClientOptions{})
	require.NoError(t, err)
	require.NotNil(t, client)
}

// TestGetCachedAzureBlobClient_ConnectionStringIsolatesCache verifies that an
// identity-provided connection string yields a distinct cache entry, so an
// emulator-backed read never aliases a real-Azure cached client (and vice versa).
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
}
