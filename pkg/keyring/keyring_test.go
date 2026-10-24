package keyring

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	zkeyring "github.com/zalando/go-keyring"
)

// Use an in-memory mock for the system (zalando) backend so tests do not touch the real OS
// keychain.
func init() {
	zkeyring.MockInit()
}

// newTestFileKeyring builds a file backend rooted in a temp dir with a test password.
func newTestFileKeyring(t *testing.T) Keyring {
	t.Helper()

	t.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-12345")
	k, err := New(Config{Type: TypeFile, ServiceName: "atmos-test", FileDir: t.TempDir()})
	require.NoError(t, err)
	return k
}

// runKeyringContract exercises the behavioral contract every backend must satisfy.
func runKeyringContract(t *testing.T, k Keyring) {
	t.Helper()

	// Missing key -> ErrNotFound.
	_, err := k.Get("absent")
	assert.ErrorIs(t, err, ErrNotFound)

	has, err := k.Has("absent")
	require.NoError(t, err)
	assert.False(t, has)

	// Set then Get round-trips the exact value.
	require.NoError(t, k.Set("k1", "v1"))
	got, err := k.Get("k1")
	require.NoError(t, err)
	assert.Equal(t, "v1", got)

	has, err = k.Has("k1")
	require.NoError(t, err)
	assert.True(t, has)

	// Overwrite.
	require.NoError(t, k.Set("k1", "v1-updated"))
	got, err = k.Get("k1")
	require.NoError(t, err)
	assert.Equal(t, "v1-updated", got)

	// Delete is idempotent.
	require.NoError(t, k.Delete("k1"))
	_, err = k.Get("k1")
	assert.ErrorIs(t, err, ErrNotFound)
	require.NoError(t, k.Delete("k1"), "deleting an absent key must succeed")
}

func TestMemoryKeyring_Contract(t *testing.T) {
	k, err := New(Config{Type: TypeMemory})
	require.NoError(t, err)
	assert.Equal(t, TypeMemory, k.Type())
	runKeyringContract(t, k)
}

func TestSystemKeyring_Contract(t *testing.T) {
	k, err := New(Config{Type: TypeSystem, ServiceName: "atmos-test"})
	require.NoError(t, err)
	assert.Equal(t, TypeSystem, k.Type())
	runKeyringContract(t, k)
}

func TestFileKeyring_Contract(t *testing.T) {
	k := newTestFileKeyring(t)
	assert.Equal(t, TypeFile, k.Type())
	runKeyringContract(t, k)
}

func TestNoopKeyring_Contract(t *testing.T) {
	k, err := New(Config{Type: TypeNoop})
	require.NoError(t, err)
	assert.Equal(t, TypeNoop, k.Type())

	// Writes are dropped; reads report nothing.
	require.NoError(t, k.Set("k", "v"))
	_, err = k.Get("k")
	assert.ErrorIs(t, err, ErrNotFound)
	has, err := k.Has("k")
	require.NoError(t, err)
	assert.False(t, has)
	require.NoError(t, k.Delete("k"))
	list, err := k.List()
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestNew_DefaultsToSystem(t *testing.T) {
	k, err := New(Config{})
	require.NoError(t, err)
	assert.Equal(t, TypeSystem, k.Type())
}

func TestNew_UnknownBackend(t *testing.T) {
	_, err := New(Config{Type: "bogus"})
	assert.ErrorIs(t, err, ErrUnknownBackend)
}

func TestSystemKeyring_ListNotSupported(t *testing.T) {
	k, err := New(Config{Type: TypeSystem, ServiceName: "atmos-test"})
	require.NoError(t, err)
	_, err = k.List()
	assert.ErrorIs(t, err, ErrListNotSupported)
}

func TestMemoryKeyring_List(t *testing.T) {
	k, err := New(Config{Type: TypeMemory})
	require.NoError(t, err)
	require.NoError(t, k.Set("a", "1"))
	require.NoError(t, k.Set("b", "2"))

	keys, err := k.List()
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a", "b"}, keys)
}

func TestFileKeyring_List(t *testing.T) {
	k := newTestFileKeyring(t)
	require.NoError(t, k.Set("alpha", "1"))
	require.NoError(t, k.Set("beta", "2"))

	keys, err := k.List()
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"alpha", "beta"}, keys)
}

func TestFileKeyring_Persistence(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-12345")

	k1, err := New(Config{Type: TypeFile, ServiceName: "atmos-test", FileDir: dir})
	require.NoError(t, err)
	require.NoError(t, k1.Set("persist", "value"))

	// A fresh instance over the same dir reads the prior value.
	k2, err := New(Config{Type: TypeFile, ServiceName: "atmos-test", FileDir: dir})
	require.NoError(t, err)
	got, err := k2.Get("persist")
	require.NoError(t, err)
	assert.Equal(t, "value", got)
}

func TestFileKeyring_MissingPasswordOnAccess(t *testing.T) {
	// No password set and no TTY -> opening succeeds but access fails.
	k, err := New(Config{Type: TypeFile, ServiceName: "atmos-test", FileDir: t.TempDir()})
	require.NoError(t, err)

	err = k.Set("k", "v")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPasswordRequired)
}

func TestFileKeyring_PasswordTooShort(t *testing.T) {
	t.Setenv("ATMOS_KEYRING_PASSWORD", "short")
	k, err := New(Config{Type: TypeFile, ServiceName: "atmos-test", FileDir: t.TempDir()})
	require.NoError(t, err)

	err = k.Set("k", "v")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPasswordTooShort)
}

func TestFileKeyring_CustomPasswordEnv(t *testing.T) {
	t.Setenv("MY_KEYRING_PW", "custom-password-12345")
	k, err := New(Config{Type: TypeFile, ServiceName: "atmos-test", FileDir: t.TempDir(), PasswordEnv: "MY_KEYRING_PW"})
	require.NoError(t, err)

	require.NoError(t, k.Set("k", "v"))
	got, err := k.Get("k")
	require.NoError(t, err)
	assert.Equal(t, "v", got)
}

func TestFileKeyring_FilePathNormalizedToDir(t *testing.T) {
	// A path with an extension is treated as a file; its parent dir is used.
	dir := t.TempDir()
	t.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-12345")
	k, err := New(Config{Type: TypeFile, ServiceName: "atmos-test", FileDir: filepath.Join(dir, "keyring.json")})
	require.NoError(t, err)
	require.NoError(t, k.Set("k", "v"))
}
