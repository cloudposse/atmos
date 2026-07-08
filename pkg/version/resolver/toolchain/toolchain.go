// Package toolchain implements the "toolchain" datasource resolver backed by
// the Atmos toolchain's Aqua registry.
package toolchain

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/toolchain"
	"github.com/cloudposse/atmos/pkg/version/resolver"
)

// Datasource is the datasource name served by this resolver.
const Datasource = "toolchain"

// Resolver resolves toolchain package versions via the Aqua registry.
type Resolver struct{}

// Names returns the datasource names this resolver serves.
func (Resolver) Names() []string {
	defer perf.Track(nil, "toolchain.Resolver.Names")()

	return []string{Datasource}
}

// Versions lists available versions for a toolchain package.
func (Resolver) Versions(ctx context.Context, req *resolver.Request) ([]resolver.Candidate, error) {
	defer perf.Track(nil, "toolchain.Resolver.Versions")()

	installer := toolchain.NewInstaller(toolchain.WithAtmosConfig(req.Config))
	owner, repo, err := installer.ParseToolSpec(req.Package)
	if err != nil {
		return nil, err
	}
	reg := toolchain.NewAquaRegistry()
	versions, err := availableVersions(ctx, reg, owner, repo)
	if err != nil {
		return nil, err
	}
	candidates := make([]resolver.Candidate, 0, len(versions))
	for _, version := range versions {
		candidates = append(candidates, resolver.Candidate{Version: version})
	}
	return candidates, nil
}

func availableVersions(ctx context.Context, reg any, owner, repo string) ([]string, error) {
	if lister, ok := reg.(interface {
		GetAvailableVersionsContext(context.Context, string, string) ([]string, error)
	}); ok {
		return lister.GetAvailableVersionsContext(ctx, owner, repo)
	}
	if lister, ok := reg.(interface {
		GetAvailableVersions(owner, repo string) ([]string, error)
	}); ok {
		return lister.GetAvailableVersions(owner, repo)
	}
	return nil, fmt.Errorf("%w: toolchain registry cannot list versions", resolver.ErrVersionListingUnsupported)
}

// Pin is unsupported: toolchain packages have no immutable digest concept.
func (Resolver) Pin(ctx context.Context, req *resolver.Request, version string) (string, error) {
	defer perf.Track(nil, "toolchain.Resolver.Pin")()

	return "", fmt.Errorf("%w: %s", resolver.ErrPinUnsupported, Datasource)
}

func init() {
	resolver.Register(Resolver{})
}
