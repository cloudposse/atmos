package github

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/perf"
)

// StartLogGroup emits a GitHub Actions workflow command that opens a
// collapsible log group.
func (p *Provider) StartLogGroup(title string) error {
	defer perf.Track(nil, "github.Provider.StartLogGroup")()

	_, err := fmt.Fprintf(workflowCommandsOut, "::group::%s\n", escapeData(title))
	return err
}

// EndLogGroup emits a GitHub Actions workflow command that closes the current
// collapsible log group.
func (p *Provider) EndLogGroup() error {
	defer perf.Track(nil, "github.Provider.EndLogGroup")()

	_, err := fmt.Fprintln(workflowCommandsOut, "::endgroup::")
	return err
}
