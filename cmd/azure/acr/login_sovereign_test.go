package acr

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
	"github.com/cloudposse/atmos/pkg/auth/cloud/docker"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
)

// TestExecuteExplicitRegistries_AmbientTokenPerRegistryCloud verifies that an explicit
// --registry login requests the ambient AAD token for the cloud that serves each
// registry (azurecr.us is US Government, azurecr.cn is China), loads it once per
// cloud, and exchanges each registry with the token of its own cloud.
func TestExecuteExplicitRegistries_AmbientTokenPerRegistryCloud(t *testing.T) {
	t.Setenv("DOCKER_CONFIG", t.TempDir())

	var loadedClouds []string
	exchanged := map[string]string{}
	stubAzureSeams(
		t,
		func(_ context.Context, cloudEnv *azureCloud.CloudEnvironment) (*authTypes.AzureCredentials, error) {
			name := "<nil>"
			if cloudEnv != nil {
				name = cloudEnv.Name
			}
			loadedClouds = append(loadedClouds, name)
			return &authTypes.AzureCredentials{AccessToken: "aad-" + name, TenantID: "tenant", CloudEnvironment: name}, nil
		},
		func(_ context.Context, creds authTypes.ICredentials, loginServer string) (*azureCloud.ACRAuthResult, error) {
			exchanged[loginServer] = creds.(*authTypes.AzureCredentials).AccessToken
			return &azureCloud.ACRAuthResult{Username: "u", Password: "p", Registry: loginServer, ExpiresAt: time.Now().Add(time.Hour)}, nil
		},
	)

	registries := []string{"gov1.azurecr.us", "Gov2.AzureCR.us", "cn.azurecr.cn", "pub.azurecr.io"}
	require.NoError(t, executeExplicitRegistries(context.Background(), registries))

	assert.Equal(t, []string{"usgovernment", "china", "public"}, loadedClouds,
		"the ambient token must be requested once per registry cloud")
	assert.Equal(t, map[string]string{
		"gov1.azurecr.us": "aad-usgovernment",
		"gov2.azurecr.us": "aad-usgovernment",
		"cn.azurecr.cn":   "aad-china",
		"pub.azurecr.io":  "aad-public",
	}, exchanged, "each registry must be exchanged with its own cloud's token")

	mgr, err := docker.NewConfigManager()
	require.NoError(t, err)
	written, err := mgr.GetAuthenticatedRegistries()
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"gov1.azurecr.us", "gov2.azurecr.us", "cn.azurecr.cn", "pub.azurecr.io"}, written)
}

// TestExecuteExplicitRegistries_InvalidRegistryFailsBeforeAnyLogin verifies a bad
// registry in the list is rejected before any credential is loaded or any Docker
// auth is written, so a typo never leaves a partial login behind.
func TestExecuteExplicitRegistries_InvalidRegistryFailsBeforeAnyLogin(t *testing.T) {
	t.Setenv("DOCKER_CONFIG", t.TempDir())

	loads := 0
	stubAzureSeams(
		t,
		func(_ context.Context, _ *azureCloud.CloudEnvironment) (*authTypes.AzureCredentials, error) {
			loads++
			return &authTypes.AzureCredentials{}, nil
		},
		func(_ context.Context, _ authTypes.ICredentials, loginServer string) (*azureCloud.ACRAuthResult, error) {
			return &azureCloud.ACRAuthResult{Username: "u", Password: "p", Registry: loginServer}, nil
		},
	)

	err := executeExplicitRegistries(context.Background(), []string{"ok.azurecr.us", "not-a-registry"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrACRInvalidRegistry), "got %v", err)
	assert.Zero(t, loads)

	mgr, err := docker.NewConfigManager()
	require.NoError(t, err)
	written, err := mgr.GetAuthenticatedRegistries()
	require.NoError(t, err)
	assert.Empty(t, written)
}
