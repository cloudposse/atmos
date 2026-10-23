package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPluginRegistryIdentityLifecycle(t *testing.T) {
	config := pluginCoverageProject(t)
	t.Setenv("ATMOS_TEST_REGISTRY_PASSWORD", "fixture-registry-secret")
	config.Auth.Identities = map[string]schema.Identity{
		"registry-reader": {Kind: "oci/user", Spec: map[string]any{
			"registry": "registry.example.com", "username": "robot", "password_env": "ATMOS_TEST_REGISTRY_PASSWORD",
		}},
	}
	writePluginCoverageConfig(t, config)
	_, err := invokePluginCoverage(t, config, "registry", "add", "corporate", "--host=registry.example.com", "--identity=registry-reader")
	require.NoError(t, err)
	reloadPluginCoverageConfig(t, config)
	require.Equal(t, "registry-reader", config.Plugins.Registries["corporate"].Identity)
	require.Empty(t, config.Plugins.Registries["corporate"].Auth)

	_, err = invokePluginCoverage(t, config, "registry", "login", "corporate")
	require.NoError(t, err)
	manager, err := auth.NewDefaultManager(&config.Auth, config.CliConfigPath)
	require.NoError(t, err)
	cached, err := manager.GetCachedCredentials(t.Context(), "registry-reader")
	require.NoError(t, err)
	require.Equal(t, "registry-reader", cached.Identity)

	// A host spelling shares the configured alias binding and cannot override it with a password.
	_, err = invokePluginCoverage(t, config, "registry", "login", "registry.example.com", "--username=other", "--password-stdin")
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	_, err = invokePluginCoverage(t, config, "registry", "logout", "corporate")
	require.NoError(t, err)
	_, err = manager.GetCachedCredentials(t.Context(), "registry-reader")
	require.Error(t, err, "convenience logout must remove the ordinary auth session")

	raw, err := os.ReadFile(filepath.Join(config.BasePath, "atmos.yaml"))
	require.NoError(t, err)
	require.NotContains(t, string(raw), "fixture-registry-secret")
}

func TestPluginRegistryRejectsUnknownIdentityWithoutEditing(t *testing.T) {
	config := pluginCoverageProject(t)
	path := filepath.Join(config.BasePath, "atmos.yaml")
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	_, err = invokePluginCoverage(t, config, "registry", "add", "corporate", "oci://registry.example.com/catalog:stable", "--identity=missing")
	require.ErrorIs(t, err, errUtils.ErrInvalidIdentityConfig)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestPluginRegistryRejectsAmbiguousLoginBinding(t *testing.T) {
	config := pluginCoverageProject(t)
	config.Auth.Identities = map[string]schema.Identity{"first": {Kind: "ambient"}, "second": {Kind: "ambient"}}
	config.Plugins.Registries = map[string]schema.PluginRegistry{
		"one": {Catalog: "oci://registry.example.com/one:stable", Identity: "first"},
		"two": {Catalog: "oci://registry.example.com/two:stable", Identity: "second"},
	}
	for _, operation := range []string{"login", "logout"} {
		_, err := invokePluginCoverage(t, config, "registry", operation, "one")
		require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
		require.ErrorContains(t, err, "conflicting identities")
	}
}

func TestPluginRegistryRejectsConflictingAddWithoutEditing(t *testing.T) {
	config := pluginCoverageProject(t)
	config.Auth.Identities = map[string]schema.Identity{"first": {Kind: "ambient"}, "second": {Kind: "ambient"}}
	config.Plugins.Registries = map[string]schema.PluginRegistry{
		"one": {Catalog: "oci://registry.example.com/one:stable", Identity: "first"},
	}
	writePluginCoverageConfig(t, config)
	path := filepath.Join(config.BasePath, "atmos.yaml")
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	_, err = invokePluginCoverage(t, config, "registry", "add", "two", "oci://registry.example.com/two:stable", "--identity=second")
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	require.ErrorContains(t, err, "conflicting identities")
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.Len(t, config.Plugins.Registries, 1, "validation must not mutate in-memory configuration")
}

func TestPluginRegistryDiscoveryConfiguration(t *testing.T) {
	for _, tt := range []struct {
		name     string
		args     []string
		expected schema.PluginRegistry
	}{
		{"host only", []string{"--host=ghcr.io"}, schema.PluginRegistry{Host: "ghcr.io", Auth: "docker"}},
		{"repositories", []string{"--repository=oci://ghcr.io/acme/one", "--repository=oci://ghcr.io/acme/two"}, schema.PluginRegistry{Repositories: []string{"oci://ghcr.io/acme/one", "oci://ghcr.io/acme/two"}, Auth: "docker"}},
		{"both", []string{"--host=ghcr.io", "--catalog=oci://ghcr.io/acme/catalog:stable", "--repository=oci://ghcr.io/acme/one"}, schema.PluginRegistry{Host: "ghcr.io", Catalog: "oci://ghcr.io/acme/catalog:stable", Repositories: []string{"oci://ghcr.io/acme/one"}, Auth: "docker"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			config := pluginCoverageProject(t)
			_, err := invokePluginCoverage(t, config, append([]string{"registry", "add", "private"}, tt.args...)...)
			require.NoError(t, err)
			reloadPluginCoverageConfig(t, config)
			require.Equal(t, tt.expected, config.Plugins.Registries["private"])
			result, err := invokePluginCoverage(t, config, "registry", "list", "--format=json")
			require.NoError(t, err)
			require.Contains(t, result, "private")
		})
	}
}

func TestPluginRegistryInvalidDiscoveryDoesNotEditProject(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"oci://ghcr.io/acme/catalog:stable", "--catalog=oci://ghcr.io/acme/other:stable"},
		{"--host=ghcr.io", "--repository=oci://other.example.com/acme/plugin"},
		{"--repository=oci://ghcr.io/acme/plugin:v1"},
	} {
		t.Run("invalid configuration", func(t *testing.T) {
			config := pluginCoverageProject(t)
			path := filepath.Join(config.BasePath, "atmos.yaml")
			before, err := os.ReadFile(path)
			require.NoError(t, err)
			_, err = invokePluginCoverage(t, config, append([]string{"registry", "add", "private"}, args...)...)
			require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}
