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
			args:     []string{"atmos", "describe", "config", AtmosProfileFlag, "managers"},
			expected: []string{"managers"},
		},
		{
			name:     "--profile=value syntax",
			args:     []string{"atmos", "describe", "config", AtmosProfileFlag + "=managers"},
			expected: []string{"managers"},
		},
		{
			name:     "comma-separated values",
			args:     []string{"atmos", "describe", "config", AtmosProfileFlag + "=dev,staging,prod"},
			expected: []string{"dev", "staging", "prod"},
		},
		{
			name:     "multiple --profile flags",
			args:     []string{"atmos", "describe", "config", AtmosProfileFlag, "dev", AtmosProfileFlag, "staging"},
			expected: []string{"dev", "staging"},
		},
		{
			name:     "no profile flag",
			args:     []string{"atmos", "describe", "config"},
			expected: nil,
		},
		{
			name:     "--profile at end without value",
			args:     []string{"atmos", "describe", "config", AtmosProfileFlag},
			expected: nil,
		},
		{
			name:     "comma-separated with spaces",
			args:     []string{"atmos", "describe", "config", AtmosProfileFlag + "=dev, staging , prod"},
			expected: []string{"dev", "staging", "prod"},
		},
		{
			name:     "comma-separated with --profile value syntax",
			args:     []string{"atmos", "describe", "config", AtmosProfileFlag, "dev,staging"},
			expected: []string{"dev", "staging"},
		},
		{
			name:     "mixed syntax",
			args:     []string{"atmos", "describe", "config", AtmosProfileFlag, "dev", AtmosProfileFlag + "=staging,prod"},
			expected: []string{"dev", "staging", "prod"},
		},
		{
			name:     "empty value in comma list",
			args:     []string{"atmos", "describe", "config", AtmosProfileFlag + "=dev,,prod"},
			expected: []string{"dev", "prod"},
		},
		{
			name:     "only whitespace in profile value",
			args:     []string{"atmos", "describe", "config", AtmosProfileFlag + "=   "},
			expected: nil,
		},
		{
			name:     "leading and trailing commas",
			args:     []string{"atmos", "describe", "config", AtmosProfileFlag + "=,dev,staging,"},
			expected: []string{"dev", "staging"},
		},
		{
			name:     "multiple consecutive commas",
			args:     []string{"atmos", "describe", "config", AtmosProfileFlag + "=dev,,,staging"},
			expected: []string{"dev", "staging"},
		},
		{
			name:     "profile value with only commas",
			args:     []string{"atmos", "describe", "config", AtmosProfileFlag + "=,,,"},
			expected: nil,
		},
		{
			name:     "mixed whitespace and empty values",
			args:     []string{"atmos", "describe", "config", AtmosProfileFlag + "=dev,  , , staging"},
			expected: []string{"dev", "staging"},
		},
		{
			name:     "profile flag with equals but no value",
			args:     []string{"atmos", "describe", "config", AtmosProfileFlag + "="},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseProfilesFromOsArgs(tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}
