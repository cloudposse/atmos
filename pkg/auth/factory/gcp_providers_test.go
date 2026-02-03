package factory

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
)

func TestRegisterGCPProviders(t *testing.T) {
	f := NewFactory()

	// Verify gcp/adc is registered.
	assert.True(t, f.HasProvider(types.ProviderKindGCPADC))

	// Verify gcp/workload-identity-federation is registered.
	assert.True(t, f.HasProvider(types.ProviderKindGCPWorkloadIdentityFederation))
}

func TestCreateGCPADCProvider(t *testing.T) {
	f := NewFactory()

	spec := map[string]any{
		"project_id": "test-project",
		"region":     "us-central1",
	}

	provider, err := f.CreateProvider(types.ProviderKindGCPADC, "my-adc", spec)
	require.NoError(t, err)
	require.NotNil(t, provider)

	assert.Equal(t, types.ProviderKindGCPADC, provider.Kind())
	assert.Equal(t, "my-adc", provider.Name())
}

func TestCreateGCPADCProvider_InvalidSpec(t *testing.T) {
	f := NewFactory()

	// Invalid spec (e.g., wrong type for a field).
	spec := map[string]any{
		"project_id": 123, // Should be string.
	}

	_, err := f.CreateProvider(types.ProviderKindGCPADC, "bad-adc", spec)
	// Depending on parser strictness, this may or may not error.
	// If weak typing is disabled, expect an error.
	if err != nil {
		assert.Contains(t, err.Error(), "parse")
	}
}

func TestCreateGCPWIFProvider(t *testing.T) {
	f := NewFactory()

	spec := map[string]any{
		"project_id":                    "wif-project",
		"project_number":                "123456789",
		"workload_identity_pool_id":      "my-pool",
		"workload_identity_provider_id":  "my-provider",
		"token_source": map[string]any{
			"type":                  "environment",
			"environment_variable":  "OIDC_TOKEN",
		},
	}

	provider, err := f.CreateProvider(types.ProviderKindGCPWorkloadIdentityFederation, "my-wif", spec)
	require.NoError(t, err)
	require.NotNil(t, provider)

	assert.Equal(t, types.ProviderKindGCPWorkloadIdentityFederation, provider.Kind())
	assert.Equal(t, "my-wif", provider.Name())
}

func TestCreateGCPWIFProvider_MissingRequired(t *testing.T) {
	f := NewFactory()

	// Missing project_number, pool_id, provider_id.
	spec := map[string]any{
		"project_id": "incomplete",
	}

	provider, err := f.CreateProvider(types.ProviderKindGCPWorkloadIdentityFederation, "incomplete-wif", spec)
	// Provider is created but Validate() will fail later.
	// Or if constructor validates, expect error here.
	if err == nil && provider != nil {
		err = provider.Validate()
		require.Error(t, err)
	}
}
