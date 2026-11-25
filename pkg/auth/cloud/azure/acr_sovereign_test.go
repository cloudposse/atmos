package azure

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	httpClient "github.com/cloudposse/atmos/pkg/http"
)

// TestLoadDefaultAzureCredentials_Failures verifies both failure branches wrap
// ErrACRAuthFailed, and that a token failure names the cloud it was requested for.
func TestLoadDefaultAzureCredentials_Failures(t *testing.T) {
	t.Run("credential construction fails", func(t *testing.T) {
		orig := newDefaultAzureCredential
		t.Cleanup(func() { newDefaultAzureCredential = orig })
		newDefaultAzureCredential = func(*azidentity.DefaultAzureCredentialOptions) (ambientTokenCredential, error) {
			return nil, cockroachErrors.New("no chain")
		}

		_, err := LoadDefaultAzureCredentials(t.Context(), GetCloudEnvironment("china"))
		require.ErrorIs(t, err, errUtils.ErrACRAuthFailed)
		assert.Contains(t, err.Error(), "no chain")
	})

	t.Run("token request fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		credential := NewMockambientTokenCredential(ctrl)
		credential.EXPECT().GetToken(gomock.Any(), gomock.Any()).Return(azcore.AccessToken{}, cockroachErrors.New("denied"))
		orig := newDefaultAzureCredential
		t.Cleanup(func() { newDefaultAzureCredential = orig })
		newDefaultAzureCredential = func(*azidentity.DefaultAzureCredentialOptions) (ambientTokenCredential, error) {
			return credential, nil
		}

		_, err := LoadDefaultAzureCredentials(t.Context(), GetCloudEnvironment("usgovernment"))
		require.ErrorIs(t, err, errUtils.ErrACRAuthFailed)

		var details []string
		for _, payload := range cockroachErrors.GetAllSafeDetails(err) {
			details = append(details, payload.SafeDetails...)
		}
		assert.Contains(t, strings.Join(details, " "), "cloud_environment=usgovernment",
			"the error context must name the cloud the token was requested for")
	})
}

// ambientTokenRequest records what the ambient credential chain was asked for.
type ambientTokenRequest struct {
	options *azidentity.DefaultAzureCredentialOptions
	scopes  []string
}

// stubAmbientCredential replaces the ambient credential constructor with a mock
// credential and records the constructor options and the requested token scopes.
// No real Azure endpoint is called.
func stubAmbientCredential(t *testing.T, accessToken string) *ambientTokenRequest {
	t.Helper()

	got := &ambientTokenRequest{}
	ctrl := gomock.NewController(t)
	credential := NewMockambientTokenCredential(ctrl)
	credential.EXPECT().GetToken(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
			got.scopes = opts.Scopes
			return azcore.AccessToken{Token: accessToken, ExpiresOn: time.Now().Add(time.Hour)}, nil
		}).AnyTimes()

	orig := newDefaultAzureCredential
	t.Cleanup(func() { newDefaultAzureCredential = orig })
	newDefaultAzureCredential = func(options *azidentity.DefaultAzureCredentialOptions) (ambientTokenCredential, error) {
		got.options = options
		return credential, nil
	}
	return got
}

// authorityHost returns the AAD authority the ambient credential chain will use
// for the given constructor options, following azidentity's own rule: an unset
// cloud configuration means the public cloud (or AZURE_AUTHORITY_HOST).
func authorityHost(options *azidentity.DefaultAzureCredentialOptions) string {
	if options == nil || options.Cloud.ActiveDirectoryAuthorityHost == "" {
		return "default (public cloud or AZURE_AUTHORITY_HOST)"
	}
	return options.Cloud.ActiveDirectoryAuthorityHost
}

// TestLoadDefaultAzureCredentials_UsesRegistryCloud verifies the ambient token for
// an explicit --registry login is requested from the AAD authority and with the
// ARM management scope of the cloud that serves the registry.
func TestLoadDefaultAzureCredentials_UsesRegistryCloud(t *testing.T) {
	tests := []struct {
		cloud         string
		wantAuthority string
		wantScope     string
	}{
		{"usgovernment", "https://login.microsoftonline.us/", "https://management.usgovcloudapi.net/.default"},
		{"china", "https://login.chinacloudapi.cn/", "https://management.chinacloudapi.cn/.default"},
		// Public keeps azidentity's defaults so AZURE_AUTHORITY_HOST is still honored.
		{"public", "default (public cloud or AZURE_AUTHORITY_HOST)", "https://management.azure.com/.default"},
	}
	for _, tt := range tests {
		t.Run(tt.cloud, func(t *testing.T) {
			token := createTestJWT(map[string]interface{}{"tid": "tenant-" + tt.cloud})
			got := stubAmbientCredential(t, token)

			creds, err := LoadDefaultAzureCredentials(t.Context(), GetCloudEnvironment(tt.cloud))
			require.NoError(t, err)

			assert.Equal(t, tt.wantAuthority, authorityHost(got.options))
			assert.Equal(t, []string{tt.wantScope}, got.scopes)
			assert.Equal(t, tt.cloud, creds.CloudEnvironment)
			assert.Equal(t, "tenant-"+tt.cloud, creds.TenantID)
			assert.Equal(t, token, creds.AccessToken)
		})
	}
}

// TestLoadDefaultAzureCredentials_NilCloudIsPublic verifies a nil cloud keeps the
// historical public-cloud behavior.
func TestLoadDefaultAzureCredentials_NilCloudIsPublic(t *testing.T) {
	got := stubAmbientCredential(t, "not-a-jwt")

	creds, err := LoadDefaultAzureCredentials(t.Context(), nil)
	require.NoError(t, err)
	assert.Nil(t, got.options)
	assert.Equal(t, []string{PublicCloud.ManagementScope}, got.scopes)
	assert.Equal(t, "public", creds.CloudEnvironment)
	assert.Empty(t, creds.TenantID, "an undecodable token has no tenant claim")
}

// TestGetAuthorizationToken_SovereignExchangeHost verifies the ACR token exchange is
// sent to the sovereign registry itself, with the registry as the service.
func TestGetAuthorizationToken_SovereignExchangeHost(t *testing.T) {
	for _, loginServer := range []string{"myregistry.azurecr.us", "myregistry.azurecr.cn"} {
		t.Run(loginServer, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := httpClient.NewMockClient(ctrl)
			client.EXPECT().Do(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "https://"+loginServer+"/oauth2/exchange", req.URL.String())
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				assert.Contains(t, string(body), "service="+loginServer)
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"refresh_token":"rt"}`)),
				}, nil
			})
			origClient := acrHTTPClient
			acrHTTPClient = client
			t.Cleanup(func() { acrHTTPClient = origClient })

			creds := &types.AzureCredentials{AccessToken: "aad", TenantID: "t", CloudEnvironment: "usgovernment"}
			result, err := GetAuthorizationToken(t.Context(), creds, loginServer)
			require.NoError(t, err)
			assert.Equal(t, loginServer, result.Registry)
		})
	}
}

// TestCloudEnvironmentForRegistry maps each ACR login-server suffix to its cloud.
func TestCloudEnvironmentForRegistry(t *testing.T) {
	tests := map[string]string{
		"myregistry.azurecr.io":          "public",
		"https://MyRegistry.AzureCR.us/": "usgovernment",
		"myregistry.azurecr.cn":          "china",
	}
	for registry, want := range tests {
		t.Run(registry, func(t *testing.T) {
			cloudEnv, err := CloudEnvironmentForRegistry(registry)
			require.NoError(t, err)
			assert.Equal(t, want, cloudEnv.Name)
		})
	}

	_, err := CloudEnvironmentForRegistry("myregistry.example.com")
	require.Error(t, err)
}

// TestBuildRegistryURLForCloud verifies the login server is built with the
// suffix of the given cloud, and that a nil or unknown cloud is public.
func TestBuildRegistryURLForCloud(t *testing.T) {
	assert.Equal(t, "myregistry.azurecr.us", BuildRegistryURLForCloud("myregistry", GetCloudEnvironment("usgovernment")))
	assert.Equal(t, "myregistry.azurecr.cn", BuildRegistryURLForCloud("myregistry", GetCloudEnvironment("china")))
	assert.Equal(t, "myregistry.azurecr.io", BuildRegistryURLForCloud("myregistry", GetCloudEnvironment("public")))
	assert.Equal(t, "myregistry.azurecr.io", BuildRegistryURLForCloud("myregistry", nil))
	assert.Equal(t, "myregistry.azurecr.io", BuildRegistryURL("myregistry"))
}

// TestRegistryURLsForAllClouds lists the registry's login server in every known
// cloud, public first, so cleanup is deterministic.
func TestRegistryURLsForAllClouds(t *testing.T) {
	assert.Equal(t,
		[]string{"myregistry.azurecr.io", "myregistry.azurecr.us", "myregistry.azurecr.cn"},
		RegistryURLsForAllClouds("myregistry"))
}

// TestACRLoginServerSuffixes_RoundTrip guards that every known cloud's ACR suffix
// is accepted by the login-server parser and maps back to the same cloud.
func TestACRLoginServerSuffixes_RoundTrip(t *testing.T) {
	for _, name := range KnownCloudEnvironments() {
		cloudEnv := GetCloudEnvironment(name)
		require.NotEmpty(t, cloudEnv.ACRLoginServerSuffix, name)

		loginServer := BuildRegistryURLForCloud("myregistry", cloudEnv)
		normalized, err := NormalizeRegistryURL(loginServer)
		require.NoError(t, err, name)
		assert.Equal(t, loginServer, normalized)

		back, err := CloudEnvironmentForRegistry(loginServer)
		require.NoError(t, err)
		assert.Equal(t, name, back.Name)
	}
}
