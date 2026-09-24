package providers

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/store"
)

// TestVaultStore_SecretScopes verifies the same exact path is used for all operations.
// Empty coordinates are intentional for stack-scoped and globally shared secrets.
func TestVaultStore_SecretScopes(t *testing.T) {
	scopes := []struct {
		name, stack, component, path string
	}{
		{"instance", "plat-prod", "app/api", "/plat/prod/app/api/TOKEN"},
		{"stack", "plat-prod", "", "/plat/prod/TOKEN"},
		{"global", "", "", "/TOKEN"},
	}
	for _, scope := range scopes {
		for _, prefix := range []string{"", "atmos"} {
			t.Run(scope.name+"/prefix="+prefix, func(t *testing.T) {
				path := prefix + scope.path
				value := map[string]any{vaultValueKey: "synthetic-secret"}
				delim := "-"
				fake := newFakeVaultKV()
				s := newTestVaultStore(fake)
				s.prefix, s.stackDelimiter = prefix, &delim

				t.Run("get", func(t *testing.T) {
					fake.data[path] = value
					got, err := s.Get(scope.stack, scope.component, "TOKEN")
					require.NoError(t, err)
					assert.Equal(t, "synthetic-secret", got)
				})
				t.Run("has uses metadata", func(t *testing.T) {
					fake.data[path] = value
					reads := fake.getCalls
					has, err := s.Has(scope.stack, scope.component, "TOKEN")
					require.NoError(t, err)
					assert.True(t, has)
					assert.Equal(t, reads, fake.getCalls)
					assert.Equal(t, 1, fake.metaCalls)
				})
				t.Run("set exact path", func(t *testing.T) {
					fake.data = map[string]map[string]any{}
					require.NoError(t, s.Set(scope.stack, scope.component, "TOKEN", "synthetic-secret"))
					assert.Equal(t, map[string]map[string]any{path: value}, fake.data)
				})
				t.Run("delete only intended path", func(t *testing.T) {
					neighbor := path + "-other"
					fake.data = map[string]map[string]any{path: value, neighbor: value}
					require.NoError(t, s.Delete(scope.stack, scope.component, "TOKEN"))
					assert.Equal(t, map[string]map[string]any{neighbor: value}, fake.data)
					has, err := s.Has(scope.stack, scope.component, "TOKEN")
					require.NoError(t, err)
					assert.False(t, has)
				})
			})
		}
	}
}

func TestVaultStore_GlobalStructuredSecret(t *testing.T) {
	fake := newFakeVaultKV()
	value := map[string]any{"username": "example", "password": "synthetic-password"}
	fake.data["shared/account"] = value
	s := newTestVaultStore(fake)
	s.prefix = "shared"

	got, err := s.Get("", "", "account")
	require.NoError(t, err)
	assert.Equal(t, value, got, "existing external secrets retain their structured fields")
}

// TestVaultStore_GlobalHTTPPath verifies an existing shared KV path through the real SDK.
func TestVaultStore_GlobalHTTPPath(t *testing.T) {
	var requests atomic.Int32
	value := map[string]any{"username": "example", "password": "synthetic-password"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v1/secret/data/shared-account", r.URL.Path)
		writeVaultJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{"data": value, "metadata": map[string]any{"version": 1}},
		})
	}))
	t.Cleanup(srv.Close)
	t.Setenv("VAULT_ADDR", "")
	t.Setenv("VAULT_TOKEN", "")
	s, err := NewVaultStore(&VaultStoreOptions{Address: srv.URL, Mount: "secret", Token: "synthetic-token"}, "")
	require.NoError(t, err)

	got, err := s.Get("", "", "shared-account")
	require.NoError(t, err)
	assert.Equal(t, value, got)
	assert.EqualValues(t, 1, requests.Load())
}

func TestVaultStore_ScopedValidation(t *testing.T) {
	for _, coord := range []struct{ stack, component string }{{"prod", "api"}, {"prod", ""}, {"", ""}} {
		t.Run(coord.stack+"/"+coord.component, func(t *testing.T) {
			// A nil client ensures invalid requests cannot contact Vault.
			s := newTestVaultStore(nil)
			assert.ErrorIs(t, s.Set(coord.stack, coord.component, "", "value"), store.ErrEmptyKey)
			assert.ErrorIs(t, s.Set(coord.stack, coord.component, "TOKEN", nil), store.ErrNilValue)
			_, err := s.Get(coord.stack, coord.component, "")
			assert.ErrorIs(t, err, store.ErrEmptyKey)
			assert.ErrorIs(t, s.Delete(coord.stack, coord.component, ""), store.ErrEmptyKey)
			_, err = s.Has(coord.stack, coord.component, "")
			assert.ErrorIs(t, err, store.ErrEmptyKey)
		})
	}
}

func TestVaultStore_GlobalBackendErrors(t *testing.T) {
	fake := newFakeVaultKV()
	s := newTestVaultStore(fake)

	_, err := s.Get("", "", "missing")
	assert.ErrorIs(t, err, store.ErrVaultEmptyData)
	fake.getErr = assert.AnError
	_, err = s.Get("", "", "TOKEN")
	assert.ErrorIs(t, err, store.ErrVaultRead)
	assert.ErrorIs(t, err, assert.AnError)

	fake.putErr = assert.AnError
	err = s.Set("", "", "TOKEN", "synthetic-secret")
	assert.ErrorIs(t, err, store.ErrVaultWrite)

	fake.delErr = assert.AnError
	err = s.Delete("", "", "TOKEN")
	assert.ErrorIs(t, err, store.ErrVaultDelete)

	fake.metaErr = assert.AnError
	_, err = s.Has("", "", "TOKEN")
	assert.ErrorIs(t, err, store.ErrVaultRead)
	assert.ErrorIs(t, err, assert.AnError)
}
