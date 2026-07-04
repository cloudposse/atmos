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

	candidate, err := ResolveEntry(atmosConfig, entry, false)
	if err != nil {
		return "", err
	}
	return candidate.Version, nil
}

// ResolveEntry resolves an entry to its full candidate (version plus digest
// and release timestamp when the datasource provides them). When pin is true
// and the resolved candidate has no digest yet, the datasource's Pin
// resolution runs; a datasource without a digest concept fails loudly so a
// misconfigured `pin: digest` is never silently ignored.
func ResolveEntry(atmosConfig *schema.AtmosConfiguration, entry *EffectiveEntry, pin bool) (resolver.Candidate, error) {
	defer perf.Track(atmosConfig, "manager.ResolveEntry")()

	if entry.Desired == "" {
		return resolver.Candidate{}, fmt.Errorf("%w: %s", ErrDesiredVersionRequired, entry.Name)
	}
	concrete := entry.Desired != "latest" && !resolver.LooksLikeConstraint(entry.Desired)
	res, datasource, ok := resolver.Lookup(entry.Datasource)
	if !ok {
		if concrete && !pin {
			return resolver.Candidate{Version: entry.Desired}, nil
		}
		return resolver.Candidate{}, fmt.Errorf("%w: datasource %q has no registered resolver; use a concrete desired version", ErrResolverUnsupported, entry.Datasource)
	}

	ctx := context.Background()
	req := resolverRequest(atmosConfig, entry, datasource)
	candidate, err := selectCandidate(ctx, res, req, entry, concrete)
	if err != nil {
		return resolver.Candidate{}, err
	}
	if pin && candidate.Digest == "" {
		digest, err := res.Pin(ctx, req, candidate.Version)
		if err != nil {
			return resolver.Candidate{}, fmt.Errorf("%s: %w", entry.Name, err)
		}
		candidate.Digest = digest
	}
	return candidate, nil
}

// selectCandidate returns the candidate for an entry: the concrete desired
// version as-is, or the best datasource candidate for latest/constraints.
func selectCandidate(ctx context.Context, res resolver.Resolver, req *resolver.Request, entry *EffectiveEntry, concrete bool) (resolver.Candidate, error) {
	if concrete {
		return resolver.Candidate{Version: entry.Desired}, nil
	}
	candidates, err := res.Versions(ctx, req)
	if err != nil {
		return resolver.Candidate{}, err
	}
	return resolver.Select(candidates, entry.Desired, entry.Allow, entry.Ignore)
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
