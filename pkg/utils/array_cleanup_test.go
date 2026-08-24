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

func TestIsArrayOrSlice(t *testing.T) {
	tests := []struct {
		name string
		val  interface{}
		want bool
	}{
		{"[]interface{} is a slice", []interface{}{1, "two"}, true},
		{"[]string is a slice", []string{"a", "b"}, true},
		{"[]int is a slice", []int{1, 2}, true},
		{"[]float64 is a slice", []float64{1.5}, true},
		{"[]bool is a slice", []bool{true, false}, true},
		{"string is not a slice", "value", false},
		{"map is not a slice", map[string]interface{}{"k": "v"}, false},
		{"int is not a slice", 42, false},
		{"nil is not a slice", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isArrayOrSlice(tt.val); got != tt.want {
				t.Errorf("isArrayOrSlice(%v) = %v, want %v", tt.val, got, tt.want)
			}
		})
	}
}
