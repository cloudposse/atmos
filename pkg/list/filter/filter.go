package filter

import (
	"fmt"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/tags"
)

// errFmtExpectedRows is the shared "wrong input type" error format used by every Filter.Apply.
const errFmtExpectedRows = "%w: expected []map[string]any, got %T"

// Filter interface for composability.
type Filter interface {
	Apply(data interface{}) (interface{}, error)
}

// GlobFilter matches patterns (e.g., "plat-*-dev").
type GlobFilter struct {
	Field   string
	Pattern string
}

// ColumnValueFilter filters rows by column value.
type ColumnValueFilter struct {
	Column string
	Value  string
}

// BoolFilter filters by boolean field.
type BoolFilter struct {
	Field string
	Value *bool // nil = all, true = enabled only, false = disabled only
}

// TagFilter filters rows whose Field ([]string) matches any of Tags (OR).
type TagFilter struct {
	Field string
	Tags  []string
}

// LabelFilter filters rows whose Field (map[string]string) matches every
// key=value pair in Labels (AND).
type LabelFilter struct {
	Field  string
	Labels map[string]string
}

// Chain combines multiple filters (AND logic).
type Chain struct {
	filters []Filter
}

// NewGlobFilter creates a filter that matches field values against glob pattern.
func NewGlobFilter(field, pattern string) (*GlobFilter, error) {
	defer perf.Track(nil, "filter.NewGlobFilter")()

	if field == "" {
		return nil, fmt.Errorf("%w: field cannot be empty", errUtils.ErrInvalidConfig)
	}
	if pattern == "" {
		return nil, fmt.Errorf("%w: pattern cannot be empty", errUtils.ErrInvalidConfig)
	}

	// Validate pattern syntax
	_, err := filepath.Match(pattern, "test")
	if err != nil {
		return nil, fmt.Errorf("%w: invalid glob pattern %q: %w", errUtils.ErrInvalidConfig, pattern, err)
	}

	return &GlobFilter{
		Field:   field,
		Pattern: pattern,
	}, nil
}

// Apply filters data by glob pattern matching.
func (f *GlobFilter) Apply(data interface{}) (interface{}, error) {
	defer perf.Track(nil, "filter.GlobFilter.Apply")()

	items, ok := data.([]map[string]any)
	if !ok {
		return nil, fmt.Errorf(errFmtExpectedRows, errUtils.ErrInvalidConfig, data)
	}

	var filtered []map[string]any
	for _, item := range items {
		value, ok := item[f.Field]
		if !ok {
			continue // Skip items without the field
		}

		valueStr := fmt.Sprintf("%v", value)
		matched, err := filepath.Match(f.Pattern, valueStr)
		if err != nil {
			return nil, fmt.Errorf("%w: pattern matching failed: %w", errUtils.ErrInvalidConfig, err)
		}

		if matched {
			filtered = append(filtered, item)
		}
	}

	return filtered, nil
}

// NewColumnFilter creates a filter for exact column value matching.
func NewColumnFilter(column, value string) *ColumnValueFilter {
	defer perf.Track(nil, "filter.NewColumnFilter")()

	return &ColumnValueFilter{
		Column: column,
		Value:  value,
	}
}

// Apply filters data by exact column value match.
func (f *ColumnValueFilter) Apply(data interface{}) (interface{}, error) {
	defer perf.Track(nil, "filter.ColumnValueFilter.Apply")()

	items, ok := data.([]map[string]any)
	if !ok {
		return nil, fmt.Errorf(errFmtExpectedRows, errUtils.ErrInvalidConfig, data)
	}

	var filtered []map[string]any
	for _, item := range items {
		value, ok := item[f.Column]
		if !ok {
			continue
		}

		valueStr := fmt.Sprintf("%v", value)
		if valueStr == f.Value {
			filtered = append(filtered, item)
		}
	}

	return filtered, nil
}

// NewBoolFilter creates a filter for boolean field values.
// Value nil = all, true = only true values, false = only false values.
func NewBoolFilter(field string, value *bool) *BoolFilter {
	defer perf.Track(nil, "filter.NewBoolFilter")()

	return &BoolFilter{
		Field: field,
		Value: value,
	}
}

// Apply filters data by boolean field value.
func (f *BoolFilter) Apply(data interface{}) (interface{}, error) {
	defer perf.Track(nil, "filter.BoolFilter.Apply")()

	items, ok := data.([]map[string]any)
	if !ok {
		return nil, fmt.Errorf(errFmtExpectedRows, errUtils.ErrInvalidConfig, data)
	}

	// nil = no filtering
	if f.Value == nil {
		return items, nil
	}

	var filtered []map[string]any
	for _, item := range items {
		value, ok := item[f.Field]
		if !ok {
			continue
		}

		boolValue, ok := value.(bool)
		if !ok {
			// Try string conversion
			strValue := strings.ToLower(fmt.Sprintf("%v", value))
			boolValue = strValue == "true" || strValue == "yes" || strValue == "1"
		}

		if boolValue == *f.Value {
			filtered = append(filtered, item)
		}
	}

	return filtered, nil
}

// NewTagFilter creates a filter matching rows whose Field contains any of
// filterTags. An empty filterTags matches everything (no filter applied).
func NewTagFilter(field string, filterTags []string) *TagFilter {
	defer perf.Track(nil, "filter.NewTagFilter")()

	return &TagFilter{Field: field, Tags: filterTags}
}

// Apply filters rows by any-match against the Field's []string value.
func (f *TagFilter) Apply(data interface{}) (interface{}, error) {
	defer perf.Track(nil, "filter.TagFilter.Apply")()

	items, ok := data.([]map[string]any)
	if !ok {
		return nil, fmt.Errorf(errFmtExpectedRows, errUtils.ErrInvalidConfig, data)
	}

	if len(f.Tags) == 0 {
		return items, nil
	}

	var filtered []map[string]any
	for _, item := range items {
		itemTags, _ := item[f.Field].([]string)
		if tags.MatchesTags(itemTags, f.Tags, tags.TagModeAny) {
			filtered = append(filtered, item)
		}
	}

	return filtered, nil
}

// NewLabelFilter creates a filter matching rows whose Field contains every
// key=value pair in filterLabels. An empty filterLabels matches everything
// (no filter applied).
func NewLabelFilter(field string, filterLabels map[string]string) *LabelFilter {
	defer perf.Track(nil, "filter.NewLabelFilter")()

	return &LabelFilter{Field: field, Labels: filterLabels}
}

// Apply filters rows by all-match against the Field's map[string]string value.
func (f *LabelFilter) Apply(data interface{}) (interface{}, error) {
	defer perf.Track(nil, "filter.LabelFilter.Apply")()

	items, ok := data.([]map[string]any)
	if !ok {
		return nil, fmt.Errorf(errFmtExpectedRows, errUtils.ErrInvalidConfig, data)
	}

	if len(f.Labels) == 0 {
		return items, nil
	}

	var filtered []map[string]any
	for _, item := range items {
		itemLabels, _ := item[f.Field].(map[string]string)
		if tags.MatchesLabels(itemLabels, f.Labels) {
			filtered = append(filtered, item)
		}
	}

	return filtered, nil
}

// NewChain creates a filter chain that applies filters in sequence (AND logic).
func NewChain(filters ...Filter) *Chain {
	defer perf.Track(nil, "filter.NewChain")()

	return &Chain{filters: filters}
}

// Apply applies all filters in sequence.
func (c *Chain) Apply(data interface{}) (interface{}, error) {
	defer perf.Track(nil, "filter.Chain.Apply")()

	current := data
	for i, filter := range c.filters {
		result, err := filter.Apply(current)
		if err != nil {
			return nil, fmt.Errorf("filter %d failed: %w", i, err)
		}
		current = result
	}
	return current, nil
}
