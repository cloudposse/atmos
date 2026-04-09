package filter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestNewGlobFilter(t *testing.T) {
	tests := []struct {
		name      string
		field     string
		pattern   string
		expectErr bool
		errType   error
	}{
		{"valid simple pattern", "stack", "plat-*", false, nil},
		{"valid complex pattern", "stack", "plat-*-dev", false, nil},
		{"valid exact match", "stack", "exact", false, nil},
		{"empty field", "", "pattern", true, errUtils.ErrInvalidConfig},
		{"empty pattern", "field", "", true, errUtils.ErrInvalidConfig},
		{"invalid pattern", "field", "[", true, errUtils.ErrInvalidConfig},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter, err := NewGlobFilter(tt.field, tt.pattern)

			if tt.expectErr {
				require.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
				assert.Nil(t, filter)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, filter)
				assert.Equal(t, tt.field, filter.Field)
				assert.Equal(t, tt.pattern, filter.Pattern)
			}
		})
	}
}

func TestGlobFilter_Apply(t *testing.T) {
	tests := []struct {
		name          string
		field         string
		pattern       string
		data          []map[string]any
		expectedCount int
		expectErr     bool
	}{
		{
			name:    "match all with wildcard",
			field:   "stack",
			pattern: "*",
			data: []map[string]any{
				{"stack": "plat-ue2-dev"},
				{"stack": "plat-ue2-prod"},
			},
			expectedCount: 2,
		},
		{
			name:    "match pattern",
			field:   "stack",
			pattern: "plat-*-dev",
			data: []map[string]any{
				{"stack": "plat-ue2-dev"},
				{"stack": "plat-ue2-prod"},
				{"stack": "plat-uw2-dev"},
			},
			expectedCount: 2,
		},
		{
			name:    "no matches",
			field:   "stack",
			pattern: "non-*",
			data: []map[string]any{
				{"stack": "plat-ue2-dev"},
				{"stack": "plat-ue2-prod"},
			},
			expectedCount: 0,
		},
		{
			name:    "missing field",
			field:   "missing",
			pattern: "*",
			data: []map[string]any{
				{"stack": "plat-ue2-dev"},
			},
			expectedCount: 0,
		},
		{
			name:    "exact match",
			field:   "stack",
			pattern: "exact",
			data: []map[string]any{
				{"stack": "exact"},
				{"stack": "notexact"},
			},
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter, err := NewGlobFilter(tt.field, tt.pattern)
			require.NoError(t, err)

			result, err := filter.Apply(tt.data)

			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				filtered, ok := result.([]map[string]any)
				require.True(t, ok)
				assert.Len(t, filtered, tt.expectedCount)
			}
		})
	}
}

func TestGlobFilter_Apply_InvalidData(t *testing.T) {
	filter, err := NewGlobFilter("field", "*")
	require.NoError(t, err)

	tests := []struct {
		name string
		data interface{}
	}{
		{"string", "invalid"},
		{"int", 123},
		{"map", map[string]string{"key": "value"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := filter.Apply(tt.data)
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrInvalidConfig)
		})
	}
}

func TestNewColumnFilter(t *testing.T) {
	filter := NewColumnFilter("component", "vpc")
	assert.NotNil(t, filter)
	assert.Equal(t, "component", filter.Column)
	assert.Equal(t, "vpc", filter.Value)
}

func TestColumnValueFilter_Apply(t *testing.T) {
	tests := []struct {
		name          string
		column        string
		value         string
		data          []map[string]any
		expectedCount int
	}{
		{
			name:   "exact match",
			column: "component",
			value:  "vpc",
			data: []map[string]any{
				{"component": "vpc"},
				{"component": "eks"},
				{"component": "vpc"},
			},
			expectedCount: 2,
		},
		{
			name:   "no matches",
			column: "component",
			value:  "nonexistent",
			data: []map[string]any{
				{"component": "vpc"},
				{"component": "eks"},
			},
			expectedCount: 0,
		},
		{
			name:   "missing column",
			column: "missing",
			value:  "value",
			data: []map[string]any{
				{"component": "vpc"},
			},
			expectedCount: 0,
		},
		{
			name:   "numeric value as string",
			column: "port",
			value:  "8080",
			data: []map[string]any{
				{"port": 8080},
				{"port": 9090},
			},
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := NewColumnFilter(tt.column, tt.value)
			result, err := filter.Apply(tt.data)

			require.NoError(t, err)
			filtered, ok := result.([]map[string]any)
			require.True(t, ok)
			assert.Len(t, filtered, tt.expectedCount)
		})
	}
}

func TestNewBoolFilter(t *testing.T) {
	trueVal := true
	falseVal := false

	tests := []struct {
		name  string
		field string
		value *bool
	}{
		{"filter true", "enabled", &trueVal},
		{"filter false", "enabled", &falseVal},
		{"filter all (nil)", "enabled", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := NewBoolFilter(tt.field, tt.value)
			assert.NotNil(t, filter)
			assert.Equal(t, tt.field, filter.Field)
			assert.Equal(t, tt.value, filter.Value)
		})
	}
}

func TestBoolFilter_Apply(t *testing.T) {
	trueVal := true
	falseVal := false

	tests := []struct {
		name          string
		field         string
		value         *bool
		data          []map[string]any
		expectedCount int
	}{
		{
			name:  "filter enabled only",
			field: "enabled",
			value: &trueVal,
			data: []map[string]any{
				{"enabled": true},
				{"enabled": false},
				{"enabled": true},
			},
			expectedCount: 2,
		},
		{
			name:  "filter disabled only",
			field: "enabled",
			value: &falseVal,
			data: []map[string]any{
				{"enabled": true},
				{"enabled": false},
				{"enabled": true},
			},
			expectedCount: 1,
		},
		{
			name:  "nil value returns all",
			field: "enabled",
			value: nil,
			data: []map[string]any{
				{"enabled": true},
				{"enabled": false},
			},
			expectedCount: 2,
		},
		{
			name:  "string true conversion",
			field: "enabled",
			value: &trueVal,
			data: []map[string]any{
				{"enabled": "true"},
				{"enabled": "yes"},
				{"enabled": "1"},
				{"enabled": "false"},
			},
			expectedCount: 3,
		},
		{
			name:  "missing field",
			field: "missing",
			value: &trueVal,
			data: []map[string]any{
				{"enabled": true},
			},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := NewBoolFilter(tt.field, tt.value)
			result, err := filter.Apply(tt.data)

			require.NoError(t, err)
			filtered, ok := result.([]map[string]any)
			require.True(t, ok)
			assert.Len(t, filtered, tt.expectedCount)
		})
	}
}

func TestNewChain(t *testing.T) {
	filter1 := NewColumnFilter("col1", "val1")
	filter2 := NewColumnFilter("col2", "val2")

	chain := NewChain(filter1, filter2)
	assert.NotNil(t, chain)
	assert.Len(t, chain.filters, 2)
}

func TestChain_Apply(t *testing.T) {
	tests := []struct {
		name          string
		filters       []Filter
		data          []map[string]any
		expectedCount int
	}{
		{
			name: "two filters AND logic",
			filters: []Filter{
				NewColumnFilter("type", "real"),
				NewBoolFilter("enabled", boolPtr(true)),
			},
			data: []map[string]any{
				{"type": "real", "enabled": true},      // match both
				{"type": "real", "enabled": false},     // match first only
				{"type": "abstract", "enabled": true},  // match second only
				{"type": "abstract", "enabled": false}, // match neither
			},
			expectedCount: 1,
		},
		{
			name: "three filters cascade",
			filters: []Filter{
				NewColumnFilter("region", "us-east-2"),
				NewColumnFilter("env", "prod"),
				NewBoolFilter("enabled", boolPtr(true)),
			},
			data: []map[string]any{
				{"region": "us-east-2", "env": "prod", "enabled": true},
				{"region": "us-east-2", "env": "prod", "enabled": false},
				{"region": "us-east-2", "env": "dev", "enabled": true},
			},
			expectedCount: 1,
		},
		{
			name:    "empty chain returns all",
			filters: []Filter{},
			data: []map[string]any{
				{"component": "vpc"},
				{"component": "eks"},
			},
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain := NewChain(tt.filters...)
			result, err := chain.Apply(tt.data)

			require.NoError(t, err)
			filtered, ok := result.([]map[string]any)
			require.True(t, ok)
			assert.Len(t, filtered, tt.expectedCount)
		})
	}
}

func TestChain_Apply_ErrorPropagation(t *testing.T) {
	goodFilter := NewColumnFilter("col", "val")

	// Create a chain where second filter will receive wrong type
	chain := NewChain(goodFilter, goodFilter)

	// Pass invalid data type - first filter will fail
	_, err := chain.Apply("invalid")
	require.Error(t, err)
}

// Helper function for test readability.
func boolPtr(b bool) *bool {
	return &b
}
