package flags

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestHasAIFlagInternal tests the HasAIFlagInternal function that parses --ai from os.Args.
func TestHasAIFlagInternal(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected bool
	}{
		{name: "no --ai flag", args: []string{"atmos", "terraform", "plan"}, expected: false},
		{name: "--ai flag present", args: []string{"atmos", "--ai", "terraform", "plan"}, expected: true},
		{name: "--ai flag at end", args: []string{"atmos", "terraform", "plan", "--ai"}, expected: true},
		{name: "--ai=true", args: []string{"atmos", "--ai=true", "terraform", "plan"}, expected: true},
		{name: "--ai=false is not enabled", args: []string{"atmos", "--ai=false", "terraform", "plan"}, expected: false},
		{name: "--ai after -- delimiter is ignored", args: []string{"atmos", "terraform", "plan", "--", "--ai"}, expected: false},
		{name: "similar flag --aim is not matched", args: []string{"atmos", "--aim", "terraform", "plan"}, expected: false},
		{name: "empty args", args: []string{}, expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasAIFlagInternal(tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestHasAIFlagInternal_EnvVarFallback tests env var fallback behavior for --ai detection.
func TestHasAIFlagInternal_EnvVarFallback(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected bool
	}{
		{name: "ATMOS_AI=true enables AI", envValue: "true", expected: true},
		{name: "ATMOS_AI=1 enables AI", envValue: "1", expected: true},
		{name: "ATMOS_AI=false disables AI", envValue: "false", expected: false},
		{name: "ATMOS_AI=0 disables AI", envValue: "0", expected: false},
		{name: "ATMOS_AI=invalid disables AI", envValue: "invalid", expected: false},
		{name: "ATMOS_AI empty disables AI", envValue: "", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ATMOS_AI", tt.envValue)
			result := HasAIFlagInternal([]string{"atmos", "terraform", "plan"})
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestHasAIFlagInternal_CLIOverridesEnv verifies CLI flag takes precedence over env var.
func TestHasAIFlagInternal_CLIOverridesEnv(t *testing.T) {
	t.Setenv("ATMOS_AI", "true")
	result := HasAIFlagInternal([]string{"atmos", "--ai=false", "terraform", "plan"})
	assert.False(t, result, "--ai=false should override ATMOS_AI=true")
}

// TestHasAIFlagInternal_InvalidBoolValue tests --ai with invalid boolean value.
func TestHasAIFlagInternal_InvalidBoolValue(t *testing.T) {
	result := HasAIFlagInternal([]string{"atmos", "--ai=maybe", "terraform", "plan"})
	assert.False(t, result, "--ai=maybe should return false (invalid boolean)")
}

// TestParseSkillFlagInternal tests the ParseSkillFlagInternal function.
func TestParseSkillFlagInternal(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected []string
	}{
		{name: "no --skill flag", args: []string{"atmos", "terraform", "plan"}, expected: nil},
		{name: "--skill with separate value", args: []string{"atmos", "--ai", "--skill", "atmos-terraform", "terraform", "plan"}, expected: []string{"atmos-terraform"}},
		{name: "--skill=value format", args: []string{"atmos", "--ai", "--skill=atmos-stacks", "describe", "stacks"}, expected: []string{"atmos-stacks"}},
		{name: "--skill at end with value", args: []string{"atmos", "terraform", "plan", "--ai", "--skill", "atmos-terraform"}, expected: []string{"atmos-terraform"}},
		{name: "--skill after -- delimiter is ignored", args: []string{"atmos", "terraform", "plan", "--", "--skill", "atmos-terraform"}, expected: nil},
		{name: "--skill without value (next arg is flag)", args: []string{"atmos", "--skill", "--ai", "terraform", "plan"}, expected: nil},
		{name: "--skill at end without value", args: []string{"atmos", "terraform", "plan", "--skill"}, expected: nil},
		{name: "empty args", args: []string{}, expected: nil},
		{name: "--skill=empty value", args: []string{"atmos", "--skill=", "terraform", "plan"}, expected: nil},
		{name: "--skilled is not matched", args: []string{"atmos", "--skilled", "atmos-terraform", "terraform", "plan"}, expected: nil},
		{name: "--skilled=value not matched", args: []string{"atmos", "--skilled=atmos-terraform", "terraform", "plan"}, expected: nil},
		{name: "--skill between other flags", args: []string{"atmos", "--logs-level=Debug", "--skill", "atmos-validation", "--ai", "validate", "stacks"}, expected: []string{"atmos-validation"}},
		{name: "hyphens in skill name", args: []string{"atmos", "--skill=my-custom-skill-v2", "terraform", "plan"}, expected: []string{"my-custom-skill-v2"}},
		{name: "multiple --skill flags", args: []string{"atmos", "--skill", "first", "--skill", "second", "terraform", "plan"}, expected: []string{"first", "second"}},
		{name: "comma-separated skills", args: []string{"atmos", "--ai", "--skill", "atmos-terraform,atmos-stacks", "terraform", "plan"}, expected: []string{"atmos-terraform", "atmos-stacks"}},
		{name: "comma-separated with =", args: []string{"atmos", "--ai", "--skill=atmos-terraform,atmos-stacks", "terraform", "plan"}, expected: []string{"atmos-terraform", "atmos-stacks"}},
		{name: "mixed repeated and comma", args: []string{"atmos", "--skill", "a,b", "--skill", "c", "terraform", "plan"}, expected: []string{"a", "b", "c"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseSkillFlagInternal(tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestParseSkillFlagInternal_EnvVarFallback tests env var fallback for --skill.
func TestParseSkillFlagInternal_EnvVarFallback(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected []string
	}{
		{name: "single skill from env", envValue: "atmos-terraform", expected: []string{"atmos-terraform"}},
		{name: "comma-separated from env", envValue: "a,b,c", expected: []string{"a", "b", "c"}},
		{name: "empty env", envValue: "", expected: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ATMOS_SKILL", tt.envValue)
			result := ParseSkillFlagInternal([]string{"atmos", "terraform", "plan"})
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestParseSkillFlagInternal_CLIOverridesEnv verifies CLI flag prevents env var fallback.
func TestParseSkillFlagInternal_CLIOverridesEnv(t *testing.T) {
	t.Setenv("ATMOS_SKILL", "from-env")
	result := ParseSkillFlagInternal([]string{"atmos", "--skill", "from-cli", "terraform", "plan"})
	assert.Equal(t, []string{"from-cli"}, result, "CLI --skill should override ATMOS_SKILL env var")
}

// TestSplitCSV tests the SplitCSV helper function.
func TestSplitCSV(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{name: "empty string", input: "", expected: nil},
		{name: "single value", input: "a", expected: []string{"a"}},
		{name: "two values", input: "a,b", expected: []string{"a", "b"}},
		{name: "values with whitespace", input: " a , b , c ", expected: []string{"a", "b", "c"}},
		{name: "trailing comma", input: "a,b,", expected: []string{"a", "b"}},
		{name: "leading comma", input: ",a,b", expected: []string{"a", "b"}},
		{name: "multiple commas", input: "a,,b", expected: []string{"a", "b"}},
		{name: "only commas", input: ",,", expected: nil},
		{name: "whitespace only values", input: " , , ", expected: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SplitCSV(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
