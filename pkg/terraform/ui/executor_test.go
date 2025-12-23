package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShouldUseStreamingUI_ExplicitlyDisabled(t *testing.T) {
	// --ui=false explicitly set.
	result := ShouldUseStreamingUI(true, false, true, "plan")
	assert.False(t, result)
}

func TestShouldUseStreamingUI_ConfigDisabled(t *testing.T) {
	// Neither flag nor config enabled.
	result := ShouldUseStreamingUI(false, false, false, "plan")
	assert.False(t, result)
}

func TestShouldUseStreamingUI_UnsupportedCommand(t *testing.T) {
	// Even if enabled, unsupported commands return false.
	unsupportedCommands := []string{"version", "workspace", "fmt", "validate", "import", "state", "output"}
	for _, cmd := range unsupportedCommands {
		result := ShouldUseStreamingUI(true, true, true, cmd)
		assert.False(t, result, "command %s should not support streaming UI", cmd)
	}
}

func TestShouldUseStreamingUI_SupportedCommands(t *testing.T) {
	// Test that supported commands (plan, apply, init, destroy) still need enablement.
	// Note: refresh is NOT supported due to poor -json streaming support.
	supportedCommands := []string{"plan", "apply", "init", "destroy"}
	for _, cmd := range supportedCommands {
		// In non-CI, non-TTY environment, this would still return false.
		// But we're testing the command filtering logic.
		result := ShouldUseStreamingUI(false, false, false, cmd)
		assert.False(t, result, "command %s needs enabled flag or config to use streaming UI", cmd)
	}
}

func TestBuildArgsWithJSON_AddsFlagForPlan(t *testing.T) {
	args := []string{"plan", "-out", "plan.out"}
	result := buildArgsWithJSON(args, "plan")
	assert.Contains(t, result, "-json")
	// -json should be after "plan".
	assert.Equal(t, "plan", result[0])
	assert.Equal(t, "-json", result[1])
}

func TestBuildArgsWithJSON_AddsFlagForApply(t *testing.T) {
	args := []string{"apply", "-auto-approve"}
	result := buildArgsWithJSON(args, "apply")
	assert.Contains(t, result, "-json")
	assert.Equal(t, "apply", result[0])
	assert.Equal(t, "-json", result[1])
}

func TestBuildArgsWithJSON_AddsFlagForInit(t *testing.T) {
	args := []string{"init", "-reconfigure"}
	result := buildArgsWithJSON(args, "init")
	assert.Contains(t, result, "-json")
	assert.Equal(t, "init", result[0])
	assert.Equal(t, "-json", result[1])
}

func TestBuildArgsWithJSON_AddsFlagForRefresh(t *testing.T) {
	args := []string{"refresh"}
	result := buildArgsWithJSON(args, "refresh")
	assert.Contains(t, result, "-json")
	assert.Equal(t, "refresh", result[0])
	assert.Equal(t, "-json", result[1])
}

func TestBuildArgsWithJSON_DoesNotDuplicateFlag(t *testing.T) {
	args := []string{"plan", "-json", "-out", "plan.out"}
	result := buildArgsWithJSON(args, "plan")
	// Count -json occurrences.
	count := 0
	for _, arg := range result {
		if arg == "-json" {
			count++
		}
	}
	assert.Equal(t, 1, count, "-json should not be duplicated")
}

func TestBuildArgsWithJSON_NoSubcommandAtStart(t *testing.T) {
	// Edge case: args don't start with a recognized subcommand.
	args := []string{"-var", "foo=bar"}
	result := buildArgsWithJSON(args, "other")
	assert.Contains(t, result, "-json")
	// In this case, -json should be prepended.
	assert.Equal(t, "-json", result[0])
}

func TestDetectOutputContentType(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected outputContentType
	}{
		{"empty string", "", outputContentTypeDefault},
		{"sensitive marker", "<sensitive>", outputContentTypeSensitive},
		{"null value", "null", outputContentTypeNull},
		{"true boolean", "true", outputContentTypeBoolean},
		{"false boolean", "false", outputContentTypeBoolean},
		{"integer", "42", outputContentTypeNumber},
		{"negative integer", "-10", outputContentTypeNumber},
		{"float", "3.14", outputContentTypeNumber},
		{"string value", "hello world", outputContentTypeDefault},
		{"url string", "https://example.com", outputContentTypeDefault},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectOutputContentType(tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsNumericString(t *testing.T) {
	tests := []struct {
		value    string
		expected bool
	}{
		{"42", true},
		{"-10", true},
		{"3.14", true},
		{"0", true},
		{"1e10", true},
		{"hello", false},
		{"", false},
		{"true", false},
		{"false", false},
		{"null", false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			result := isNumericString(tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatOutputValue(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		expected string
	}{
		{"string", "hello", "hello"},
		{"integer as float64", float64(42), "42"},
		{"float", 3.14, "3.14"},
		{"boolean true", true, "true"},
		{"boolean false", false, "false"},
		{"nil", nil, "null"},
		{"simple map", map[string]any{"key": "value"}, "{\n  \"key\": \"value\"\n}"},
		{"slice", []any{"a", "b"}, "[\n  \"a\",\n  \"b\"\n]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatOutputValue(tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}
