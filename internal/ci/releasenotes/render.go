package releasenotes

import (
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// uncategorizedHeading is given to entries release-drafter left without a
// category heading - it puts PRs matching no category first, headingless -
// whenever the release also has categorized groups, so the first group does
// not read as a missing chapter.
const uncategorizedHeading = "## 📦 Other Changes"

// summaryToken matches the two pieces of Markdown release-drafter puts in a
// summary line that GitHub will not render inside an HTML <summary> (blank
// lines fix the block's body, not the tag itself): a link (bot authors come
// through as `[dependabot[bot]](https://github.com/apps/dependabot)` - note
// the nested brackets) and a backtick code span (PR titles like "bump foo
// from `abc` to `def`").
var summaryToken = regexp.MustCompile("\\[((?:[^\\[\\]]|\\[[^\\]]*\\])*)\\]\\(([^)\\s]+)\\)|`([^`]+)`")

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

	mixed := hasCategorized(entries)
	var b strings.Builder
	lastCategory := ""
	for i, e := range entries {
		if i > 0 {
			b.WriteString("\n")
		}
		if e.Category != lastCategory || i == 0 {
			heading := e.Category
			if heading == "" && mixed {
				heading = uncategorizedHeading
			}
			if heading != "" {
				b.WriteString(heading)
				b.WriteString("\n\n")
			}
			lastCategory = e.Category
		}
		if summary := strings.TrimSpace(summaries[i]); summary != "" {
			fmt.Fprintf(&b, "<details>\n\n<summary>%s</summary>\n\n%s\n\n</details>\n", renderSummaryLine(e.Summary), bulletize(summary))
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

func hasCategorized(entries []PREntry) bool {
	for _, e := range entries {
		if e.Category != "" {
			return true
		}
	}
	return false
}

// renderSummaryLine converts a release-drafter summary line for use inside
// an HTML <summary> element, where GitHub renders HTML (and @mentions and
// #refs) but not Markdown: Markdown links become <a>, backtick spans become
// <code>, and the rest is HTML-escaped.
func renderSummaryLine(s string) string {
	var b strings.Builder
	last := 0
	for _, m := range summaryToken.FindAllStringSubmatchIndex(s, -1) {
		b.WriteString(html.EscapeString(s[last:m[0]]))
		if m[2] >= 0 {
			fmt.Fprintf(&b, `<a href="%s">%s</a>`, html.EscapeString(s[m[4]:m[5]]), html.EscapeString(s[m[2]:m[3]]))
		} else {
			fmt.Fprintf(&b, "<code>%s</code>", html.EscapeString(s[m[6]:m[7]]))
		}
		last = m[1]
	}
	b.WriteString(html.EscapeString(s[last:]))
	return b.String()
}
