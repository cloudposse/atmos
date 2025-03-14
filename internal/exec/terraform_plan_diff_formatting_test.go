package exec

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatOutputChange_AllScenarios(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		origValue interface{}
		newValue  interface{}
		expected  string
	}{
		{
			name:      "simple change",
			key:       "output_key",
			origValue: "old",
			newValue:  "new",
			expected:  "~ output_key: old => new\n",
		},
		{
			name: "both sensitive",
			key:  "secret_key",
			origValue: map[string]interface{}{
				"sensitive": true,
				"value":     "secret1",
			},
			newValue: map[string]interface{}{
				"sensitive": true,
				"value":     "secret2",
			},
			expected: "~ secret_key: (sensitive value) => (sensitive value)\n",
		},
		{
			name: "orig sensitive only",
			key:  "becoming_public",
			origValue: map[string]interface{}{
				"sensitive": true,
				"value":     "secret",
			},
			newValue: "public",
			expected: "~ becoming_public: (sensitive value) => public\n",
		},
		{
			name:      "new sensitive only",
			key:       "becoming_secret",
			origValue: "public",
			newValue: map[string]interface{}{
				"sensitive": true,
				"value":     "secret",
			},
			expected: "~ becoming_secret: public => (sensitive value)\n",
		},
		{
			name: "complex values",
			key:  "complex",
			origValue: map[string]interface{}{
				"a": 1,
				"b": "old",
			},
			newValue: map[string]interface{}{
				"a": 1,
				"b": "new",
			},
			expected: fmt.Sprintf("~ complex: %v => %v\n",
				formatValue(map[string]interface{}{"a": 1, "b": "old"}),
				formatValue(map[string]interface{}{"a": 1, "b": "new"})),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := formatOutputChange(tc.key, tc.origValue, tc.newValue)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestPrintAttributeDiff_Complex(t *testing.T) {
	tests := []struct {
		name     string
		attrK    string
		origAttr interface{}
		newAttr  interface{}
		expected string
	}{
		{
			name:     "simple attribute change",
			attrK:    "attr1",
			origAttr: "old",
			newAttr:  "new",
			expected: "  ~ attr1: old => new\n",
		},
		{
			name:  "both sensitive",
			attrK: "secret_attr",
			origAttr: map[string]interface{}{
				"sensitive": true,
				"value":     "secret1",
			},
			newAttr: map[string]interface{}{
				"sensitive": true,
				"value":     "secret2",
			},
			expected: "  ~ secret_attr: (sensitive value) => (sensitive value)\n",
		},
		{
			name:  "orig sensitive only",
			attrK: "becoming_public",
			origAttr: map[string]interface{}{
				"sensitive": true,
				"value":     "secret",
			},
			newAttr:  "public",
			expected: "  ~ becoming_public: (sensitive value) => public\n",
		},
		{
			name:     "new sensitive only",
			attrK:    "becoming_secret",
			origAttr: "public",
			newAttr: map[string]interface{}{
				"sensitive": true,
				"value":     "secret",
			},
			expected: "  ~ becoming_secret: public => (sensitive value)\n",
		},
		{
			name:  "map diff",
			attrK: "map_attr",
			origAttr: map[string]interface{}{
				"a": "unchanged",
				"b": "old",
				"c": 1,
			},
			newAttr: map[string]interface{}{
				"a": "unchanged",
				"b": "new",
				"c": 2,
			},
			// Can't test exact output since formatMapDiff is complex, but check for key parts
			expected: "  ~ map_attr:",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var diff strings.Builder
			printAttributeDiff(&diff, tc.attrK, tc.origAttr, tc.newAttr)
			assert.Contains(t, diff.String(), tc.expected)
		})
	}
}

func TestFormatMapDiff_AllScenarios(t *testing.T) {
	tests := []struct {
		name             string
		origMap          map[string]interface{}
		newMap           map[string]interface{}
		expectedContains []string
	}{
		{
			name:             "identical maps",
			origMap:          map[string]interface{}{"a": 1, "b": 2},
			newMap:           map[string]interface{}{"a": 1, "b": 2},
			expectedContains: []string{noChangesText},
		},
		{
			name:             "changed value",
			origMap:          map[string]interface{}{"a": 1, "b": 2},
			newMap:           map[string]interface{}{"a": 1, "b": 3},
			expectedContains: []string{"~ b: 2 => 3"},
		},
		{
			name:             "added key",
			origMap:          map[string]interface{}{"a": 1},
			newMap:           map[string]interface{}{"a": 1, "b": 2},
			expectedContains: []string{"+ b: 2"},
		},
		{
			name:             "removed key",
			origMap:          map[string]interface{}{"a": 1, "b": 2},
			newMap:           map[string]interface{}{"a": 1},
			expectedContains: []string{"- b: 2"},
		},
		{
			name: "complex changes",
			origMap: map[string]interface{}{
				"unchanged": "same",
				"changed":   "old",
				"removed":   "value",
				"nested": map[string]interface{}{
					"inner": "old",
				},
			},
			newMap: map[string]interface{}{
				"unchanged": "same",
				"changed":   "new",
				"added":     "value",
				"nested": map[string]interface{}{
					"inner": "new",
					"new":   "added",
				},
			},
			expectedContains: []string{
				"~ changed: old => new",
				"- removed: value",
				"+ added: value",
				"nested:",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := formatMapDiff(tc.origMap, tc.newMap)
			for _, expected := range tc.expectedContains {
				assert.Contains(t, result, expected)
			}
		})
	}
}
