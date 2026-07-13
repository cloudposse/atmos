package merge

import (
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ThreeWayMerger handles 3-way merging with automatic file type detection.
type ThreeWayMerger struct {
	thresholdPercent int              // Percentage threshold (0-100) for change detection
	conflictStrategy ConflictStrategy // How to resolve a real ours/theirs divergence
}

// NewThreeWayMerger creates a new 3-way merger with the specified percentage threshold.
func NewThreeWayMerger(thresholdPercent int) *ThreeWayMerger {
	defer perf.Track(nil, "merge.NewThreeWayMerger")()

	return &ThreeWayMerger{
		thresholdPercent: thresholdPercent,
	}
}

// SetConflictStrategy sets how a real ours/theirs divergence is resolved.
// The zero value (ConflictStrategyManual) is today's existing behavior:
// record the conflict rather than silently picking a side.
func (m *ThreeWayMerger) SetConflictStrategy(strategy ConflictStrategy) {
	defer perf.Track(nil, "merge.ThreeWayMerger.SetConflictStrategy")()

	m.conflictStrategy = strategy
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
	defer perf.Track(nil, "merge.ThreeWayMerger.Merge")()

	// Detect file type based on extension
	ext := strings.ToLower(filepath.Ext(fileName))

	if ext == ".yaml" || ext == ".yml" {
		// Use YAML-aware merge for YAML files
		merger := NewYAMLMerger(m.thresholdPercent)
		merger.SetConflictStrategy(m.conflictStrategy)
		return merger.Merge(base, ours, theirs)
	}

	// Use text-based merge for all other files
	merger := NewTextMerger(m.thresholdPercent)
	merger.SetConflictStrategy(m.conflictStrategy)
	return merger.Merge(base, ours, theirs)
}

// MergeWithStrategy allows explicit strategy selection, bypassing auto-detection.
// This is useful when the file extension doesn't accurately represent the content type.
func (m *ThreeWayMerger) MergeWithStrategy(base, ours, theirs string, strategy MergeStrategy) (*MergeResult, error) {
	defer perf.Track(nil, "merge.ThreeWayMerger.MergeWithStrategy")()

	switch strategy {
	case StrategyYAML:
		merger := NewYAMLMerger(m.thresholdPercent)
		merger.SetConflictStrategy(m.conflictStrategy)
		return merger.Merge(base, ours, theirs)
	case StrategyText:
		merger := NewTextMerger(m.thresholdPercent)
		merger.SetConflictStrategy(m.conflictStrategy)
		return merger.Merge(base, ours, theirs)
	default:
		return nil, errUtils.Build(errUtils.ErrUnknownMergeStrategy).
			WithExplanationf("Unknown merge strategy: %v", strategy).
			Err()
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

// ConflictStrategy controls how a genuine ours/theirs divergence is resolved
// during a 3-way merge. Named distinctly from MergeStrategy (above), which
// instead picks WHICH merger algorithm runs (YAML vs text) — an unrelated axis.
type ConflictStrategy int

const (
	// ConflictStrategyManual is the default (zero value): surface the
	// conflict rather than silently picking a side. This is today's existing
	// behavior on any real divergence, so it's the safe default — defaulting
	// to "ours" or "theirs" instead would silently change behavior for
	// existing callers (a merge that errors out today would newly succeed
	// silently, a data-loss risk).
	ConflictStrategyManual ConflictStrategy = iota
	// ConflictStrategyOurs auto-resolves by keeping the user's value.
	ConflictStrategyOurs
	// ConflictStrategyTheirs auto-resolves by keeping the template's value.
	ConflictStrategyTheirs
)

// ParseConflictStrategy parses a --merge-strategy flag value. An empty string
// (flag not set) maps to the default ConflictStrategyManual.
func ParseConflictStrategy(s string) (ConflictStrategy, error) {
	defer perf.Track(nil, "merge.ParseConflictStrategy")()

	switch s {
	case "", "manual":
		return ConflictStrategyManual, nil
	case "ours":
		return ConflictStrategyOurs, nil
	case "theirs":
		return ConflictStrategyTheirs, nil
	default:
		return ConflictStrategyManual, errUtils.Build(errUtils.ErrUnknownMergeStrategy).
			WithExplanationf("Invalid --merge-strategy value: `%s`", s).
			WithHint("Valid values are: manual, ours, theirs").
			WithExitCode(2).
			Err()
	}
}
