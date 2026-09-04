package releasenotes

import (
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// RenderBody rebuilds a release body from entries and their summaries (same
// length and order as entries) in the shape of the org-wide release-drafter
// change-template: category headings, then one collapsible `<details>` block
// per pull request with the title/author/number line as its `<summary>` and
// the condensed text as the block's body. An entry whose summary is empty
// is rendered as the plain skeleton bullet instead of an empty block.
func RenderBody(entries []PREntry, summaries []string) (string, error) {
	defer perf.Track(nil, "releasenotes.RenderBody")()

	if len(entries) != len(summaries) {
		return "", fmt.Errorf("%w: got %d, want %d", errSummaryMismatch, len(summaries), len(entries))
	}

	var b strings.Builder
	lastCategory := ""
	for i, e := range entries {
		if i > 0 {
			b.WriteString("\n")
		}
		if e.Category != lastCategory {
			if e.Category != "" {
				b.WriteString(e.Category)
				b.WriteString("\n\n")
			}
			lastCategory = e.Category
		}
		if summary := strings.TrimSpace(summaries[i]); summary != "" {
			fmt.Fprintf(&b, "<details>\n  <summary>%s</summary>\n%s\n</details>\n", e.Summary, summary)
		} else {
			fmt.Fprintf(&b, "- %s\n", e.Summary)
		}
	}
	return b.String(), nil
}
