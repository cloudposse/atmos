package column

import (
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestNewSelector(t *testing.T) {
	tests := []struct {
		name      string
		configs   []Config
		funcMap   template.FuncMap
		expectErr bool
		errType   error
	}{
		{
			name: "valid single column",
			configs: []Config{
				{Name: "Component", Value: "{{ .atmos_component }}"},
			},
			funcMap:   BuildColumnFuncMap(),
			expectErr: false,
		},
		{
			name: "valid multiple columns",
			configs: []Config{
				{Name: "Component", Value: "{{ .atmos_component }}"},
				{Name: "Stack", Value: "{{ .atmos_stack }}"},
				{Name: "Region", Value: "{{ .vars.region }}"},
			},
			funcMap:   BuildColumnFuncMap(),
			expectErr: false,
		},
		{
			name:      "empty configs",
			configs:   []Config{},
			funcMap:   BuildColumnFuncMap(),
			expectErr: true,
			errType:   errUtils.ErrInvalidConfig,
		},
		{
			name: "empty column name",
			configs: []Config{
				{Name: "", Value: "{{ .atmos_component }}"},
			},
			funcMap:   BuildColumnFuncMap(),
			expectErr: true,
			errType:   errUtils.ErrInvalidConfig,
		},
		{
			name: "empty column value",
			configs: []Config{
				{Name: "Component", Value: ""},
			},
			funcMap:   BuildColumnFuncMap(),
			expectErr: true,
			errType:   errUtils.ErrInvalidConfig,
		},
		{
			name: "invalid template syntax",
			configs: []Config{
				{Name: "Component", Value: "{{ .atmos_component "},
			},
			funcMap:   BuildColumnFuncMap(),
			expectErr: true,
			errType:   errUtils.ErrInvalidConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selector, err := NewSelector(tt.configs, tt.funcMap)

			if tt.expectErr {
				require.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
				assert.Nil(t, selector)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, selector)
				assert.Equal(t, len(tt.configs), len(selector.configs))
			}
		})
	}
}

func TestSelector_Select(t *testing.T) {
	configs := []Config{
		{Name: "Component", Value: "{{ .atmos_component }}"},
		{Name: "Stack", Value: "{{ .atmos_stack }}"},
		{Name: "Region", Value: "{{ .vars.region }}"},
	}

	selector, err := NewSelector(configs, BuildColumnFuncMap())
	require.NoError(t, err)

	tests := []struct {
		name        string
		columns     []string
		expectErr   bool
		errType     error
		expectedLen int
	}{
		{
			name:        "select single column",
			columns:     []string{"Component"},
			expectErr:   false,
			expectedLen: 1,
		},
		{
			name:        "select multiple columns",
			columns:     []string{"Component", "Stack"},
			expectErr:   false,
			expectedLen: 2,
		},
		{
			name:        "select all columns (empty slice)",
			columns:     []string{},
			expectErr:   false,
			expectedLen: 3,
		},
		{
			name:        "select all columns (nil)",
			columns:     nil,
			expectErr:   false,
			expectedLen: 3,
		},
		{
			name:      "select non-existent column",
			columns:   []string{"NonExistent"},
			expectErr: true,
			errType:   errUtils.ErrInvalidConfig,
		},
		{
			name:      "select mix of valid and invalid columns",
			columns:   []string{"Component", "NonExistent"},
			expectErr: true,
			errType:   errUtils.ErrInvalidConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := selector.Select(tt.columns)

			if tt.expectErr {
				require.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
			} else {
				require.NoError(t, err)
				headers := selector.Headers()
				assert.Len(t, headers, tt.expectedLen)
			}
		})
	}
}

func TestSelector_Extract(t *testing.T) {
	configs := []Config{
		{Name: "Component", Value: "{{ .atmos_component }}"},
		{Name: "Stack", Value: "{{ .atmos_stack }}"},
		{Name: "Enabled", Value: "{{ ternary .enabled \"✓\" \"✗\" }}"},
	}

	selector, err := NewSelector(configs, BuildColumnFuncMap())
	require.NoError(t, err)

	tests := []struct {
		name           string
		data           []map[string]any
		selectColumns  []string
		expectedRows   [][]string
		expectedHeader []string
		expectErr      bool
	}{
		{
			name: "extract single row",
			data: []map[string]any{
				{
					"atmos_component": "vpc",
					"atmos_stack":     "plat-ue2-dev",
					"enabled":         true,
				},
			},
			expectedHeader: []string{"Component", "Stack", "Enabled"},
			expectedRows: [][]string{
				{"vpc", "plat-ue2-dev", "✓"},
			},
			expectErr: false,
		},
		{
			name: "extract multiple rows",
			data: []map[string]any{
				{
					"atmos_component": "vpc",
					"atmos_stack":     "plat-ue2-dev",
					"enabled":         true,
				},
				{
					"atmos_component": "eks",
					"atmos_stack":     "plat-ue2-prod",
					"enabled":         false,
				},
			},
			expectedHeader: []string{"Component", "Stack", "Enabled"},
			expectedRows: [][]string{
				{"vpc", "plat-ue2-dev", "✓"},
				{"eks", "plat-ue2-prod", "✗"},
			},
			expectErr: false,
		},
		{
			name:           "extract empty data",
			data:           []map[string]any{},
			expectedHeader: []string{"Component", "Stack", "Enabled"},
			expectedRows:   [][]string{},
			expectErr:      false,
		},
		{
			name: "extract with column selection",
			data: []map[string]any{
				{
					"atmos_component": "vpc",
					"atmos_stack":     "plat-ue2-dev",
					"enabled":         true,
				},
			},
			selectColumns:  []string{"Component", "Enabled"},
			expectedHeader: []string{"Component", "Enabled"},
			expectedRows: [][]string{
				{"vpc", "✓"},
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.selectColumns) > 0 {
				err := selector.Select(tt.selectColumns)
				require.NoError(t, err)
			} else {
				err := selector.Select(nil)
				require.NoError(t, err)
			}

			headers, rows, err := selector.Extract(tt.data)

			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedHeader, headers)
				assert.Equal(t, tt.expectedRows, rows)
			}
		})
	}
}

func TestSelector_Extract_NestedFields(t *testing.T) {
	configs := []Config{
		{Name: "Component", Value: "{{ .atmos_component }}"},
		{Name: "Region", Value: "{{ .vars.region }}"},
		{Name: "Namespace", Value: "{{ .vars.namespace }}"},
	}

	selector, err := NewSelector(configs, BuildColumnFuncMap())
	require.NoError(t, err)

	data := []map[string]any{
		{
			"atmos_component": "vpc",
			"vars": map[string]any{
				"region":    "us-east-2",
				"namespace": "platform",
			},
		},
	}

	headers, rows, err := selector.Extract(data)
	require.NoError(t, err)
	assert.Equal(t, []string{"Component", "Region", "Namespace"}, headers)
	assert.Equal(t, [][]string{{"vpc", "us-east-2", "platform"}}, rows)
}

func TestSelector_Extract_TemplateFunctions(t *testing.T) {
	configs := []Config{
		{Name: "Component", Value: "{{ .atmos_component | upper }}"},
		{Name: "Description", Value: "{{ truncate .description 10 }}"},
		{Name: "Status", Value: "{{ ternary .enabled \"active\" \"inactive\" }}"},
		{Name: "Count", Value: "{{ toString (len .items) }}"},
	}

	selector, err := NewSelector(configs, BuildColumnFuncMap())
	require.NoError(t, err)

	data := []map[string]any{
		{
			"atmos_component": "vpc",
			"description":     "This is a very long description that should be truncated",
			"enabled":         true,
			"items":           []any{"a", "b", "c"},
		},
	}

	_, rows, err := selector.Extract(data)
	require.NoError(t, err)
	assert.Equal(t, [][]string{{"VPC", "This is...", "active", "3"}}, rows)
}

func TestBuildColumnFuncMap(t *testing.T) {
	funcMap := BuildColumnFuncMap()

	// Verify expected functions exist
	expectedFuncs := []string{
		"toString", "toInt", "toBool",
		"truncate", "pad", "upper", "lower",
		"get", "getOr", "has",
		"len", "join", "split",
		"ternary",
	}

	for _, funcName := range expectedFuncs {
		_, ok := funcMap[funcName]
		assert.True(t, ok, "Function %q should exist in FuncMap", funcName)
	}
}

func TestTemplateFunctions_ToString(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{"nil", nil, ""},
		{"string", "hello", "hello"},
		{"int", 42, "42"},
		{"bool", true, "true"},
		{"float", 3.14, "3.14"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTemplateFunctions_ToInt(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected int
	}{
		{"int", 42, 42},
		{"int64", int64(42), 42},
		{"float64", float64(42.7), 42},
		{"string", "42", 42},
		{"invalid string", "abc", 0},
		{"nil", nil, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toInt(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTemplateFunctions_ToBool(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected bool
	}{
		{"bool true", true, true},
		{"bool false", false, false},
		{"string true", "true", true},
		{"string yes", "yes", true},
		{"string 1", "1", true},
		{"string false", "false", false},
		{"int non-zero", 42, true},
		{"int zero", 0, false},
		{"nil", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toBool(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTemplateFunctions_Truncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		length   int
		expected string
	}{
		{"no truncation needed", "hello", 10, "hello"},
		{"truncate with ellipsis", "hello world", 8, "hello..."},
		{"truncate short", "hello", 3, "hel"}, // length <= 3, no ellipsis
		{"empty string", "", 5, ""},
		{"exact length", "hello", 5, "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncate(tt.input, tt.length)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTemplateFunctions_Pad(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		length   int
		expected string
	}{
		{"no padding needed", "hello", 5, "hello"},
		{"padding needed", "hi", 5, "hi   "},
		{"already longer", "hello world", 5, "hello world"},
		{"empty string", "", 3, "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pad(tt.input, tt.length)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTemplateFunctions_MapGet(t *testing.T) {
	m := map[string]any{
		"key1": "value1",
		"key2": 42,
	}

	tests := []struct {
		name     string
		m        map[string]any
		key      string
		expected any
	}{
		{"existing key", m, "key1", "value1"},
		{"existing key int", m, "key2", 42},
		{"non-existent key", m, "key3", nil},
		{"nil map", nil, "key", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapGet(tt.m, tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTemplateFunctions_MapGetOr(t *testing.T) {
	m := map[string]any{
		"key1": "value1",
	}

	tests := []struct {
		name       string
		m          map[string]any
		key        string
		defaultVal any
		expected   any
	}{
		{"existing key", m, "key1", "default", "value1"},
		{"non-existent key", m, "key2", "default", "default"},
		{"nil map", nil, "key", "default", "default"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapGetOr(tt.m, tt.key, tt.defaultVal)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTemplateFunctions_MapHas(t *testing.T) {
	m := map[string]any{
		"key1": "value1",
	}

	tests := []struct {
		name     string
		m        map[string]any
		key      string
		expected bool
	}{
		{"existing key", m, "key1", true},
		{"non-existent key", m, "key2", false},
		{"nil map", nil, "key", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapHas(tt.m, tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTemplateFunctions_Length(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected int
	}{
		{"string", "hello", 5},
		{"empty string", "", 0},
		{"slice", []any{"a", "b", "c"}, 3},
		{"map", map[string]any{"a": 1, "b": 2}, 2},
		{"nil", nil, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := length(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTemplateFunctions_Ternary(t *testing.T) {
	tests := []struct {
		name      string
		condition bool
		trueVal   any
		falseVal  any
		expected  any
	}{
		{"true condition", true, "yes", "no", "yes"},
		{"false condition", false, "yes", "no", "no"},
		{"true with numbers", true, 1, 0, 1},
		{"false with numbers", false, 1, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ternary(tt.condition, tt.trueVal, tt.falseVal)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTemplateFunction_Pad_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		length   any
		expected string
	}{
		{"int length", "hi", 5, "hi   "},
		{"int64 length", "hi", int64(5), "hi   "},
		{"float64 length", "hi", float64(5.0), "hi   "},
		{"invalid length type", "hi", "invalid", "hi"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pad(tt.input, toInt(tt.length))
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSelector_Extract_MissingFields(t *testing.T) {
	configs := []Config{
		{Name: "Component", Value: "{{ .atmos_component }}"},
		{Name: "Missing", Value: "{{ .nonexistent }}"},
	}

	selector, err := NewSelector(configs, BuildColumnFuncMap())
	require.NoError(t, err)

	data := []map[string]any{
		{
			"atmos_component": "vpc",
		},
	}

	// Should handle missing fields gracefully (Go template returns "<no value>")
	headers, rows, err := selector.Extract(data)
	require.NoError(t, err)
	assert.Equal(t, []string{"Component", "Missing"}, headers)
	assert.Equal(t, [][]string{{"vpc", "<no value>"}}, rows)
}

func TestSelector_Headers(t *testing.T) {
	configs := []Config{
		{Name: "Col1", Value: "{{ .field1 }}"},
		{Name: "Col2", Value: "{{ .field2 }}"},
		{Name: "Col3", Value: "{{ .field3 }}"},
	}

	selector, err := NewSelector(configs, BuildColumnFuncMap())
	require.NoError(t, err)

	// Test all headers
	headers := selector.Headers()
	assert.Equal(t, []string{"Col1", "Col2", "Col3"}, headers)

	// Test selected headers
	err = selector.Select([]string{"Col1", "Col3"})
	require.NoError(t, err)
	headers = selector.Headers()
	assert.Equal(t, []string{"Col1", "Col3"}, headers)
}

func TestBuildTemplateContext(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]any
		expected map[string]any
	}{
		{
			name: "component data",
			data: map[string]any{
				"atmos_component":      "vpc",
				"atmos_stack":          "plat-ue2-dev",
				"atmos_component_type": "real",
				"vars": map[string]any{
					"region": "us-east-2",
				},
				"enabled": true,
			},
			expected: map[string]any{
				"atmos_component":      "vpc",
				"atmos_stack":          "plat-ue2-dev",
				"atmos_component_type": "real",
				"vars": map[string]any{
					"region": "us-east-2",
				},
				"enabled": true,
			},
		},
		{
			name: "workflow data",
			data: map[string]any{
				"file":        "workflows/deploy.yaml",
				"name":        "deploy",
				"description": "Deploy workflow",
			},
			expected: map[string]any{
				"file":        "workflows/deploy.yaml",
				"name":        "deploy",
				"description": "Deploy workflow",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildTemplateContext(tt.data)
			resultMap, ok := result.(map[string]any)
			require.True(t, ok, "Result should be a map")

			// Verify expected fields are present
			for key, expectedVal := range tt.expected {
				actualVal, exists := resultMap[key]
				assert.True(t, exists, "Key %q should exist in context", key)
				assert.Equal(t, expectedVal, actualVal, "Value for key %q should match", key)
			}

			// Verify raw field exists and contains all data
			raw, exists := resultMap["raw"]
			assert.True(t, exists, "raw field should exist in context")
			assert.Equal(t, tt.data, raw, "raw field should contain all original data")
		})
	}
}
