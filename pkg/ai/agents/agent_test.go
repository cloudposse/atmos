package agents

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgent_LoadSystemPrompt_WithFile(t *testing.T) {
	// Test loading prompt from embedded file.
	agent := &Agent{
		Name:             GeneralAgent,
		DisplayName:      "General",
		SystemPromptPath: "general.md",
	}

	prompt, err := agent.LoadSystemPrompt()
	require.NoError(t, err)
	assert.NotEmpty(t, prompt)

	// Verify it's Markdown content.
	assert.True(t, strings.HasPrefix(prompt, "# Agent: General"))
	assert.Contains(t, prompt, "## Role")
	assert.Contains(t, prompt, "## Your Expertise")
	assert.Contains(t, prompt, "IMPORTANT")
}

func TestAgent_LoadSystemPrompt_WithoutFile(t *testing.T) {
	// Test falling back to hardcoded prompt when no path is set.
	hardcodedPrompt := "This is a hardcoded system prompt."
	agent := &Agent{
		Name:         "test-agent",
		DisplayName:  "Test Agent",
		SystemPrompt: hardcodedPrompt,
		// SystemPromptPath is empty, should use SystemPrompt.
	}

	prompt, err := agent.LoadSystemPrompt()
	require.NoError(t, err)
	assert.Equal(t, hardcodedPrompt, prompt)
}

func TestAgent_LoadSystemPrompt_FileNotFound(t *testing.T) {
	// Test error handling when file doesn't exist.
	agent := &Agent{
		Name:             "test-agent",
		DisplayName:      "Test Agent",
		SystemPromptPath: "nonexistent.md",
	}

	prompt, err := agent.LoadSystemPrompt()
	assert.Error(t, err)
	assert.Empty(t, prompt)
	assert.Contains(t, err.Error(), "failed to load system prompt")
	assert.Contains(t, err.Error(), "nonexistent.md")
}

func TestBuiltInAgents_AllPromptsLoad(t *testing.T) {
	// Test that all built-in agents can load their prompts successfully.
	builtInAgents := GetBuiltInAgents()
	require.NotEmpty(t, builtInAgents, "Should have built-in agents")

	for _, agent := range builtInAgents {
		t.Run(agent.Name, func(t *testing.T) {
			prompt, err := agent.LoadSystemPrompt()
			require.NoError(t, err, "Failed to load prompt for %s", agent.Name)
			assert.NotEmpty(t, prompt, "Prompt should not be empty for %s", agent.Name)

			// Verify prompt structure.
			assert.True(t, strings.HasPrefix(prompt, "# Agent:"), "Prompt should start with '# Agent:' for %s", agent.Name)
			assert.Contains(t, prompt, "## Role", "Prompt should have '## Role' section for %s", agent.Name)
			assert.Contains(t, prompt, "## Your Expertise", "Prompt should have '## Your Expertise' section for %s", agent.Name)
			// Instructions section may be named differently (## Instructions or ## Core Instructions).
			hasInstructions := strings.Contains(prompt, "## Instructions") || strings.Contains(prompt, "## Core Instructions")
			assert.True(t, hasInstructions, "Prompt should have instructions section for %s", agent.Name)

			// Verify prompt has reasonable length (at least 1KB).
			assert.Greater(t, len(prompt), 1024, "Prompt should be substantial (>1KB) for %s", agent.Name)
		})
	}
}

func TestBuiltInAgents_PromptConsistency(t *testing.T) {
	// Test that loading the same agent's prompt multiple times returns consistent results.
	agent := &Agent{
		Name:             StackAnalyzerAgent,
		DisplayName:      "Stack Analyzer",
		SystemPromptPath: "stack-analyzer.md",
	}

	prompt1, err1 := agent.LoadSystemPrompt()
	require.NoError(t, err1)

	prompt2, err2 := agent.LoadSystemPrompt()
	require.NoError(t, err2)

	assert.Equal(t, prompt1, prompt2, "Prompt should be consistent across multiple loads")
}

func TestAgent_LoadSystemPrompt_UpdatesAgent(t *testing.T) {
	// Test that loading a prompt doesn't modify the original agent's SystemPrompt field.
	agent := &Agent{
		Name:             GeneralAgent,
		DisplayName:      "General",
		SystemPrompt:     "Original prompt",
		SystemPromptPath: "general.md",
	}

	loadedPrompt, err := agent.LoadSystemPrompt()
	require.NoError(t, err)

	// Original SystemPrompt should be unchanged.
	assert.Equal(t, "Original prompt", agent.SystemPrompt)

	// Loaded prompt should be different and come from the file.
	assert.NotEqual(t, "Original prompt", loadedPrompt)
	assert.Contains(t, loadedPrompt, "# Agent: General")
}
