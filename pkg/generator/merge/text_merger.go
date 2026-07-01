package merge

import (
	"bytes"
	"strings"

	"github.com/epiclabs-io/diff3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// TextMerger handles 3-way merging of text files using the diff3 algorithm.
type TextMerger struct {
	thresholdPercent int // Percentage threshold (0-100) for change detection.
}

// NewTextMerger creates a new text merger with the specified percentage threshold.
func NewTextMerger(thresholdPercent int) *TextMerger {
	defer perf.Track(nil, "merge.NewTextMerger")()

	return &TextMerger{
		thresholdPercent: thresholdPercent,
	}
}

// MergeResult contains the result of a merge operation.
type MergeResult struct {
	Content       string
	HasConflicts  bool
	ConflictCount int
}

// Merge performs a 3-way merge using the diff3 algorithm.
// Parameters:
//   - base: The original content (common ancestor)
//   - ours: The user's version (with their changes)
//   - theirs: The template's version (with template updates)
//
// Returns the merged content or an error if conflicts exceed threshold.
func (m *TextMerger) Merge(base, ours, theirs string) (*MergeResult, error) {
	defer perf.Track(nil, "merge.TextMerger.Merge")()

	// Perform the 3-way merge using diff3.
	// Parameter order: (mine/ours, original/base, yours/theirs).
	mergeResult, err := diff3.Merge(
		strings.NewReader(ours),
		strings.NewReader(base),
		strings.NewReader(theirs),
		false, // Don't show base in conflict markers.
		"Ours",
		"Theirs",
	)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrThreeWayMerge).
			WithCause(err).
			WithExplanation("Failed to perform diff3 merge").
			Err()
	}

	// Read the merged content from the Result reader.
	var buf bytes.Buffer
	_, err = buf.ReadFrom(mergeResult.Result)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrReadFile).
			WithCause(err).
			WithExplanation("Failed to read merge result").
			Err()
	}

	mergedContent := buf.String()

	// Check for conflicts - diff3 provides this info directly.
	hasConflicts := mergeResult.Conflicts
	conflictCount := strings.Count(mergedContent, "<<<<<<<")

	// If there are conflicts, check if they exceed threshold.
	if hasConflicts && m.thresholdPercent > 0 {
		// Calculate change percentage based on conflict size vs total size.
		changePercentage := m.calculateChangePercentage(base, ours, theirs)

		if changePercentage > m.thresholdPercent {
			return nil, errUtils.Build(errUtils.ErrMergeThresholdExceeded).
				WithExplanationf("Too many changes detected (%d%% changes, threshold: %d%%). %d conflicts found", changePercentage, m.thresholdPercent, conflictCount).
				WithHint("Use --force to overwrite or manually merge").
				Err()
		}
	}

	return &MergeResult{
		Content:       mergedContent,
		HasConflicts:  hasConflicts,
		ConflictCount: conflictCount,
	}, nil
}

// calculateChangePercentage calculates the percentage of content that has changed.
// This compares how much base, ours, and theirs differ from each other, relative to base size.
func (m *TextMerger) calculateChangePercentage(base, ours, theirs string) int {
	// Calculate how many lines changed in ours vs base.
	baseLines := strings.Split(base, "\n")
	oursLines := strings.Split(ours, "\n")
	theirsLines := strings.Split(theirs, "\n")

	// Count lines that differ from base.
	oursChanged := countDifferentLines(baseLines, oursLines)
	theirsChanged := countDifferentLines(baseLines, theirsLines)

	// Total changed lines (may overlap in conflicts).
	totalChanged := oursChanged + theirsChanged

	// Calculate percentage based on base size.
	baseSize := len(baseLines)
	if baseSize == 0 {
		baseSize = 1 // Avoid division by zero.
	}

	return int(float64(totalChanged) / float64(baseSize) * 100.0)
}

// countDifferentLines counts how many lines from base are absent in changed
// using an LCS-based approach.  Positional (index-by-index) comparison is
// intentionally avoided: a single insertion near the top shifts every
// downstream line and inflates the count far beyond the real edit size.  LCS
// finds the longest common subsequence and reports only the lines that were
// deleted from base (i.e., len(base) - lcsLength).  Insertions in changed
// are not counted here to avoid double-counting when oursChanged and
// theirsChanged are summed in calculateChangePercentage.
func countDifferentLines(base, changed []string) int {
	m := len(base)
	n := len(changed)

	// dp[i][j] = length of LCS of base[:i] and changed[:j].
	// Allocate a (m+1) x (n+1) table.
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}

	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if base[i-1] == changed[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else {
				if dp[i-1][j] > dp[i][j-1] {
					dp[i][j] = dp[i-1][j]
				} else {
					dp[i][j] = dp[i][j-1]
				}
			}
		}
	}

	lcsLen := dp[m][n]
	// Count only the lines deleted from base; insertions are already implicitly
	// covered by theirsChanged when the two sides are summed.
	return m - lcsLen
}

// HasConflictMarkers checks if the content contains diff3 conflict markers.
func HasConflictMarkers(content string) bool {
	defer perf.Track(nil, "merge.HasConflictMarkers")()

	return strings.Contains(content, "<<<<<<<") ||
		strings.Contains(content, "=======") ||
		strings.Contains(content, ">>>>>>>")
}
