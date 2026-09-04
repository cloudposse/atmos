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
	assert.Contains(t, got, "<details>\n  <summary>feat: add widget @alice (#101)</summary>\nAdds a new widget component.\n</details>\n")
	assert.Contains(t, got, "<details>\n  <summary>fix: correct gizmo @bob (#102)</summary>\nFixes an off-by-one in the gizmo.\n</details>\n")
	assert.Contains(t, got, "<details>\n  <summary>fix: another gizmo fix @carol (#103)</summary>\nFixes a related gizmo edge case.\n</details>\n")
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
	summaries := []string{"Adds a widget.", "Fixes the gizmo.", "Fixes another gizmo."}

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

func TestRenderBody_EmptySummaryKeepsOriginalBody(t *testing.T) {
	entries := []PREntry{{Category: "", Summary: "chore: bump deps @dependabot (#5)", Number: 5, Body: "Bumps foo from 1 to 2.\n"}}
	got, err := RenderBody(entries, []string{""})
	require.NoError(t, err)
	assert.Contains(t, got, "<details>\n  <summary>chore: bump deps @dependabot (#5)</summary>\nBumps foo from 1 to 2.\n</details>\n")
}

func TestRenderBody_MismatchedLengthsError(t *testing.T) {
	entries := []PREntry{{Summary: "a", Number: 1}, {Summary: "b", Number: 2}}
	_, err := RenderBody(entries, []string{"only one"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errSummaryMismatch)
}
