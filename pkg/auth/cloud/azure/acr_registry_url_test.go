package azure

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestNormalizeRegistryURL verifies ACR login servers for every Azure cloud
// (public .azurecr.io, US Government .azurecr.us, China .azurecr.cn) are accepted
// in any case and normalized to the lowercase login server ACR expects.
func TestNormalizeRegistryURL(t *testing.T) {
	tests := []struct {
		input           string
		wantName        string
		wantLoginServer string
	}{
		{"myregistry.azurecr.io", "myregistry", "myregistry.azurecr.io"},
		{"myregistry.azurecr.us", "myregistry", "myregistry.azurecr.us"},
		{"myregistry.azurecr.cn", "myregistry", "myregistry.azurecr.cn"},
		{"MyRegistry.azurecr.io", "myregistry", "myregistry.azurecr.io"},
		{"MYREGISTRY.AZURECR.US", "myregistry", "myregistry.azurecr.us"},
		{"https://myregistry.azurecr.cn", "myregistry", "myregistry.azurecr.cn"},
		{"HTTPS://MyRegistry.azurecr.io/", "myregistry", "myregistry.azurecr.io"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			loginServer, err := NormalizeRegistryURL(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantLoginServer, loginServer)

			name, err := ParseRegistryURL(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantName, name)

			assert.True(t, IsACRRegistry(tt.input))
		})
	}
}

// TestNormalizeRegistryURL_Invalid verifies non-ACR hosts are still rejected.
func TestNormalizeRegistryURL_Invalid(t *testing.T) {
	for _, input := range []string{
		"",
		"not-a-registry",
		"myregistry.example.com",
		"myregistry.azurecr.de.example.com",
		"my-registry.azurecr.io",
		"myregistry.azurecr.io/path",
		"evil.com/myregistry.azurecr.io",
	} {
		t.Run(input, func(t *testing.T) {
			_, err := NormalizeRegistryURL(input)
			require.ErrorIs(t, err, errUtils.ErrACRInvalidRegistry)
			_, err = ParseRegistryURL(input)
			require.ErrorIs(t, err, errUtils.ErrACRInvalidRegistry)
			assert.False(t, IsACRRegistry(input))
		})
	}
}
