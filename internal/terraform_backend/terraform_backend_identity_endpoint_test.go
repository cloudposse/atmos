package terraform_backend

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/gcp"
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

// TestGCSAuthOptions verifies the GCS reader authenticates with the target identity's
// credentials, and never silently falls back to ambient credentials in their place.
func TestGCSAuthOptions(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	identity := func(g *schema.GCPAuthContext) *schema.AuthContext { return &schema.AuthContext{GCP: g} }
	credsFile := filepath.Join(t.TempDir(), "target-credentials.json")
	refreshableFile := writeGCSCredsFile(t, `{"type":"service_account"}`)

	tests := []struct {
		name        string
		credentials string
		authContext *schema.AuthContext
		want        gcp.AuthOptions
	}{
		{name: "no identity uses ambient", want: gcp.AuthOptions{}},
		{name: "nil gcp identity uses ambient", authContext: &schema.AuthContext{}, want: gcp.AuthOptions{}},
		{
			name:        "explicit backend credentials take precedence over the identity",
			credentials: "/backend/creds.json",
			authContext: identity(&schema.GCPAuthContext{AccessToken: "target-token", CredentialsFile: credsFile}),
			want:        gcp.AuthOptions{Credentials: "/backend/creds.json"},
		},
		{
			name:        "identity access token is used",
			authContext: identity(&schema.GCPAuthContext{AccessToken: "target-token", TokenExpiry: now.Add(time.Hour), CredentialsFile: credsFile}),
			want:        gcp.AuthOptions{AccessToken: "target-token", TokenExpiry: now.Add(time.Hour)},
		},
		{
			name:        "identity token without expiry is used",
			authContext: identity(&schema.GCPAuthContext{AccessToken: "target-token"}),
			want:        gcp.AuthOptions{AccessToken: "target-token"},
		},
		{
			name:        "expired identity token falls back to a refreshable credentials file",
			authContext: identity(&schema.GCPAuthContext{AccessToken: "stale", TokenExpiry: now.Add(-time.Minute), CredentialsFile: refreshableFile}),
			want:        gcp.AuthOptions{Credentials: refreshableFile},
		},
		{
			name:        "identity credentials file is used without a token",
			authContext: identity(&schema.GCPAuthContext{CredentialsFile: credsFile}),
			want:        gcp.AuthOptions{Credentials: credsFile},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := gcsAuthOptions(tt.credentials, tt.authContext, now)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// writeGCSCredsFile writes a credentials file with the given JSON content and returns its path.
func writeGCSCredsFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "creds.json")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// TestGCSAuthOptions_ExpiredTokenWithUnrefreshableCredentialsFileFails verifies an expired token
// is an error when the credentials file only replays that token, and that refreshable files are kept.
func TestGCSAuthOptions_ExpiredTokenWithUnrefreshableCredentialsFileFails(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	expired := func(file string) *schema.AuthContext {
		return &schema.AuthContext{GCP: &schema.GCPAuthContext{AccessToken: "stale", TokenExpiry: now.Add(-time.Minute), CredentialsFile: file}}
	}

	tests := []struct {
		name    string
		file    string
		wantErr bool
	}{
		{name: "authorized_user with only an access token", file: writeGCSCredsFile(t, `{"type":"authorized_user","access_token":"stale","client_id":"c","client_secret":"s"}`), wantErr: true},
		{name: "missing file", file: filepath.Join(t.TempDir(), "missing.json"), wantErr: true},
		{name: "malformed file", file: writeGCSCredsFile(t, `not json`), wantErr: true},
		{name: "authorized_user with a refresh token", file: writeGCSCredsFile(t, `{"type":"authorized_user","refresh_token":"r"}`)},
		{name: "service account key", file: writeGCSCredsFile(t, `{"type":"service_account"}`)},
		{name: "external account", file: writeGCSCredsFile(t, `{"type":"external_account"}`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := gcsAuthOptions("", expired(tt.file), now)
			if tt.wantErr {
				require.ErrorIs(t, err, errUtils.ErrGCSIdentityTokenExpired)
				assert.Equal(t, gcp.AuthOptions{}, opts)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, gcp.AuthOptions{Credentials: tt.file}, opts)
		})
	}

	t.Run("valid token is unaffected by an unrefreshable file", func(t *testing.T) {
		file := writeGCSCredsFile(t, `{"type":"authorized_user","access_token":"t"}`)
		ac := &schema.AuthContext{GCP: &schema.GCPAuthContext{AccessToken: "t", TokenExpiry: now.Add(time.Hour), CredentialsFile: file}}
		opts, err := gcsAuthOptions("", ac, now)
		require.NoError(t, err)
		assert.Equal(t, gcp.AuthOptions{AccessToken: "t", TokenExpiry: now.Add(time.Hour)}, opts)
	})
}

// TestGCSAuthOptions_ExpiredTokenWithoutCredentialsFileFails verifies an expired identity token
// with no credentials file is an error rather than empty options that would use ambient credentials.
func TestGCSAuthOptions_ExpiredTokenWithoutCredentialsFileFails(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	ac := &schema.AuthContext{GCP: &schema.GCPAuthContext{AccessToken: "stale", TokenExpiry: now.Add(-time.Minute)}}

	opts, err := gcsAuthOptions("", ac, now)
	require.ErrorIs(t, err, errUtils.ErrGCSIdentityTokenExpired)
	assert.Equal(t, gcp.AuthOptions{}, opts)

	t.Run("explicit backend credentials still win", func(t *testing.T) {
		opts, err := gcsAuthOptions("/backend/creds.json", ac, now)
		require.NoError(t, err)
		assert.Equal(t, gcp.AuthOptions{Credentials: "/backend/creds.json"}, opts)
	})

	t.Run("client creation fails instead of using ambient credentials", func(t *testing.T) {
		client, err := createGCSClient(context.Background(), &map[string]any{}, ac)
		require.ErrorIs(t, err, errUtils.ErrGCSIdentityTokenExpired)
		assert.Nil(t, client)

		client, err = getCachedGCSClient(&map[string]any{}, ac)
		require.ErrorIs(t, err, errUtils.ErrGCSIdentityTokenExpired)
		assert.Nil(t, client)
	})

	t.Run("emulator identity is not affected", func(t *testing.T) {
		emu := &schema.AuthContext{GCP: &schema.GCPAuthContext{
			AccessToken: "stale", TokenExpiry: now.Add(-time.Minute), StorageEmulatorHost: "localhost:9000", WithoutAuthentication: true,
		}}
		client, err := createGCSClient(context.Background(), &map[string]any{}, emu)
		require.NoError(t, err)
		assert.NotNil(t, client)
	})
}

// TestGCSClientCacheKey_IsolatesTargetCredentials verifies two identities never share a client.
func TestGCSClientCacheKey_IsolatesTargetCredentials(t *testing.T) {
	now := time.Now()
	key := func(ac *schema.AuthContext) string {
		opts, err := gcsAuthOptions("", ac, now)
		require.NoError(t, err)
		return gcsClientCacheKey(&opts, "", "")
	}
	ambient := key(nil)
	a := key(&schema.AuthContext{GCP: &schema.GCPAuthContext{AccessToken: "token-a"}})
	b := key(&schema.AuthContext{GCP: &schema.GCPAuthContext{AccessToken: "token-b"}})
	file := key(&schema.AuthContext{GCP: &schema.GCPAuthContext{CredentialsFile: filepath.Join(t.TempDir(), "c.json")}})

	assert.Len(t, map[string]struct{}{ambient: {}, a: {}, b: {}, file: {}}, 4, "ambient and every target identity need distinct keys")
	assert.NotContains(t, a, "token-a", "the token must not appear in the cache key")
	assert.Equal(t, a, key(&schema.AuthContext{GCP: &schema.GCPAuthContext{AccessToken: "token-a"}}), "the same identity reuses its client")
}
