package merge

import (
	"bytes"
	"strings"

	"github.com/epiclabs-io/diff3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// newlineSeparator splits/joins line-based content throughout this file.
const newlineSeparator = "\n"

// TextMerger handles 3-way merging of text files using the diff3 algorithm.
type TextMerger struct {
	thresholdPercent int              // Percentage threshold (0-100) for change detection.
	conflictStrategy ConflictStrategy // How to resolve a real ours/theirs divergence.
}

// NewTextMerger creates a new text merger with the specified percentage threshold.
func NewTextMerger(thresholdPercent int) *TextMerger {
	defer perf.Track(nil, "merge.NewTextMerger")()

	return &TextMerger{
		thresholdPercent: thresholdPercent,
	}
}

// SetConflictStrategy sets how a real ours/theirs divergence is resolved. The
// zero value (ConflictStrategyManual) is today's existing behavior: diff3's
// inline <<<<<<< / ======= / >>>>>>> conflict markers are left in place for
// the caller to resolve by hand. Ours/theirs instead auto-resolve every
// conflict block to the chosen side, so the flag isn't YAML-only.
func (m *TextMerger) SetConflictStrategy(strategy ConflictStrategy) {
	defer perf.Track(nil, "merge.TextMerger.SetConflictStrategy")()

	m.conflictStrategy = strategy
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
	mergedContent, hasConflicts, conflictCount = m.applyConflictStrategy(mergedContent, hasConflicts, conflictCount)

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

// applyConflictStrategy auto-resolves every conflict block to the chosen
// side when the strategy is ours/theirs, mirroring
// YAMLMerger.pickConflictValue: no conflict is recorded (so the threshold
// gate in Merge is skipped too) and the write proceeds. Manual (default)
// leaves content/hasConflicts/conflictCount untouched.
func (m *TextMerger) applyConflictStrategy(content string, hasConflicts bool, conflictCount int) (string, bool, int) {
	if hasConflicts && m.conflictStrategy != ConflictStrategyManual {
		return resolveTextConflicts(content, m.conflictStrategy), false, 0
	}
	return content, hasConflicts, conflictCount
}

// calculateChangePercentage calculates the percentage of content that has changed.
// This compares how much base, ours, and theirs differ from each other, relative to base size.
func (m *TextMerger) calculateChangePercentage(base, ours, theirs string) int {
	// Calculate how many lines changed in ours vs base.
	baseLines := strings.Split(base, newlineSeparator)
	oursLines := strings.Split(ours, newlineSeparator)
	theirsLines := strings.Split(theirs, newlineSeparator)

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

// countDifferentLines counts how many lines differ between base and changed
// using an LCS-based approach.  Positional (index-by-index) comparison is
// intentionally avoided: a single insertion near the top shifts every
// downstream line and inflates the count far beyond the real edit size.  LCS
// finds the longest common subsequence; lines present in base but not in the
// LCS were deleted, and lines present in changed but not in the LCS were
// inserted.  Both must be counted here: oursChanged and theirsChanged in
// calculateChangePercentage are independent comparisons against base, so an
// insertion-only diff on both sides (base fully contained as a subsequence of
// each) would otherwise report 0 changed lines on both sides even though real
// content was added.
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
	// Lines deleted from base, plus lines inserted in changed.
	return (m - lcsLen) + (n - lcsLen)
}

// resolveTextConflicts rewrites diff3 conflict blocks (<<<<<<< Ours /
// ======= / >>>>>>> Theirs) to keep only the chosen side's content, dropping
// the markers and the other side entirely. Uses the same conflict-block
// line-scanning idiom a 3-way text merger conflict resolver would, but with a
// deterministic pick instead of keeping both sides.
func resolveTextConflicts(content string, strategy ConflictStrategy) string {
	lines := strings.Split(content, newlineSeparator)
	var resolved []string
	var oursLines, theirsLines []string
	var inConflict, inTheirsSide bool

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "<<<<<<<"):
			inConflict = true
			inTheirsSide = false
			oursLines = nil
			theirsLines = nil
			continue
		case strings.HasPrefix(line, "======="):
			inTheirsSide = true
			continue
		case strings.HasPrefix(line, ">>>>>>>"):
			inConflict = false
			if strategy == ConflictStrategyTheirs {
				resolved = append(resolved, theirsLines...)
			} else {
				resolved = append(resolved, oursLines...)
			}
			continue
		}

		switch {
		case !inConflict:
			resolved = append(resolved, line)
		case inTheirsSide:
			theirsLines = append(theirsLines, line)
		default:
			oursLines = append(oursLines, line)
		}
	}

	return strings.Join(resolved, newlineSeparator)
}

// HasConflictMarkers checks if the content contains diff3 conflict markers.
func HasConflictMarkers(content string) bool {
	defer perf.Track(nil, "merge.HasConflictMarkers")()

	return strings.Contains(content, "<<<<<<<") ||
		strings.Contains(content, "=======") ||
		strings.Contains(content, ">>>>>>>")
}
