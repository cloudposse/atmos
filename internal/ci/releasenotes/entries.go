// Package releasenotes summarizes drafted GitHub release notes with an LLM
// so the release body stays well under GitHub's 125,000-character limit on
// repositories that merge many pull requests between releases. See
// docs/fixes for the incident this closes.
package releasenotes

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// errNoEntries means a drafted release body had no `<details>` blocks to
// summarize - either release-drafter's template changed shape, or the body
// was empty.
var errNoEntries = errors.New("releasenotes: no pull request entries found in body")

// PREntry is one pull request as release-drafter rendered it: a category
// (the nearest preceding `## ` heading, "" if none), the exact summary line
// release-drafter produced (title, author, number), the PR number extracted
// from that line, and the full PR body release-drafter embedded beneath it.
type PREntry struct {
	Category string
	Summary  string
	Number   int
	Body     string
}

var (
	// The detailsOpen/detailsClose markers bound one release-drafter entry;
	// category headings and blank filler outside these markers are not part
	// of any entry.
	detailsOpen  = regexp.MustCompile(`(?m)^<details>\s*$`)
	detailsClose = regexp.MustCompile(`(?m)^</details>\s*$`)
	// Non-greedy: matches up to the first </summary>, not the last, in the
	// unlikely event a PR title contains that literal substring.
	summaryLine  = regexp.MustCompile(`(?m)^\s*<summary>(.*?)</summary>\s*$`)
	categoryLine = regexp.MustCompile(`(?m)^(## .+)$`)
	prNumber     = regexp.MustCompile(`#(\d+)\)?\s*$`)
)

// ParseDraftedBody extracts every pull request entry from a release-drafter
// body built from the default `<details><summary>$TITLE @$AUTHOR
// (#$NUMBER)</summary>$BODY</details>` change-template, in document order,
// tagging each with the nearest preceding `## ` category heading (release-drafter
// emits one per matched category, "" for entries under no heading).
func ParseDraftedBody(raw string) ([]PREntry, error) {
	defer perf.Track(nil, "releasenotes.ParseDraftedBody")()

	opens := detailsOpen.FindAllStringIndex(raw, -1)
	closes := detailsClose.FindAllStringIndex(raw, -1)
	if len(opens) == 0 || len(opens) != len(closes) {
		return nil, errNoEntries
	}

	entries := make([]PREntry, 0, len(opens))
	tracker := newCategoryTracker(raw)

	for i, open := range opens {
		// A malformed body (mismatched marker order despite equal counts) can
		// put a close marker before its paired open; reject it rather than
		// slicing with a low bound past the high bound below.
		if closes[i][0] < open[1] {
			return nil, errNoEntries
		}

		category := tracker.advanceTo(open[0])
		block := raw[open[1]:closes[i][0]]
		// Consume the block even when it fails to parse below: the region is
		// still spoken for, so a heading inside a skipped block must not
		// count as a real category heading for the next entry either.
		tracker.consumeBlock(closes[i][0])

		entry, err := parseBlock(block, category)
		if err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	if len(entries) == 0 {
		return nil, errNoEntries
	}
	return entries, nil
}

// categoryTracker walks a drafted body's `## ` category headings in document
// order alongside its <details> blocks, so ParseDraftedBody can ask "what's
// the current category" at each block's opening marker. Headings that fall
// inside a previously-consumed block are skipped rather than adopted: a PR
// body containing its own `## ` heading must not become the category for the
// *next* entry.
type categoryTracker struct {
	raw     string
	matches [][]int
	idx     int
	current string
	// prevClose is the end of the last consumed <details> block; -1 before
	// the first block, so nothing is excluded yet.
	prevClose int
}

func newCategoryTracker(raw string) *categoryTracker {
	return &categoryTracker{
		raw:       raw,
		matches:   categoryLine.FindAllStringSubmatchIndex(raw, -1),
		prevClose: -1,
	}
}

// advanceTo consumes every heading match before limit, adopting it as the
// current category unless it falls inside the previously consumed block, and
// returns the resulting category.
func (c *categoryTracker) advanceTo(limit int) string {
	for c.idx < len(c.matches) && c.matches[c.idx][0] < limit {
		if c.prevClose == -1 || c.matches[c.idx][0] >= c.prevClose {
			c.current = c.raw[c.matches[c.idx][2]:c.matches[c.idx][3]]
		}
		c.idx++
	}
	return c.current
}

// consumeBlock records the end of the block just processed, so future
// advanceTo calls exclude any heading inside it.
func (c *categoryTracker) consumeBlock(closeStart int) {
	c.prevClose = closeStart
}

func parseBlock(block, category string) (PREntry, error) {
	m := summaryLine.FindStringSubmatchIndex(block)
	if m == nil {
		return PREntry{}, errNoEntries
	}
	summary := block[m[2]:m[3]]
	number := extractPRNumber(summary)
	body := strings.TrimSpace(block[m[1]:])
	return PREntry{Category: category, Summary: summary, Number: number, Body: body}, nil
}

// extractPRNumber pulls the trailing `#123` out of a release-drafter summary
// line; 0 when the line doesn't end in one (unexpected template, not fatal -
// the entry is still summarized, just unlabeled in the prompt).
func extractPRNumber(summary string) int {
	m := prNumber.FindStringSubmatch(summary)
	if m == nil {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return n
}
