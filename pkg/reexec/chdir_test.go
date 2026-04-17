package reexec

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripChdirArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected []string
	}{
		{
			name:     "no chdir flags",
			args:     []string{"atmos", "terraform", "plan", "--stack", "dev"},
			expected: []string{"atmos", "terraform", "plan", "--stack", "dev"},
		},
		{
			name:     "long form with separate value",
			args:     []string{"atmos", "--chdir", "examples/demo-stacks", "terraform", "plan"},
			expected: []string{"atmos", "terraform", "plan"},
		},
		{
			name:     "long form with equals",
			args:     []string{"atmos", "--chdir=examples/demo-stacks", "terraform", "plan"},
			expected: []string{"atmos", "terraform", "plan"},
		},
		{
			name:     "short form with separate value",
			args:     []string{"atmos", "-C", "examples/demo-stacks", "terraform", "plan"},
			expected: []string{"atmos", "terraform", "plan"},
		},
		{
			name:     "short form with equals",
			args:     []string{"atmos", "-C=examples/demo-stacks", "terraform", "plan"},
			expected: []string{"atmos", "terraform", "plan"},
		},
		{
			name:     "short form concatenated -Cvalue",
			args:     []string{"atmos", "-C../foo", "terraform", "plan"},
			expected: []string{"atmos", "terraform", "plan"},
		},
		{
			name:     "chdir at end with separate value",
			args:     []string{"atmos", "terraform", "plan", "--chdir", "examples/demo-stacks"},
			expected: []string{"atmos", "terraform", "plan"},
		},
		{
			name:     "chdir at end with equals",
			args:     []string{"atmos", "terraform", "plan", "--chdir=examples/demo-stacks"},
			expected: []string{"atmos", "terraform", "plan"},
		},
		{
			name:     "multiple chdir flags mixed forms",
			args:     []string{"atmos", "--chdir=/first", "plan", "-C", "/second", "component"},
			expected: []string{"atmos", "plan", "component"},
		},
		{
			name:     "mixed chdir and other flags preserves other flags",
			args:     []string{"atmos", "--use-version", "1.199.0", "--chdir", "examples/demo-stacks", "terraform", "plan"},
			expected: []string{"atmos", "--use-version", "1.199.0", "terraform", "plan"},
		},
		{
			name:     "empty args",
			args:     []string{},
			expected: []string{},
		},
		{
			name:     "only program name",
			args:     []string{"atmos"},
			expected: []string{"atmos"},
		},
		{
			name:     "chdir without value at end",
			args:     []string{"atmos", "terraform", "plan", "--chdir"},
			expected: []string{"atmos", "terraform", "plan"},
		},
		{
			name:     "bare -C at end",
			args:     []string{"atmos", "-C"},
			expected: []string{"atmos"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripChdirArgs(tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStripChdirArgs_DoesNotMutateInput(t *testing.T) {
	input := []string{"atmos", "--chdir", "/tmp", "plan"}
	original := make([]string, len(input))
	copy(original, input)

	_ = StripChdirArgs(input)

	assert.Equal(t, original, input, "StripChdirArgs must not mutate its input")
}

func TestFilterChdirEnv(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "no ATMOS_CHDIR",
			input:    []string{"PATH=/usr/bin", "HOME=/home/user"},
			expected: []string{"PATH=/usr/bin", "HOME=/home/user"},
		},
		{
			name:     "contains ATMOS_CHDIR",
			input:    []string{"PATH=/usr/bin", "ATMOS_CHDIR=/some/path", "HOME=/home/user"},
			expected: []string{"PATH=/usr/bin", "HOME=/home/user", "ATMOS_CHDIR="},
		},
		{
			name:     "ATMOS_CHDIR at beginning",
			input:    []string{"ATMOS_CHDIR=/path", "PATH=/usr/bin"},
			expected: []string{"PATH=/usr/bin", "ATMOS_CHDIR="},
		},
		{
			name:     "ATMOS_CHDIR at end",
			input:    []string{"PATH=/usr/bin", "ATMOS_CHDIR=/path"},
			expected: []string{"PATH=/usr/bin", "ATMOS_CHDIR="},
		},
		{
			name:     "empty value ATMOS_CHDIR still triggers override",
			input:    []string{"PATH=/usr/bin", "ATMOS_CHDIR=", "HOME=/home"},
			expected: []string{"PATH=/usr/bin", "HOME=/home", "ATMOS_CHDIR="},
		},
		{
			name:     "empty input",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "only ATMOS_CHDIR",
			input:    []string{"ATMOS_CHDIR=/path"},
			expected: []string{"ATMOS_CHDIR="},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterChdirEnv(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFilterChdirEnv_DoesNotMutateInput(t *testing.T) {
	input := []string{"PATH=/usr/bin", "ATMOS_CHDIR=/foo", "HOME=/home"}
	original := make([]string, len(input))
	copy(original, input)

	_ = FilterChdirEnv(input)

	assert.Equal(t, original, input, "FilterChdirEnv must not mutate its input")
}
