package generic

import (
	"context"

	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Ensure Provider implements provider.SARIFReporter.
var _ provider.SARIFReporter = (*Provider)(nil)

// ReportSARIF notes that the report was produced but cannot be uploaded without a CI provider.
func (p *Provider) ReportSARIF(_ context.Context, report provider.SARIFReport) error {
	defer perf.Track(nil, "generic.Provider.ReportSARIF")()

	p.out().Infof("SARIF report %q (%d bytes) not uploaded: no CI provider detected", report.Category, len(report.Body))
	return nil
}
