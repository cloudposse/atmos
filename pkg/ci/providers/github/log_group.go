package github

import (
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/perf"
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
