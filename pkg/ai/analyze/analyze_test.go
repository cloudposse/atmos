package analyze

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestValidateAIConfig_NotEnabled(t *testing.T) {
	cfg := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled: false,
		},
	}

	err := ValidateAIConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AI features are not enabled")
}

func TestValidateAIConfig_NoProvider(t *testing.T) {
	cfg := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled:   true,
			Providers: nil,
		},
	}

	err := ValidateAIConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported AI provider")
}

func TestValidateAIConfig_ProviderNotConfigured(t *testing.T) {
	cfg := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled:         true,
			DefaultProvider: "openai",
			Providers: map[string]*schema.AIProviderConfig{
				"anthropic": {Model: "claude-sonnet-4-5-20250514", ApiKey: "sk-test"},
			},
		},
	}

	err := ValidateAIConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "openai")
}

func TestValidateAIConfig_NoAPIKey(t *testing.T) {
	cfg := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled:         true,
			DefaultProvider: "anthropic",
			Providers: map[string]*schema.AIProviderConfig{
				"anthropic": {Model: "claude-sonnet-4-5-20250514", ApiKey: ""},
			},
		},
	}

	err := ValidateAIConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API key not found")
}

func TestValidateAIConfig_Valid(t *testing.T) {
	cfg := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled:         true,
			DefaultProvider: "anthropic",
			Providers: map[string]*schema.AIProviderConfig{
				"anthropic": {Model: "claude-sonnet-4-5-20250514", ApiKey: "sk-test-key"},
			},
		},
	}

	err := ValidateAIConfig(cfg)
	assert.NoError(t, err)
}

func TestValidateAIConfig_DefaultsToAnthropic(t *testing.T) {
	// When DefaultProvider is empty, it defaults to "anthropic".
	cfg := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled:         true,
			DefaultProvider: "", // Should default to "anthropic".
			Providers: map[string]*schema.AIProviderConfig{
				"anthropic": {Model: "claude-sonnet-4-5-20250514", ApiKey: "sk-test-key"},
			},
		},
	}

	err := ValidateAIConfig(cfg)
	assert.NoError(t, err)
}

func TestBuildAnalysisPrompt_Success(t *testing.T) {
	prompt := buildAnalysisPrompt("atmos terraform plan vpc -s prod", "Plan: 3 to add, 0 to change, 0 to destroy.", "", nil)

	assert.Contains(t, prompt, "atmos terraform plan vpc -s prod")
	assert.Contains(t, prompt, "**Status:** Success")
	assert.Contains(t, prompt, "Plan: 3 to add")
	assert.Contains(t, prompt, "concise summary")
}

func TestBuildAnalysisPrompt_Error(t *testing.T) {
	cmdErr := errors.New("exit status 1")
	prompt := buildAnalysisPrompt(
		"atmos terraform apply vpc -s prod",
		"",
		"Error: No valid credential sources found",
		cmdErr,
	)

	assert.Contains(t, prompt, "atmos terraform apply vpc -s prod")
	assert.Contains(t, prompt, "**Status:** Failed")
	assert.Contains(t, prompt, "exit status 1")
	assert.Contains(t, prompt, "No valid credential sources found")
	assert.Contains(t, prompt, "step-by-step instructions to fix")
}

func TestBuildAnalysisPrompt_EmptyOutput(t *testing.T) {
	prompt := buildAnalysisPrompt("atmos version", "", "", nil)

	assert.Empty(t, prompt, "empty output with no error should produce empty prompt")
}

func TestBuildAnalysisPrompt_OnlyStderr(t *testing.T) {
	prompt := buildAnalysisPrompt("atmos terraform plan", "", "Warning: something happened", nil)

	assert.Contains(t, prompt, "**Standard Error:**")
	assert.Contains(t, prompt, "Warning: something happened")
	assert.NotContains(t, prompt, "**Standard Output:**")
}

func TestBuildAnalysisPrompt_ErrorWithNoOutput(t *testing.T) {
	// Even with no stdout/stderr, an error should produce a prompt.
	cmdErr := errors.New("command failed")
	prompt := buildAnalysisPrompt("atmos terraform plan", "", "", cmdErr)

	assert.NotEmpty(t, prompt)
	assert.Contains(t, prompt, "**Status:** Failed")
	assert.Contains(t, prompt, "command failed")
}

func TestBuildAnalysisPrompt_BothStreams(t *testing.T) {
	prompt := buildAnalysisPrompt(
		"atmos describe stacks",
		"stack1:\n  components:\n    vpc: {}",
		"Warning: deprecated feature",
		nil,
	)

	assert.Contains(t, prompt, "**Standard Output:**")
	assert.Contains(t, prompt, "**Standard Error:**")
	assert.Contains(t, prompt, "stack1:")
	assert.Contains(t, prompt, "deprecated feature")
	assert.Contains(t, prompt, "**Status:** Success")
}

func TestBuildAnalysisPrompt_ContainsSystemPrompt(t *testing.T) {
	prompt := buildAnalysisPrompt("atmos version", "1.209.0", "", nil)

	assert.Contains(t, prompt, "Atmos AI")
	assert.Contains(t, prompt, "infrastructure-as-code")
}

func TestBuildAnalysisPrompt_WhitespaceOnlyOutput(t *testing.T) {
	prompt := buildAnalysisPrompt("atmos version", "   \n  \t  ", "  \n  ", nil)

	// Whitespace-only output with no error should produce empty prompt.
	assert.Empty(t, prompt)
}

func TestTruncateOutput(t *testing.T) {
	t.Run("short output unchanged", func(t *testing.T) {
		result := truncateOutput("short")
		assert.Equal(t, "short", result)
	})

	t.Run("output at limit unchanged", func(t *testing.T) {
		input := strings.Repeat("a", maxOutputLength)
		result := truncateOutput(input)
		assert.Equal(t, input, result)
	})

	t.Run("long output truncated", func(t *testing.T) {
		input := strings.Repeat("a", maxOutputLength+100)
		result := truncateOutput(input)
		assert.Len(t, result, maxOutputLength+len("\n... (output truncated)"))
		assert.True(t, strings.HasSuffix(result, "\n... (output truncated)"))
	})
}
