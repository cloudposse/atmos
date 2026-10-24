package discovery

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	pluginconfig "github.com/cloudposse/atmos/pkg/plugin/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDefaultDiscoverySources(t *testing.T) {
	tests := []struct {
		name       string
		disabled   bool
		registries map[string]schema.PluginRegistry
		selected   string
		want       []source
		invalid    bool
	}{
		{name: "implicit default", want: []source{{alias: "cloudposse", reference: pluginconfig.DefaultCatalog, kind: "catalog"}}},
		{name: "explicit selection", selected: "cloudposse", want: []source{{alias: "cloudposse", reference: pluginconfig.DefaultCatalog, kind: "catalog"}}},
		{name: "disabled", disabled: true, want: []source{}},
		{name: "disabled selection", disabled: true, selected: "cloudposse", invalid: true},
		{name: "host override removes catalog", registries: map[string]schema.PluginRegistry{"cloudposse": {Host: "example.com"}}, want: []source{}},
		{name: "explicit override survives disabled default", disabled: true, registries: map[string]schema.PluginRegistry{"cloudposse": {Catalog: "oci://example.com/catalog:stable"}}, want: []source{{alias: "cloudposse", reference: "oci://example.com/catalog:stable", kind: "catalog"}}},
		{name: "selected private registry excludes default", selected: "private", registries: map[string]schema.PluginRegistry{"private": {Repositories: []string{"oci://example.com/tools"}}}, want: []source{{alias: "private", reference: "oci://example.com/tools", kind: "repository"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &Service{Config: &schema.AtmosConfiguration{Plugins: schema.PluginsConfig{Registries: tt.registries}}}
			if tt.disabled {
				service.Config.Plugins.Discovery.DefaultRegistry = new(false)
			}
			actual, err := service.sources(tt.selected)
			if tt.invalid {
				require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, actual)
		})
	}
}

func TestDefaultCatalogSearchReportsRegistryErrors(t *testing.T) {
	registry := NewMockRegistry(gomock.NewController(t))
	reference, err := name.ParseReference(strings.TrimPrefix(pluginconfig.DefaultCatalog, "oci://"), name.StrictValidation)
	require.NoError(t, err)
	unavailable := errors.New("catalog unavailable")
	registry.EXPECT().Head(gomock.Any(), reference).Return(nil, unavailable)
	service := &Service{Config: &schema.AtmosConfiguration{Plugins: schema.PluginsConfig{CacheDir: t.TempDir()}}, Registry: registry}
	_, err = service.Search(t.Context(), Request{})
	require.ErrorIs(t, err, unavailable)
	require.Contains(t, err.Error(), "cloudposse")
}

func TestDisabledDefaultSearchNeedsNoRegistry(t *testing.T) {
	registry := NewMockRegistry(gomock.NewController(t)) // No transport method may be called.
	service := &Service{Config: &schema.AtmosConfiguration{Plugins: schema.PluginsConfig{Discovery: schema.PluginDiscoveryConfig{DefaultRegistry: new(false)}}}, Registry: registry}
	results, err := service.Search(t.Context(), Request{})
	require.NoError(t, err)
	require.Empty(t, results)
}
