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

func TestSliceRemoveFlag(t *testing.T) {
	testCases := []struct {
		name     string
		input    []string
		flagName string
		expected []string
	}{
		{
			name:     "remove flag without value",
			input:    []string{"--flag1", "value1", "--flag2", "value2"},
			flagName: "flag1",
			expected: []string{"value1", "--flag2", "value2"},
		},
		{
			name:     "remove flag with value",
			input:    []string{"--flag1=value1", "--flag2", "value2"},
			flagName: "flag1",
			expected: []string{"--flag2", "value2"},
		},
		{
			name:     "remove multiple occurrences of same flag",
			input:    []string{"--flag1", "--flag1=value1", "other", "--flag1=value2"},
			flagName: "flag1",
			expected: []string{"other"},
		},
		{
			name:     "remove flag with different values",
			input:    []string{"--flag1=value1", "--flag1=value2", "other"},
			flagName: "flag1",
			expected: []string{"other"},
		},
		{
			name:     "flag not present",
			input:    []string{"--flag1", "value1", "--flag2", "value2"},
			flagName: "flag3",
			expected: []string{"--flag1", "value1", "--flag2", "value2"},
		},
		{
			name:     "empty slice",
			input:    []string{},
			flagName: "flag1",
			expected: []string{},
		},
		{
			name:     "nil slice",
			input:    nil,
			flagName: "flag1",
			expected: nil,
		},
		{
			name:     "only flag without value",
			input:    []string{"--flag1"},
			flagName: "flag1",
			expected: []string{},
		},
		{
			name:     "only flag with value",
			input:    []string{"--flag1=value1"},
			flagName: "flag1",
			expected: []string{},
		},
		{
			name:     "mixed flags and values",
			input:    []string{"--flag1", "--flag2=value2", "--flag1=value1", "other", "--flag2"},
			flagName: "flag1",
			expected: []string{"--flag2=value2", "other", "--flag2"},
		},
		{
			name:     "flag name with special characters",
			input:    []string{"--flag-name", "--flag-name=value", "other"},
			flagName: "flag-name",
			expected: []string{"other"},
		},
	}

	for _, tc := range testCases {
		tc := tc // rebind to avoid range-variable capture
		t.Run(tc.name, func(t *testing.T) {
			result := SliceRemoveFlag(tc.input, tc.flagName)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestSliceRemoveFlagAndValue(t *testing.T) {
	testCases := []struct {
		name     string
		input    []string
		flagName string
		expected []string
	}{
		{
			name:     "remove flag without value",
			input:    []string{"--flag1", "value1", "--flag2", "value2"},
			flagName: "flag1",
			expected: []string{"--flag2", "value2"},
		},
		{
			name:     "remove flag with spaced value",
			input:    []string{"--flag1", "value1", "--flag2", "value2"},
			flagName: "flag1",
			expected: []string{"--flag2", "value2"},
		},
		{
			name:     "remove flag with equals value",
			input:    []string{"--flag1=value1", "--flag2", "value2"},
			flagName: "flag1",
			expected: []string{"--flag2", "value2"},
		},
		{
			name:     "remove flag with spaced value followed by another flag",
			input:    []string{"--flag1", "value1", "--flag2", "value2"},
			flagName: "flag1",
			expected: []string{"--flag2", "value2"},
		},
		{
			name:     "remove flag with spaced value followed by non-flag",
			input:    []string{"--flag1", "value1", "other", "args"},
			flagName: "flag1",
			expected: []string{"other", "args"},
		},
		{
			name:     "remove flag with spaced value followed by another flag",
			input:    []string{"--flag1", "value1", "--flag2", "value2"},
			flagName: "flag1",
			expected: []string{"--flag2", "value2"},
		},
		{
			name:     "remove multiple occurrences",
			input:    []string{"--flag1", "value1", "--flag1", "value2", "other"},
			flagName: "flag1",
			expected: []string{"other"},
		},
		{
			name:     "flag not present",
			input:    []string{"--flag1", "value1", "--flag2", "value2"},
			flagName: "flag3",
			expected: []string{"--flag1", "value1", "--flag2", "value2"},
		},
		{
			name:     "empty slice",
			input:    []string{},
			flagName: "flag1",
			expected: []string{},
		},
		{
			name:     "nil slice",
			input:    nil,
			flagName: "flag1",
			expected: nil,
		},
		{
			name:     "empty flag name",
			input:    []string{"--flag1", "value1"},
			flagName: "",
			expected: []string{"--flag1", "value1"},
		},
		{
			name:     "flag at end without value",
			input:    []string{"other", "--flag1"},
			flagName: "flag1",
			expected: []string{"other"},
		},
		{
			name:     "flag at end with value",
			input:    []string{"other", "--flag1", "value1"},
			flagName: "flag1",
			expected: []string{"other"},
		},
		{
			name:     "mixed flag forms",
			input:    []string{"--flag1", "value1", "--flag1=value2", "other"},
			flagName: "flag1",
			expected: []string{"other"},
		},
	}

	for _, tc := range testCases {
		tc := tc // rebind to avoid range-variable capture
		t.Run(tc.name, func(t *testing.T) {
			result := SliceRemoveFlagAndValue(tc.input, tc.flagName)
			assert.Equal(t, tc.expected, result)
		})
	}
}
