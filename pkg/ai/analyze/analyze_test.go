package analyze

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// mockClient implements messageSender for testing.
type mockClient struct {
	response string
	err      error
	called   bool
	prompt   string
}

func (m *mockClient) SendMessage(_ context.Context, message string) (string, error) {
	m.called = true
	m.prompt = message
	return m.response, m.err
}

// withMockClient sets a mock client factory for the duration of a test.
func withMockClient(t *testing.T, client messageSender, clientErr error) {
	t.Helper()
	original := clientFactory
	clientFactory = func(_ *schema.AtmosConfiguration) (messageSender, error) {
		return client, clientErr
	}
	t.Cleanup(func() { clientFactory = original })
}

// newInput creates an AnalysisInput for testing.
func newInput(cmd, stdout, stderr string, cmdErr error, skillPrompt string) *AnalysisInput {
	return &AnalysisInput{
		CommandName: cmd,
		Stdout:      stdout,
		Stderr:      stderr,
		CmdErr:      cmdErr,
		SkillPrompt: skillPrompt,
	}
}

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
	prompt := buildAnalysisPrompt(newInput("atmos terraform plan vpc -s prod", "Plan: 3 to add, 0 to change, 0 to destroy.", "", nil, ""))

	assert.Contains(t, prompt, "atmos terraform plan vpc -s prod")
	assert.Contains(t, prompt, "**Status:** Success")
	assert.Contains(t, prompt, "Plan: 3 to add")
	assert.Contains(t, prompt, "concise summary")
}

func TestBuildAnalysisPrompt_Error(t *testing.T) {
	cmdErr := errors.New("exit status 1")
	prompt := buildAnalysisPrompt(newInput(
		"atmos terraform apply vpc -s prod",
		"",
		"Error: No valid credential sources found",
		cmdErr,
		"",
	))

	assert.Contains(t, prompt, "atmos terraform apply vpc -s prod")
	assert.Contains(t, prompt, "**Status:** Failed")
	assert.Contains(t, prompt, "exit status 1")
	assert.Contains(t, prompt, "No valid credential sources found")
	assert.Contains(t, prompt, "step-by-step instructions to fix")
}

func TestBuildAnalysisPrompt_EmptyOutput(t *testing.T) {
	prompt := buildAnalysisPrompt(newInput("atmos version", "", "", nil, ""))

	assert.Empty(t, prompt, "empty output with no error should produce empty prompt")
}

func TestBuildAnalysisPrompt_OnlyStderr(t *testing.T) {
	prompt := buildAnalysisPrompt(newInput("atmos terraform plan", "", "Warning: something happened", nil, ""))

	assert.Contains(t, prompt, "**Standard Error:**")
	assert.Contains(t, prompt, "Warning: something happened")
	assert.NotContains(t, prompt, "**Standard Output:**")
}

func TestBuildAnalysisPrompt_ErrorWithNoOutput(t *testing.T) {
	// Even with no stdout/stderr, an error should produce a prompt.
	cmdErr := errors.New("command failed")
	prompt := buildAnalysisPrompt(newInput("atmos terraform plan", "", "", cmdErr, ""))

	assert.NotEmpty(t, prompt)
	assert.Contains(t, prompt, "**Status:** Failed")
	assert.Contains(t, prompt, "command failed")
}

func TestBuildAnalysisPrompt_BothStreams(t *testing.T) {
	prompt := buildAnalysisPrompt(newInput(
		"atmos describe stacks",
		"stack1:\n  components:\n    vpc: {}",
		"Warning: deprecated feature",
		nil,
		"",
	))

	assert.Contains(t, prompt, "**Standard Output:**")
	assert.Contains(t, prompt, "**Standard Error:**")
	assert.Contains(t, prompt, "stack1:")
	assert.Contains(t, prompt, "deprecated feature")
	assert.Contains(t, prompt, "**Status:** Success")
}

func TestBuildAnalysisPrompt_ContainsSystemPrompt(t *testing.T) {
	prompt := buildAnalysisPrompt(newInput("atmos version", "1.209.0", "", nil, ""))

	assert.Contains(t, prompt, "Atmos AI")
	assert.Contains(t, prompt, "infrastructure-as-code")
}

func TestBuildAnalysisPrompt_WhitespaceOnlyOutput(t *testing.T) {
	prompt := buildAnalysisPrompt(newInput("atmos version", "   \n  \t  ", "  \n  ", nil, ""))

	// Whitespace-only output with no error should produce empty prompt.
	assert.Empty(t, prompt)
}

func TestBuildAnalysisPrompt_OutputWithoutTrailingNewline(t *testing.T) {
	prompt := buildAnalysisPrompt(newInput("atmos version", "1.209.0", "", nil, ""))

	// Output without trailing newline should get one added before the closing ```.
	assert.Contains(t, prompt, "1.209.0\n```")
}

func TestBuildAnalysisPrompt_OutputWithTrailingNewline(t *testing.T) {
	prompt := buildAnalysisPrompt(newInput("atmos version", "1.209.0\n", "", nil, ""))

	// Output with trailing newline should not get a double newline.
	assert.Contains(t, prompt, "1.209.0\n```")
	assert.NotContains(t, prompt, "1.209.0\n\n```")
}

func TestTruncateOutput(t *testing.T) {
	t.Run("short output unchanged", func(t *testing.T) {
		result := truncateOutput("short")
		assert.Equal(t, "short", result)
	})

	t.Run("empty output unchanged", func(t *testing.T) {
		result := truncateOutput("")
		assert.Equal(t, "", result)
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

func TestAnalyzeOutput_EmptyOutput(t *testing.T) {
	mock := &mockClient{response: "analysis"}
	withMockClient(t, mock, nil)

	cfg := &schema.AtmosConfiguration{}

	// Empty output with no error should skip analysis entirely.
	AnalyzeOutput(cfg, newInput("atmos version", "", "", nil, ""))
	assert.False(t, mock.called, "should not call AI client for empty output")
}

func TestAnalyzeOutput_ClientCreationError(t *testing.T) {
	withMockClient(t, nil, errors.New("failed to create client"))

	cfg := &schema.AtmosConfiguration{}

	// Should not panic when client creation fails.
	AnalyzeOutput(cfg, newInput("atmos terraform plan", "some output", "", nil, ""))
}

func TestAnalyzeOutput_SendMessageSuccess(t *testing.T) {
	mock := &mockClient{response: "## Summary\nAll good!"}
	withMockClient(t, mock, nil)

	cfg := &schema.AtmosConfiguration{}

	AnalyzeOutput(cfg, newInput("atmos terraform plan vpc -s prod", "Plan: 1 to add", "", nil, ""))

	assert.True(t, mock.called, "should call AI client")
	assert.Contains(t, mock.prompt, "atmos terraform plan vpc -s prod")
	assert.Contains(t, mock.prompt, "Plan: 1 to add")
	assert.Contains(t, mock.prompt, "**Status:** Success")
}

func TestAnalyzeOutput_SendMessageError(t *testing.T) {
	mock := &mockClient{err: errors.New("API rate limit exceeded")}
	withMockClient(t, mock, nil)

	cfg := &schema.AtmosConfiguration{}

	// Should not panic when AI call fails.
	AnalyzeOutput(cfg, newInput("atmos terraform plan", "output", "", nil, ""))

	assert.True(t, mock.called)
}

func TestAnalyzeOutput_WithCommandError(t *testing.T) {
	mock := &mockClient{response: "Error analysis"}
	withMockClient(t, mock, nil)

	cfg := &schema.AtmosConfiguration{}
	cmdErr := errors.New("exit status 1")

	AnalyzeOutput(cfg, newInput("atmos terraform apply", "", "Error: access denied", cmdErr, ""))

	assert.True(t, mock.called)
	assert.Contains(t, mock.prompt, "**Status:** Failed")
	assert.Contains(t, mock.prompt, "exit status 1")
	assert.Contains(t, mock.prompt, "Error: access denied")
}

func TestAnalyzeOutput_CustomTimeout(t *testing.T) {
	mock := &mockClient{response: "done"}
	withMockClient(t, mock, nil)

	cfg := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			TimeoutSeconds: 30,
		},
	}

	AnalyzeOutput(cfg, newInput("atmos list stacks", "stack1\nstack2", "", nil, ""))

	assert.True(t, mock.called)
}

func TestAnalyzeOutput_WhitespaceOnlyOutput(t *testing.T) {
	mock := &mockClient{response: "analysis"}
	withMockClient(t, mock, nil)

	cfg := &schema.AtmosConfiguration{}

	// Whitespace-only output with no error should skip analysis.
	AnalyzeOutput(cfg, newInput("atmos version", "  \n  ", "  \t  ", nil, ""))
	assert.False(t, mock.called, "should not call AI client for whitespace-only output")
}

func TestAnalyzeOutput_ErrorWithNoOutput(t *testing.T) {
	mock := &mockClient{response: "error explanation"}
	withMockClient(t, mock, nil)

	cfg := &schema.AtmosConfiguration{}
	cmdErr := errors.New("command not found")

	// Error with no output should still trigger analysis.
	AnalyzeOutput(cfg, newInput("atmos foo", "", "", cmdErr, ""))

	assert.True(t, mock.called, "should call AI client when there's an error even with no output")
	assert.Contains(t, mock.prompt, "command not found")
}

func TestBuildAnalysisPrompt_WithSkillPrompt(t *testing.T) {
	skillPrompt := "You are an expert in Terraform orchestration. Analyze plan output for security issues."
	prompt := buildAnalysisPrompt(newInput("atmos terraform plan vpc -s prod", "Plan: 1 to add", "", nil, skillPrompt))

	// Skill prompt should appear before the system prompt.
	skillIdx := strings.Index(prompt, skillPrompt)
	systemIdx := strings.Index(prompt, "Atmos AI")
	assert.Greater(t, systemIdx, skillIdx, "skill prompt should appear before system prompt")

	// Both should be present.
	assert.Contains(t, prompt, skillPrompt)
	assert.Contains(t, prompt, "Atmos AI")
	assert.Contains(t, prompt, "Plan: 1 to add")
}

func TestBuildAnalysisPrompt_WithoutSkillPrompt(t *testing.T) {
	// Empty skill prompt should not add any prefix.
	prompt := buildAnalysisPrompt(newInput("atmos version", "1.209.0", "", nil, ""))

	// Should start with the system prompt (no skill prefix).
	assert.True(t, strings.HasPrefix(prompt, systemPrompt), "prompt should start with system prompt when no skill is provided")
}

func TestAnalyzeOutput_WithSkillPrompt(t *testing.T) {
	mock := &mockClient{response: "## Terraform Analysis\nLooks good!"}
	withMockClient(t, mock, nil)

	cfg := &schema.AtmosConfiguration{}
	skillPrompt := "You are a Terraform expert."

	AnalyzeOutput(cfg, newInput("atmos terraform plan vpc -s prod", "Plan: 1 to add", "", nil, skillPrompt))

	assert.True(t, mock.called, "should call AI client")
	assert.Contains(t, mock.prompt, "You are a Terraform expert.")
	assert.Contains(t, mock.prompt, "Plan: 1 to add")
}
