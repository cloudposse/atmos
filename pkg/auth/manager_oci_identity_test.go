package auth_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestOCIIdentityAuthManagerLoginCacheRefreshLogout(t *testing.T) {
	t.Setenv("TEST_OCI_MANAGER_PASSWORD", "original-private-token")
	config := &schema.AuthConfig{
		Realm:      "oci-manager-test",
		Keyring:    schema.KeyringConfig{Type: "memory"},
		Identities: map[string]schema.Identity{"registry-bot": {Kind: types.IdentityKindOCIUser, Spec: map[string]any{"registry": "ghcr.io", "username": "company-bot", "password_env": "TEST_OCI_MANAGER_PASSWORD"}}},
	}
	store := credentials.NewCredentialStoreWithConfig(config)
	manager, err := auth.NewAuthManager(config, store, validation.NewValidator(), nil, t.TempDir())
	require.NoError(t, err)
	require.NoError(t, manager.Validate())
	info, err := manager.Authenticate(t.Context(), "registry-bot")
	require.NoError(t, err)
	require.Equal(t, "company-bot", info.Principal)
	require.Equal(t, "ghcr.io", info.Account)
	raw, err := json.Marshal(info)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "private-token")
	persisted, err := store.Retrieve("registry-bot", manager.GetRealm().Value)
	require.NoError(t, err)
	require.Equal(t, &types.OCICredentials{Registry: "ghcr.io", Username: "company-bot", Password: "original-private-token"}, persisted)

	// A fresh manager reuses the realm-scoped keyring with no environment secret.
	t.Setenv("TEST_OCI_MANAGER_PASSWORD", "")
	manager, err = auth.NewAuthManager(config, store, validation.NewValidator(), nil, t.TempDir())
	require.NoError(t, err)
	cached, err := manager.GetCachedCredentials(t.Context(), "registry-bot")
	require.NoError(t, err)
	require.NoError(t, cached.Rehydrate(store))
	require.Equal(t, persisted, cached.Credentials)

	// Explicit login rereads the configured source rather than retaining the old PAT.
	t.Setenv("TEST_OCI_MANAGER_PASSWORD", "replacement-private-token")
	_, err = manager.Authenticate(t.Context(), "registry-bot")
	require.NoError(t, err)
	persisted, err = store.Retrieve("registry-bot", manager.GetRealm().Value)
	require.NoError(t, err)
	require.Equal(t, "replacement-private-token", persisted.(*types.OCICredentials).Password)
	require.NoError(t, store.Store("other-identity", persisted, manager.GetRealm().Value))

	// Plain auth logout removes OCI credentials without requiring --keychain.
	require.NoError(t, manager.Logout(t.Context(), "registry-bot", false))
	_, err = manager.GetCachedCredentials(t.Context(), "registry-bot")
	require.Error(t, err)
	_, err = store.Retrieve("registry-bot", manager.GetRealm().Value)
	require.ErrorIs(t, err, credentials.ErrCredentialsNotFound)
	other, err := store.Retrieve("other-identity", manager.GetRealm().Value)
	require.NoError(t, err)
	require.Equal(t, persisted, other)
	t.Setenv("TEST_OCI_MANAGER_PASSWORD", "")
	_, err = manager.Authenticate(t.Context(), "registry-bot")
	require.Error(t, err, "logout must not leave a reusable process cache")
}
