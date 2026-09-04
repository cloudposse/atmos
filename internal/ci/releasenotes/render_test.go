package releasenotes

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderBody_KeepsDetailsBlocksWithSummarizedBodies(t *testing.T) {
	entries := []PREntry{
		{Category: "", Summary: "feat: add widget @alice (#101)", Number: 101, Body: "long body 1"},
		{Category: "## 🚀 Enhancements", Summary: "fix: correct gizmo @bob (#102)", Number: 102, Body: "long body 2"},
		{Category: "## 🚀 Enhancements", Summary: "fix: another gizmo fix @carol (#103)", Number: 103, Body: "long body 3"},
	}
	summaries := []string{
		"Adds a new widget component.",
		"Fixes an off-by-one in the gizmo.",
		"Fixes a related gizmo edge case.",
	}

	got, err := RenderBody(entries, summaries)
	require.NoError(t, err)

	// Exactly release-drafter's change-template shape, with the summary
	// where the full PR body used to be.
	assert.Contains(t, got, "<details>\n\n<summary>feat: add widget @alice (#101)</summary>\n\n- Adds a new widget component.\n\n</details>\n")
	assert.Contains(t, got, "<details>\n\n<summary>fix: correct gizmo @bob (#102)</summary>\n\n- Fixes an off-by-one in the gizmo.\n\n</details>\n")
	assert.Contains(t, got, "<details>\n\n<summary>fix: another gizmo fix @carol (#103)</summary>\n\n- Fixes a related gizmo edge case.\n\n</details>\n")
	assert.NotContains(t, got, "long body")

	// The category heading appears exactly once, before the entries it
	// groups and after the uncategorized one.
	assert.Equal(t, 1, strings.Count(got, "## 🚀 Enhancements"))
	assert.Less(t, strings.Index(got, "(#101)"), strings.Index(got, "## 🚀 Enhancements"))
	assert.Less(t, strings.Index(got, "## 🚀 Enhancements"), strings.Index(got, "(#102)"))
}

// The rendered body must be parseable by ParseDraftedBody as the same
// entries: same order, categories, summary lines and numbers, with the
// condensed text as each body. That is what makes a second summarization
// pass (or a human re-reading the draft) see the same structure
// release-drafter produced.
func TestRenderBody_RoundTripsThroughParseDraftedBody(t *testing.T) {
	entries := []PREntry{
		{Category: "", Summary: "feat: add widget @alice (#101)", Number: 101, Body: "long body 1"},
		{Category: "## 🚀 Enhancements", Summary: "fix: correct gizmo @bob (#102)", Number: 102, Body: "long body 2"},
		{Category: "## 🐛 Bug Fixes", Summary: "fix: another gizmo fix @carol (#103)", Number: 103, Body: "long body 3"},
	}
	// Bullets already, so bulletize leaves them as they are and the round trip is exact.
	summaries := []string{"- Adds a widget.", "- Fixes the gizmo.", "- Fixes another gizmo."}

	rendered, err := RenderBody(entries, summaries)
	require.NoError(t, err)

	parsed, err := ParseDraftedBody(rendered)
	require.NoError(t, err)
	require.Len(t, parsed, len(entries))
	for i := range entries {
		assert.Equal(t, entries[i].Category, parsed[i].Category, "entry %d category", i)
		assert.Equal(t, entries[i].Summary, parsed[i].Summary, "entry %d summary line", i)
		assert.Equal(t, entries[i].Number, parsed[i].Number, "entry %d number", i)
		assert.Equal(t, summaries[i], strings.TrimSpace(parsed[i].Body), "entry %d body", i)
	}
}

func TestRenderBody_EmptySummaryRendersSkeletonBullet(t *testing.T) {
	entries := []PREntry{
		{Category: "", Summary: "chore: bump deps @dependabot (#5)", Number: 5, Body: "Bumps foo from 1 to 2.\n"},
		{Category: "", Summary: "feat: x @a (#6)", Number: 6},
	}
	got, err := RenderBody(entries, []string{"", "Does x."})
	require.NoError(t, err)
	assert.Contains(t, got, "- chore: bump deps @dependabot (#5)\n")
	assert.NotContains(t, got, "Bumps foo", "an empty summary must not fall back to re-embedding the body")
	assert.Contains(t, got, "<details>\n\n<summary>feat: x @a (#6)</summary>\n\n- Does x.\n\n</details>\n")
}

// A line starting with <details> opens a GFM HTML block that swallows
// everything up to the next blank line as raw HTML, so without the blank
// lines the Markdown in the summary line (bot author links, backticks) and
// in the body renders raw - seen live on dependabot entries. The summary
// line itself is passed through untouched.
func TestRenderBody_BlankLinesKeepMarkdownRendering(t *testing.T) {
	entries := []PREntry{{
		Summary: "build(deps): bump base from `025b74b` to `b8c3669` @[dependabot[bot]](https://github.com/apps/dependabot) (#3014)",
		Number:  3014,
	}}
	got, err := RenderBody(entries, []string{"- Bumps the **base** image."})
	require.NoError(t, err)
	assert.Equal(t, "<details>\n\n<summary>build(deps): bump base from `025b74b` to `b8c3669` @[dependabot[bot]](https://github.com/apps/dependabot) (#3014)</summary>\n\n- Bumps the **base** image.\n\n</details>\n", got)
}

func TestBulletize(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "prose becomes one bullet", in: "Does x. Also y.", want: "- Does x. Also y."},
		{name: "wrapped prose joins lines", in: "Does x\nand y.", want: "- Does x and y."},
		{name: "existing bullets untouched", in: "- Does x.\n- Does y.", want: "- Does x.\n- Does y."},
		{name: "star bullets untouched", in: "* Does x.", want: "* Does x."},
		{name: "paragraphs become bullets", in: "Does x.\n\nDoes y.", want: "- Does x.\n- Does y."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, bulletize(tt.in))
		})
	}
}

func TestRenderBody_ProseSummaryIsIndentedAsBullet(t *testing.T) {
	entries := []PREntry{{Summary: "feat: x @a (#1)", Number: 1}}
	got, err := RenderBody(entries, []string{"Does x, so that y."})
	require.NoError(t, err)
	assert.Contains(t, got, "<summary>feat: x @a (#1)</summary>\n\n- Does x, so that y.\n\n</details>")
}

func TestRenderBody_MismatchedLengthsError(t *testing.T) {
	entries := []PREntry{{Summary: "a", Number: 1}, {Summary: "b", Number: 2}}
	_, err := RenderBody(entries, []string{"only one"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errSummaryMismatch)
}
