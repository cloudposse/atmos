package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCoalesce(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected string
	}{
		{
			name:     "first value is non-empty",
			input:    []string{"value1", "value2", "value3"},
			expected: "value1",
		},
		{
			name:     "first value is empty, second is non-empty",
			input:    []string{"", "value2", "value3"},
			expected: "value2",
		},
		{
			name:     "all values are empty",
			input:    []string{"", "", ""},
			expected: "",
		},
		{
			name:     "single non-empty value",
			input:    []string{"value1"},
			expected: "value1",
		},
		{
			name:     "single empty value",
			input:    []string{""},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Coalesce(tt.input...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCoalesceInt(t *testing.T) {
	tests := []struct {
		name     string
		input    []int
		expected int
	}{
		{
			name:     "first value is non-zero",
			input:    []int{1, 2, 3},
			expected: 1,
		},
		{
			name:     "first value is zero, second is non-zero",
			input:    []int{0, 2, 3},
			expected: 2,
		},
		{
			name:     "all values are zero",
			input:    []int{0, 0, 0},
			expected: 0,
		},
		{
			name:     "single non-zero value",
			input:    []int{42},
			expected: 42,
		},
		{
			name:     "single zero value",
			input:    []int{0},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Coalesce(tt.input...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCoalesceBool(t *testing.T) {
	tests := []struct {
		name     string
		input    []bool
		expected bool
	}{
		{
			name:     "first value is true",
			input:    []bool{true, false, true},
			expected: true,
		},
		{
			name:     "first value is false, second is true",
			input:    []bool{false, true, false},
			expected: true,
		},
		{
			name:     "all values are false",
			input:    []bool{false, false, false},
			expected: false,
		},
		{
			name:     "single true value",
			input:    []bool{true},
			expected: true,
		},
		{
			name:     "single false value",
			input:    []bool{false},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Coalesce(tt.input...)
			assert.Equal(t, tt.expected, result)
		})
	}
}
