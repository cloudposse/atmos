package bootstrap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/auth"
	authcredentials "github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	cfg "github.com/cloudposse/atmos/pkg/config"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/plugin/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
	"github.com/cloudposse/atmos/pkg/store/authbridge"
	"github.com/cloudposse/atmos/pkg/store/providers"
)

func TestNativeEnvironmentIdentityAuthenticatesVault(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and executes the auth-secrets native plugin")
	}
	packageDir := buildSecretPlugin(t)
	for _, test := range []struct {
		name     string
		env      map[string]any
		wantRead bool
	}{
		{"selected credential", map[string]any{"VAULT_TOKEN": `{{ index .credentials "api-token" }}`}, true},
		{"missing credential", map[string]any{"VAULT_TOKEN": `{{ index .credentials "missing" }}`}, false},
		{"missing vault token", map[string]any{"OTHER_TOKEN": `{{ index .credentials "api-token" }}`}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("ATMOS_KEYRING_TYPE", "memory")
			t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
			t.Setenv("VAULT_TOKEN", "ambient-must-not-be-used")
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				assert.Equal(t, "/v1/secret/data/service/config", r.URL.Path)
				assert.Equal(t, "native-identity-private-token", r.Header.Get("X-Vault-Token"))
				w.Header().Set("Content-Type", "application/json")
				assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
					"data": map[string]any{"access_token": "selected-document-field"}, "metadata": map[string]any{"version": 1},
				}}))
			}))
			t.Cleanup(server.Close)
			bootstrapStore := store.NewMockFieldStore(gomock.NewController(t))
			bootstrapStore.EXPECT().GetKeyField("bootstrap/session", "token").Return("native-identity-private-token", nil)
			providerConfig := schema.Provider{Kind: "secrets/store", Spec: map[string]any{"credentials": map[string]any{
				"api-token": map[string]any{"store": "bootstrap", "path": "bootstrap/session", "field": "token"},
			}}}
			identityConfig := schema.Identity{Kind: "secrets/env", Via: &schema.IdentityVia{Provider: "session-credentials"}, Spec: map[string]any{"env": test.env}}
			atmos := loadVaultEnvironmentConfig(t, &providerConfig, &identityConfig)
			atmos.Stores = store.StoreRegistry{"bootstrap": bootstrapStore}
			atmos.StoresConfig = store.StoresConfig{"bootstrap": {Secret: true}}
			cleanup, err := registerSelected(t.Context(), atmos, &config.Selections{Packages: map[string]config.Selection{"secrets": {Path: packageDir, Local: true}}})
			require.NoError(t, err)
			t.Cleanup(cleanup)
			manager, err := auth.NewAuthManager(&atmos.Auth, authcredentials.NewCredentialStoreWithConfig(&atmos.Auth), validation.NewValidator(), nil, "")
			require.NoError(t, err)
			vault, err := providers.NewVaultStore(&providers.VaultStoreOptions{Address: server.URL, Mount: "secret", Token: "static-must-not-be-used"}, "openbao-session")
			require.NoError(t, err)
			vault.(store.IdentityAwareStore).SetAuthContext(authbridge.NewResolver(manager, nil), "openbao-session")

			value, err := vault.(store.FieldStore).GetKeyField("service/config", "access_token")
			if test.wantRead {
				require.NoError(t, err)
				require.Equal(t, "selected-document-field", value)
				require.Equal(t, int32(1), requests.Load())
			} else {
				require.Error(t, err)
				require.NotContains(t, err.Error(), "native-identity-private-token")
				require.Zero(t, requests.Load(), "failed identity authentication must not reach Vault")
			}
			require.Equal(t, "ambient-must-not-be-used", os.Getenv("VAULT_TOKEN"))
			require.NotContains(t, atmosio.MaskString("value=native-identity-private-token"), "native-identity-private-token")
		})
	}
}

func loadVaultEnvironmentConfig(t *testing.T, provider *schema.Provider, identity *schema.Identity) *schema.AtmosConfiguration {
	t.Helper()
	project := t.TempDir()
	t.Chdir(project)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", project)
	authConfig := schema.AuthConfig{
		Providers:  map[string]schema.Provider{"session-credentials": *provider},
		Identities: map[string]schema.Identity{"openbao-session": *identity},
	}
	content, err := yaml.Marshal(map[string]any{"base_path": project, "auth": authConfig})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(project, "atmos.yaml"), content, 0o600))
	loaded, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)
	require.Equal(t, identity.Spec, loaded.Auth.Identities["openbao-session"].Spec)
	return &loaded
}
