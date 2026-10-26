package generic

import (
	"os"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ResolveBase returns the base commit for affected detection in the generic provider.
// It reads the ATMOS_CI_BASE_REF environment variable for explicit base override.
// Returns nil when no env var is set (falls through to default behavior).
func (p *Provider) ResolveBase() (*provider.BaseResolution, error) {
	defer perf.Track(nil, "generic.Provider.ResolveBase")()

	base := os.Getenv("ATMOS_CI_BASE_REF")
	if base == "" {
		return nil, nil
	}

	if ci.IsCommitSHA(base) {
		return &provider.BaseResolution{
			SHA:    base,
			Source: "ATMOS_CI_BASE_REF",
		}, nil
	}

	return &provider.BaseResolution{
		Ref:    "refs/remotes/origin/" + base,
		Source: "ATMOS_CI_BASE_REF",
	}, nil
}
