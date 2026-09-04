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
//
// The blank lines inside each block matter: in GitHub-flavored Markdown a
// line starting with `<details>` opens an HTML block that swallows every
// following line up to the next blank line as raw HTML. With a blank line
// after `<details>`, the `<summary>` line is an ordinary paragraph with
// inline HTML - so the Markdown release-drafter puts in it (bot author
// links, backticks) renders - and the body after the next blank line is
// ordinary Markdown too.
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
			fmt.Fprintf(&b, "<details>\n\n<summary>%s</summary>\n\n%s\n\n</details>\n", e.Summary, bulletize(summary))
		} else {
			fmt.Fprintf(&b, "- %s\n", e.Summary)
		}
	}
	return b.String(), nil
}

// bulletize makes every paragraph of a summary a Markdown list item unless
// it already is one. Inside a <details> block, flush-left prose is visually
// indistinguishable from the summary line above it; the indent is what
// makes the notes scannable, so the layout must not depend on whether the
// model chose bullets.
func bulletize(summary string) string {
	paras := strings.Split(summary, "\n\n")
	for i, para := range paras {
		para = strings.TrimSpace(para)
		if para == "" || strings.HasPrefix(para, "- ") || strings.HasPrefix(para, "* ") {
			paras[i] = para
			continue
		}
		paras[i] = "- " + strings.Join(strings.Split(para, "\n"), " ")
	}
	return strings.Join(paras, "\n")
}
