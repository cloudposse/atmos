package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth"
	authcredentials "github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	"github.com/cloudposse/atmos/pkg/plugin/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

// TestNativeEnvironmentIdentityPassivePathInFreshProcess models two processes: one authenticates
// and stores the identity credential, a second builds a brand new manager (and so brand new
// adapter instances) over the same credential store and only uses the passive paths. The plugin
// must not run again, and the stored credential must supply the environment.
func TestNativeEnvironmentIdentityPassivePathInFreshProcess(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and executes the auth-secrets native plugin")
	}
	packageDir := buildSecretPlugin(t)
	t.Setenv("ATMOS_KEYRING_TYPE", "memory")
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())

	backend := store.NewMockFieldStore(gomock.NewController(t))
	// The provider secret is read once: by the first process. A second read means the plugin re-ran.
	backend.EXPECT().GetKeyField("service/account", "api_token").Return("passive-private-token", nil).Times(1)
	providerConfig := schema.Provider{Kind: "secrets/store", Spec: map[string]any{"credentials": map[string]any{"api-token": map[string]any{"store": "vault", "path": "service/account", "field": "api_token"}}}}
	identityConfig := schema.Identity{Kind: "secrets/env", Via: &schema.IdentityVia{Provider: "vault-provider"}, Spec: map[string]any{"env": map[string]any{"SERVICE_TOKEN": `{{ index .credentials "api-token" }}`}}}
	atmos := &schema.AtmosConfiguration{
		Auth:   schema.AuthConfig{Providers: map[string]schema.Provider{"vault-provider": providerConfig}, Identities: map[string]schema.Identity{"service": identityConfig}},
		Stores: store.StoreRegistry{"vault": backend}, StoresConfig: store.StoresConfig{"vault": {Secret: true}},
	}
	cleanup, err := registerSelected(t.Context(), atmos, &config.Selections{Packages: map[string]config.Selection{"secrets": {Path: packageDir, Local: true}}})
	require.NoError(t, err)
	t.Cleanup(cleanup)

	// Process one: authenticate, which stores the identity credential.
	sharedStore := authcredentials.NewCredentialStoreWithConfig(&atmos.Auth)
	first, err := auth.NewAuthManager(&atmos.Auth, sharedStore, validation.NewValidator(), nil, "")
	require.NoError(t, err)
	_, err = first.Authenticate(t.Context(), "service")
	require.NoError(t, err)

	// Process two: a new manager over the same stored credentials, never calling Authenticate.
	newProcess := func(t *testing.T) auth.AuthManager {
		t.Helper()
		manager, err := auth.NewAuthManager(&atmos.Auth, sharedStore, validation.NewValidator(), nil, "")
		require.NoError(t, err)
		_, err = manager.GetCachedCredentials(t.Context(), "service")
		require.NoError(t, err, "the stored credential must be found on the passive path")
		return manager
	}

	want := map[string]string{"SERVICE_TOKEN": "passive-private-token"}
	t.Run("GetEnvironmentVariables", func(t *testing.T) {
		environment, err := newProcess(t).GetEnvironmentVariables("service")
		require.NoError(t, err)
		assert.Equal(t, want, environment)
	})
	t.Run("PrepareShellEnvironment", func(t *testing.T) {
		shell, err := newProcess(t).PrepareShellEnvironment(t.Context(), "service", []string{"OTHER=preserved"})
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"SERVICE_TOKEN=passive-private-token", "OTHER=preserved"}, shell)
	})
	t.Run("EnsureIdentityEnvironment", func(t *testing.T) {
		environment, err := newProcess(t).EnsureIdentityEnvironment(t.Context(), "service")
		require.NoError(t, err)
		assert.Equal(t, want, environment)
	})
}
