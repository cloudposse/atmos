package registryauth

import (
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestMatchingIntegrationACRSovereignHosts verifies an azure/acr integration
// configured by registry name serves its login server in every Azure cloud. The
// integration builds the login server for its provider's cloud (azurecr.us for
// US Government, azurecr.cn for China), so a plugin registry on that host must
// reach the integration instead of falling back to the identity's own credentials.
func TestMatchingIntegrationACRSovereignHosts(t *testing.T) {
	config := &schema.AuthConfig{Integrations: map[string]schema.Integration{
		"acr": {Kind: "azure/acr", Spec: &schema.IntegrationSpec{Registry: &schema.Registry{Name: "corporate"}}},
	}}
	for host, want := range map[string]string{
		"corporate.azurecr.io": "acr",
		"corporate.azurecr.us": "acr",
		"corporate.azurecr.cn": "acr",
		"other.azurecr.us":     "",
	} {
		actual, err := matchingIntegration(config, host)
		require.NoError(t, err)
		require.Equal(t, want, actual, host)
	}

	// Two integrations for the same registry name are still ambiguous on a sovereign host.
	config.Integrations["second"] = config.Integrations["acr"]
	_, err := matchingIntegration(config, "corporate.azurecr.us")
	require.ErrorIs(t, err, errUtils.ErrIntegrationFailed)
}
