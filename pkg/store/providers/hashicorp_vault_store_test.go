package providers

import (
	"context"
	"errors"
	"testing"

	vault "github.com/hashicorp/vault/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/store"
)

// fakeVaultKV is an in-memory VaultKVClient for testing the VaultStore logic.
type fakeVaultKV struct {
	data map[string]map[string]any

	putErr error // returned by Put when set.
	getErr error // returned by Get when set.
	delErr error // returned by Delete when set.
}

func newFakeVaultKV() *fakeVaultKV {
	return &fakeVaultKV{data: make(map[string]map[string]any)}
}

func (f *fakeVaultKV) Put(_ context.Context, path string, data map[string]any) error {
	if f.putErr != nil {
		return f.putErr
	}
	f.data[path] = data
	return nil
}

func (f *fakeVaultKV) Get(_ context.Context, path string) (map[string]any, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	v, ok := f.data[path]
	if !ok {
		return nil, nil
	}
	return v, nil
}

func (f *fakeVaultKV) Delete(_ context.Context, path string) error {
	if f.delErr != nil {
		return f.delErr
	}
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
	assert.ErrorIs(t, s.Set("", "api", "k", "v"), store.ErrEmptyStack)
	assert.ErrorIs(t, s.Set("prod", "", "k", "v"), store.ErrEmptyComponent)
	assert.ErrorIs(t, s.Set("prod", "api", "", "v"), store.ErrEmptyKey)
	assert.ErrorIs(t, s.Set("prod", "api", "k", nil), store.ErrNilValue)
}

func TestVaultStore_ImplementsInterfaces(t *testing.T) {
	var s store.Store = newTestVaultStore(newFakeVaultKV())
	_, ok := s.(store.DeletableStore)
	assert.True(t, ok)
	_, ok = s.(store.StatusStore)
	assert.True(t, ok)
}

func TestVaultStore_Get_Validation(t *testing.T) {
	s := newTestVaultStore(newFakeVaultKV())
	_, err := s.Get("", "api", "k")
	assert.ErrorIs(t, err, store.ErrEmptyStack)
	_, err = s.Get("prod", "", "k")
	assert.ErrorIs(t, err, store.ErrEmptyComponent)
	_, err = s.Get("prod", "api", "")
	assert.ErrorIs(t, err, store.ErrEmptyKey)
}

func TestVaultStore_Delete_Validation(t *testing.T) {
	s := newTestVaultStore(newFakeVaultKV())
	assert.ErrorIs(t, s.Delete("", "api", "k"), store.ErrEmptyStack)
	assert.ErrorIs(t, s.Delete("prod", "", "k"), store.ErrEmptyComponent)
	assert.ErrorIs(t, s.Delete("prod", "api", ""), store.ErrEmptyKey)
}

func TestVaultStore_KeyBuilding(t *testing.T) {
	fake := newFakeVaultKV()
	prefix := "atmos"
	delim := "-"
	s := &VaultStore{client: fake, mount: "secret", prefix: prefix, stackDelimiter: &delim}

	require.NoError(t, s.Set("plat-prod-ue1", "vpc/flow-logs", "API_KEY", "v"))
	// stack split on "-", component split on "/", joined with "/" final delimiter.
	const wantPath = "atmos/plat/prod/ue1/vpc/flow-logs/API_KEY"
	_, ok := fake.data[wantPath]
	assert.True(t, ok, "expected key %q, have %v", wantPath, fake.data)
}

func TestVaultStore_GetKey(t *testing.T) {
	t.Run("with prefix", func(t *testing.T) {
		fake := newFakeVaultKV()
		fake.data["atmos/api/token"] = map[string]any{vaultValueKey: "tok"}
		s := &VaultStore{client: fake, mount: "secret", prefix: "atmos"}

		got, err := s.GetKey("api/token")
		require.NoError(t, err)
		assert.Equal(t, "tok", got)
	})

	t.Run("empty key rejected", func(t *testing.T) {
		s := newTestVaultStore(newFakeVaultKV())
		_, err := s.GetKey("")
		assert.ErrorIs(t, err, store.ErrEmptyKey)
	})

	t.Run("missing path reports empty data", func(t *testing.T) {
		s := newTestVaultStore(newFakeVaultKV())
		_, err := s.GetKey("nope")
		assert.ErrorIs(t, err, store.ErrVaultEmptyData)
	})
}

func TestVaultStore_GetByPath_FallbackWholeMap(t *testing.T) {
	// A secret not written by Atmos has no "value" field; the whole data map is returned.
	fake := newFakeVaultKV()
	fake.data["external"] = map[string]any{"username": "admin", "password": "p"}
	s := &VaultStore{client: fake, mount: "secret"}

	got, err := s.GetKey("external")
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"username": "admin", "password": "p"}, got)
}

func TestVaultStore_Set_WriteErrorWrapped(t *testing.T) {
	fake := newFakeVaultKV()
	fake.putErr = errors.New("permission denied")
	s := newTestVaultStore(fake)

	err := s.Set("prod", "api", "k", "v")
	assert.ErrorIs(t, err, store.ErrVaultWrite)
}

func TestVaultStore_Get_ReadErrorWrapped(t *testing.T) {
	fake := newFakeVaultKV()
	fake.getErr = errors.New("connection refused")
	s := newTestVaultStore(fake)

	_, err := s.Get("prod", "api", "k")
	assert.ErrorIs(t, err, store.ErrVaultRead)
}

func TestVaultStore_Delete_ErrorWrapped(t *testing.T) {
	fake := newFakeVaultKV()
	fake.delErr = errors.New("permission denied")
	s := newTestVaultStore(fake)

	err := s.Delete("prod", "api", "k")
	assert.ErrorIs(t, err, store.ErrVaultDelete)
}

func TestVaultStore_Has_EmptyDataMapsToFalse(t *testing.T) {
	// The common "not found" path: Get returns ErrVaultEmptyData for a missing path, which Has
	// treats as absence.
	s := newTestVaultStore(newFakeVaultKV())

	has, err := s.Has("prod", "api", "missing")
	require.NoError(t, err)
	assert.False(t, has)
}

func TestVaultStore_Has_TransportErrorPropagates(t *testing.T) {
	fake := newFakeVaultKV()
	fake.getErr = errors.New("connection refused")
	s := newTestVaultStore(fake)

	_, err := s.Has("prod", "api", "k")
	require.Error(t, err)
	assert.ErrorIs(t, err, store.ErrVaultRead)
}

func TestVaultStore_SetAuthContext(t *testing.T) {
	s := newTestVaultStore(newFakeVaultKV())
	s.SetAuthContext(nil, "aws/admin")
	assert.Equal(t, "aws/admin", s.identityName)
	// Empty identity name leaves the existing one untouched.
	s.SetAuthContext(nil, "")
	assert.Equal(t, "aws/admin", s.identityName)
}

func TestIsVaultNotFound(t *testing.T) {
	assert.True(t, isVaultNotFound(&vault.ResponseError{StatusCode: vaultHTTPNotFound}))
	assert.False(t, isVaultNotFound(&vault.ResponseError{StatusCode: 500}))
	assert.False(t, isVaultNotFound(errors.New("plain error")))
}

func TestVaultStore_GetKey_StackDelimiterNotSet(t *testing.T) {
	s := &VaultStore{client: newFakeVaultKV(), mount: "secret", stackDelimiter: nil}
	_, err := s.Get("prod", "api", "k")
	assert.ErrorIs(t, err, store.ErrGetKey)
}

func TestFirstNonEmpty(t *testing.T) {
	assert.Equal(t, "b", firstNonEmpty("", "b", "c"))
	assert.Equal(t, "", firstNonEmpty("", ""))
	assert.Equal(t, "a", firstNonEmpty("a"))
}
