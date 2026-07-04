package manager

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version/resolver"
)

// Errors surfaced from the resolver registry, aliased so existing callers and
// errors.Is checks against the manager package keep working.
var (
	ErrResolverUnsupported = resolver.ErrResolverUnsupported
	ErrNoVersionMatch      = resolver.ErrNoVersionMatch
)

// ResolveTarget returns the desired concrete version for an entry. Concrete
// desired versions pass through unchanged; "latest" and SemVer constraints are
// resolved against the entry's datasource via the resolver registry, honoring
// the entry's allow and ignore rules.
func ResolveTarget(atmosConfig *schema.AtmosConfiguration, entry *EffectiveEntry) (string, error) {
	defer perf.Track(atmosConfig, "manager.ResolveTarget")()

	if entry.Desired == "" {
		return "", fmt.Errorf("%w: %s", ErrDesiredVersionRequired, entry.Name)
	}
	concrete := entry.Desired != "latest" && !resolver.LooksLikeConstraint(entry.Desired)
	res, datasource, ok := resolver.Lookup(entry.Datasource)
	if !ok {
		if concrete {
			return entry.Desired, nil
		}
		return "", fmt.Errorf("%w: datasource %q has no registered resolver; use a concrete desired version", ErrResolverUnsupported, entry.Datasource)
	}
	if concrete {
		return entry.Desired, nil
	}
	candidates, err := res.Versions(context.Background(), resolverRequest(atmosConfig, entry, datasource))
	if err != nil {
		return "", err
	}
	selected, err := resolver.Select(candidates, entry.Desired, entry.Allow, entry.Ignore)
	if err != nil {
		return "", err
	}
	return selected.Version, nil
}

// resolverRequest builds a resolver request for an entry, attaching the
// configured provider when one is referenced.
func resolverRequest(atmosConfig *schema.AtmosConfiguration, entry *EffectiveEntry, datasource string) *resolver.Request {
	req := &resolver.Request{
		Package:    entry.Package,
		Datasource: datasource,
		Config:     atmosConfig,
	}
	if atmosConfig != nil && entry.Provider != "" {
		req.Provider = atmosConfig.Version.Providers[entry.Provider]
	}
	return req
}
