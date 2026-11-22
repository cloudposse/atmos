package azure

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/public"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	httpClient "github.com/cloudposse/atmos/pkg/http"
)

// Compile-time guards: the tenant swap depends on these credential fields.
var _ = types.AzureCredentials{AuthMethod: types.AzureAuthMethodDeviceCode, HomeAccountID: "oid.tid", CloudEnvironment: "public"}

const (
	testIdentityTenant = "11111111-1111-1111-1111-111111111111"
	testRegistryTenant = "22222222-2222-2222-2222-222222222222"
	testClientID       = "33333333-3333-3333-3333-333333333333"
	testHomeAccountID  = "44444444-4444-4444-4444-444444444444." + testIdentityTenant
)

// tenantClientRequest records how newACRTenantTokenClient was called.
type tenantClientRequest struct {
	clientID, authority, realm string
}

// stubTenantTokenClient replaces newACRTenantTokenClient with one that returns
// client (or err) and records its arguments.
func stubTenantTokenClient(t *testing.T, client acrTenantTokenClient, err error) *[]tenantClientRequest {
	t.Helper()

	orig := newACRTenantTokenClient
	t.Cleanup(func() { newACRTenantTokenClient = orig })

	var requests []tenantClientRequest
	newACRTenantTokenClient = func(clientID, authority, realm string) (acrTenantTokenClient, error) {
		requests = append(requests, tenantClientRequest{clientID: clientID, authority: authority, realm: realm})
		return client, err
	}
	return &requests
}

// identityCreds returns device-code credentials whose token was issued in the
// identity's tenant to testClientID.
func identityCreds() *types.AzureCredentials {
	return &types.AzureCredentials{
		AccessToken:    makeJWTWithClaims(map[string]any{"tid": testIdentityTenant, "appid": testClientID}),
		TokenType:      "Bearer",
		TenantID:       testIdentityTenant,
		SubscriptionID: "sub-1",
		AuthMethod:     types.AzureAuthMethodDeviceCode,
		HomeAccountID:  testHomeAccountID,
	}
}

func TestValidateACRTenantID(t *testing.T) {
	tests := []struct {
		tenantID string
		valid    bool
	}{
		{testRegistryTenant, true},
		{"ABCDEF01-2345-6789-ABCD-EF0123456789", true},
		{"contoso.onmicrosoft.com", true},
		{"contoso.com", true},
		{"my-org.example.co.uk", true},
		{"", false},
		{"common", false},
		{"organizations", false},
		{"not a tenant", false},
		{"{" + testRegistryTenant + "}", false},
		{"https://login.microsoftonline.com/contoso.com", false},
		{"contoso.com/", false},
		{"-contoso.com", false},
		{"contoso..com", false},
		{" contoso.com", false},
		{strings.Repeat("a.", 127) + "com", false},
	}
	for _, tt := range tests {
		t.Run(tt.tenantID, func(t *testing.T) {
			err := ValidateACRTenantID(tt.tenantID)
			if tt.valid {
				assert.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, errUtils.ErrACRInvalidTenant)
		})
	}
}

// TestCredentialsForACRTenant_MintsTokenInRegistryTenant verifies a device-code
// identity gets a token from the registry tenant's authority, for the identity's
// client and cached account, and that the copy carries that tenant.
func TestCredentialsForACRTenant_MintsTokenInRegistryTenant(t *testing.T) {
	for _, method := range []string{types.AzureAuthMethodDeviceCode, types.AzureAuthMethodInteractive} {
		t.Run(method, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := NewMockacrTenantTokenClient(ctrl)
			requests := stubTenantTokenClient(t, client, nil)

			account := public.Account{HomeAccountID: testHomeAccountID, Realm: testIdentityTenant}
			client.EXPECT().Accounts(gomock.Any()).Return([]public.Account{
				{HomeAccountID: "other." + testIdentityTenant, Realm: testIdentityTenant},
				account,
			}, nil)
			tenantToken := makeJWTWithClaims(map[string]any{"tid": testRegistryTenant})
			expiresOn := time.Now().Add(time.Hour).Truncate(time.Second)
			client.EXPECT().AcquireTokenSilent(gomock.Any(), []string{PublicCloud.ManagementScope}, gomock.Any()).
				Return(public.AuthResult{AccessToken: tenantToken, ExpiresOn: expiresOn}, nil)

			creds := identityCreds()
			creds.AuthMethod = method
			original := *creds

			got, err := CredentialsForACRTenant(t.Context(), creds, testRegistryTenant, "my-realm")
			require.NoError(t, err)

			require.Len(t, *requests, 1)
			assert.Equal(t, tenantClientRequest{
				clientID:  testClientID,
				authority: "https://login.microsoftonline.com/" + testRegistryTenant,
				realm:     "my-realm",
			}, (*requests)[0])

			tenantCreds, ok := got.(*types.AzureCredentials)
			require.True(t, ok)
			assert.Equal(t, tenantToken, tenantCreds.AccessToken)
			assert.Equal(t, testRegistryTenant, tenantCreds.TenantID)
			assert.Equal(t, expiresOn.Format(time.RFC3339), tenantCreds.Expiration)
			assert.Equal(t, "sub-1", tenantCreds.SubscriptionID)
			assert.Equal(t, method, tenantCreds.AuthMethod)
			assert.Equal(t, original, *creds, "the identity's credentials must not change")
		})
	}
}

// TestCredentialsForACRTenant_SovereignCloud verifies the token is requested from
// the identity's cloud authority and ARM scope.
func TestCredentialsForACRTenant_SovereignCloud(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockacrTenantTokenClient(ctrl)
	requests := stubTenantTokenClient(t, client, nil)

	gov := GetCloudEnvironment("usgovernment")
	client.EXPECT().Accounts(gomock.Any()).Return([]public.Account{{HomeAccountID: testHomeAccountID}}, nil)
	client.EXPECT().AcquireTokenSilent(gomock.Any(), []string{gov.ManagementScope}, gomock.Any()).
		Return(public.AuthResult{AccessToken: "not-a-jwt", ExpiresOn: time.Now().Add(time.Hour)}, nil)

	creds := identityCreds()
	creds.CloudEnvironment = "usgovernment"

	got, err := CredentialsForACRTenant(t.Context(), creds, testRegistryTenant, "")
	require.NoError(t, err)
	require.Len(t, *requests, 1)
	assert.Equal(t, "https://login.microsoftonline.us/"+testRegistryTenant, (*requests)[0].authority)
	assert.Equal(t, "usgovernment", got.(*types.AzureCredentials).CloudEnvironment)
	assert.Equal(t, testRegistryTenant, got.(*types.AzureCredentials).TenantID)
}

// TestCredentialsForACRTenant_DomainTenantUsesIssuedTenantID verifies a domain
// tenant_id is resolved to the tenant ID the new token was issued by.
func TestCredentialsForACRTenant_DomainTenantUsesIssuedTenantID(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockacrTenantTokenClient(ctrl)
	requests := stubTenantTokenClient(t, client, nil)

	client.EXPECT().Accounts(gomock.Any()).Return([]public.Account{{HomeAccountID: testHomeAccountID}}, nil)
	client.EXPECT().AcquireTokenSilent(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(public.AuthResult{AccessToken: makeJWTWithClaims(map[string]any{"tid": testRegistryTenant}), ExpiresOn: time.Now()}, nil)

	got, err := CredentialsForACRTenant(t.Context(), identityCreds(), "contoso.onmicrosoft.com", "")
	require.NoError(t, err)
	assert.Equal(t, "https://login.microsoftonline.com/contoso.onmicrosoft.com", (*requests)[0].authority)
	assert.Equal(t, testRegistryTenant, got.(*types.AzureCredentials).TenantID)
}

// TestCredentialsForACRTenant_IdentityTenantIsUnchanged verifies no token is
// minted when tenant_id is the tenant the identity's token was issued by, for any
// credential type.
func TestCredentialsForACRTenant_IdentityTenantIsUnchanged(t *testing.T) {
	requests := stubTenantTokenClient(t, nil, errors.New("must not be called"))

	tests := []struct {
		name     string
		tenantID string
		method   string
	}{
		{"same tenant ID", testIdentityTenant, types.AzureAuthMethodCLI},
		{"same tenant ID other case", strings.ToUpper(testIdentityTenant), types.AzureAuthMethodOIDC},
		{"token tid claim", testIdentityTenant, types.AzureAuthMethodDeviceCode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds := identityCreds()
			creds.AuthMethod = tt.method
			if tt.name == "token tid claim" {
				creds.TenantID = "contoso.onmicrosoft.com"
			}
			got, err := CredentialsForACRTenant(t.Context(), creds, tt.tenantID, "")
			require.NoError(t, err)
			assert.Same(t, creds, got)
		})
	}
	assert.Empty(t, *requests)
}

// TestCredentialsForACRTenant_UnsupportedCredentials verifies credentials that
// cannot mint a token in another tenant fail instead of being sent unchanged.
func TestCredentialsForACRTenant_UnsupportedCredentials(t *testing.T) {
	requests := stubTenantTokenClient(t, nil, errors.New("must not be called"))

	for _, method := range []string{types.AzureAuthMethodCLI, types.AzureAuthMethodOIDC, ""} {
		t.Run("method="+method, func(t *testing.T) {
			creds := identityCreds()
			creds.AuthMethod = method
			_, err := CredentialsForACRTenant(t.Context(), creds, testRegistryTenant, "")
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrACRTenantOverrideUnsupported)
			assert.Contains(t, err.Error(), testRegistryTenant)
		})
	}
	assert.Empty(t, *requests)
}

func TestCredentialsForACRTenant_InvalidInput(t *testing.T) {
	requests := stubTenantTokenClient(t, nil, errors.New("must not be called"))

	_, err := CredentialsForACRTenant(t.Context(), &types.AWSCredentials{}, testRegistryTenant, "")
	assert.ErrorIs(t, err, errUtils.ErrACRAuthFailed)

	_, err = CredentialsForACRTenant(t.Context(), (*types.AzureCredentials)(nil), testRegistryTenant, "")
	assert.ErrorIs(t, err, errUtils.ErrACRAuthFailed)

	_, err = CredentialsForACRTenant(t.Context(), identityCreds(), "common", "")
	assert.ErrorIs(t, err, errUtils.ErrACRInvalidTenant)

	assert.Empty(t, *requests)
}

// TestCredentialsForACRTenant_Failures verifies every failure to mint the token
// in the registry tenant is reported as an ACR auth failure.
func TestCredentialsForACRTenant_Failures(t *testing.T) {
	errSilent := errors.New("AADSTS50076: interaction required")
	errAccounts := errors.New("cache read failed")

	tests := []struct {
		name      string
		setup     func(client *MockacrTenantTokenClient)
		clientErr error
		creds     func() *types.AzureCredentials
		wantErr   error
	}{
		{
			name:      "client creation",
			clientErr: errors.New("bad authority"),
			wantErr:   errUtils.ErrACRAuthFailed,
		},
		{
			name: "accounts error",
			setup: func(client *MockacrTenantTokenClient) {
				client.EXPECT().Accounts(gomock.Any()).Return(nil, errAccounts)
			},
			wantErr: errAccounts,
		},
		{
			name: "empty cache",
			setup: func(client *MockacrTenantTokenClient) {
				client.EXPECT().Accounts(gomock.Any()).Return(nil, nil)
			},
			wantErr: errUtils.ErrAzureNoAccountsInCache,
		},
		{
			name: "no account for home account ID",
			setup: func(client *MockacrTenantTokenClient) {
				client.EXPECT().Accounts(gomock.Any()).Return([]public.Account{{HomeAccountID: "other." + testIdentityTenant, Realm: testIdentityTenant}}, nil)
			},
			wantErr: errUtils.ErrAzureNoAccountForTenant,
		},
		{
			name: "no account for tenant",
			setup: func(client *MockacrTenantTokenClient) {
				client.EXPECT().Accounts(gomock.Any()).Return([]public.Account{{HomeAccountID: "x.y", Realm: "other"}}, nil)
			},
			creds: func() *types.AzureCredentials {
				creds := identityCreds()
				creds.HomeAccountID = ""
				return creds
			},
			wantErr: errUtils.ErrAzureNoAccountForTenant,
		},
		{
			name: "silent acquisition",
			setup: func(client *MockacrTenantTokenClient) {
				client.EXPECT().Accounts(gomock.Any()).Return([]public.Account{{HomeAccountID: testHomeAccountID}}, nil)
				client.EXPECT().AcquireTokenSilent(gomock.Any(), gomock.Any(), gomock.Any()).Return(public.AuthResult{}, errSilent)
			},
			wantErr: errSilent,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := NewMockacrTenantTokenClient(ctrl)
			if tt.setup != nil {
				tt.setup(client)
			}
			stubTenantTokenClient(t, client, tt.clientErr)

			creds := identityCreds()
			if tt.creds != nil {
				creds = tt.creds()
			}
			_, err := CredentialsForACRTenant(t.Context(), creds, testRegistryTenant, "")
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrACRAuthFailed)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// TestCredentialsForACRTenant_AccountByTenantWithoutHomeAccountID verifies the
// cached account is matched by tenant when the provider recorded no home account ID.
func TestCredentialsForACRTenant_AccountByTenantWithoutHomeAccountID(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockacrTenantTokenClient(ctrl)
	stubTenantTokenClient(t, client, nil)

	account := public.Account{HomeAccountID: testHomeAccountID, Realm: testIdentityTenant}
	client.EXPECT().Accounts(gomock.Any()).Return([]public.Account{{HomeAccountID: "x.y", Realm: "other"}, account}, nil)
	client.EXPECT().AcquireTokenSilent(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(public.AuthResult{AccessToken: "tenant-token", ExpiresOn: time.Now()}, nil)

	creds := identityCreds()
	creds.HomeAccountID = ""
	got, err := CredentialsForACRTenant(t.Context(), creds, testRegistryTenant, "")
	require.NoError(t, err)
	assert.Equal(t, "tenant-token", got.(*types.AzureCredentials).AccessToken)
}

func TestTokenClientID(t *testing.T) {
	assert.Equal(t, testClientID, tokenClientID(map[string]interface{}{"appid": testClientID}))
	assert.Equal(t, testClientID, tokenClientID(map[string]interface{}{"azp": testClientID}))
	assert.Equal(t, acrDefaultPublicClientID, tokenClientID(map[string]interface{}{"appid": ""}))
	assert.Equal(t, acrDefaultPublicClientID, tokenClientID(nil))
}

// TestNewACRTenantTokenClient_RealmCache verifies the real factory builds an MSAL
// client over the realm's cache directory without calling Azure.
func TestNewACRTenantTokenClient_RealmCache(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	client, err := newACRTenantTokenClient(testClientID, "https://login.microsoftonline.com/"+testRegistryTenant, "my-realm")
	require.NoError(t, err)
	require.NotNil(t, client)
	assert.DirExists(t, filepath.Join(home, ".azure", "atmos", "my-realm"))

	_, err = newACRTenantTokenClient(testClientID, "not a url", "my-realm")
	assert.Error(t, err)
}

// TestNewACRTenantTokenClient_CacheError verifies the factory fails when the realm
// cache directory cannot be created.
func TestNewACRTenantTokenClient_CacheError(t *testing.T) {
	notADir := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(notADir, []byte("x"), 0o600))
	home := filepath.Join(notADir, "home")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	_, err := newACRTenantTokenClient(testClientID, "https://login.microsoftonline.com/"+testRegistryTenant, "my-realm")
	assert.Error(t, err)
}

// TestACRTenantExchange_SendsRegistryTenant verifies end to end that the token
// exchange posts the registry tenant and the token issued by it.
func TestACRTenantExchange_SendsRegistryTenant(t *testing.T) {
	ctrl := gomock.NewController(t)
	tokenClient := NewMockacrTenantTokenClient(ctrl)
	stubTenantTokenClient(t, tokenClient, nil)
	tenantToken := makeJWTWithClaims(map[string]any{"tid": testRegistryTenant})
	tokenClient.EXPECT().Accounts(gomock.Any()).Return([]public.Account{{HomeAccountID: testHomeAccountID}}, nil)
	tokenClient.EXPECT().AcquireTokenSilent(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(public.AuthResult{AccessToken: tenantToken, ExpiresOn: time.Now().Add(time.Hour)}, nil)

	var form url.Values
	httpMock := httpClient.NewMockClient(ctrl)
	httpMock.EXPECT().Do(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		form, err = url.ParseQuery(string(body))
		require.NoError(t, err)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"refresh_token":"rt"}`))}, nil
	})
	origClient := acrHTTPClient
	acrHTTPClient = httpMock
	t.Cleanup(func() { acrHTTPClient = origClient })

	creds, err := CredentialsForACRTenant(context.Background(), identityCreds(), testRegistryTenant, "")
	require.NoError(t, err)
	_, err = GetAuthorizationToken(t.Context(), creds, "myregistry.azurecr.io")
	require.NoError(t, err)

	assert.Equal(t, testRegistryTenant, form.Get("tenant"))
	assert.Equal(t, tenantToken, form.Get("access_token"))
	assert.Equal(t, "myregistry.azurecr.io", form.Get("service"))
	assert.Equal(t, "access_token", form.Get("grant_type"))
}
