package azure

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
	"github.com/cloudposse/atmos/pkg/auth/cloud/docker"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestACRExchangeReturnsMintedCredentialsWithoutSharedDockerState(t *testing.T) {
	originalToken, originalDocker := acrGetAuthToken, acrDockerConfigFactory
	t.Cleanup(func() { acrGetAuthToken, acrDockerConfigFactory = originalToken, originalDocker })
	acrDockerConfigFactory = func() (*docker.ConfigManager, error) {
		t.Fatal("exchange must not access shared Docker credentials")
		return nil, nil
	}
	directory := t.TempDir()
	t.Setenv("DOCKER_CONFIG", directory)
	before := []byte(`{"auths":{"corporate.azurecr.io":{"auth":"other-user-token"}}}`)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "config.json"), before, 0o600))
	input := &types.AzureCredentials{AccessToken: "selected-identity"}
	expiry := time.Now().Add(time.Hour)
	acrGetAuthToken = func(ctx context.Context, actual types.ICredentials, host string) (*azureCloud.ACRAuthResult, error) {
		require.Same(t, input, actual)
		require.Equal(t, "corporate.azurecr.io", host)
		return &azureCloud.ACRAuthResult{Username: "azure", Password: "minted-selected-token", ExpiresAt: expiry}, nil
	}
	integration := &ACRIntegration{registry: &schema.Registry{Name: "corporate"}}
	result, err := integration.ExchangeCredentials(t.Context(), input)
	require.NoError(t, err)
	require.Equal(t, &types.OCICredentials{Registry: "corporate.azurecr.io", Username: "azure", Password: "minted-selected-token", Expiration: &expiry}, result)
	after, err := os.ReadFile(filepath.Join(directory, "config.json"))
	require.NoError(t, err)
	require.Equal(t, before, after)
	acrGetAuthToken = func(context.Context, types.ICredentials, string) (*azureCloud.ACRAuthResult, error) {
		return &azureCloud.ACRAuthResult{Username: "azure", Password: "unknown-expiry"}, nil
	}
	result, err = integration.ExchangeCredentials(t.Context(), input)
	require.NoError(t, err)
	require.Nil(t, result.Expiration)
	acrGetAuthToken = func(context.Context, types.ICredentials, string) (*azureCloud.ACRAuthResult, error) { return nil, nil }
	_, err = integration.ExchangeCredentials(t.Context(), input)
	require.ErrorIs(t, err, errUtils.ErrIdentityCredentialsNone)
}
