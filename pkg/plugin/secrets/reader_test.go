package secrets

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

var _ = store.StoreConfig{Secret: true, Identity: ""}

func secretConfig(backend store.Store) *schema.AtmosConfiguration {
	return &schema.AtmosConfiguration{
		Auth:   schema.AuthConfig{Providers: map[string]schema.Provider{"vault": {Kind: "secrets/store", Spec: map[string]any{"credentials": map[string]any{"token": map[string]any{"store": "vault", "path": "private/path"}}}}}, Identities: map[string]schema.Identity{}},
		Stores: store.StoreRegistry{"vault": backend}, StoresConfig: store.StoresConfig{"vault": {Secret: true}},
	}
}

func TestFactoryGrantsOnlyConfiguredProvider(t *testing.T) {
	config := secretConfig(nil)
	factory := Factory(config)
	for _, tt := range []struct {
		name     string
		settings map[string]any
	}{
		{"missing", map[string]any{"kind": "secrets/store"}},
		{"vault", map[string]any{"kind": "other"}},
		{"vault", map[string]any{"kind": "secrets/store", "principal": ""}},
	} {
		require.Nil(t, factory(tt.name, tt.settings))
	}
	require.NotNil(t, factory("vault", map[string]any{"kind": "secrets/store"}))
	config.Auth.Providers["other"] = schema.Provider{Kind: "other"}
	require.Nil(t, factory("other", map[string]any{"kind": "other"}))
}

func TestReadSecretUsesOnlyGrantedReferences(t *testing.T) {
	for _, scoped := range []bool{false, true} {
		t.Run(map[bool]string{false: "raw key", true: "stack component"}[scoped], func(t *testing.T) {
			backend := store.NewMockStore(gomock.NewController(t))
			config := secretConfig(backend)
			if scoped {
				ref := config.Auth.Providers["vault"].Spec["credentials"].(map[string]any)["token"].(map[string]any)
				ref["stack"], ref["component"] = "dev", "app"
				backend.EXPECT().Get("dev", "app", "private/path").Return("private-value", nil)
			} else {
				backend.EXPECT().GetKey("private/path").Return("private-value", nil)
			}
			reader := Factory(config)("vault", map[string]any{"kind": "secrets/store"})
			value, err := reader.ReadSecret(t.Context(), "token")
			require.NoError(t, err)
			require.Equal(t, "private-value", value)
			_, err = reader.ReadSecret(t.Context(), "private/path")
			require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
		})
	}
}

func TestReadSecretSelectsExactConfiguredField(t *testing.T) {
	for _, scoped := range []bool{false, true} {
		t.Run(map[bool]string{false: "raw key", true: "stack component"}[scoped], func(t *testing.T) {
			backend := store.NewMockFieldStore(gomock.NewController(t))
			config := secretConfig(backend)
			ref := config.Auth.Providers["vault"].Spec["credentials"].(map[string]any)["token"].(map[string]any)
			ref["field"] = "api_token"
			if scoped {
				ref["stack"], ref["component"] = "dev", "app"
				backend.EXPECT().GetField("dev", "app", "private/path", "api_token").Return("private-value", nil)
			} else {
				backend.EXPECT().GetKeyField("private/path", "api_token").Return("private-value", nil)
			}

			reader := Factory(config)("vault", map[string]any{"kind": "secrets/store"})
			value, err := reader.ReadSecret(t.Context(), "token")
			require.NoError(t, err)
			require.Equal(t, "private-value", value)
		})
	}
}

func TestReadSecretRejectsInvalidBackendValuesAndRedactsErrors(t *testing.T) {
	for _, tt := range []struct {
		name             string
		value            any
		backendErr, want error
	}{
		{"error", nil, errors.New("private-backend-diagnostic"), errUtils.ErrAuthenticationFailed},
		{"non string", 42, nil, errUtils.ErrPluginProtocol},
		{"nul", "a\x00b", nil, errUtils.ErrPluginProtocol},
		{"large", strings.Repeat("x", (1<<20)+1), nil, errUtils.ErrPluginProtocol},
	} {
		t.Run(tt.name, func(t *testing.T) {
			backend := store.NewMockStore(gomock.NewController(t))
			backend.EXPECT().GetKey("private/path").Return(tt.value, tt.backendErr)
			reader := Factory(secretConfig(backend))("vault", map[string]any{"kind": "secrets/store"})
			value, err := reader.ReadSecret(t.Context(), "token")
			require.ErrorIs(t, err, tt.want)
			require.Empty(t, value)
			require.NotContains(t, err.Error(), "private-backend-diagnostic")
		})
	}
}

func TestReadSecretCancellation(t *testing.T) {
	backend := store.NewMockStore(gomock.NewController(t))
	reader := Factory(secretConfig(backend))("vault", map[string]any{"kind": "secrets/store"})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := reader.ReadSecret(ctx, "token")
	require.ErrorIs(t, err, context.Canceled)
	ctx, cancel = context.WithCancel(t.Context())
	defer cancel()
	backend.EXPECT().GetKey("private/path").DoAndReturn(func(string) (any, error) { cancel(); return "secret", nil })
	value, err := reader.ReadSecret(ctx, "token")
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, value)
}

func TestReadSecretInvalidConfigurationNeverReadsBackend(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*schema.AtmosConfiguration)
	}{
		{"missing backend", func(c *schema.AtmosConfiguration) { delete(c.Stores, "vault") }},
		{"empty path", func(c *schema.AtmosConfiguration) {
			c.Auth.Providers["vault"].Spec["credentials"].(map[string]any)["token"].(map[string]any)["path"] = ""
		}},
		{"non-string path", func(c *schema.AtmosConfiguration) {
			c.Auth.Providers["vault"].Spec["credentials"].(map[string]any)["token"].(map[string]any)["path"] = 42
		}},
		{"legacy key is not a path", func(c *schema.AtmosConfiguration) {
			ref := c.Auth.Providers["vault"].Spec["credentials"].(map[string]any)["token"].(map[string]any)
			delete(ref, "path")
			ref["key"] = "private/path"
		}},
		{"partial scope", func(c *schema.AtmosConfiguration) {
			c.Auth.Providers["vault"].Spec["credentials"].(map[string]any)["token"].(map[string]any)["stack"] = "dev"
		}},
		{"empty field", func(c *schema.AtmosConfiguration) {
			c.Auth.Providers["vault"].Spec["credentials"].(map[string]any)["token"].(map[string]any)["field"] = ""
		}},
		{"non-string field", func(c *schema.AtmosConfiguration) {
			c.Auth.Providers["vault"].Spec["credentials"].(map[string]any)["token"].(map[string]any)["field"] = 42
		}},
		{"field unsupported", func(c *schema.AtmosConfiguration) {
			c.Auth.Providers["vault"].Spec["credentials"].(map[string]any)["token"].(map[string]any)["field"] = "api_token"
		}},
		{"not secret", func(c *schema.AtmosConfiguration) { c.StoresConfig["vault"] = store.StoreConfig{} }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			backend := store.NewMockStore(gomock.NewController(t))
			config := secretConfig(backend)
			tt.change(config)
			reader := Factory(config)("vault", map[string]any{"kind": "secrets/store"})
			_, err := reader.ReadSecret(t.Context(), "token")
			require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
		})
	}
}

func TestSecretDependenciesRejectCyclesAndMissingNodes(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*schema.AtmosConfiguration)
		want   error
	}{
		{"ambient", func(*schema.AtmosConfiguration) {}, nil},
		{"missing provider", func(c *schema.AtmosConfiguration) { delete(c.Auth.Providers, "vault") }, errUtils.ErrInvalidAuthConfig},
		{"missing references", func(c *schema.AtmosConfiguration) { delete(c.Auth.Providers["vault"].Spec, "credentials") }, errUtils.ErrPluginInvalidConfig},
		{"invalid reference", func(c *schema.AtmosConfiguration) {
			c.Auth.Providers["vault"].Spec["credentials"].(map[string]any)["token"] = 1
		}, errUtils.ErrPluginInvalidConfig},
		{"missing store", func(c *schema.AtmosConfiguration) { delete(c.StoresConfig, "vault") }, errUtils.ErrPluginInvalidConfig},
		{"missing identity", func(c *schema.AtmosConfiguration) {
			c.StoresConfig["vault"] = store.StoreConfig{Secret: true, Identity: "reader"}
		}, errUtils.ErrInvalidAuthConfig},
		{"provider cycle", func(c *schema.AtmosConfiguration) {
			c.StoresConfig["vault"] = store.StoreConfig{Secret: true, Identity: "reader"}
			c.Auth.Identities["reader"] = schema.Identity{Provider: "vault"}
		}, errUtils.ErrInvalidAuthConfig},
		{"via provider cycle", func(c *schema.AtmosConfiguration) {
			c.StoresConfig["vault"] = store.StoreConfig{Secret: true, Identity: "reader"}
			c.Auth.Identities["reader"] = schema.Identity{Via: &schema.IdentityVia{Provider: "vault"}}
		}, errUtils.ErrInvalidAuthConfig},
		{"identity cycle", func(c *schema.AtmosConfiguration) {
			c.StoresConfig["vault"] = store.StoreConfig{Secret: true, Identity: "reader"}
			c.Auth.Identities["reader"] = schema.Identity{Via: &schema.IdentityVia{Identity: "reader"}}
		}, errUtils.ErrInvalidAuthConfig},
		{"independent provider", func(c *schema.AtmosConfiguration) {
			c.StoresConfig["vault"] = store.StoreConfig{Secret: true, Identity: "reader"}
			c.Auth.Identities["reader"] = schema.Identity{Via: &schema.IdentityVia{Identity: "root"}}
			c.Auth.Identities["root"] = schema.Identity{Provider: "independent"}
			c.Auth.Providers["independent"] = schema.Provider{Kind: "aws/user"}
		}, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			config := secretConfig(nil)
			tt.change(config)
			err := validateGraph(config, "provider:vault", make(map[string]bool))
			if tt.want != nil {
				require.ErrorIs(t, err, tt.want)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSharedSecretDependenciesAndDepthLimit(t *testing.T) {
	config := secretConfig(nil)
	refs := config.Auth.Providers["vault"].Spec["credentials"].(map[string]any)
	refs["second"] = map[string]any{"store": "vault", "path": "other/path"}
	config.StoresConfig["vault"] = store.StoreConfig{Secret: true, Identity: "leaf"}
	config.Auth.Identities["leaf"] = schema.Identity{}
	require.NoError(t, validateGraph(config, "provider:vault", make(map[string]bool)), "sharing a dependency must not be reported as a cycle")
	config.Auth.Identities["leaf"] = schema.Identity{Provider: "vault"}
	require.ErrorIs(t, validateGraph(config, "provider:vault", make(map[string]bool)), errUtils.ErrInvalidAuthConfig, "completed nodes from an earlier validation cannot hide a new cycle")

	for i := range maxDependencyDepth + 2 {
		name, next := fmt.Sprintf("identity-%d", i), fmt.Sprintf("identity-%d", i+1)
		config.Auth.Identities[name] = schema.Identity{Via: &schema.IdentityVia{Identity: next}}
	}
	require.ErrorIs(t, validateGraph(config, "identity:identity-0", make(map[string]bool)), errUtils.ErrInvalidAuthConfig)
}
