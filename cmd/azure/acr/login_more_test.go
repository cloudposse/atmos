package acr

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
)

func okACRCreds(context.Context) (*authTypes.AzureCredentials, error) {
	return &authTypes.AzureCredentials{TenantID: "tenant-123"}, nil
}

// TestExecuteExplicitRegistries_NoExpiry covers the success branch where the token has no
// expiration (zero ExpiresAt) — the "ACR login: <server>" message without an "expires in" suffix.
func TestExecuteExplicitRegistries_NoExpiry(t *testing.T) {
	t.Setenv("DOCKER_CONFIG", t.TempDir())
	stubAzureSeams(
		t, okACRCreds,
		func(_ context.Context, _ authTypes.ICredentials, loginServer string) (*azureCloud.ACRAuthResult, error) {
			return &azureCloud.ACRAuthResult{Username: "u", Password: "p", Registry: loginServer}, nil // zero ExpiresAt
		},
	)

	require.NoError(t, executeExplicitRegistries(context.Background(), []string{"myregistry.azurecr.io"}))
}

// TestExecuteLoginCommand_ExplicitRegistry drives RunE (executeLoginCommand) down the
// explicit-registry branch, which needs no atmos config.
func TestExecuteLoginCommand_ExplicitRegistry(t *testing.T) {
	t.Setenv("DOCKER_CONFIG", t.TempDir())
	stubAzureSeams(
		t, okACRCreds,
		func(_ context.Context, _ authTypes.ICredentials, loginServer string) (*azureCloud.ACRAuthResult, error) {
			return validACRAuthResult(), nil
		},
	)

	require.NoError(t, loginCmd.Flags().Set("registry", "myregistry.azurecr.io"))
	t.Cleanup(func() { _ = loginCmd.Flags().Set("registry", "") })

	require.NoError(t, executeLoginCommand(loginCmd, []string{}))
}

// TestExecuteLoginCommand_RegistryWithIntegrationIsExclusive covers the guard that --registry
// cannot be combined with an integration argument.
func TestExecuteLoginCommand_RegistryWithIntegrationIsExclusive(t *testing.T) {
	require.NoError(t, loginCmd.Flags().Set("registry", "myregistry.azurecr.io"))
	t.Cleanup(func() { _ = loginCmd.Flags().Set("registry", "") })

	require.Error(t, executeLoginCommand(loginCmd, []string{"dev/acr"}))
}

// TestExecuteLoginCommand_HelpArg covers the positional "help" short-circuit.
func TestExecuteLoginCommand_HelpArg(t *testing.T) {
	require.NoError(t, executeLoginCommand(loginCmd, []string{"help"}))
}
