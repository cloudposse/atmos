package providers

import (
	"path/filepath"
	"testing"

	"filippo.io/age"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/keyring"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
	storeproviders "github.com/cloudposse/atmos/pkg/store/providers"
)

// newKeychainBackedConfig returns an AtmosConfiguration whose store registry has an in-memory
// keychain store named "keychain" (no OS keychain required).
func newKeychainBackedConfig(t *testing.T) *schema.AtmosConfiguration {
	t.Helper()
	kc, err := storeproviders.NewKeychainStore(&storeproviders.KeychainStoreOptions{Backend: keyring.TypeMemory})
	require.NoError(t, err)
	return &schema.AtmosConfiguration{Stores: store.StoreRegistry{"keychain": kc}}
}

func ageStoreProvider(t *testing.T, cfg *schema.AtmosConfiguration, spec map[string]any) Provider {
	t.Helper()
	section := map[string]any{"dev-sops": map[string]any{"kind": "sops/age", "spec": spec}}
	p, err := newSopsProvider(cfg, "dev-sops", section)
	require.NoError(t, err)
	return p
}

// TestSopsProvider_AgeKeyStore_ReadRoundTrip seeds the age private key into the keychain store and
// proves the provider decrypts via `age_key.store` with SOPS_AGE_KEY_FILE unset.
func TestSopsProvider_AgeKeyStore_ReadRoundTrip(t *testing.T) {
	t.Setenv("SOPS_AGE_KEY_FILE", filepath.Join(t.TempDir(), "absent.txt"))
	t.Setenv("SOPS_AGE_KEY", "")

	identity, err := age.GenerateX25519Identity()
	require.NoError(t, err)

	cfg := newKeychainBackedConfig(t)
	// Seed the key at the provider-owned triple (stack=vault, component=age-key, key=vault).
	require.NoError(t, cfg.Stores["keychain"].Set("dev-sops", ageKeyStoreComponent, "dev-sops", identity.String()))

	file := filepath.Join(t.TempDir(), "dev.enc.yaml")
	p := ageStoreProvider(t, cfg, map[string]any{
		"file":           file,
		"age_recipients": identity.Recipient().String(), // encrypt with the matching recipient
		"age_key":        map[string]any{"store": "keychain"},
	})

	coord := Coordinate{Stack: "dev", Component: "api", Key: "DATADOG_API_KEY"}
	require.NoError(t, p.Set(coord, "dd-secret"))

	got, err := p.Get(coord)
	require.NoError(t, err, "decryption must use the age key from the keychain store")
	assert.Equal(t, "dd-secret", got)
}

// TestSopsProvider_AgeKeyStore_KeygenWritesAndReads proves the full round-trip: keygen writes the
// generated private key into the store, then a provider decrypts with it (no key file, no env).
func TestSopsProvider_AgeKeyStore_KeygenWritesAndReads(t *testing.T) {
	t.Setenv("SOPS_AGE_KEY_FILE", filepath.Join(t.TempDir(), "absent.txt"))
	t.Setenv("SOPS_AGE_KEY", "")

	cfg := newKeychainBackedConfig(t)
	base := t.TempDir()
	file := filepath.Join(base, "dev.enc.yaml")

	gen := ageStoreProvider(t, cfg, map[string]any{
		"file":            file,
		"age_key":         map[string]any{"store": "keychain"},
		"recipients_file": filepath.Join(base, ".sops.yaml"),
	})

	kg, ok := gen.(KeyGenerator)
	require.True(t, ok)
	require.False(t, kg.HasKey(), "store starts without a key")

	res, err := kg.GenerateKey(base)
	require.NoError(t, err)
	require.NotEmpty(t, res.Public)
	assert.True(t, kg.HasKey(), "store holds the key after keygen")

	// A provider using the generated recipient + the keychain-held key must round-trip.
	reader := ageStoreProvider(t, cfg, map[string]any{
		"file":           file,
		"age_recipients": res.Public,
		"age_key":        map[string]any{"store": "keychain"},
	})
	coord := Coordinate{Stack: "dev", Component: "api", Key: "K"}
	require.NoError(t, reader.Set(coord, "v"))
	got, err := reader.Get(coord)
	require.NoError(t, err)
	assert.Equal(t, "v", got)
}

// TestSopsProvider_AgeKeyStore_NotConfigured proves an unknown store name is an actionable error.
func TestSopsProvider_AgeKeyStore_NotConfigured(t *testing.T) {
	cfg := newKeychainBackedConfig(t)
	identity, err := age.GenerateX25519Identity()
	require.NoError(t, err)
	file := filepath.Join(t.TempDir(), "dev.enc.yaml")

	p := ageStoreProvider(t, cfg, map[string]any{
		"file":           file,
		"age_recipients": identity.Recipient().String(),
		"age_key":        map[string]any{"store": "nope"},
	})
	require.NoError(t, p.Set(Coordinate{Stack: "dev", Component: "api", Key: "K"}, "v"))

	_, err = p.Get(Coordinate{Stack: "dev", Component: "api", Key: "K"})
	require.ErrorIs(t, err, ErrSopsAgeKey)
}

func TestParseAgeKeySpec(t *testing.T) {
	t.Run("bare string is inline value", func(t *testing.T) {
		ak := parseAgeKeySpec(map[string]any{"age_key": "AGE-SECRET-KEY-1xxx"})
		assert.Equal(t, "AGE-SECRET-KEY-1xxx", ak.value)
		assert.Empty(t, ak.storeName)
		assert.Empty(t, ak.path)
		assert.Empty(t, ak.file)
	})
	t.Run("age_key_file shorthand", func(t *testing.T) {
		ak := parseAgeKeySpec(map[string]any{"age_key_file": "keys.txt"})
		assert.Empty(t, ak.value)
		assert.Empty(t, ak.storeName)
		assert.Empty(t, ak.path)
		assert.Equal(t, "keys.txt", ak.file)
	})
	t.Run("object store mode", func(t *testing.T) {
		ak := parseAgeKeySpec(map[string]any{"age_key": map[string]any{"store": "keychain", "path": "k"}})
		assert.Empty(t, ak.value)
		assert.Equal(t, "keychain", ak.storeName)
		assert.Equal(t, "k", ak.path)
		assert.Empty(t, ak.file)
	})
	t.Run("object file mode maps path to file", func(t *testing.T) {
		ak := parseAgeKeySpec(map[string]any{"age_key": map[string]any{"store": "file", "path": "keys.txt"}})
		assert.Empty(t, ak.value)
		assert.Empty(t, ak.storeName)
		assert.Empty(t, ak.path)
		assert.Equal(t, "keys.txt", ak.file)
	})
	t.Run("object value wins", func(t *testing.T) {
		ak := parseAgeKeySpec(map[string]any{"age_key": map[string]any{"value": "AGE-SECRET-KEY-1yyy", "store": "keychain"}})
		assert.Equal(t, "AGE-SECRET-KEY-1yyy", ak.value)
		assert.Equal(t, "keychain", ak.storeName) // store still parsed; value takes precedence in keyClient.
	})
}
