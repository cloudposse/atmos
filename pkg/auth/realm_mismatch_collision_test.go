package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/realm"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel for the schema field referenced by these tests.
var _ = schema.AuthConfig{Keyring: schema.KeyringConfig{Type: types.CredentialStoreTypeMemory}}

// legacyCleanupCollisionStore returns a memory credential store holding realm "prod"'s
// credentials for identity "admin" in both the current layout and the previous
// "atmos_prod_admin" layout (written through the raw-key API).
func legacyCleanupCollisionStore(t *testing.T) types.CredentialStore {
	t.Helper()
	store := credentials.NewCredentialStoreWithConfig(&schema.AuthConfig{Keyring: schema.KeyringConfig{Type: types.CredentialStoreTypeMemory}})
	require.NoError(t, store.Store("admin", &types.OIDCCredentials{Token: "prod-current"}, "prod"))
	raw, ok := store.(interface {
		SetAny(key string, value any) error
	})
	require.True(t, ok, "the memory store must expose raw-key writes")
	require.NoError(t, raw.SetAny("atmos_prod_reader", map[string]any{
		"type": "oidc",
		"data": map[string]any{"token": "prod-legacy"},
	}))
	// Check the fixture with a raw-key read: Retrieve would migrate and delete the legacy key.
	assertLegacyReaderKeyExists(t, store)
	return store
}

// assertLegacyReaderKeyExists verifies the previous-layout key "atmos_prod_reader" is still stored.
func assertLegacyReaderKeyExists(t *testing.T, store types.CredentialStore) {
	t.Helper()
	raw, ok := store.(interface {
		GetAny(key string, dest interface{}) error
	})
	require.True(t, ok, "the memory store must expose raw-key reads")
	var entry map[string]any
	require.NoError(t, raw.GetAny("atmos_prod_reader", &entry), "the previous-layout key must still exist")
}

// TestLegacyKeyringCleanup_NeverDeletesAnotherRealm reproduces #3720 finding 3.4 for the
// legacy-key cleanup: a manager in realm "acme" removing the pre-realm entry for its own
// identity "prod_reader" (key "atmos_prod_reader") deleted realm "prod"'s "reader" entry.
// An identity named "prod__admin" maps to realm "prod"'s current-layout key the same way.
func TestLegacyKeyringCleanup_NeverDeletesAnotherRealm(t *testing.T) {
	cleanups := map[string]func(m *manager, alias string){
		"deleteLegacyKeyringEntry": func(m *manager, alias string) { m.deleteLegacyKeyringEntry(alias) },
		"purgeCachedCredentials":   func(m *manager, alias string) { m.purgeCachedCredentials(alias) },
	}
	for name, cleanup := range cleanups {
		t.Run(name, func(t *testing.T) {
			store := legacyCleanupCollisionStore(t)
			m := &manager{credentialStore: store, realm: realm.RealmInfo{Value: "acme", Source: realm.SourceConfig}}

			cleanup(m, "prod_reader")
			cleanup(m, "prod__admin")

			assertLegacyReaderKeyExists(t, store)
			got, err := store.Retrieve("reader", "prod")
			require.NoError(t, err, "realm prod's previous-layout entry must survive realm acme's cleanup")
			assert.Equal(t, "prod-legacy", got.(*types.OIDCCredentials).Token)
			got, err = store.Retrieve("admin", "prod")
			require.NoError(t, err, "realm prod's current entry must survive realm acme's cleanup")
			assert.Equal(t, "prod-current", got.(*types.OIDCCredentials).Token)
		})
	}
}

// TestLegacyKeyringCleanup_RemovesUnambiguousLegacyEntry verifies the cleanup still
// removes a pre-realm entry whose key no realm-scoped entry can share.
func TestLegacyKeyringCleanup_RemovesUnambiguousLegacyEntry(t *testing.T) {
	store := credentials.NewCredentialStoreWithConfig(&schema.AuthConfig{Keyring: schema.KeyringConfig{Type: types.CredentialStoreTypeMemory}})
	require.NoError(t, store.Store("deployer", &types.OIDCCredentials{Token: "legacy"}, ""))
	m := &manager{credentialStore: store, realm: realm.RealmInfo{Value: "acme", Source: realm.SourceConfig}}

	m.deleteLegacyKeyringEntry("deployer")

	_, err := store.Retrieve("deployer", "")
	require.ErrorIs(t, err, credentials.ErrCredentialsNotFound)
}
