package releasenotes

import (
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// RenderBody rebuilds a release body from entries and their summaries (same
// length and order as entries, e.g. Summarize's return value) in exactly the
// shape release-drafter's change-template produced it: category headings,
// then one collapsible `<details>` block per pull request with the
// title/author/number line as its `<summary>`. The only difference is that
// each block's body is the condensed summary instead of the full embedded PR
// description, so the notes read and expand the same - just shorter inside.
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
		writeDetails(&b, e, summaries[i])
	}
	return b.String(), nil
}

// writeDetails mirrors release-drafter's default change-template
// (`<details>\n  <summary>$TITLE @$AUTHOR (#$NUMBER)</summary>\n$BODY\n</details>`).
// An empty summary keeps the original body: an entry the model had nothing
// to say about should still show what the PR did, not an empty block.
func writeDetails(b *strings.Builder, e PREntry, summary string) {
	body := strings.TrimSpace(summary)
	if body == "" {
		body = strings.TrimSpace(e.Body)
	}
	fmt.Fprintf(b, "<details>\n  <summary>%s</summary>\n%s\n</details>\n", e.Summary, body)
}
