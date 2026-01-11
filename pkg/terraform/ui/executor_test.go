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
	assert.Contains(t, result, "-compact-warnings")
	// Flags should be after "plan".
	assert.Equal(t, "plan", result[0])
	assert.Equal(t, "-json", result[1])
	assert.Equal(t, "-compact-warnings", result[2])
}

func TestBuildArgsWithJSON_AddsFlagForApply(t *testing.T) {
	args := []string{"apply", "-auto-approve"}
	result := buildArgsWithJSON(args, "apply")
	assert.Contains(t, result, "-json")
	assert.Contains(t, result, "-compact-warnings")
	assert.Equal(t, "apply", result[0])
	assert.Equal(t, "-json", result[1])
	assert.Equal(t, "-compact-warnings", result[2])
}

func TestBuildArgsWithJSON_AddsFlagForInit(t *testing.T) {
	args := []string{"init", "-reconfigure"}
	result := buildArgsWithJSON(args, "init")
	assert.Contains(t, result, "-json")
	assert.Contains(t, result, "-compact-warnings")
	assert.Equal(t, "init", result[0])
	assert.Equal(t, "-json", result[1])
	assert.Equal(t, "-compact-warnings", result[2])
}

func TestBuildArgsWithJSON_AddsFlagForRefresh(t *testing.T) {
	args := []string{"refresh"}
	result := buildArgsWithJSON(args, "refresh")
	assert.Contains(t, result, "-json")
	assert.Contains(t, result, "-compact-warnings")
	assert.Equal(t, "refresh", result[0])
	assert.Equal(t, "-json", result[1])
	assert.Equal(t, "-compact-warnings", result[2])
}

func TestBuildArgsWithJSON_DoesNotDuplicateFlag(t *testing.T) {
	args := []string{"plan", "-json", "-compact-warnings", "-out", "plan.out"}
	result := buildArgsWithJSON(args, "plan")
	// Count flag occurrences.
	jsonCount := 0
	compactCount := 0
	for _, arg := range result {
		if arg == "-json" {
			jsonCount++
		}
		if arg == "-compact-warnings" {
			compactCount++
		}
	}
	assert.Equal(t, 1, jsonCount, "-json should not be duplicated")
	assert.Equal(t, 1, compactCount, "-compact-warnings should not be duplicated")
}

func TestBuildArgsWithJSON_AddsCompactWarningsWhenOnlyJSONPresent(t *testing.T) {
	args := []string{"plan", "-json", "-out", "plan.out"}
	result := buildArgsWithJSON(args, "plan")
	assert.Contains(t, result, "-json")
	assert.Contains(t, result, "-compact-warnings")
	// -json should still be at position 1, -compact-warnings added after.
	assert.Equal(t, "plan", result[0])
}

func TestBuildArgsWithJSON_NoSubcommandAtStart(t *testing.T) {
	// Edge case: args don't start with a recognized subcommand.
	args := []string{"-var", "foo=bar"}
	result := buildArgsWithJSON(args, "other")
	assert.Contains(t, result, "-json")
	assert.Contains(t, result, "-compact-warnings")
	// In this case, flags should be prepended.
	assert.Equal(t, "-json", result[0])
	assert.Equal(t, "-compact-warnings", result[1])
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

func TestExtractOutFlag(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{"flag with separate value", []string{"plan", "-out", "myplan.tfplan"}, "myplan.tfplan"},
		{"flag with equals", []string{"plan", "-out=myplan.tfplan"}, "myplan.tfplan"},
		{"no flag", []string{"plan"}, ""},
		{"other flags only", []string{"plan", "-var", "foo=bar"}, ""},
		{"flag at end without value", []string{"plan", "-out"}, ""},
		{"empty args", []string{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractOutFlag(tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasFlag(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		flag     string
		expected bool
	}{
		{"flag present", []string{"apply", "-auto-approve"}, "-auto-approve", true},
		{"flag absent", []string{"apply"}, "-auto-approve", false},
		{"multiple flags with target", []string{"apply", "-var", "x=1", "-auto-approve"}, "-auto-approve", true},
		{"similar prefix not matched", []string{"apply", "-auto-approve-all"}, "-auto-approve", false},
		{"empty args", []string{}, "-auto-approve", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasFlag(tt.args, tt.flag)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractPlanFile(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{"--from-plan with separate value", []string{"apply", "--from-plan", "plan.tfplan"}, "plan.tfplan"},
		{"--from-plan with equals", []string{"apply", "--from-plan=plan.tfplan"}, "plan.tfplan"},
		{"--planfile with separate value", []string{"apply", "--planfile", "plan.tfplan"}, "plan.tfplan"},
		{"--planfile with equals", []string{"apply", "--planfile=plan.tfplan"}, "plan.tfplan"},
		{"positional planfile", []string{"apply", "myplan.tfplan"}, "myplan.tfplan"},
		{"no planfile", []string{"apply", "-auto-approve"}, ""},
		{"positional non-tfplan file", []string{"apply", "config.tf"}, ""},
		{"empty args", []string{}, ""},
		{"single arg apply only", []string{"apply"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractPlanFile(tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildPlanArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		planFile string
		expected []string
	}{
		{
			name:     "basic apply to plan",
			args:     []string{"apply", "-var", "foo=bar"},
			planFile: "/tmp/plan.tfplan",
			expected: []string{"plan", "-var", "foo=bar", "-out=/tmp/plan.tfplan"},
		},
		{
			name:     "strips auto-approve",
			args:     []string{"apply", "-auto-approve", "-var", "x=1"},
			planFile: "/tmp/plan.tfplan",
			expected: []string{"plan", "-var", "x=1", "-out=/tmp/plan.tfplan"},
		},
		{
			name:     "apply only",
			args:     []string{"apply"},
			planFile: "/tmp/plan.tfplan",
			expected: []string{"plan", "-out=/tmp/plan.tfplan"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildPlanArgs(tt.args, tt.planFile)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildApplyArgs(t *testing.T) {
	tests := []struct {
		name     string
		planFile string
		expected []string
	}{
		{"basic planfile", "/tmp/plan.tfplan", []string{"apply", "/tmp/plan.tfplan"}},
		{"different path", "/var/tmp/my-plan.tfplan", []string{"apply", "/var/tmp/my-plan.tfplan"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildApplyArgs(tt.planFile)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildDestroyPlanArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		planFile string
		expected []string
	}{
		{
			name:     "basic destroy to plan",
			args:     []string{"destroy", "-var", "foo=bar"},
			planFile: "/tmp/destroy.tfplan",
			expected: []string{"plan", "-destroy", "-var", "foo=bar", "-out=/tmp/destroy.tfplan"},
		},
		{
			name:     "strips auto-approve",
			args:     []string{"destroy", "-auto-approve"},
			planFile: "/tmp/destroy.tfplan",
			expected: []string{"plan", "-destroy", "-out=/tmp/destroy.tfplan"},
		},
		{
			name:     "destroy only",
			args:     []string{"destroy"},
			planFile: "/tmp/destroy.tfplan",
			expected: []string{"plan", "-destroy", "-out=/tmp/destroy.tfplan"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildDestroyPlanArgs(tt.args, tt.planFile)
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
