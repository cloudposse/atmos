package config

import (
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

var _ = schema.PluginsConfig{Policy: schema.PluginPolicy{}}

func TestValidateReservedConfiguration(t *testing.T) {
	t.Parallel()
	cases := map[string]schema.PluginsConfig{
		"negative concurrency": {MaxConcurrency: -1},
		"catalog TTL":          {Policy: schema.PluginPolicy{CacheTTL: "1h"}},
		"user scope":           {Packages: map[string]schema.PluginPackage{"corporate": {Scope: "user"}}},
		"custom auth":          {Registries: map[string]schema.PluginRegistry{"private": {Host: "ghcr.io", Auth: "custom-helper"}}},
	}
	for name, config := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, Validate(&config), errUtils.ErrPluginInvalidConfig)
		})
	}
	require.NoError(t, Validate(&schema.PluginsConfig{Packages: map[string]schema.PluginPackage{"corporate": {Scope: "project", Source: "oci://r/a", Version: "^1"}}, Registries: map[string]schema.PluginRegistry{"private": {Host: "ghcr.io", Auth: "docker"}}}))
}

func TestValidatePluginConcurrency(t *testing.T) {
	t.Parallel()
	for _, limit := range []int{0, 1, 4, 16} {
		require.NoError(t, Validate(&schema.PluginsConfig{MaxConcurrency: limit}))
	}
}
