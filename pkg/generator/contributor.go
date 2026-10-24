package generator

import (
	"context"
	"sort"
	"sync"

	"github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ProviderContributor contributes a Terraform provider-config fragment to a
// component's ProvidersSection before generation.
//
// Contributors let cross-cutting concerns inject provider behavior flags that
// environment variables cannot set — mirroring how Terraform RC management
// assembles `.terraformrc` from contributions. The first consumer is the emulator
// binding (endpoints + skip-flags + dummy creds); auth and the registry cache can
// register contributors later without reworking the core.
type ProviderContributor interface {
	// Name is the unique contributor identifier.
	Name() string

	// Contribute returns a provider fragment keyed by Terraform provider name
	// (e.g. {"aws": {"skip_requesting_account_id": true, ...}}), or nil/empty when
	// this contributor does not apply to the component in genCtx.
	Contribute(ctx context.Context, genCtx *GeneratorContext) (map[string]any, error)
}

var (
	contributorRegistry   = map[string]ProviderContributor{}
	contributorRegistryMu sync.RWMutex
)

// RegisterProviderContributor adds a provider-config contributor to the registry.
// Typically called from a contributor package's init().
func RegisterProviderContributor(c ProviderContributor) {
	defer perf.Track(nil, "generator.RegisterProviderContributor")()

	contributorRegistryMu.Lock()
	defer contributorRegistryMu.Unlock()
	contributorRegistry[c.Name()] = c
}

// ProviderContributors returns the registered contributors sorted by name (stable order).
func ProviderContributors() []ProviderContributor {
	defer perf.Track(nil, "generator.ProviderContributors")()

	contributorRegistryMu.RLock()
	defer contributorRegistryMu.RUnlock()

	names := make([]string, 0, len(contributorRegistry))
	for name := range contributorRegistry {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]ProviderContributor, 0, len(names))
	for _, name := range names {
		out = append(out, contributorRegistry[name])
	}
	return out
}

// ApplyProviderContributors runs all registered contributors and deep-merges their
// fragments UNDER the component's existing ProvidersSection — so an explicit stack
// `providers:` value always wins over a contribution. It mutates and returns
// genCtx.ProvidersSection.
func ApplyProviderContributors(ctx context.Context, genCtx *GeneratorContext) (map[string]any, error) {
	defer perf.Track(nil, "generator.ApplyProviderContributors")()

	if genCtx == nil {
		return nil, nil
	}

	for _, contributor := range ProviderContributors() {
		fragment, err := contributor.Contribute(ctx, genCtx)
		if err != nil {
			return nil, err
		}
		if len(fragment) == 0 {
			continue
		}

		existing := genCtx.ProvidersSection
		if existing == nil {
			existing = map[string]any{}
		}
		// Contribution first, explicit providers second → explicit wins.
		merged, err := merge.Merge(genCtx.AtmosConfig, []map[string]any{fragment, existing})
		if err != nil {
			return nil, err
		}
		genCtx.ProvidersSection = merged
	}

	return genCtx.ProvidersSection, nil
}
