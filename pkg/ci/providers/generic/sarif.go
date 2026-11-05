package generic

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Ensure Provider implements provider.SARIFReporter.
var _ provider.SARIFReporter = (*Provider)(nil)

// ReportSARIF notes that the report was produced but not uploaded. It names the report's file when
// known, and says why: the switch that is off when the reporter routed here because of a gate,
// otherwise that no CI provider is detected.
func (p *Provider) ReportSARIF(_ context.Context, report provider.SARIFReport) error {
	defer perf.Track(nil, "generic.Provider.ReportSARIF")()

	subject := fmt.Sprintf("SARIF report %q", report.Category)
	if report.Path != "" {
		subject += " from " + report.Path
	}
	reason := "no CI provider detected"
	if report.SkipReason != "" {
		reason = report.SkipReason
	}
	p.out().Infof("%s (%d bytes) not uploaded: %s", subject, len(report.Body), reason)
	return nil
}
