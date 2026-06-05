package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeVaultKV is an in-memory VaultKVClient for testing the VaultStore logic.
type fakeVaultKV struct {
	data map[string]map[string]any
}

func newFakeVaultKV() *fakeVaultKV {
	return &fakeVaultKV{data: make(map[string]map[string]any)}
}

func (f *fakeVaultKV) Put(_ context.Context, path string, data map[string]any) error {
	f.data[path] = data
	return nil
}

func (f *fakeVaultKV) Get(_ context.Context, path string) (map[string]any, error) {
	v, ok := f.data[path]
	if !ok {
		return nil, nil
	}
	return v, nil
}

func (f *fakeVaultKV) Delete(_ context.Context, path string) error {
	delete(f.data, path)
	return nil
}

// newTestVaultStore builds a VaultStore wired to an in-memory KV client.
func newTestVaultStore(client VaultKVClient) *VaultStore {
	delim := "/"
	return &VaultStore{
		client:         client,
		mount:          "secret",
		stackDelimiter: &delim,
	}
}

func TestVaultStore_SetGetDeleteHas(t *testing.T) {
	fake := newFakeVaultKV()
	s := newTestVaultStore(fake)

	require.NoError(t, s.Set("prod", "api", "API_KEY", "secret-value"))

	got, err := s.Get("prod", "api", "API_KEY")
	require.NoError(t, err)
	assert.Equal(t, "secret-value", got)

	has, err := s.Has("prod", "api", "API_KEY")
	require.NoError(t, err)
	assert.True(t, has)

	require.NoError(t, s.Delete("prod", "api", "API_KEY"))

	has, err = s.Has("prod", "api", "API_KEY")
	require.NoError(t, err)
	assert.False(t, has)
}

func TestVaultStore_Set_Validation(t *testing.T) {
	s := newTestVaultStore(newFakeVaultKV())
	assert.ErrorIs(t, s.Set("", "api", "k", "v"), ErrEmptyStack)
	assert.ErrorIs(t, s.Set("prod", "", "k", "v"), ErrEmptyComponent)
	assert.ErrorIs(t, s.Set("prod", "api", "", "v"), ErrEmptyKey)
	assert.ErrorIs(t, s.Set("prod", "api", "k", nil), ErrNilValue)
}

func TestVaultStore_ImplementsInterfaces(t *testing.T) {
	var s Store = newTestVaultStore(newFakeVaultKV())
	_, ok := s.(DeletableStore)
	assert.True(t, ok)
	_, ok = s.(StatusStore)
	assert.True(t, ok)
}
