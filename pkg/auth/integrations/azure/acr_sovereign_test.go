package azure

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
	"github.com/cloudposse/atmos/pkg/auth/cloud/docker"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guard: these tests depend on the provider's cloud reaching the
// integration through AzureCredentials.CloudEnvironment.
var _ = types.AzureCredentials{CloudEnvironment: "usgovernment"}

// stubACRSeams replaces the ACR token exchange with a stub that records the login
// server it was asked for, and points the Docker config at a temp directory.
func stubACRSeams(t *testing.T) *string {
	t.Helper()

	t.Setenv("DOCKER_CONFIG", t.TempDir())
	origGetAuth := acrGetAuthToken
	origDockerFactory := acrDockerConfigFactory
	t.Cleanup(func() {
		acrGetAuthToken = origGetAuth
		acrDockerConfigFactory = origDockerFactory
	})

	var requested string
	acrGetAuthToken = func(_ context.Context, _ types.ICredentials, loginServer string) (*azureCloud.ACRAuthResult, error) {
		requested = loginServer
		return &azureCloud.ACRAuthResult{
			Username:  "00000000-0000-0000-0000-000000000000",
			Password:  "refresh-token",
			Registry:  loginServer,
			ExpiresAt: time.Now().Add(time.Hour),
		}, nil
	}
	acrDockerConfigFactory = docker.NewConfigManager
	return &requested
}

// TestACRIntegration_LoginServerFollowsProviderCloud verifies that an integration
// configured by registry name builds the login server for the Azure cloud of the
// identity's provider (spec.cloud_environment, carried on the credentials), and
// defaults to the public cloud when no cloud is known.
func TestACRIntegration_LoginServerFollowsProviderCloud(t *testing.T) {
	tests := []struct {
		name  string
		creds types.ICredentials
		want  string
	}{
		{"usgovernment", &types.AzureCredentials{CloudEnvironment: "usgovernment"}, "myregistry.azurecr.us"},
		{"china", &types.AzureCredentials{CloudEnvironment: "china"}, "myregistry.azurecr.cn"},
		{"public", &types.AzureCredentials{CloudEnvironment: "public"}, "myregistry.azurecr.io"},
		{"unset cloud defaults to public", &types.AzureCredentials{}, "myregistry.azurecr.io"},
		{"nil credentials default to public", nil, "myregistry.azurecr.io"},
	}
	for _, tt := range tests {
		t.Run(tt.name+"/Execute", func(t *testing.T) {
			requested := stubACRSeams(t)
			integration := &ACRIntegration{name: "acr", registry: &schema.Registry{Name: "myregistry"}}

			require.NoError(t, integration.Execute(context.Background(), tt.creds))
			assert.Equal(t, tt.want, *requested, "token exchange must target the provider cloud's login server")

			mgr, err := docker.NewConfigManager()
			require.NoError(t, err)
			registries, err := mgr.GetAuthenticatedRegistries()
			require.NoError(t, err)
			assert.Equal(t, []string{tt.want}, registries, "Docker auth must be written for the same login server")
		})
		t.Run(tt.name+"/ExchangeCredentials", func(t *testing.T) {
			requested := stubACRSeams(t)
			integration := &ACRIntegration{name: "acr", registry: &schema.Registry{Name: "myregistry"}}

			minted, err := integration.ExchangeCredentials(context.Background(), tt.creds)
			require.NoError(t, err)
			assert.Equal(t, tt.want, *requested)
			assert.Equal(t, tt.want, minted.Registry)
		})
	}
}

// TestACRIntegration_CleanupRemovesSovereignLoginServer verifies logout removes the
// Docker auth that Execute wrote for a sovereign-cloud registry. Cleanup runs without
// credentials, so it cannot know the provider's cloud and must cover every Azure cloud.
func TestACRIntegration_CleanupRemovesSovereignLoginServer(t *testing.T) {
	for _, loginServer := range []string{"myregistry.azurecr.io", "myregistry.azurecr.us", "myregistry.azurecr.cn"} {
		t.Run(loginServer, func(t *testing.T) {
			t.Setenv("DOCKER_CONFIG", t.TempDir())
			origDockerFactory := acrDockerConfigFactory
			t.Cleanup(func() { acrDockerConfigFactory = origDockerFactory })
			acrDockerConfigFactory = docker.NewConfigManager

			mgr, err := docker.NewConfigManager()
			require.NoError(t, err)
			require.NoError(t, mgr.WriteAuth(loginServer, "00000000-0000-0000-0000-000000000000", "token"))
			// An unrelated registry must survive the cleanup.
			require.NoError(t, mgr.WriteAuth("other.azurecr.us", "00000000-0000-0000-0000-000000000000", "token"))

			integration := &ACRIntegration{name: "acr", registry: &schema.Registry{Name: "myregistry"}}
			require.NoError(t, integration.Cleanup(context.Background()))

			registries, err := mgr.GetAuthenticatedRegistries()
			require.NoError(t, err)
			assert.Equal(t, []string{"other.azurecr.us"}, registries)
		})
	}
}
