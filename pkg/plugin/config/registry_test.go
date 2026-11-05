package config

import (
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

var (
	_ = schema.PluginRegistry{Host: "", Catalog: "", Repositories: []string{}}
	_ = schema.PluginsConfig{Discovery: schema.PluginDiscoveryConfig{CacheTTL: ""}}
)

func TestRegistryHost(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		registry schema.PluginRegistry
		host     string
	}{
		{"host only", schema.PluginRegistry{Host: "registry.example.com:5000"}, "registry.example.com:5000"},
		{"normalize", schema.PluginRegistry{Host: "GHCR.IO"}, "ghcr.io"},
		{"catalog", schema.PluginRegistry{Catalog: "oci://ghcr.io/acme/catalog:stable"}, "ghcr.io"},
		{"repositories", schema.PluginRegistry{Repositories: []string{"oci://ghcr.io/acme/one", "oci://ghcr.io/acme/two"}}, "ghcr.io"},
		{"combined", schema.PluginRegistry{Host: "GHCR.IO", Catalog: "oci://ghcr.io/acme/catalog:stable", Repositories: []string{"oci://ghcr.io/acme/one"}}, "ghcr.io"},
		{"empty", schema.PluginRegistry{}, ""},
		{"bad host", schema.PluginRegistry{Host: "https://ghcr.io"}, ""},
		{"host mismatch", schema.PluginRegistry{Host: "ghcr.io", Catalog: "oci://other.example.com/catalog:stable"}, ""},
		{"repo mismatch", schema.PluginRegistry{Repositories: []string{"oci://ghcr.io/acme/one", "oci://other.example.com/acme/two"}}, ""},
		{"tagged repo", schema.PluginRegistry{Repositories: []string{"oci://ghcr.io/acme/one:v1"}}, ""},
		{"digest repo", schema.PluginRegistry{Repositories: []string{"oci://ghcr.io/acme/one@sha256:abcd"}}, ""},
		{"missing scheme", schema.PluginRegistry{Repositories: []string{"ghcr.io/acme/one"}}, ""},
		{"empty repo", schema.PluginRegistry{Repositories: []string{""}}, ""},
		{"missing repository", schema.PluginRegistry{Repositories: []string{"oci://ghcr.io"}}, ""},
		{"implicit host", schema.PluginRegistry{Repositories: []string{"oci://acme/one"}}, ""},
		{"repository matching tagged catalog", schema.PluginRegistry{Catalog: "oci://ghcr.io/acme/catalog:stable", Repositories: []string{"oci://ghcr.io/acme/catalog:stable"}}, ""},
		{"invalid catalog", schema.PluginRegistry{Catalog: "oci://ghcr.io/bad catalog"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			host, err := RegistryHost(&tt.registry)
			if tt.host == "" {
				require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.host, host)
		})
	}
}

func TestRegistrySourcesPreservesInputs(t *testing.T) {
	t.Parallel()
	require.Nil(t, RegistrySources(nil))
	_, err := RegistryHost(nil)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	registry := schema.PluginRegistry{Catalog: "oci://ghcr.io/acme/catalog:stable", Repositories: []string{"oci://ghcr.io/acme/one", "oci://ghcr.io/acme/two"}}
	sources := RegistrySources(&registry)
	require.Equal(t, []string{registry.Catalog, registry.Repositories[0], registry.Repositories[1]}, sources)
	sources[1] = "changed"
	require.Equal(t, "oci://ghcr.io/acme/one", registry.Repositories[0])
	registry.Repositories[1] = "changed"
	require.Equal(t, "oci://ghcr.io/acme/two", sources[2])
}

func TestDiscoveryCacheTTLValidation(t *testing.T) {
	t.Parallel()
	for _, ttl := range []string{"", "0", "0s", "15m", "1h30m"} {
		require.NoError(t, Validate(&schema.PluginsConfig{Discovery: schema.PluginDiscoveryConfig{CacheTTL: ttl}}))
	}
	for _, ttl := range []string{"-1s", "forever", "15"} {
		require.ErrorIs(t, Validate(&schema.PluginsConfig{Discovery: schema.PluginDiscoveryConfig{CacheTTL: ttl}}), errUtils.ErrPluginInvalidConfig)
	}
	require.ErrorIs(t, Validate(&schema.PluginsConfig{Registries: map[string]schema.PluginRegistry{"empty": {}}}), errUtils.ErrPluginInvalidConfig)
}
