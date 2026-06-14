package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/keyring"
)

// newTestKeychainStore builds a keychain store over the in-memory keyring backend.
func newTestKeychainStore(t *testing.T) Store {
	t.Helper()

	s, err := NewKeychainStore(&KeychainStoreOptions{Backend: keyring.TypeMemory})
	require.NoError(t, err)
	return s
}

func TestKeychainStore_ImplementsInterfaces(t *testing.T) {
	s := newTestKeychainStore(t)
	_, ok := s.(DeletableStore)
	assert.True(t, ok, "keychain store must be deletable")
	_, ok = s.(StatusStore)
	assert.True(t, ok, "keychain store must support Has")
}

func TestKeychainStore_SetGetRoundTrip(t *testing.T) {
	s := newTestKeychainStore(t)

	require.NoError(t, s.Set("dev", "vpc", "token", "s3cr3t"))

	got, err := s.Get("dev", "vpc", "token")
	require.NoError(t, err)
	assert.Equal(t, "s3cr3t", got)
}

func TestKeychainStore_SetGetStructuredValue(t *testing.T) {
	s := newTestKeychainStore(t)

	require.NoError(t, s.Set("dev", "vpc", "cfg", map[string]any{"a": "1", "b": "2"}))

	got, err := s.Get("dev", "vpc", "cfg")
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"a": "1", "b": "2"}, got)
}

func TestKeychainStore_GetMissing(t *testing.T) {
	s := newTestKeychainStore(t)

	_, err := s.Get("dev", "vpc", "absent")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrKeychainNotFound)
}

func TestKeychainStore_Has(t *testing.T) {
	s := newTestKeychainStore(t)
	ss := s.(StatusStore)

	has, err := ss.Has("dev", "vpc", "token")
	require.NoError(t, err)
	assert.False(t, has)

	require.NoError(t, s.Set("dev", "vpc", "token", "v"))
	has, err = ss.Has("dev", "vpc", "token")
	require.NoError(t, err)
	assert.True(t, has)
}

func TestKeychainStore_Delete(t *testing.T) {
	s := newTestKeychainStore(t)
	ds := s.(DeletableStore)

	require.NoError(t, s.Set("dev", "vpc", "token", "v"))
	require.NoError(t, ds.Delete("dev", "vpc", "token"))

	_, err := s.Get("dev", "vpc", "token")
	assert.ErrorIs(t, err, ErrKeychainNotFound)

	// Delete is idempotent.
	require.NoError(t, ds.Delete("dev", "vpc", "token"))
}

func TestKeychainStore_KeyComposition(t *testing.T) {
	s, err := NewKeychainStore(&KeychainStoreOptions{Backend: keyring.TypeMemory, Prefix: "atmos"})
	require.NoError(t, err)

	// Store via the composed key, then read the same value back by the raw key to pin the layout.
	require.NoError(t, s.Set("plat-ue2-dev", "vpc/flow-logs", "token", "v"))

	got, err := s.GetKey("atmos/plat/ue2/dev/vpc/flow-logs/token")
	require.NoError(t, err)
	assert.Equal(t, "v", got)
}

func TestKeychainStore_ValidatesEmptyArgs(t *testing.T) {
	s := newTestKeychainStore(t)

	assert.ErrorIs(t, s.Set("dev", "vpc", "", "v"), ErrEmptyKey)
	assert.ErrorIs(t, s.Set("dev", "vpc", "k", nil), ErrNilValue)

	_, err := s.GetKey("")
	assert.ErrorIs(t, err, ErrEmptyKey)
}

// TestKeychainStore_ScopedCoordinates proves the empty-segment contract for scoped secret
// coordinates: a stack-scoped coordinate (empty component) omits the component segment, and a
// global coordinate (empty stack and component) collapses to `prefix/key`. The raw GetKey reads
// pin the exact composed layout.
func TestKeychainStore_ScopedCoordinates(t *testing.T) {
	s, err := NewKeychainStore(&KeychainStoreOptions{Backend: keyring.TypeMemory, Prefix: "atmos"})
	require.NoError(t, err)

	require.NoError(t, s.Set("plat-ue2-dev", "", "STACK_KEY", "stack-v"))
	got, err := s.GetKey("atmos/plat/ue2/dev/STACK_KEY")
	require.NoError(t, err)
	assert.Equal(t, "stack-v", got)

	require.NoError(t, s.Set("", "", "GLOBAL_KEY", "global-v"))
	got, err = s.GetKey("atmos/GLOBAL_KEY")
	require.NoError(t, err)
	assert.Equal(t, "global-v", got)
}

func TestKeychainStore_RegistryBuildsAndIsSecretByDefault(t *testing.T) {
	// The keychain kind is secret-by-default.
	cfg := StoresConfig{
		"local": {Kind: KindKeychain, Options: map[string]any{"backend": keyring.TypeMemory}},
	}
	ApplySecretDefaults(cfg)
	assert.True(t, cfg["local"].Secret, "keychain stores should default to secret: true")

	registry, err := NewStoreRegistry(&cfg)
	require.NoError(t, err)

	s, ok := registry["local"]
	require.True(t, ok)
	require.NoError(t, s.Set("dev", "vpc", "k", "v"))
	got, err := s.Get("dev", "vpc", "k")
	require.NoError(t, err)
	assert.Equal(t, "v", got)
}

func TestKeychainStore_LegacyTypeAlias(t *testing.T) {
	// Both "keychain" and "keyring" map to the keychain kind.
	assert.Equal(t, KindKeychain, mapLegacyType("keychain"))
	assert.Equal(t, KindKeychain, mapLegacyType("keyring"))
}
