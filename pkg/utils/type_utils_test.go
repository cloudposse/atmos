package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCoalesce(t *testing.T) {
	t.Run("string type", func(t *testing.T) {
		tests := []struct {
			name     string
			args     []string
			expected string
		}{
			{
				name:     "first non-empty string",
				args:     []string{"", "", "foo", "bar"},
				expected: "foo",
			},
			{
				name:     "all empty strings",
				args:     []string{"", "", ""},
				expected: "",
			},
			{
				name:     "first element non-empty",
				args:     []string{"first", "second", "third"},
				expected: "first",
			},
			{
				name:     "single non-empty",
				args:     []string{"only"},
				expected: "only",
			},
			{
				name:     "single empty",
				args:     []string{""},
				expected: "",
			},
			{
				name:     "empty slice",
				args:     []string{},
				expected: "",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := Coalesce(tt.args...)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("int type", func(t *testing.T) {
		tests := []struct {
			name     string
			args     []int
			expected int
		}{
			{
				name:     "first non-zero int",
				args:     []int{0, 0, 42, 100},
				expected: 42,
			},
			{
				name:     "all zero",
				args:     []int{0, 0, 0},
				expected: 0,
			},
			{
				name:     "first element non-zero",
				args:     []int{1, 2, 3},
				expected: 1,
			},
			{
				name:     "negative numbers",
				args:     []int{0, -1, -2},
				expected: -1,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := Coalesce(tt.args...)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("bool type", func(t *testing.T) {
		tests := []struct {
			name     string
			args     []bool
			expected bool
		}{
			{
				name:     "first true",
				args:     []bool{false, false, true, false},
				expected: true,
			},
			{
				name:     "all false",
				args:     []bool{false, false, false},
				expected: false,
			},
			{
				name:     "first element true",
				args:     []bool{true, false, false},
				expected: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := Coalesce(tt.args...)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("float64 type", func(t *testing.T) {
		tests := []struct {
			name     string
			args     []float64
			expected float64
		}{
			{
				name:     "first non-zero float",
				args:     []float64{0.0, 0.0, 3.14, 2.71},
				expected: 3.14,
			},
			{
				name:     "all zero",
				args:     []float64{0.0, 0.0},
				expected: 0.0,
			},
			{
				name:     "negative numbers",
				args:     []float64{0.0, -1.5, -2.5},
				expected: -1.5,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := Coalesce(tt.args...)
				assert.Equal(t, tt.expected, result)
			})
		}
	})
}
