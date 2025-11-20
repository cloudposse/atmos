package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseProfilesFromArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected []string
	}{
		{
			name:     "--profile value syntax",
			args:     []string{"atmos", "describe", "config", "--profile", "managers"},
			expected: []string{"managers"},
		},
		{
			name:     "--profile=value syntax",
			args:     []string{"atmos", "describe", "config", "--profile=managers"},
			expected: []string{"managers"},
		},
		{
			name:     "comma-separated values",
			args:     []string{"atmos", "describe", "config", "--profile=dev,staging,prod"},
			expected: []string{"dev", "staging", "prod"},
		},
		{
			name:     "multiple --profile flags",
			args:     []string{"atmos", "describe", "config", "--profile", "dev", "--profile", "staging"},
			expected: []string{"dev", "staging"},
		},
		{
			name:     "no profile flag",
			args:     []string{"atmos", "describe", "config"},
			expected: nil,
		},
		{
			name:     "--profile at end without value",
			args:     []string{"atmos", "describe", "config", "--profile"},
			expected: nil,
		},
		{
			name:     "comma-separated with spaces",
			args:     []string{"atmos", "describe", "config", "--profile=dev, staging , prod"},
			expected: []string{"dev", "staging", "prod"},
		},
		{
			name:     "comma-separated with --profile value syntax",
			args:     []string{"atmos", "describe", "config", "--profile", "dev,staging"},
			expected: []string{"dev", "staging"},
		},
		{
			name:     "mixed syntax",
			args:     []string{"atmos", "describe", "config", "--profile", "dev", "--profile=staging,prod"},
			expected: []string{"dev", "staging", "prod"},
		},
		{
			name:     "empty value in comma list",
			args:     []string{"atmos", "describe", "config", "--profile=dev,,prod"},
			expected: []string{"dev", "prod"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseProfilesFromArgs(tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}
