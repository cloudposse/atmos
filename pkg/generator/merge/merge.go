package merge

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ThreeWayMerger handles 3-way merging with automatic file type detection.
type ThreeWayMerger struct {
	thresholdPercent int // Percentage threshold (0-100) for change detection
}

// NewThreeWayMerger creates a new 3-way merger with the specified percentage threshold.
func NewThreeWayMerger(thresholdPercent int) *ThreeWayMerger {
	return &ThreeWayMerger{
		thresholdPercent: thresholdPercent,
	}
}

// Merge performs a 3-way merge with automatic file type detection.
// Parameters:
//   - base: The original content (common ancestor)
//   - ours: The user's version (with their changes)
//   - theirs: The template's version (with template updates)
//   - fileName: The file name (used for type detection)
//
// The merger automatically selects the appropriate strategy:
//   - YAML files (.yaml, .yml): Structure-aware YAML merge with comment preservation
//   - All other files: Text-based diff3 merge
//
// Returns the merged content or an error if conflicts exceed threshold.
func (m *ThreeWayMerger) Merge(base, ours, theirs, fileName string) (*MergeResult, error) {
	// Detect file type based on extension
	ext := strings.ToLower(filepath.Ext(fileName))

	if ext == ".yaml" || ext == ".yml" {
		// Use YAML-aware merge for YAML files
		merger := NewYAMLMerger(m.thresholdPercent)
		return merger.Merge(base, ours, theirs)
	}

	// Use text-based merge for all other files
	merger := NewTextMerger(m.thresholdPercent)
	return merger.Merge(base, ours, theirs)
}

// MergeWithStrategy allows explicit strategy selection, bypassing auto-detection.
// This is useful when the file extension doesn't accurately represent the content type.
func (m *ThreeWayMerger) MergeWithStrategy(base, ours, theirs string, strategy MergeStrategy) (*MergeResult, error) {
	switch strategy {
	case StrategyYAML:
		merger := NewYAMLMerger(m.thresholdPercent)
		return merger.Merge(base, ours, theirs)
	case StrategyText:
		merger := NewTextMerger(m.thresholdPercent)
		return merger.Merge(base, ours, theirs)
	default:
		return nil, fmt.Errorf("unknown merge strategy: %v", strategy)
	}
}

// MergeStrategy represents the merge algorithm to use.
type MergeStrategy int

const (
	// StrategyText uses text-based diff3 merge.
	StrategyText MergeStrategy = iota
	// StrategyYAML uses structure-aware YAML merge.
	StrategyYAML
)
