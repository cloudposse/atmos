package store

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptrTo[T any](v T) *T { return &v }

func TestNewAzureKeyVaultStore_EndpointFallback(t *testing.T) {
	tests := []struct {
		name         string
		vaultURL     string
		endpoint     *string
		wantVaultURL string
		wantErr      bool
	}{
		{
			name:     "no vault url and no endpoint errors",
			vaultURL: "",
			endpoint: nil,
			wantErr:  true,
		},
		{
			name:         "endpoint used when vault url empty",
			vaultURL:     "",
			endpoint:     ptrTo("https://endpoint.vault.azure.net"),
			wantVaultURL: "https://endpoint.vault.azure.net",
		},
		{
			name:         "vault url takes precedence over endpoint",
			vaultURL:     "https://vault.vault.azure.net",
			endpoint:     ptrTo("https://endpoint.vault.azure.net"),
			wantVaultURL: "https://vault.vault.azure.net",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewAzureKeyVaultStore(AzureKeyVaultStoreOptions{
				VaultURL:              tt.vaultURL,
				Endpoint:              tt.endpoint,
				WithoutAuthentication: true, // avoid contacting the Azure credential chain.
			}, "identity") // identity defers client init so no network call is made.
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrVaultURLRequired)
				return
			}
			require.NoError(t, err)
			akv, ok := s.(*AzureKeyVaultStore)
			require.True(t, ok)
			assert.Equal(t, tt.wantVaultURL, akv.vaultURL)
		})
	}
}

func TestNewAzureKeyVaultStore_EndpointInsecureRewrite(t *testing.T) {
	s, err := NewAzureKeyVaultStore(AzureKeyVaultStoreOptions{
		Endpoint:              ptrTo("http://127.0.0.1:8443"),
		EndpointInsecure:      true,
		WithoutAuthentication: true,
	}, "identity")
	require.NoError(t, err)

	akv, ok := s.(*AzureKeyVaultStore)
	require.True(t, ok)

	// The http:// endpoint is rewritten to https:// so the SDK accepts it...
	assert.Equal(t, "https://127.0.0.1:8443", akv.vaultURL)
	// ...and an insecure transport is installed to rewrite requests back to http on the wire.
	require.NotNil(t, akv.clientOptions)
	_, ok = akv.clientOptions.Transport.(azureInsecureEndpointTransport)
	assert.True(t, ok, "expected azureInsecureEndpointTransport to be installed")
}

func TestNewAzureKeyVaultStore_WithoutAuthenticationSetsInsecureFlag(t *testing.T) {
	s, err := NewAzureKeyVaultStore(AzureKeyVaultStoreOptions{
		Endpoint:              ptrTo("https://vault.vault.azure.net"),
		WithoutAuthentication: true,
	}, "identity")
	require.NoError(t, err)

	akv, ok := s.(*AzureKeyVaultStore)
	require.True(t, ok)
	require.NotNil(t, akv.clientOptions)
	assert.True(t, akv.clientOptions.InsecureAllowCredentialWithHTTP)
	assert.True(t, akv.withoutAuth)
}

func TestAzureKeyVaultStore_DefaultCredential_WithoutAuth(t *testing.T) {
	s := &AzureKeyVaultStore{withoutAuth: true}

	cred, err := s.defaultCredential(nil)
	require.NoError(t, err)
	_, ok := cred.(localAzureTokenCredential)
	assert.True(t, ok, "expected localAzureTokenCredential stub when withoutAuth is set")
}

func TestAzureKeyVaultStore_InitDefaultClient_WithoutAuth(t *testing.T) {
	s := &AzureKeyVaultStore{
		vaultURL:      "https://x.vault.azure.net",
		withoutAuth:   true,
		clientOptions: &azsecrets.ClientOptions{},
	}

	require.NoError(t, s.initDefaultClient())
	assert.NotNil(t, s.client)
}

func TestLocalAzureTokenCredential_GetToken(t *testing.T) {
	cred := localAzureTokenCredential{}

	token, err := cred.GetToken(context.Background(), policy.TokenRequestOptions{})
	require.NoError(t, err)
	assert.Equal(t, "floci-local-token", token.Token)
	assert.WithinDuration(t, time.Now().Add(time.Hour), token.ExpiresOn, time.Minute)
}

// fakeTransporter records the request it receives so the test can inspect the rewritten scheme.
type fakeTransporter struct {
	gotURLScheme string
}

func (f *fakeTransporter) Do(req *http.Request) (*http.Response, error) {
	f.gotURLScheme = req.URL.Scheme
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
}

func TestAzureInsecureEndpointTransport_Do_RewritesScheme(t *testing.T) {
	base := &fakeTransporter{}
	transport := azureInsecureEndpointTransport{base: base}

	req, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:8443/secrets/x", http.NoBody)
	require.NoError(t, err)

	resp, err := transport.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	// The request forwarded to the base transport has its scheme rewritten to http.
	assert.Equal(t, "http", base.gotURLScheme)
	// The original request is cloned, not mutated.
	assert.Equal(t, "https", req.URL.Scheme)
}

func TestAzureInsecureEndpointTransport_Do_NilBaseUsesDefaultClient(t *testing.T) {
	var gotScheme string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotScheme = r.URL.Scheme
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	transport := azureInsecureEndpointTransport{base: nil}

	// Use an https:// URL pointing at the http test server; the transport rewrites it to http.
	req, err := http.NewRequest(http.MethodGet, "https"+server.URL[len("http"):], http.NoBody)
	require.NoError(t, err)

	resp, err := transport.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	// The server is plain HTTP, so the scheme observed server-side is empty (default for server requests);
	// the key assertion is that the request succeeded against the http server after rewrite.
	_ = gotScheme
}
