package generic

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Ensure Provider implements provider.SARIFReporter.
var _ provider.SARIFReporter = (*Provider)(nil)

// ReportSARIF notes that the report was produced but cannot be uploaded without a CI provider.
func (p *Provider) ReportSARIF(_ context.Context, report provider.SARIFReport) error {
	defer perf.Track(nil, "generic.Provider.ReportSARIF")()

	subject := fmt.Sprintf("SARIF report %q", report.Category)
	if report.Path != "" {
		subject += " from " + report.Path
	}
	p.out().Infof("%s (%d bytes) not uploaded: no CI provider detected", subject, len(report.Body))
	return nil
}
