package generic

import (
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Ensure Provider implements provider.LogGrouper.
var _ provider.LogGrouper = (*Provider)(nil)

// StartLogGroup renders the group title as a heading line.
func (p *Provider) StartLogGroup(title string) error {
	defer perf.Track(nil, "generic.Provider.StartLogGroup")()

	p.out().Infof("── %s", title)
	return nil
}

// EndLogGroup is a no-op: a plain terminal has nothing to close.
func (p *Provider) EndLogGroup() error {
	defer perf.Track(nil, "generic.Provider.EndLogGroup")()

	return nil
}
