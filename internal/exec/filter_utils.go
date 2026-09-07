package exec

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	DefaultIncludeEmpty = true
	// DefaultEagerEvaluation preserves the optimized (scope-before-evaluate) behavior by
	// default; see schema.DescribeSettings.EagerEvaluation.
	DefaultEagerEvaluation = false
)

// SectionFilter defines the interface for filtering map sections.
type SectionFilter interface {
	Filter(data map[string]any) map[string]any
}

type sectionFilter struct{}

func (f *sectionFilter) Filter(data map[string]any) map[string]any {
	defer perf.Track(nil, "exec.Filter")()

	result := make(map[string]any)

	for key, originalValue := range data {
		filteredValue := f.filterValue(originalValue)
		if filteredValue != nil || originalValue == nil {
			result[key] = filteredValue
		}
	}

	return result
}

func (f *sectionFilter) filterValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		if filteredMap := f.Filter(v); len(filteredMap) > 0 {
			return filteredMap
		}
		return nil
	case string:
		if v != "" {
			return v
		}
		return nil
	default:
		return value
	}
}

func FilterEmptySections(data map[string]any, includeEmpty bool) map[string]any {
	defer perf.Track(nil, "exec.FilterEmptySections")()

	if includeEmpty {
		return data
	}

	filter := &sectionFilter{}
	return filter.Filter(data)
}

func GetIncludeEmptySetting(atmosConfig *schema.AtmosConfiguration) bool {
	defer perf.Track(atmosConfig, "exec.GetIncludeEmptySetting")()

	if atmosConfig == nil || atmosConfig.Describe.Settings.IncludeEmpty == nil {
		return DefaultIncludeEmpty
	}
	return *atmosConfig.Describe.Settings.IncludeEmpty
}

// GetEagerEvaluationSetting reports whether the tags/labels scope-before-evaluate
// optimization should be disabled (see schema.DescribeSettings.EagerEvaluation).
func GetEagerEvaluationSetting(atmosConfig *schema.AtmosConfiguration) bool {
	defer perf.Track(atmosConfig, "exec.GetEagerEvaluationSetting")()

	if atmosConfig == nil || atmosConfig.Describe.Settings.EagerEvaluation == nil {
		return DefaultEagerEvaluation
	}
	return *atmosConfig.Describe.Settings.EagerEvaluation
}
