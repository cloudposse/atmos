package filter

import (
	"fmt"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
)

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

// Chain combines multiple filters (AND logic).
type Chain struct {
	filters []Filter
}

// NewGlobFilter creates a filter that matches field values against glob pattern.
func NewGlobFilter(field, pattern string) (*GlobFilter, error) {
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
	items, ok := data.([]map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: expected []map[string]any, got %T", errUtils.ErrInvalidConfig, data)
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
	return &ColumnValueFilter{
		Column: column,
		Value:  value,
	}
}

// Apply filters data by exact column value match.
func (f *ColumnValueFilter) Apply(data interface{}) (interface{}, error) {
	items, ok := data.([]map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: expected []map[string]any, got %T", errUtils.ErrInvalidConfig, data)
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
	return &BoolFilter{
		Field: field,
		Value: value,
	}
}

// Apply filters data by boolean field value.
func (f *BoolFilter) Apply(data interface{}) (interface{}, error) {
	items, ok := data.([]map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: expected []map[string]any, got %T", errUtils.ErrInvalidConfig, data)
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

// NewChain creates a filter chain that applies filters in sequence (AND logic).
func NewChain(filters ...Filter) *Chain {
	return &Chain{filters: filters}
}

// Apply applies all filters in sequence.
func (c *Chain) Apply(data interface{}) (interface{}, error) {
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
