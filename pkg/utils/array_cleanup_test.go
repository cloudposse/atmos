package utils

import (
	"reflect"
	"testing"
)

func TestCleanupArrayIndexKeys(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected interface{}
	}{
		{
			name: "cleanup indexed keys when array exists",
			input: map[string]interface{}{
				"steps": []interface{}{
					map[string]interface{}{"run": "command1"},
					map[string]interface{}{"run": "command2"},
				},
				"steps[0]": map[string]interface{}{"run": "command1"},
				"steps[1]": map[string]interface{}{"run": "command2"},
			},
			expected: map[string]interface{}{
				"steps": []interface{}{
					map[string]interface{}{"run": "command1"},
					map[string]interface{}{"run": "command2"},
				},
			},
		},
		{
			name: "nested cleanup",
			input: map[string]interface{}{
				"workflow-1": map[string]interface{}{
					"apply": map[string]interface{}{
						"steps": []interface{}{
							map[string]interface{}{"run": "apply"},
						},
						"steps[0]": map[string]interface{}{"run": "apply"},
					},
					"plan": map[string]interface{}{
						"steps": []interface{}{
							map[string]interface{}{"run": "init"},
							map[string]interface{}{"run": "plan"},
						},
						"steps[0]": map[string]interface{}{"run": "init"},
						"steps[1]": map[string]interface{}{"run": "plan"},
					},
				},
			},
			expected: map[string]interface{}{
				"workflow-1": map[string]interface{}{
					"apply": map[string]interface{}{
						"steps": []interface{}{
							map[string]interface{}{"run": "apply"},
						},
					},
					"plan": map[string]interface{}{
						"steps": []interface{}{
							map[string]interface{}{"run": "init"},
							map[string]interface{}{"run": "plan"},
						},
					},
				},
			},
		},
		{
			name: "preserve indexed keys when no array exists",
			input: map[string]interface{}{
				"items[0]": "first",
				"items[1]": "second",
				"other":    "value",
			},
			expected: map[string]interface{}{
				"items[0]": "first",
				"items[1]": "second",
				"other":    "value",
			},
		},
		{
			name: "handle mixed content",
			input: map[string]interface{}{
				"normal":   "value",
				"array":    []interface{}{"a", "b"},
				"array[0]": "a",
				"array[1]": "b",
				"map": map[string]interface{}{
					"key": "value",
				},
			},
			expected: map[string]interface{}{
				"normal": "value",
				"array":  []interface{}{"a", "b"},
				"map": map[string]interface{}{
					"key": "value",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CleanupArrayIndexKeys(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("CleanupArrayIndexKeys() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGetArrayBaseFieldName(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		wantBase  string
		wantIndex int
		wantOk    bool
	}{
		{
			name:      "valid indexed key",
			key:       "steps[0]",
			wantBase:  "steps",
			wantIndex: 0,
			wantOk:    true,
		},
		{
			name:      "valid indexed key with higher index",
			key:       "items[123]",
			wantBase:  "items",
			wantIndex: 123,
			wantOk:    true,
		},
		{
			name:      "nested field indexed key",
			key:       "deeply.nested.field[5]",
			wantBase:  "deeply.nested.field",
			wantIndex: 5,
			wantOk:    true,
		},
		{
			name:      "not an indexed key",
			key:       "regular_field",
			wantBase:  "",
			wantIndex: -1,
			wantOk:    false,
		},
		{
			name:      "brackets but no index",
			key:       "field[]",
			wantBase:  "",
			wantIndex: -1,
			wantOk:    false,
		},
		{
			name:      "non-numeric index",
			key:       "field[abc]",
			wantBase:  "",
			wantIndex: -1,
			wantOk:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBase, gotIndex, gotOk := GetArrayBaseFieldName(tt.key)
			if gotBase != tt.wantBase || gotIndex != tt.wantIndex || gotOk != tt.wantOk {
				t.Errorf("GetArrayBaseFieldName(%q) = (%q, %d, %v), want (%q, %d, %v)",
					tt.key, gotBase, gotIndex, gotOk, tt.wantBase, tt.wantIndex, tt.wantOk)
			}
		})
	}
}

func TestRebuildArrayFromIndexedKeys(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name: "rebuild array from indexed keys",
			input: map[string]interface{}{
				"items[0]": "first",
				"items[1]": "second",
				"items[2]": "third",
				"other":    "value",
			},
			expected: map[string]interface{}{
				"items": []interface{}{"first", "second", "third"},
				"other": "value",
			},
		},
		{
			name: "skip rebuild when array exists",
			input: map[string]interface{}{
				"items":    []interface{}{"a", "b"},
				"items[0]": "ignored",
				"items[1]": "ignored",
			},
			expected: map[string]interface{}{
				"items": []interface{}{"a", "b"},
			},
		},
		{
			name: "handle sparse arrays",
			input: map[string]interface{}{
				"sparse[0]": "first",
				"sparse[2]": "third",
			},
			expected: map[string]interface{}{
				"sparse": []interface{}{"first", nil, "third"},
			},
		},
		{
			name: "mixed indexed and regular keys",
			input: map[string]interface{}{
				"regular": "value",
				"list[0]": map[string]interface{}{"key": "value1"},
				"list[1]": map[string]interface{}{"key": "value2"},
				"another": "field",
			},
			expected: map[string]interface{}{
				"regular": "value",
				"list": []interface{}{
					map[string]interface{}{"key": "value1"},
					map[string]interface{}{"key": "value2"},
				},
				"another": "field",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RebuildArrayFromIndexedKeys(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("RebuildArrayFromIndexedKeys() = %v, want %v", result, tt.expected)
			}
		})
	}
}
