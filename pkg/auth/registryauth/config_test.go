package registryauth

import (
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPluginBindingsRejectsAmbiguousOrUnknownIdentities(t *testing.T) {
	config := &schema.AtmosConfiguration{Auth: schema.AuthConfig{Identities: map[string]schema.Identity{"reader": {}, "writer": {}}}, Plugins: schema.PluginsConfig{Registries: map[string]schema.PluginRegistry{
		"corporate":     {Catalog: "oci://REGISTRY.EXAMPLE.COM:5000/catalog:stable", Identity: "reader"},
		"ambient-alias": {Catalog: "oci://registry.example.com:5000/other:stable"},
		"public":        {Catalog: "oci://public.example.com/catalog:stable"},
	}}}
	bindings, err := PluginBindings(config)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"registry.example.com:5000": "reader"}, bindings)
	config.Plugins.Registries["conflict"] = schema.PluginRegistry{Catalog: "oci://registry.example.com:5000/writer:stable", Identity: "writer"}
	_, err = PluginBindings(config)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	delete(config.Plugins.Registries, "conflict")
	config.Plugins.Registries["unknown"] = schema.PluginRegistry{Catalog: "oci://unknown.example.com/catalog:stable", Identity: "missing"}
	_, err = PluginBindings(config)
	require.ErrorIs(t, err, errUtils.ErrIdentityNotFound)
	delete(config.Plugins.Registries, "unknown")
	config.Plugins.Registries["bad"] = schema.PluginRegistry{Catalog: "invalid://[catalog", Identity: "reader"}
	_, err = PluginBindings(config)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
}

func TestScopedConfigRetainsOnlySelectedIdentityChain(t *testing.T) {
	config := &schema.AuthConfig{Realm: "corporate", Identities: map[string]schema.Identity{
		"reader":      {Kind: "aws/assume-role", Via: &schema.IdentityVia{Identity: "base"}},
		"base":        {Kind: "aws/permission-set", Via: &schema.IdentityVia{Provider: "sso"}},
		"unavailable": {Kind: "not-installed/plugin"},
	}, Providers: map[string]schema.Provider{"sso": {Kind: "aws/iam-identity-center"}, "unavailable": {Kind: "not-installed/plugin"}}, Integrations: map[string]schema.Integration{
		"matching":  {Kind: "aws/ecr", Via: &schema.IntegrationVia{Identity: "reader"}},
		"unrelated": {Kind: "aws/ecr", Via: &schema.IntegrationVia{Identity: "unavailable"}},
	}}
	scoped, err := scopedConfig(config, "reader")
	require.NoError(t, err)
	require.Equal(t, "corporate", scoped.Realm)
	require.Equal(t, map[string]schema.Identity{"reader": config.Identities["reader"], "base": config.Identities["base"]}, scoped.Identities)
	require.Equal(t, map[string]schema.Provider{"sso": config.Providers["sso"]}, scoped.Providers)
	require.Equal(t, map[string]schema.Integration{"matching": config.Integrations["matching"]}, scoped.Integrations)
	require.Contains(t, config.Identities, "unavailable")
	_, err = scopedConfig(config, "missing")
	require.ErrorIs(t, err, errUtils.ErrIdentityNotFound)
	config.Identities["base"] = schema.Identity{Via: &schema.IdentityVia{Identity: "reader"}}
	_, err = scopedConfig(config, "reader")
	require.ErrorContains(t, err, "cycle")
	config.Identities["base"] = schema.Identity{Via: &schema.IdentityVia{Provider: "missing"}}
	_, err = scopedConfig(config, "reader")
	require.ErrorIs(t, err, errUtils.ErrProviderNotFound)
}

func TestMatchingIntegrationRequiresUniqueExactHost(t *testing.T) {
	config := &schema.AuthConfig{Integrations: map[string]schema.Integration{
		"ecr":       {Kind: "aws/ecr", Spec: &schema.IntegrationSpec{Registry: &schema.Registry{AccountID: "123456789012", Region: "us-east-1"}}},
		"acr":       {Kind: "azure/acr", Spec: &schema.IntegrationSpec{Registry: &schema.Registry{Name: "corporate"}}},
		"public":    {Kind: "aws/ecr-public"},
		"unrelated": {Kind: "aws/eks"},
	}}
	for host, want := range map[string]string{"123456789012.dkr.ecr.us-east-1.amazonaws.com": "ecr", "corporate.azurecr.io": "acr", "public.ecr.aws": "public", "other.example.com": ""} {
		actual, err := matchingIntegration(config, host)
		require.NoError(t, err)
		require.Equal(t, want, actual)
	}
	config.Integrations["second"] = config.Integrations["acr"]
	_, err := matchingIntegration(config, "corporate.azurecr.io")
	require.ErrorIs(t, err, errUtils.ErrIntegrationFailed)
}

func TestPluginBindingsWithoutCatalog(t *testing.T) {
	t.Parallel()
	for _, registry := range []schema.PluginRegistry{
		{Host: "REGISTRY.EXAMPLE.COM:5000", Identity: "reader"},
		{Repositories: []string{"oci://registry.example.com:5000/plugins/one", "oci://registry.example.com:5000/plugins/two"}, Identity: "reader"},
	} {
		config := &schema.AtmosConfiguration{
			Auth:    schema.AuthConfig{Identities: map[string]schema.Identity{"reader": {}}},
			Plugins: schema.PluginsConfig{Registries: map[string]schema.PluginRegistry{"private": registry}},
		}
		identity, err := Identity(config, "REGISTRY.EXAMPLE.COM:5000")
		require.NoError(t, err)
		require.Equal(t, "reader", identity)
		identity, err = Identity(config, "unrelated.example.com")
		require.NoError(t, err)
		require.Empty(t, identity)
	}
}
