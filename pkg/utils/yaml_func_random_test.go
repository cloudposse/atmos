package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProcessTagRandom(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantErr     bool
		errContains string
		checkRange  bool
		min         int
		max         int
	}{
		{
			name:       "no arguments - use defaults",
			input:      "!random",
			wantErr:    false,
			checkRange: true,
			min:        0,
			max:        65535,
		},
		{
			name:       "one argument - max only",
			input:      "!random 100",
			wantErr:    false,
			checkRange: true,
			min:        0,
			max:        100,
		},
		{
			name:       "one argument - large max",
			input:      "!random 9999",
			wantErr:    false,
			checkRange: true,
			min:        0,
			max:        9999,
		},
		{
			name:       "two arguments - valid range 1024-65535",
			input:      "!random 1024 65535",
			wantErr:    false,
			checkRange: true,
			min:        1024,
			max:        65535,
		},
		{
			name:       "two arguments - valid range 1-100",
			input:      "!random 1 100",
			wantErr:    false,
			checkRange: true,
			min:        1,
			max:        100,
		},
		{
			name:       "two arguments - valid small range 5-10",
			input:      "!random 5 10",
			wantErr:    false,
			checkRange: true,
			min:        5,
			max:        10,
		},
		{
			name:        "too many arguments",
			input:       "!random 1024 65535 extra",
			wantErr:     true,
			errContains: "invalid number of arguments",
		},
		{
			name:        "invalid max value with one argument",
			input:       "!random xyz",
			wantErr:     true,
			errContains: "invalid max value",
		},
		{
			name:        "invalid min value with two arguments",
			input:       "!random abc 65535",
			wantErr:     true,
			errContains: "invalid min value",
		},
		{
			name:        "invalid max value with two arguments",
			input:       "!random 1024 xyz",
			wantErr:     true,
			errContains: "invalid max value",
		},
		{
			name:        "min >= max",
			input:       "!random 65535 1024",
			wantErr:     true,
			errContains: "min value",
		},
		{
			name:        "min == max",
			input:       "!random 1024 1024",
			wantErr:     true,
			errContains: "min value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ProcessTagRandom(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
				if tt.checkRange {
					assert.GreaterOrEqual(t, result, tt.min, "Result should be >= min")
					assert.LessOrEqual(t, result, tt.max, "Result should be <= max")
				}
			}
		})
	}
}

// TestProcessTagRandomDistribution verifies that the random function produces different values.
func TestProcessTagRandomDistribution(t *testing.T) {
	seen := make(map[int]bool)
	iterations := 100
	input := "!random 1 1000"

	for i := 0; i < iterations; i++ {
		result, err := ProcessTagRandom(input)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, result, 1)
		assert.LessOrEqual(t, result, 1000)
		seen[result] = true
	}

	// With 100 iterations in a range of 1000, we should see multiple different values.
	// This is a statistical test - it's theoretically possible (but extremely unlikely)
	// to get the same number 100 times.
	assert.Greater(t, len(seen), 10, "Expected to see at least 10 different values in 100 iterations")
}
