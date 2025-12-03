package env

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMergeGlobalEnv(t *testing.T) {
	tests := []struct {
		name      string
		baseEnv   []string
		globalEnv map[string]string
		expected  []string
	}{
		{
			name:      "empty global env returns base unchanged",
			baseEnv:   []string{"PATH=/usr/bin", "HOME=/home/user"},
			globalEnv: map[string]string{},
			expected:  []string{"PATH=/usr/bin", "HOME=/home/user"},
		},
		{
			name:      "nil global env returns base unchanged",
			baseEnv:   []string{"PATH=/usr/bin"},
			globalEnv: nil,
			expected:  []string{"PATH=/usr/bin"},
		},
		{
			name:      "global env appended to base",
			baseEnv:   []string{"PATH=/usr/bin"},
			globalEnv: map[string]string{"AWS_REGION": "us-east-1"},
			expected:  []string{"PATH=/usr/bin", "AWS_REGION=us-east-1"},
		},
		{
			name:      "multiple global env vars appended",
			baseEnv:   []string{"PATH=/usr/bin"},
			globalEnv: map[string]string{"AWS_REGION": "us-east-1", "TF_VAR_foo": "bar"},
			expected:  []string{"PATH=/usr/bin", "AWS_REGION=us-east-1", "TF_VAR_foo=bar"},
		},
		{
			name:      "empty base with global env",
			baseEnv:   []string{},
			globalEnv: map[string]string{"AWS_REGION": "us-east-1"},
			expected:  []string{"AWS_REGION=us-east-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MergeGlobalEnv(tt.baseEnv, tt.globalEnv)
			// Sort for comparison since map iteration is not ordered.
			sort.Strings(result)
			sort.Strings(tt.expected)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertMapToSlice(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		expected []string
	}{
		{
			name:     "nil map returns empty slice",
			input:    nil,
			expected: []string{},
		},
		{
			name:     "empty map returns empty slice",
			input:    map[string]string{},
			expected: []string{},
		},
		{
			name:     "single entry",
			input:    map[string]string{"KEY": "value"},
			expected: []string{"KEY=value"},
		},
		{
			name:     "multiple entries",
			input:    map[string]string{"KEY1": "value1", "KEY2": "value2"},
			expected: []string{"KEY1=value1", "KEY2=value2"},
		},
		{
			name:     "empty value",
			input:    map[string]string{"KEY": ""},
			expected: []string{"KEY="},
		},
		{
			name:     "value with equals sign",
			input:    map[string]string{"KEY": "value=with=equals"},
			expected: []string{"KEY=value=with=equals"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertMapToSlice(tt.input)
			sort.Strings(result)
			sort.Strings(tt.expected)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestApplyGlobalEnvToSlice(t *testing.T) {
	tests := []struct {
		name      string
		envSlice  []string
		globalEnv map[string]string
		expected  []string
	}{
		{
			name:      "empty global env returns slice unchanged",
			envSlice:  []string{"PATH=/usr/bin", "HOME=/home/user"},
			globalEnv: map[string]string{},
			expected:  []string{"PATH=/usr/bin", "HOME=/home/user"},
		},
		{
			name:      "nil global env returns slice unchanged",
			envSlice:  []string{"PATH=/usr/bin"},
			globalEnv: nil,
			expected:  []string{"PATH=/usr/bin"},
		},
		{
			name:      "new key is added",
			envSlice:  []string{"PATH=/usr/bin"},
			globalEnv: map[string]string{"AWS_REGION": "us-east-1"},
			expected:  []string{"PATH=/usr/bin", "AWS_REGION=us-east-1"},
		},
		{
			name:      "existing key is NOT overwritten",
			envSlice:  []string{"PATH=/usr/bin", "AWS_REGION=us-west-2"},
			globalEnv: map[string]string{"AWS_REGION": "us-east-1"},
			expected:  []string{"PATH=/usr/bin", "AWS_REGION=us-west-2"},
		},
		{
			name:      "mixed: some added, some preserved",
			envSlice:  []string{"PATH=/usr/bin", "HOME=/home/user"},
			globalEnv: map[string]string{"PATH": "/custom/bin", "AWS_REGION": "us-east-1"},
			expected:  []string{"PATH=/usr/bin", "HOME=/home/user", "AWS_REGION=us-east-1"},
		},
		{
			name:      "empty slice with global env",
			envSlice:  []string{},
			globalEnv: map[string]string{"AWS_REGION": "us-east-1", "TF_VAR_foo": "bar"},
			expected:  []string{"AWS_REGION=us-east-1", "TF_VAR_foo=bar"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ApplyGlobalEnvToSlice(tt.envSlice, tt.globalEnv)
			sort.Strings(result)
			sort.Strings(tt.expected)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGlobalEnvPrecedence(t *testing.T) {
	// Integration test: verify that command-specific env overrides global env.
	// This simulates the real workflow: os.Environ() + global env + command env.

	// Start with "system" env.
	systemEnv := []string{"PATH=/usr/bin", "HOME=/home/user"}

	// Apply global env (atmos.yaml env section).
	globalEnv := map[string]string{
		"AWS_REGION":      "us-east-1",
		"TF_PLUGIN_CACHE": "/cache",
		"OVERRIDE_ME":     "global-value",
	}
	withGlobal := MergeGlobalEnv(systemEnv, globalEnv)

	// Apply command-specific env (simulating stack/component env).
	commandEnv := []string{"OVERRIDE_ME=command-value", "COMMAND_SPECIFIC=true"}
	final := append(withGlobal, commandEnv...)

	// Find the OVERRIDE_ME values in order.
	var overrideValues []string
	for _, envVar := range final {
		pair := splitStringAtFirstOccurrence(envVar, "=")
		if pair[0] == "OVERRIDE_ME" {
			overrideValues = append(overrideValues, pair[1])
		}
	}

	// The command value should come last (will be used by shell).
	assert.Equal(t, 2, len(overrideValues), "Should have both global and command values")
	assert.Equal(t, "global-value", overrideValues[0], "Global value should come first")
	assert.Equal(t, "command-value", overrideValues[1], "Command value should come last (wins)")
}
