package providers

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
	"github.com/cloudposse/atmos/pkg/store/authbridge"
)

func TestVaultStoreConfiguredIdentityOverridesStaticAndAmbientTokens(t *testing.T) {
	t.Setenv("VAULT_TOKEN", "ambient-token")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "identity-token", r.Header.Get("X-Vault-Token"))
		writeVaultJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"data":     map[string]any{vaultValueKey: "secret-value"},
				"metadata": map[string]any{"version": 1},
			},
		})
	}))
	t.Cleanup(server.Close)

	raw, err := NewVaultStore(&VaultStoreOptions{Address: server.URL, Mount: "secret", Token: "static-token"}, "openbao")
	require.NoError(t, err)
	vaultStore := raw.(*VaultStore)
	vaultStore.SetAuthContext(authbridge.NewResolvedContext(&schema.AuthContext{
		Environment: map[string]string{"VAULT_TOKEN": "identity-token"},
	}), "")

	value, err := vaultStore.GetKey("service/account")
	require.NoError(t, err)
	assert.Equal(t, "secret-value", value)
}

func TestVaultStoreConfiguredIdentityFailsClosed(t *testing.T) {
	for _, test := range []struct {
		name     string
		resolver store.AuthContextResolver
		want     error
	}{
		{name: "missing resolver", want: store.ErrIdentityNotConfigured},
		{name: "missing environment context", resolver: authbridge.NewResolvedContext(&schema.AuthContext{}), want: store.ErrAuthContextNotAvailable},
		{name: "missing token", resolver: authbridge.NewResolvedContext(&schema.AuthContext{Environment: map[string]string{"OTHER_TOKEN": "unrelated"}}), want: store.ErrAuthContextNotAvailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("VAULT_TOKEN", "must-not-be-used")
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			t.Cleanup(server.Close)

			raw, err := NewVaultStore(&VaultStoreOptions{Address: server.URL, Mount: "secret", Token: "must-not-be-used"}, "openbao")
			require.NoError(t, err)
			vaultStore := raw.(*VaultStore)
			if test.resolver != nil {
				vaultStore.SetAuthContext(test.resolver, "")
			}

			_, err = vaultStore.GetKey("service/account")
			require.ErrorIs(t, err, test.want)
			assert.Zero(t, requests.Load(), "identity failure must stop before contacting Vault")
		})
	}

	t.Run("nil environment result", func(t *testing.T) {
		var requests atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			requests.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(server.Close)
		resolver := store.NewMockEnvironmentAuthContextResolver(gomock.NewController(t))
		resolver.EXPECT().ResolveEnvironmentAuthContext(gomock.Any(), "openbao").Return(nil, nil)
		raw, err := NewVaultStore(&VaultStoreOptions{Address: server.URL, Mount: "secret", Token: "must-not-be-used"}, "openbao")
		require.NoError(t, err)
		vaultStore := raw.(*VaultStore)
		vaultStore.SetAuthContext(resolver, "")

		_, err = vaultStore.GetKey("service/account")
		require.ErrorIs(t, err, store.ErrAuthContextNotAvailable)
		assert.Zero(t, requests.Load(), "identity failure must stop before contacting Vault")
	})
}

func TestVaultStoreResetAuthContextRestoresConfiguredToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "static-token", r.Header.Get("X-Vault-Token"))
		writeVaultJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"data":     map[string]any{vaultValueKey: "secret-value"},
				"metadata": map[string]any{"version": 1},
			},
		})
	}))
	t.Cleanup(server.Close)

	raw, err := NewVaultStore(&VaultStoreOptions{Address: server.URL, Mount: "secret", Token: "static-token"}, "openbao")
	require.NoError(t, err)
	vaultStore := raw.(*VaultStore)
	vaultStore.SetAuthContext(authbridge.NewResolvedContext(&schema.AuthContext{
		Environment: map[string]string{"VAULT_TOKEN": "identity-token"},
	}), "")
	vaultStore.ResetAuthContext()

	value, err := vaultStore.GetKey("service/account")
	require.NoError(t, err)
	assert.Equal(t, "secret-value", value)
}

func TestVaultStoreSelectsExactDocumentField(t *testing.T) {
	fake := newFakeVaultKV()
	fake.data["service/account"] = map[string]any{
		vaultValueKey: "legacy-value",
		"api_token":   "selected-value",
	}
	fake.data["/prod/app/account"] = map[string]any{
		vaultValueKey:   "scoped-legacy-value",
		"client_secret": "scoped-value",
	}
	vaultStore := newTestVaultStore(fake)

	value, err := vaultStore.GetKeyField("service/account", "api_token")
	require.NoError(t, err)
	assert.Equal(t, "selected-value", value)
	value, err = vaultStore.GetField("prod", "app", "account", "client_secret")
	require.NoError(t, err)
	assert.Equal(t, "scoped-value", value)

	_, err = vaultStore.GetKeyField("service/account", "missing")
	require.ErrorIs(t, err, store.ErrFieldNotFound)
	_, err = vaultStore.GetKeyField("service/account", "")
	require.ErrorIs(t, err, store.ErrEmptyField)
}
