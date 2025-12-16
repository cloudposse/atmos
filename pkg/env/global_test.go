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
	withGlobal = append(withGlobal, commandEnv...)
	final := withGlobal

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

func TestMergeSystemEnvWithGlobal(t *testing.T) {
	// Set test environment variables.
	t.Setenv("TEST_SYSTEM_VAR", "system-value")
	t.Setenv("OVERRIDE_VAR", "system-override")

	tests := []struct {
		name          string
		componentEnv  []string
		globalEnv     map[string]string
		checkKey      string
		expectedValue string
		shouldContain bool
	}{
		{
			name:          "system env is present",
			componentEnv:  []string{},
			globalEnv:     nil,
			checkKey:      "TEST_SYSTEM_VAR",
			expectedValue: "system-value",
			shouldContain: true,
		},
		{
			name:          "global env overrides system env",
			componentEnv:  []string{},
			globalEnv:     map[string]string{"OVERRIDE_VAR": "global-override"},
			checkKey:      "OVERRIDE_VAR",
			expectedValue: "global-override",
			shouldContain: true,
		},
		{
			name:          "component env overrides global env",
			componentEnv:  []string{"OVERRIDE_VAR=component-override"},
			globalEnv:     map[string]string{"OVERRIDE_VAR": "global-override"},
			checkKey:      "OVERRIDE_VAR",
			expectedValue: "component-override",
			shouldContain: true,
		},
		{
			name:          "new variable from global env",
			componentEnv:  []string{},
			globalEnv:     map[string]string{"NEW_GLOBAL_VAR": "new-value"},
			checkKey:      "NEW_GLOBAL_VAR",
			expectedValue: "new-value",
			shouldContain: true,
		},
		{
			name:          "new variable from component env",
			componentEnv:  []string{"NEW_COMPONENT_VAR=component-new"},
			globalEnv:     nil,
			checkKey:      "NEW_COMPONENT_VAR",
			expectedValue: "component-new",
			shouldContain: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MergeSystemEnvWithGlobal(tt.componentEnv, tt.globalEnv)

			// Find the key in the result.
			found := false
			var actualValue string
			for _, envVar := range result {
				pair := splitStringAtFirstOccurrence(envVar, "=")
				if pair[0] == tt.checkKey {
					found = true
					actualValue = pair[1]
					break
				}
			}

			if tt.shouldContain {
				assert.True(t, found, "Expected to find key %s in result", tt.checkKey)
				assert.Equal(t, tt.expectedValue, actualValue, "Value mismatch for key %s", tt.checkKey)
			} else {
				assert.False(t, found, "Expected key %s to NOT be in result", tt.checkKey)
			}
		})
	}
}

func TestMergeSystemEnvWithGlobal_TFCliArgsHandling(t *testing.T) {
	// Test special handling for TF_CLI_ARGS_* variables.
	t.Setenv("TF_CLI_ARGS_plan", "-input=false")

	tests := []struct {
		name          string
		componentEnv  []string
		globalEnv     map[string]string
		expectedValue string
	}{
		{
			name:          "TF_CLI_ARGS prepends component value to system value",
			componentEnv:  []string{"TF_CLI_ARGS_plan=-compact-warnings"},
			globalEnv:     nil,
			expectedValue: "-compact-warnings -input=false",
		},
		{
			name:          "TF_CLI_ARGS with global env prepends component to global+system",
			componentEnv:  []string{"TF_CLI_ARGS_plan=-compact-warnings"},
			globalEnv:     map[string]string{"TF_CLI_ARGS_plan": "-no-color"},
			expectedValue: "-compact-warnings -no-color",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MergeSystemEnvWithGlobal(tt.componentEnv, tt.globalEnv)

			// Find TF_CLI_ARGS_plan in result.
			var actualValue string
			for _, envVar := range result {
				pair := splitStringAtFirstOccurrence(envVar, "=")
				if pair[0] == "TF_CLI_ARGS_plan" {
					actualValue = pair[1]
					break
				}
			}

			assert.Equal(t, tt.expectedValue, actualValue)
		})
	}
}

func TestMergeSystemEnvSimpleWithGlobal(t *testing.T) {
	// Test that Simple version does NOT do TF_CLI_ARGS_* special handling.
	t.Setenv("TF_CLI_ARGS_plan", "-input=false")
	t.Setenv("TEST_VAR", "system")

	// With Simple version, component env should just override, not prepend.
	result := MergeSystemEnvSimpleWithGlobal(
		[]string{"TF_CLI_ARGS_plan=-compact-warnings", "TEST_VAR=component"},
		map[string]string{"GLOBAL_VAR": "global-value"},
	)

	// Find values in result.
	var tfCliValue, testVarValue, globalVarValue string
	for _, envVar := range result {
		pair := splitStringAtFirstOccurrence(envVar, "=")
		switch pair[0] {
		case "TF_CLI_ARGS_plan":
			tfCliValue = pair[1]
		case "TEST_VAR":
			testVarValue = pair[1]
		case "GLOBAL_VAR":
			globalVarValue = pair[1]
		}
	}

	// Simple version just overrides, doesn't prepend.
	assert.Equal(t, "-compact-warnings", tfCliValue, "Simple merge should override, not prepend")
	assert.Equal(t, "component", testVarValue, "Component should override system")
	assert.Equal(t, "global-value", globalVarValue, "Global should be present")
}

func TestMergeSystemEnv(t *testing.T) {
	// Test without global env.
	t.Setenv("TEST_SYSTEM_ONLY", "system-value")

	result := MergeSystemEnv([]string{"NEW_VAR=new-value"})

	// Find values in result.
	var systemValue, newValue string
	for _, envVar := range result {
		pair := splitStringAtFirstOccurrence(envVar, "=")
		switch pair[0] {
		case "TEST_SYSTEM_ONLY":
			systemValue = pair[1]
		case "NEW_VAR":
			newValue = pair[1]
		}
	}

	assert.Equal(t, "system-value", systemValue)
	assert.Equal(t, "new-value", newValue)
}

func TestMergeSystemEnvSimple(t *testing.T) {
	// Test without global env and without TF_CLI_ARGS handling.
	t.Setenv("TEST_SIMPLE", "system-value")

	result := MergeSystemEnvSimple([]string{"TEST_SIMPLE=override"})

	// Find value in result.
	var testValue string
	for _, envVar := range result {
		pair := splitStringAtFirstOccurrence(envVar, "=")
		if pair[0] == "TEST_SIMPLE" {
			testValue = pair[1]
			break
		}
	}

	assert.Equal(t, "override", testValue, "Simple merge should override system value")
}

func TestMergeSystemEnvInternal_InvalidEnvFormat(t *testing.T) {
	// Test that invalid env var format (no = sign) is skipped.
	t.Setenv("VALID_VAR", "value")

	// Pass invalid format (no equals sign) in envList - should be skipped.
	result := MergeSystemEnvSimple([]string{"INVALID_NO_EQUALS", "VALID_NEW=new-value"})

	// INVALID_NO_EQUALS should not appear in result.
	foundInvalid := false
	foundValidNew := false
	for _, envVar := range result {
		pair := splitStringAtFirstOccurrence(envVar, "=")
		if pair[0] == "INVALID_NO_EQUALS" {
			foundInvalid = true
		}
		if pair[0] == "VALID_NEW" {
			foundValidNew = true
		}
	}

	assert.False(t, foundInvalid, "Invalid env var format should be skipped")
	assert.True(t, foundValidNew, "Valid env var should be present")
}
