package configuration

import (
	"context"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ImportFragment wraps imported units as a stack manifest fragment, so the ordinary
// import merge adds them to the importing manifest as Terraform components.
func ImportFragment(components map[string]any) map[string]any {
	defer perf.Track(nil, "configuration.ImportFragment")()

	return map[string]any{componentsKey: map[string]any{terraformType: components}}
}

// unitSource returns the source file of a provider-owned component, or "" for a YAML component.
func unitSource(component any) string {
	section, _ := component.(map[string]any)
	settings, _ := section["settings"].(map[string]any)
	configuration, _ := settings["configuration"].(map[string]any)
	source, _ := configuration["source"].(string)
	return source
}

// CheckImportConflicts rejects a manifest in which two different units become one component
// name. A YAML component of the same name as a unit is not a conflict: the unit's fragment is
// the base layer of the ordinary import merge, so the YAML component deep-merges over (or under,
// when it comes from an earlier import) the unit like any other stack configuration. Configs are
// the manifest's imports in order, followed by the manifest's own content.
func CheckImportConflicts(manifest string, configs []map[string]any) error {
	defer perf.Track(nil, "configuration.CheckImportConflicts")()

	units := map[string]string{}
	for _, config := range configs {
		section, _ := config[componentsKey].(map[string]any)
		terraform, _ := section[terraformType].(map[string]any)
		for name, component := range terraform {
			source := unitSource(component)
			if source == "" {
				continue
			}
			if previous, seen := units[name]; seen && previous != source {
				return importConflict(name, previous, source, manifest)
			}
			units[name] = source
		}
	}
	return nil
}

// OwnsComponent reports whether a configuration provider supplies a component section, so
// callers can avoid attributing it to unrelated YAML files.
func OwnsComponent(component map[string]any) bool {
	defer perf.Track(nil, "configuration.OwnsComponent")()

	settings, _ := component["settings"].(map[string]any)
	configuration, _ := settings["configuration"].(map[string]any)
	return configuration["provider"] != nil
}

// ImportProviderComponents routes configured provider schemes before YAML or remote-source
// resolution. The provider's units come back as a manifest fragment that joins the importing
// manifest, so they are components of the importing stack. The boolean reports whether the
// import was handled by a provider.
func ImportProviderComponents(config *schema.AtmosConfiguration, uri string, declaration *schema.StackImport) (map[string]any, bool, error) {
	defer perf.Track(nil, "configuration.ImportProviderComponents")()

	scheme, _, found := strings.Cut(uri, "://")
	if !found || config == nil {
		return nil, false, nil
	}
	settings, configured := config.Components.Terraform.ConfigurationProviders[scheme]
	if !configured {
		return nil, false, nil
	}
	if err := checkProviderImport(config, scheme, &settings, declaration); err != nil {
		return nil, true, err
	}
	imports := config.ComponentConfigurations.(schema.ComponentConfigurationImportRuntime)
	components, handled, err := imports.ImportComponents(context.Background(), uri)
	if err != nil || !handled {
		return nil, handled, err
	}
	return ImportFragment(components), true, nil
}

// checkProviderImport rejects an import the configured provider cannot serve.
func checkProviderImport(config *schema.AtmosConfiguration, scheme string, settings *schema.ComponentConfiguration, declaration *schema.StackImport) error {
	if !settings.Enabled {
		return fmt.Errorf("%w: configuration provider %s is disabled", errUtils.ErrPluginInvalidConfig, scheme)
	}
	if len(declaration.Context) != 0 || declaration.SkipIfMissing || declaration.TTL != "" || declaration.NestedImports != "" {
		return fmt.Errorf("%w: provider imports do not support context, skip_if_missing, TTL, or nested_imports", errUtils.ErrPluginUnsupportedExtension)
	}
	if _, ok := config.ComponentConfigurations.(schema.ComponentConfigurationImportRuntime); !ok {
		return fmt.Errorf("%w: provider %s has no import runtime", errUtils.ErrPluginUnsupportedExtension, scheme)
	}
	return nil
}
