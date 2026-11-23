package azure

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guard: these tests depend on the azure/acr registry tenant field.
var _ = schema.Registry{TenantID: registryTenant}

const (
	identityTenant = "11111111-1111-1111-1111-111111111111"
	registryTenant = "22222222-2222-2222-2222-222222222222"
)

// tenantSeamCall records one call of the acrCredentialsForTenant seam.
type tenantSeamCall struct {
	creds    types.ICredentials
	tenantID string
	realm    string
}

// stubTenantSeams replaces the registry-tenant credential swap and the ACR token
// exchange with recorders, so no MSAL cache or Azure endpoint is touched. The swap
// returns tenantCreds (or swapErr). It returns the recorded swap calls and the
// credentials the exchange received.
func stubTenantSeams(t *testing.T, tenantCreds types.ICredentials, swapErr error) (*[]tenantSeamCall, *types.ICredentials) {
	t.Helper()

	origGetAuth := acrGetAuthToken
	origForTenant := acrCredentialsForTenant
	t.Cleanup(func() {
		acrGetAuthToken = origGetAuth
		acrCredentialsForTenant = origForTenant
	})

	var calls []tenantSeamCall
	acrCredentialsForTenant = func(_ context.Context, creds types.ICredentials, tenantID, realm string) (types.ICredentials, error) {
		calls = append(calls, tenantSeamCall{creds: creds, tenantID: tenantID, realm: realm})
		return tenantCreds, swapErr
	}

	var exchanged types.ICredentials
	acrGetAuthToken = func(_ context.Context, creds types.ICredentials, loginServer string) (*azureCloud.ACRAuthResult, error) {
		exchanged = creds
		return &azureCloud.ACRAuthResult{Username: "u", Password: "p", Registry: loginServer, ExpiresAt: time.Now().Add(time.Hour)}, nil
	}
	return &calls, &exchanged
}

func newTenantIntegration(t *testing.T, tenantID, realm string) *ACRIntegration {
	t.Helper()

	integration, err := NewACRIntegration(&integrations.IntegrationConfig{
		Name:  "dev/acr",
		Realm: realm,
		Config: &schema.Integration{
			Kind: integrations.KindAzureACR,
			Via:  &schema.IntegrationVia{Identity: "azure-dev"},
			Spec: &schema.IntegrationSpec{Registry: &schema.Registry{Name: "myregistry", TenantID: tenantID}},
		},
	})
	require.NoError(t, err)
	acr, ok := integration.(*ACRIntegration)
	require.True(t, ok)
	return acr
}

// TestACRIntegration_TenantOverrideReachesExchange verifies that with
// spec.registry.tenant_id set, the exchange receives the credentials issued by
// that tenant, requested with the integration's realm.
func TestACRIntegration_TenantOverrideReachesExchange(t *testing.T) {
	identityCreds := &types.AzureCredentials{AccessToken: "aad-identity", TenantID: identityTenant, AuthMethod: types.AzureAuthMethodDeviceCode}
	tenantCreds := &types.AzureCredentials{AccessToken: "aad-registry-tenant", TenantID: registryTenant, AuthMethod: types.AzureAuthMethodDeviceCode}
	calls, exchanged := stubTenantSeams(t, tenantCreds, nil)

	integration := newTenantIntegration(t, registryTenant, "my-realm")
	result, err := integration.ExchangeCredentials(t.Context(), identityCreds)
	require.NoError(t, err)
	assert.Equal(t, "myregistry.azurecr.io", result.Registry)

	require.Len(t, *calls, 1)
	assert.Same(t, identityCreds, (*calls)[0].creds)
	assert.Equal(t, registryTenant, (*calls)[0].tenantID)
	assert.Equal(t, "my-realm", (*calls)[0].realm)

	exchangedCreds, ok := (*exchanged).(*types.AzureCredentials)
	require.True(t, ok)
	assert.Equal(t, registryTenant, exchangedCreds.TenantID, "the exchange must use spec.registry.tenant_id")
	assert.Equal(t, "aad-registry-tenant", exchangedCreds.AccessToken, "the exchange must send the registry tenant's token")
	assert.Equal(t, identityTenant, identityCreds.TenantID, "the identity's credentials must not change")
}

// TestACRIntegration_TenantOverrideKeepsProviderCloud verifies the login server
// still follows the identity's cloud when a registry tenant is set.
func TestACRIntegration_TenantOverrideKeepsProviderCloud(t *testing.T) {
	identityCreds := &types.AzureCredentials{TenantID: identityTenant, CloudEnvironment: "usgovernment", AuthMethod: types.AzureAuthMethodDeviceCode}
	tenantCreds := &types.AzureCredentials{TenantID: registryTenant, CloudEnvironment: "usgovernment"}
	stubTenantSeams(t, tenantCreds, nil)
	t.Setenv("DOCKER_CONFIG", t.TempDir())

	integration := newTenantIntegration(t, registryTenant, "")
	result, err := integration.ExchangeCredentials(t.Context(), identityCreds)
	require.NoError(t, err)
	assert.Equal(t, "myregistry.azurecr.us", result.Registry)

	require.NoError(t, integration.Execute(t.Context(), identityCreds))
}

// TestACRIntegration_NoTenantOverrideUsesIdentityTenant verifies that without
// spec.registry.tenant_id the identity's credentials go to the exchange unchanged
// and no tenant swap happens.
func TestACRIntegration_NoTenantOverrideUsesIdentityTenant(t *testing.T) {
	identityCreds := &types.AzureCredentials{AccessToken: "aad-identity", TenantID: identityTenant}
	calls, exchanged := stubTenantSeams(t, nil, nil)

	integration := newTenantIntegration(t, "", "my-realm")
	_, err := integration.ExchangeCredentials(t.Context(), identityCreds)
	require.NoError(t, err)

	assert.Empty(t, *calls, "no tenant swap without spec.registry.tenant_id")
	assert.Same(t, identityCreds, *exchanged)
}

// TestACRIntegration_TenantOverrideFailureStopsExchange verifies a failure to get
// credentials in the registry tenant fails the login instead of silently falling
// back to the identity's tenant.
func TestACRIntegration_TenantOverrideFailureStopsExchange(t *testing.T) {
	_, exchanged := stubTenantSeams(t, nil, errUtils.ErrACRTenantOverrideUnsupported)

	integration := newTenantIntegration(t, registryTenant, "")
	_, err := integration.ExchangeCredentials(t.Context(), &types.AzureCredentials{TenantID: identityTenant, AuthMethod: types.AzureAuthMethodCLI})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrACRAuthFailed)
	assert.ErrorIs(t, err, errUtils.ErrACRTenantOverrideUnsupported)
	assert.Nil(t, *exchanged, "the exchange must not run with the identity's tenant")
}

// TestACRIntegration_TenantOverrideUnsupportedCredentials runs the real tenant swap
// for Azure CLI credentials, which cannot mint a token in another tenant.
func TestACRIntegration_TenantOverrideUnsupportedCredentials(t *testing.T) {
	origGetAuth := acrGetAuthToken
	t.Cleanup(func() { acrGetAuthToken = origGetAuth })
	exchangeCalled := false
	acrGetAuthToken = func(context.Context, types.ICredentials, string) (*azureCloud.ACRAuthResult, error) {
		exchangeCalled = true
		return &azureCloud.ACRAuthResult{}, nil
	}

	integration := newTenantIntegration(t, registryTenant, "")
	_, err := integration.ExchangeCredentials(t.Context(), &types.AzureCredentials{AccessToken: "aad", TenantID: identityTenant, AuthMethod: types.AzureAuthMethodCLI})
	assert.ErrorIs(t, err, errUtils.ErrACRTenantOverrideUnsupported)
	assert.NotEmpty(t, errUtils.AllHints(err), "the hint must survive the integration's wrapping")
	assert.False(t, exchangeCalled)
}

// TestNewACRIntegration_TenantIDValidation verifies spec.registry.tenant_id is
// validated when the integration is created.
func TestNewACRIntegration_TenantIDValidation(t *testing.T) {
	tests := []struct {
		name     string
		tenantID string
		valid    bool
	}{
		{"guid", registryTenant, true},
		{"domain", "contoso.onmicrosoft.com", true},
		{"not a tenant", "not a tenant", false},
		{"single label", "common", false},
		{"url", "https://login.microsoftonline.com/contoso.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewACRIntegration(&integrations.IntegrationConfig{
				Name: "dev/acr",
				Config: &schema.Integration{
					Kind: integrations.KindAzureACR,
					Spec: &schema.IntegrationSpec{Registry: &schema.Registry{Name: "myregistry", TenantID: tt.tenantID}},
				},
			})
			if tt.valid {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrIntegrationFailed)
			assert.ErrorIs(t, err, errUtils.ErrACRInvalidTenant)
			assert.Contains(t, err.Error(), "dev/acr")
		})
	}
}
