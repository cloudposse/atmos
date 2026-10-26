package github

import (
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Compile-time assertions that the GitHub provider implements both grouping
// capabilities: it can emit groups, and it can require grouping to be
// suppressed (in legacy actions) to keep stdout free of CI metadata.
var (
	_ provider.LogGrouper            = (*Provider)(nil)
	_ provider.LogGroupingSuppressor = (*Provider)(nil)
)

// StartLogGroup emits a GitHub Actions workflow command that opens a
// collapsible log group.
func (p *Provider) StartLogGroup(title string) error {
	defer perf.Track(nil, "github.Provider.StartLogGroup")()

	return data.Writef("::group::%s\n", escapeData(title))
}

// EndLogGroup emits a GitHub Actions workflow command that closes the current
// collapsible log group.
func (p *Provider) EndLogGroup() error {
	defer perf.Track(nil, "github.Provider.EndLogGroup")()

	return data.Writeln("::endgroup::")
}

// SuppressLogGrouping implements provider.LogGroupingSuppressor. It reports
// true while atmos runs inside a deprecated marketplace action (see
// LegacyActionRepo), because those actions capture the atmos process's stdout
// and parse it as JSON. A `::group::`/`::endgroup::` marker on stdout would
// corrupt that JSON, and the markers cannot move to stderr (they must bracket
// the stdout content they fold, and the runner does not guarantee ordering
// across streams), so the only safe option is to emit no group markers at all.
// Annotations are unaffected — they stay on stderr regardless. See #3309.
func (p *Provider) SuppressLogGrouping() bool {
	defer perf.Track(nil, "github.Provider.SuppressLogGrouping")()

	_, ok := LegacyActionRepo()
	return ok
}
