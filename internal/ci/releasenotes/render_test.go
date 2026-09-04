package releasenotes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderBody(t *testing.T) {
	entries := []PREntry{
		{Category: "", Summary: "feat: add widget @alice (#101)", Number: 101},
		{Category: "## 🚀 Enhancements", Summary: "fix: correct gizmo @bob (#102)", Number: 102},
		{Category: "## 🚀 Enhancements", Summary: "fix: another gizmo fix @carol (#103)", Number: 103},
	}
	summaries := []string{
		"Adds a new widget component.",
		"Fixes an off-by-one in the gizmo.",
		"Fixes a related gizmo edge case.",
	}

	got, err := RenderBody(entries, summaries)
	require.NoError(t, err)

	assert.Contains(t, got, "- feat: add widget @alice (#101): Adds a new widget component.")
	assert.Contains(t, got, "## 🚀 Enhancements")
	assert.Contains(t, got, "- fix: correct gizmo @bob (#102): Fixes an off-by-one in the gizmo.")
	assert.Contains(t, got, "- fix: another gizmo fix @carol (#103): Fixes a related gizmo edge case.")

	// The category heading appears exactly once, grouping both entries under it.
	assert.Equal(t, 1, countOccurrences(got, "## 🚀 Enhancements"))
}

func TestRenderBody_EmptySummaryFallsBackToTitleOnly(t *testing.T) {
	entries := []PREntry{{Category: "", Summary: "chore: bump deps @dependabot (#5)", Number: 5}}
	got, err := RenderBody(entries, []string{""})
	require.NoError(t, err)
	assert.Contains(t, got, "- chore: bump deps @dependabot (#5)\n")
	assert.NotContains(t, got, ": \n")
}

func TestRenderBody_MismatchedLengthsError(t *testing.T) {
	entries := []PREntry{{Summary: "a", Number: 1}, {Summary: "b", Number: 2}}
	_, err := RenderBody(entries, []string{"only one"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errSummaryMismatch)
}

func countOccurrences(haystack, needle string) int {
	count := 0
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			count++
		}
	}
	return count
}
