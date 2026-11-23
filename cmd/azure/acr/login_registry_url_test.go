package acr

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
	"github.com/cloudposse/atmos/pkg/auth/cloud/docker"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
)

// TestExecuteExplicitRegistries_SovereignAndUppercase verifies an explicit
// --registry keeps its own cloud's login server (azurecr.us / azurecr.cn) instead
// of being rebuilt as *.azurecr.io, and that mixed-case input is normalized.
func TestExecuteExplicitRegistries_SovereignAndUppercase(t *testing.T) {
	tests := []struct {
		registry        string
		wantLoginServer string
	}{
		{"myregistry.azurecr.us", "myregistry.azurecr.us"},
		{"https://myregistry.azurecr.cn", "myregistry.azurecr.cn"},
		{"MyRegistry.azurecr.io", "myregistry.azurecr.io"},
	}
	for _, tt := range tests {
		t.Run(tt.registry, func(t *testing.T) {
			t.Setenv("DOCKER_CONFIG", t.TempDir())
			var gotLoginServer string
			stubAzureSeams(
				t,
				func(_ context.Context, _ *azureCloud.CloudEnvironment) (*authTypes.AzureCredentials, error) {
					return &authTypes.AzureCredentials{TenantID: "tenant-123"}, nil
				},
				func(_ context.Context, _ authTypes.ICredentials, loginServer string) (*azureCloud.ACRAuthResult, error) {
					gotLoginServer = loginServer
					return &azureCloud.ACRAuthResult{
						Username:  "00000000-0000-0000-0000-000000000000",
						Password:  "token",
						Registry:  loginServer,
						ExpiresAt: time.Now().Add(time.Hour),
					}, nil
				},
			)

			require.NoError(t, executeExplicitRegistries(context.Background(), []string{tt.registry}))
			assert.Equal(t, tt.wantLoginServer, gotLoginServer)

			mgr, err := docker.NewConfigManager()
			require.NoError(t, err)
			registries, err := mgr.GetAuthenticatedRegistries()
			require.NoError(t, err)
			assert.Equal(t, []string{tt.wantLoginServer}, registries)
		})
	}
}
