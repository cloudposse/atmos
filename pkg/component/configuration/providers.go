package configuration

import (
	"context"
	"maps"
	"slices"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// builtinProviders are the configuration providers Atmos knows by name. A section that selects one is a
// provider section even before its plugin package is installed, so a missing package is reported.
var builtinProviders = map[string]bool{"terragrunt": true}

// ProviderCatalog lists the configuration providers that the enabled plugin packages export.
// The boolean result is false when an enabled package's exports cannot be read without installing it.
type ProviderCatalog func(ctx context.Context, config *schema.AtmosConfiguration) (names map[string]bool, complete bool, err error)

var catalogs = struct {
	sync.RWMutex
	value ProviderCatalog
}{}

// RegisterProviderCatalog sets the catalog that tells provider sections from unrelated mappings under
// `components.terraform`. It is registered once during host composition.
func RegisterProviderCatalog(catalog ProviderCatalog) error {
	defer perf.Track(nil, "configuration.RegisterProviderCatalog")()

	if catalog == nil {
		return errUtils.ErrPluginInvalidConfig
	}
	catalogs.Lock()
	defer catalogs.Unlock()
	if catalogs.value != nil {
		return errUtils.ErrDuplicatePluginExtension
	}
	catalogs.value = catalog
	return nil
}

// providerName is the configuration provider a `components.terraform` section selects.
func providerName(section string, settings *schema.ComponentConfiguration) string {
	if settings.Provider != "" {
		return settings.Provider
	}
	return section
}

// selectProviders keeps only the `components.terraform` sections that select a configuration provider:
// a built-in provider name or one an enabled plugin package exports. Any other mapping, such as a
// team's own `team_defaults: {enabled: true}`, is dropped, so it neither starts a provider session nor
// appears as a provider section. The dropped sections and the keys the loader already dropped (see
// schema.Terraform.UnknownSections) are unclaimed: reportUnclaimedSections warns about them or rejects
// them, depending on the edition. When the exports cannot be determined, every section is kept and the
// provider that uses it reports the problem.
func selectProviders(ctx context.Context, config *schema.AtmosConfiguration) error {
	sections := config.Components.Terraform.ConfigurationProviders
	unknown, unmarked := providerCandidates(config)
	if len(unknown) == 0 && len(unmarked) == 0 {
		return nil
	}
	catalogs.RLock()
	catalog := catalogs.value
	catalogs.RUnlock()
	if catalog == nil {
		return nil
	}
	exported, complete, err := catalog(ctx, config)
	if err != nil || !complete {
		log.Debug("Could not determine the configuration providers of the plugin packages; keeping every section under components.terraform", "error", err)
		return nil
	}
	kept := maps.Clone(sections)
	var unclaimed []string
	for _, section := range unknown {
		settings := sections[section]
		name := providerName(section, &settings)
		if exported[name] {
			continue
		}
		log.Debug("Dropping a section under components.terraform that selects no known configuration provider", "section", section, "provider", name)
		delete(kept, section)
		unclaimed = append(unclaimed, section)
	}
	config.Components.Terraform.ConfigurationProviders = kept
	// An unmarked section is not enabled, but a section named after a provider is still the provider's.
	for _, section := range unmarked {
		if !exported[section] {
			unclaimed = append(unclaimed, section)
		}
	}
	slices.Sort(unclaimed)
	return reportUnclaimedSections(config.Edition, unclaimed)
}

// providerCandidates returns the sections that a built-in provider does not account for: provider
// sections (marked with enabled or provider) and the unmarked keys the loader recorded.
func providerCandidates(config *schema.AtmosConfiguration) (unknown, unmarked []string) {
	sections := config.Components.Terraform.ConfigurationProviders
	for _, section := range slices.Sorted(maps.Keys(sections)) {
		settings := sections[section]
		if !builtinProviders[providerName(section, &settings)] {
			unknown = append(unknown, section)
		}
	}
	for _, section := range config.Components.Terraform.UnknownSections {
		if !builtinProviders[section] {
			unmarked = append(unmarked, section)
		}
	}
	return unknown, unmarked
}
