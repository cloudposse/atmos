package utils

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
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

func TestSliceOfInterfacesToSliceOfStringsWithTypeAssertion(t *testing.T) {
	testCases := []struct {
		name        string
		input       []any
		expected    []string
		expectError bool
		errorType   error
	}{
		{
			name:        "nil input",
			input:       nil,
			expected:    nil,
			expectError: true,
			errorType:   errUtils.ErrNilInput,
		},
		{
			name:        "empty slice",
			input:       []any{},
			expected:    []string{},
			expectError: false,
		},
		{
			name:        "all strings",
			input:       []any{"hello", "world", "test"},
			expected:    []string{"hello", "world", "test"},
			expectError: false,
		},
		{
			name:        "single string",
			input:       []any{"single"},
			expected:    []string{"single"},
			expectError: false,
		},
		{
			name:        "non-string element at index 0",
			input:       []any{42, "hello"},
			expected:    nil,
			expectError: true,
			errorType:   errUtils.ErrNonStringElement,
		},
		{
			name:        "non-string element at index 1",
			input:       []any{"hello", 42, "world"},
			expected:    nil,
			expectError: true,
			errorType:   errUtils.ErrNonStringElement,
		},
		{
			name:        "non-string element at end",
			input:       []any{"hello", "world", 3.14},
			expected:    nil,
			expectError: true,
			errorType:   errUtils.ErrNonStringElement,
		},
		{
			name:        "multiple non-string elements",
			input:       []any{42, 3.14, true},
			expected:    nil,
			expectError: true,
			errorType:   errUtils.ErrNonStringElement,
		},
		{
			name:        "mixed types with non-string first",
			input:       []any{true, "hello", "world"},
			expected:    nil,
			expectError: true,
			errorType:   errUtils.ErrNonStringElement,
		},
		{
			name:        "nil element",
			input:       []any{"hello", nil, "world"},
			expected:    nil,
			expectError: true,
			errorType:   errUtils.ErrNonStringElement,
		},
		{
			name:        "slice element",
			input:       []any{"hello", []string{"nested"}, "world"},
			expected:    nil,
			expectError: true,
			errorType:   errUtils.ErrNonStringElement,
		},
		{
			name:        "map element",
			input:       []any{"hello", map[string]string{"key": "value"}, "world"},
			expected:    nil,
			expectError: true,
			errorType:   errUtils.ErrNonStringElement,
		},
	}

	for _, tc := range testCases {
		tc := tc // rebind to avoid range-variable capture
		t.Run(tc.name, func(t *testing.T) {
			result, err := SliceOfInterfacesToSliceOfStringsWithTypeAssertion(tc.input)

			if tc.expectError {
				assertErrorCase(t, &tc, err, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

// assertErrorCase validates error cases for SliceOfInterfacesToSliceOfStringsWithTypeAssertion tests.
func assertErrorCase(t *testing.T, tc *struct {
	name        string
	input       []any
	expected    []string
	expectError bool
	errorType   error
}, err error, result []string,
) {
	assert.Error(t, err)
	assert.Nil(t, result)

	switch tc.errorType {
	case errUtils.ErrNilInput:
		assert.Equal(t, errUtils.ErrNilInput, err)
	case errUtils.ErrNonStringElement:
		assertNonStringElementError(t, tc.input, err)
	}
}

// assertNonStringElementError validates ErrNonStringElement specific assertions.
func assertNonStringElementError(t *testing.T, input []any, err error) {
	// Verify the error wraps ErrNonStringElement
	assert.ErrorIs(t, err, errUtils.ErrNonStringElement)

	// For non-string element errors, verify the error message contains index and type info
	errorMsg := err.Error()
	assert.Contains(t, errorMsg, "index=")
	assert.Contains(t, errorMsg, "got=")

	// Find the actual index and type for verification
	for i, item := range input {
		if _, ok := item.(string); !ok {
			assert.Contains(t, errorMsg, fmt.Sprintf("index=%d", i))
			assert.Contains(t, errorMsg, fmt.Sprintf("got=%T", item))
			break
		}
	}
}
