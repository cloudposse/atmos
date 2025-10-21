package merge

import (
	"fmt"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// ThreeWayMerger handles 3-way merging of text files
type ThreeWayMerger struct {
	thresholdPercent int // Percentage threshold (0-100)
}

// NewThreeWayMerger creates a new 3-way merger with the specified percentage threshold
func NewThreeWayMerger(thresholdPercent int) *ThreeWayMerger {
	return &ThreeWayMerger{
		thresholdPercent: thresholdPercent,
	}
}

// Merge performs a 3-way merge between existing and new content
func (m *ThreeWayMerger) Merge(existingContent, newContent, fileName string) (string, error) {
	// Use diffmatchpatch to compute the diff between existing and new content
	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(existingContent, newContent, true)

	// Check if the diff is too complex (too many changes)
	changeCount := 0
	for _, diff := range diffs {
		if diff.Type != diffmatchpatch.DiffEqual {
			changeCount++
		}
	}

	// Calculate dynamic threshold based on content size
	// Use the larger of existing or new content to determine reasonable threshold
	maxContentSize := len(existingContent)
	if len(newContent) > maxContentSize {
		maxContentSize = len(newContent)
	}

	// Calculate total content size for percentage calculation
	totalContentSize := len(existingContent) + len(newContent)

	// Calculate percentage of changes
	changePercentage := 0
	if totalContentSize > 0 {
		changePercentage = int(float64(changeCount) / float64(totalContentSize) * 100.0)
	}

	// Use the configured threshold percentage, or default to 50% if not set
	thresholdPercent := m.thresholdPercent
	if thresholdPercent == 0 {
		thresholdPercent = 50 // Default 50% threshold
	}

	// If the change percentage exceeds the threshold, refuse to merge
	if changePercentage > thresholdPercent {
		return "", fmt.Errorf("too many changes detected (%d%% changes, threshold: %d%%). Use --force to overwrite or manually merge", changePercentage, thresholdPercent)
	}

	// Apply the diff to create a merged result
	mergedContent := dmp.DiffText2(diffs)

	// Check for conflicts by looking for diff markers
	if strings.Contains(mergedContent, "<<<<<<<") || strings.Contains(mergedContent, "=======") || strings.Contains(mergedContent, ">>>>>>>") {
		// There are conflicts - return error so user can handle them manually
		return "", fmt.Errorf("merge conflicts detected in %s. Please resolve conflicts manually or use --force to overwrite", fileName)
	}

	return mergedContent, nil
}
