package configuration

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// useCatalog replaces the registered provider catalog for one test.
func useCatalog(t *testing.T, catalog ProviderCatalog) {
	t.Helper()
	catalogs.Lock()
	previous := catalogs.value
	catalogs.value = catalog
	catalogs.Unlock()
	t.Cleanup(func() { catalogs.Lock(); catalogs.value = previous; catalogs.Unlock() })
}

func sectionsConfig(sections map[string]schema.ComponentConfiguration) *schema.AtmosConfiguration {
	config := &schema.AtmosConfiguration{}
	config.Components.Terraform.ConfigurationProviders = sections
	return config
}

func TestSelectProvidersDropsUnknownSections(t *testing.T) {
	calls := 0
	useCatalog(t, func(context.Context, *schema.AtmosConfiguration) (map[string]bool, bool, error) {
		calls++
		return map[string]bool{"custom-plugin": true}, true, nil
	})
	original := map[string]schema.ComponentConfiguration{
		"team_defaults": {Enabled: true, Options: map[string]any{"owner": "platform"}},
		"archived":      {Enabled: false},
		"terragrunt":    {Enabled: true},
		"custom":        {Enabled: true, Provider: "custom-plugin"},
		"tg":            {Enabled: true, Provider: "terragrunt"},
	}
	config := sectionsConfig(original)
	config.Edition = legacyPin
	captureWarnings(t)
	require.NoError(t, selectProviders(t.Context(), config))
	require.Equal(t, 1, calls)
	require.Equal(t, map[string]schema.ComponentConfiguration{
		"terragrunt": {Enabled: true},
		"custom":     {Enabled: true, Provider: "custom-plugin"},
		"tg":         {Enabled: true, Provider: "terragrunt"},
	}, config.Components.Terraform.ConfigurationProviders)
	require.Len(t, original, 5, "the loaded configuration map is never changed in place")
}

func TestSelectProvidersSkipsCatalogForBuiltins(t *testing.T) {
	useCatalog(t, func(context.Context, *schema.AtmosConfiguration) (map[string]bool, bool, error) {
		t.Fatal("built-in providers need no catalog lookup")
		return nil, false, nil
	})
	config := sectionsConfig(map[string]schema.ComponentConfiguration{"terragrunt": {Enabled: true}})
	config.Components.Terraform.UnknownSections = []string{"terragrunt"}
	require.NoError(t, selectProviders(t.Context(), config))
	require.Contains(t, config.Components.Terraform.ConfigurationProviders, "terragrunt")
	require.NoError(t, selectProviders(t.Context(), sectionsConfig(nil)))
}

// TestSelectProvidersKeepsSectionsWhenExportsAreUnknown is the conservative path: an unreadable or
// uninstalled package might provide the section, so the provider that uses it reports the problem.
func TestSelectProvidersKeepsSectionsWhenExportsAreUnknown(t *testing.T) {
	for name, catalog := range map[string]ProviderCatalog{
		"error": func(context.Context, *schema.AtmosConfiguration) (map[string]bool, bool, error) {
			return nil, false, errors.New("lock file unreadable")
		},
		"incomplete": func(context.Context, *schema.AtmosConfiguration) (map[string]bool, bool, error) {
			return map[string]bool{}, false, nil
		},
		"unregistered": nil,
	} {
		t.Run(name, func(t *testing.T) {
			useCatalog(t, catalog)
			config := sectionsConfig(map[string]schema.ComponentConfiguration{"tool": {Enabled: true}})
			require.NoError(t, selectProviders(t.Context(), config))
			require.Contains(t, config.Components.Terraform.ConfigurationProviders, "tool")
		})
	}
}

// TestAttachIgnoresUnknownSections covers editions before the unknown-sections transition.
func TestAttachIgnoresUnknownSections(t *testing.T) {
	useCatalog(t, func(context.Context, *schema.AtmosConfiguration) (map[string]bool, bool, error) {
		return map[string]bool{}, true, nil
	})
	factories.Lock()
	previous := factories.values
	factories.values = map[string]Factory{"plugin": func(context.Context, *schema.AtmosConfiguration, string) (schema.ComponentConfigurationRuntime, error) {
		t.Fatal("an unknown section must not start a provider session")
		return nil, nil
	}}
	factories.Unlock()
	t.Cleanup(func() { factories.Lock(); factories.values = previous; factories.Unlock() })
	config := sectionsConfig(map[string]schema.ComponentConfiguration{"team_defaults": {Enabled: true}})
	config.Edition = legacyPin
	captureWarnings(t)
	require.NoError(t, Attach(t.Context(), config, "list"))
	require.Nil(t, config.ComponentConfigurations)
	require.Empty(t, config.Components.Terraform.ConfigurationProviders)
}

func TestRegisterProviderCatalog(t *testing.T) {
	useCatalog(t, nil)
	require.ErrorIs(t, RegisterProviderCatalog(nil), errUtils.ErrPluginInvalidConfig)
	catalog := func(context.Context, *schema.AtmosConfiguration) (map[string]bool, bool, error) {
		return nil, true, nil
	}
	require.NoError(t, RegisterProviderCatalog(catalog))
	require.ErrorIs(t, RegisterProviderCatalog(catalog), errUtils.ErrDuplicatePluginExtension)
}
