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
	lister, ok := reg.(interface {
		GetAvailableVersions(owner, repo string) ([]string, error)
	})
	if !ok {
		return nil, fmt.Errorf("%w: toolchain registry cannot list versions", resolver.ErrVersionListingUnsupported)
	}
	versions, err := lister.GetAvailableVersions(owner, repo)
	if err != nil {
		return nil, err
	}
	candidates := make([]resolver.Candidate, 0, len(versions))
	for _, version := range versions {
		candidates = append(candidates, resolver.Candidate{Version: version})
	}
	return candidates, nil
}

// Pin is unsupported: toolchain packages have no immutable digest concept.
func (Resolver) Pin(ctx context.Context, req *resolver.Request, version string) (string, error) {
	defer perf.Track(nil, "toolchain.Resolver.Pin")()

	return "", fmt.Errorf("%w: %s", resolver.ErrPinUnsupported, Datasource)
}

func init() {
	resolver.Register(Resolver{})
}
