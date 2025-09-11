package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSliceOfInterfacesToSliceOfStrings(t *testing.T) {
	var input []any
	input = append(input, "a")
	input = append(input, "b")
	input = append(input, "c")

	result := SliceOfInterfacesToSliceOfStrings(input)
	assert.Equal(t, len(input), len(result))
	assert.Equal(t, input[0].(string), result[0])
	assert.Equal(t, input[1].(string), result[1])
	assert.Equal(t, input[2].(string), result[2])
}

func TestSliceRemoveString(t *testing.T) {
	testCases := []struct {
		name     string
		input    []string
		remove   string
		expected []string
	}{
		{
			name:     "remove existing string",
			input:    []string{"a", "b", "c"},
			remove:   "b",
			expected: []string{"a", "c"},
		},
		{
			name:     "remove non-existent string",
			input:    []string{"a", "b", "c"},
			remove:   "d",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "remove from empty slice",
			input:    []string{},
			remove:   "a",
			expected: []string{},
		},
		{
			name:     "remove with duplicates",
			input:    []string{"a", "b", "a", "c"},
			remove:   "a",
			expected: []string{"b", "a", "c"},
		},
		{
			name:     "remove last element",
			input:    []string{"a", "b", "c"},
			remove:   "c",
			expected: []string{"a", "b"},
		},
	}

	for _, tc := range testCases {
		tc := tc // rebind to avoid range-variable capture
		t.Run(tc.name, func(t *testing.T) {
			result := SliceRemoveString(tc.input, tc.remove)
			assert.Equal(t, tc.expected, result)
		})
	}
}
