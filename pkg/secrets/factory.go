package secrets

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets/providers"
)

// providerFor resolves the appropriate backend provider for a declaration. The component
// section is consulted so SOPS providers declared in a stack/component `secrets.providers`
// block are found (with the atmos.yaml-level `secrets.providers` as a fallback).
func providerFor(atmosConfig *schema.AtmosConfiguration, decl Declaration, componentSection map[string]any) (providers.Provider, error) {
	defer perf.Track(atmosConfig, "secrets.providerFor")()

	switch decl.BackendType {
	case BackendStore:
		return providers.NewStore(atmosConfig, decl.BackendName)
	case BackendSops:
		return providers.NewSops(atmosConfig, decl.BackendName, ExtractProviders(componentSection))
	default:
		return nil, ErrNoBackend
	}
}

// ExtractProviders reads the `secrets.providers` map from a resolved component section. This
// lets SOPS providers be declared in a stack/component rather than only in atmos.yaml.
func ExtractProviders(componentSection map[string]any) map[string]any {
	defer perf.Track(nil, "secrets.ExtractProviders")()

	secretsSection, ok := componentSection[secretsSectionKey].(map[string]any)
	if !ok {
		return nil
	}
	providersSection, _ := secretsSection[providersSectionKey].(map[string]any)
	return providersSection
}
