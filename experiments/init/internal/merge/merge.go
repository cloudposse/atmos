package merge

import (
	"fmt"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// ThreeWayMerger handles 3-way merging of text files
type ThreeWayMerger struct {
	maxChanges int
}

// NewThreeWayMerger creates a new 3-way merger with the specified max changes threshold
func NewThreeWayMerger(maxChanges int) *ThreeWayMerger {
	return &ThreeWayMerger{
		maxChanges: maxChanges,
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

	// If there are too many changes, refuse to merge
	if changeCount > m.maxChanges {
		return "", fmt.Errorf("too many changes detected (%d changes). Use --force to overwrite or manually merge", changeCount)
	}

	// Apply the diff to create a merged result
	mergedContent := dmp.DiffText2(diffs)

	// Check for conflicts by looking for diff markers
	if strings.Contains(mergedContent, "<<<<<<<") || strings.Contains(mergedContent, "=======") || strings.Contains(mergedContent, ">>>>>>>") {
		// There are conflicts, let's handle them intelligently
		mergedContent = m.resolveConflicts(mergedContent, fileName)
	}

	return mergedContent, nil
}

// resolveConflicts handles merge conflicts by preserving user customizations
func (m *ThreeWayMerger) resolveConflicts(content, fileName string) string {
	lines := strings.Split(content, "\n")
	var resolvedLines []string
	var inConflict bool
	var conflictBuffer []string

	for _, line := range lines {
		if strings.HasPrefix(line, "<<<<<<<") {
			inConflict = true
			conflictBuffer = []string{}
			continue
		}

		if strings.HasPrefix(line, "=======") {
			// Middle of conflict - switch to "theirs" side
			conflictBuffer = []string{}
			continue
		}

		if strings.HasPrefix(line, ">>>>>>>") {
			inConflict = false
			// Resolve the conflict by preferring existing content (user customizations)
			resolvedLines = append(resolvedLines, m.resolveConflictBlock(conflictBuffer, fileName)...)
			continue
		}

		if inConflict {
			conflictBuffer = append(conflictBuffer, line)
		} else {
			resolvedLines = append(resolvedLines, line)
		}
	}

	return strings.Join(resolvedLines, "\n")
}

// resolveConflictBlock resolves a single conflict block
func (m *ThreeWayMerger) resolveConflictBlock(conflictLines []string, fileName string) []string {
	var resolved []string

	// Add conflict resolution marker
	resolved = append(resolved, fmt.Sprintf("# CONFLICT RESOLVED for %s", fileName))
	resolved = append(resolved, "# Preserving user customizations and adding new template content")
	resolved = append(resolved, "")

	// For now, preserve all lines from the conflict
	// In a more sophisticated implementation, you'd analyze the content
	// and make intelligent decisions about what to keep
	for _, line := range conflictLines {
		if strings.TrimSpace(line) != "" {
			resolved = append(resolved, line)
		}
	}

	resolved = append(resolved, "")
	return resolved
}
