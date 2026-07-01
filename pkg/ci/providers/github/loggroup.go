package github

import (
	"fmt"
	"io"

	"github.com/cloudposse/atmos/pkg/perf"
)

// StartGroup implements provider.LogGrouper by emitting a GitHub Actions
// group-start workflow command:
//
//	::group::terraform init (bounded)
//
// GitHub folds every subsequent log line under this label until the matching
// `::endgroup::` is emitted. The name is escaped per GitHub's workflow-command
// spec (reusing escapeData) so newlines and `%` in a step label cannot break
// out of the command.
func (p *Provider) StartGroup(w io.Writer, name string) {
	defer perf.Track(nil, "github.Provider.StartGroup")()

	fmt.Fprintln(w, "::group::"+escapeData(name))
}

// EndGroup implements provider.LogGrouper by emitting the GitHub Actions
// group-end workflow command that closes the most recently opened group.
func (p *Provider) EndGroup(w io.Writer) {
	defer perf.Track(nil, "github.Provider.EndGroup")()

	fmt.Fprintln(w, "::endgroup::")
}
