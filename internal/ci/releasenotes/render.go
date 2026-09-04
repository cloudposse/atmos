package releasenotes

import (
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// RenderBody rebuilds a release body from entries and their summaries (same
// length and order as entries, e.g. Summarize's return value), grouping
// consecutive entries under their category heading the way release-drafter's
// own template does, but with one summarized bullet per entry instead of the
// full embedded PR body.
func RenderBody(entries []PREntry, summaries []string) (string, error) {
	defer perf.Track(nil, "releasenotes.RenderBody")()

	if len(entries) != len(summaries) {
		return "", fmt.Errorf("%w: got %d, want %d", errSummaryMismatch, len(summaries), len(entries))
	}

	var b strings.Builder
	lastCategory := ""
	first := true
	for i, e := range entries {
		if e.Category != lastCategory {
			if !first {
				b.WriteString("\n")
			}
			if e.Category != "" {
				b.WriteString(e.Category)
				b.WriteString("\n\n")
			}
			lastCategory = e.Category
		}
		fmt.Fprintf(&b, "- %s\n", renderBullet(e, summaries[i]))
		first = false
	}
	return strings.TrimRight(b.String(), "\n") + "\n", nil
}

func renderBullet(e PREntry, summary string) string {
	if summary == "" {
		return e.Summary
	}
	return fmt.Sprintf("%s: %s", e.Summary, summary)
}
