package bootstrap

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	authcredentials "github.com/cloudposse/atmos/pkg/auth/credentials"
	authregistry "github.com/cloudposse/atmos/pkg/auth/registry"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	"github.com/cloudposse/atmos/pkg/plugin/config"
	"github.com/cloudposse/atmos/pkg/plugin/install"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

func buildSecretPlugin(t *testing.T) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(source))))
	pluginSource := filepath.Join(root, "plugins", "auth-secrets")
	packageDir := t.TempDir()
	for _, relative := range []string{"plugin.yaml", "schemas/provider.json", "schemas/identity.json"} {
		raw, err := os.ReadFile(filepath.Join(pluginSource, relative))
		require.NoError(t, err)
		destination := filepath.Join(packageDir, relative)
		require.NoError(t, os.MkdirAll(filepath.Dir(destination), 0o700))
		require.NoError(t, os.WriteFile(destination, raw, 0o600))
	}
	definition, err := manifest.Load(packageDir)
	require.NoError(t, err)
	executable := filepath.Join(packageDir, manifest.Executable(definition, install.CurrentPlatform()))
	require.NoError(t, os.MkdirAll(filepath.Dir(executable), 0o700))
	command := exec.CommandContext(t.Context(), "go", "build", "-o", executable, ".")
	command.Dir = pluginSource
	output, err := command.CombinedOutput()
	require.NoError(t, err, "%s", output)
	return packageDir
}

func TestNativeSecretStoreProviderAndEnvironmentIdentity(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and executes the auth-secrets native plugin")
	}
	packageDir := buildSecretPlugin(t)
	t.Setenv("ATMOS_KEYRING_TYPE", "memory")
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
	backend := store.NewMockFieldStore(gomock.NewController(t))
	backend.EXPECT().GetKeyField("service/account", "api_token").Return("integration-private-token", nil)
	providerConfig := schema.Provider{Kind: "secrets/store", Spec: map[string]any{"credentials": map[string]any{"api-token": map[string]any{"store": "vault", "path": "service/account", "field": "api_token"}}}}
	identityConfig := schema.Identity{Kind: "secrets/env", Via: &schema.IdentityVia{Provider: "vault-provider"}, Spec: map[string]any{"env": map[string]any{"SERVICE_TOKEN": `{{ index .credentials "api-token" }}`}}}
	atmos := &schema.AtmosConfiguration{
		Auth:   schema.AuthConfig{Providers: map[string]schema.Provider{"vault-provider": providerConfig}, Identities: map[string]schema.Identity{"service": identityConfig}},
		Stores: store.StoreRegistry{"vault": backend}, StoresConfig: store.StoresConfig{"vault": {Secret: true}},
	}
	cleanup, err := registerSelected(t.Context(), atmos, &config.Selections{Packages: map[string]config.Selection{"secrets": {Path: packageDir, Local: true}}})
	require.NoError(t, err)
	t.Cleanup(cleanup)
	provider, err := authregistry.NewProvider("vault-provider", &providerConfig)
	require.NoError(t, err)
	require.NoError(t, provider.Validate())
	credentials, err := provider.Authenticate(t.Context())
	require.NoError(t, err)
	require.Equal(t, "secret-values/v1", credentials.(*types.PluginCredentials).Type)
	identity, err := authregistry.NewIdentity("service", &identityConfig)
	require.NoError(t, err)
	require.NoError(t, identity.Validate())
	environmentCredentials, err := identity.Authenticate(t.Context(), credentials)
	require.NoError(t, err)
	authContext := &schema.AuthContext{}
	require.NoError(t, identity.PostAuthenticate(t.Context(), &types.PostAuthenticateParams{AuthContext: authContext, Credentials: environmentCredentials}))
	require.Equal(t, map[string]string{"SERVICE_TOKEN": "integration-private-token"}, authContext.Environment)
	environment, err := identity.PrepareEnvironment(t.Context(), map[string]string{"service_token": "old", "OTHER": "preserved"})
	require.NoError(t, err)
	require.Equal(t, map[string]string{"SERVICE_TOKEN": "integration-private-token", "OTHER": "preserved"}, environment)
	info := &types.WhoamiInfo{Credentials: environmentCredentials}
	environmentCredentials.BuildWhoamiInfo(info)
	require.Empty(t, info.Principal)
	require.NoError(t, identity.Logout(t.Context()))
	_, err = identity.Environment()
	require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	require.NoError(t, provider.Logout(t.Context()))

	// Exercise ordinary auth-manager orchestration and cached environment reuse.
	backend.EXPECT().GetKeyField("service/account", "api_token").Return("integration-private-token", nil)
	manager, err := auth.NewAuthManager(&atmos.Auth, authcredentials.NewCredentialStoreWithConfig(&atmos.Auth), validation.NewValidator(), nil, "")
	require.NoError(t, err)
	for range 2 {
		environment, err := manager.EnsureIdentityEnvironment(t.Context(), "service")
		require.NoError(t, err)
		require.Equal(t, map[string]string{"SERVICE_TOKEN": "integration-private-token"}, environment)
	}
	shell, err := manager.PrepareShellEnvironment(t.Context(), "service", []string{"service_token=old", "OTHER=preserved"})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"SERVICE_TOKEN=integration-private-token", "OTHER=preserved"}, shell)
}
