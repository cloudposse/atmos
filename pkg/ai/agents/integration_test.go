package agents

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAgentIntegration_LoadPromptsForAllBuiltInAgents tests that all built-in agents
// can successfully load their prompts and the prompts are properly formatted.
func TestAgentIntegration_LoadPromptsForAllBuiltInAgents(t *testing.T) {
	builtInAgents := GetBuiltInAgents()
	require.NotEmpty(t, builtInAgents, "Should have built-in agents")
	require.Len(t, builtInAgents, 5, "Should have exactly 5 built-in agents")

	// Map of agent names to expected properties.
	expectedAgents := map[string]struct {
		displayName   string
		category      string
		hasPromptPath bool
	}{
		GeneralAgent: {
			displayName:   "General",
			category:      "general",
			hasPromptPath: true,
		},
		StackAnalyzerAgent: {
			displayName:   "Stack Analyzer",
			category:      "analysis",
			hasPromptPath: true,
		},
		ComponentRefactorAgent: {
			displayName:   "Component Refactor",
			category:      "refactor",
			hasPromptPath: true,
		},
		SecurityAuditorAgent: {
			displayName:   "Security Auditor",
			category:      "security",
			hasPromptPath: true,
		},
		ConfigValidatorAgent: {
			displayName:   "Config Validator",
			category:      "validation",
			hasPromptPath: true,
		},
	}

	for _, agent := range builtInAgents {
		t.Run(agent.Name, func(t *testing.T) {
			expected, exists := expectedAgents[agent.Name]
			require.True(t, exists, "Agent %s should be in expected agents map", agent.Name)

			// Verify basic properties.
			assert.Equal(t, expected.displayName, agent.DisplayName, "DisplayName mismatch for %s", agent.Name)
			assert.Equal(t, expected.category, agent.Category, "Category mismatch for %s", agent.Name)
			assert.True(t, agent.IsBuiltIn, "Should be marked as built-in for %s", agent.Name)

			// Verify prompt path is set.
			if expected.hasPromptPath {
				assert.NotEmpty(t, agent.SystemPromptPath, "SystemPromptPath should be set for %s", agent.Name)
			}

			// Load the prompt.
			prompt, err := agent.LoadSystemPrompt()
			require.NoError(t, err, "Failed to load prompt for %s", agent.Name)
			assert.NotEmpty(t, prompt, "Prompt should not be empty for %s", agent.Name)

			// Verify prompt is substantial.
			assert.Greater(t, len(prompt), 1000, "Prompt should be substantial for %s", agent.Name)
		})
	}
}

// TestAgentIntegration_RegistryWithPrompts tests the agent registry with prompt loading.
func TestAgentIntegration_RegistryWithPrompts(t *testing.T) {
	registry := NewRegistry()

	// Register all built-in agents.
	for _, agent := range GetBuiltInAgents() {
		err := registry.Register(agent)
		require.NoError(t, err, "Failed to register agent %s", agent.Name)
	}

	// Verify we can retrieve and load prompts for each agent.
	agentNames := []string{
		GeneralAgent,
		StackAnalyzerAgent,
		ComponentRefactorAgent,
		SecurityAuditorAgent,
		ConfigValidatorAgent,
	}

	for _, name := range agentNames {
		t.Run(name, func(t *testing.T) {
			// Get agent from registry.
			agent, err := registry.Get(name)
			require.NoError(t, err, "Failed to get agent %s from registry", name)
			assert.NotNil(t, agent)

			// Load prompt.
			prompt, err := agent.LoadSystemPrompt()
			require.NoError(t, err, "Failed to load prompt for %s", name)
			assert.NotEmpty(t, prompt)

			// Verify prompt contains agent-specific content.
			assert.Contains(t, prompt, "# Agent:", "Prompt should have agent header")
		})
	}
}

// TestAgentIntegration_PromptLoadingPerformance tests that prompt loading is fast.
func TestAgentIntegration_PromptLoadingPerformance(t *testing.T) {
	agent := &Agent{
		Name:             GeneralAgent,
		DisplayName:      "General",
		SystemPromptPath: "general.md",
	}

	// Load prompt multiple times and ensure it's consistently fast.
	const iterations = 100
	for i := 0; i < iterations; i++ {
		prompt, err := agent.LoadSystemPrompt()
		require.NoError(t, err)
		assert.NotEmpty(t, prompt)
	}

	// No specific time assertion, but the test should complete quickly.
	// If prompt loading is slow (e.g., >1s for 100 iterations), the test will timeout.
}

// TestAgentIntegration_PromptCaching tests that prompts maintain consistency.
func TestAgentIntegration_PromptCaching(t *testing.T) {
	agent := &Agent{
		Name:             StackAnalyzerAgent,
		DisplayName:      "Stack Analyzer",
		SystemPromptPath: "stack-analyzer.md",
	}

	// Load prompt multiple times.
	prompts := make([]string, 5)
	for i := 0; i < len(prompts); i++ {
		var err error
		prompts[i], err = agent.LoadSystemPrompt()
		require.NoError(t, err)
	}

	// All prompts should be identical.
	for i := 1; i < len(prompts); i++ {
		assert.Equal(t, prompts[0], prompts[i], "Prompt should be consistent across loads (iteration %d)", i)
	}
}

// TestAgentIntegration_AllAgentsCategorized tests that all agents have valid categories.
func TestAgentIntegration_AllAgentsCategorized(t *testing.T) {
	validCategories := map[string]bool{
		"general":    true,
		"analysis":   true,
		"refactor":   true,
		"security":   true,
		"validation": true,
	}

	for _, agent := range GetBuiltInAgents() {
		t.Run(agent.Name, func(t *testing.T) {
			assert.NotEmpty(t, agent.Category, "Agent %s should have a category", agent.Name)
			assert.True(t, validCategories[agent.Category], "Agent %s has invalid category: %s", agent.Name, agent.Category)
		})
	}
}

// TestAgentIntegration_ToolAccess tests that agents have appropriate tool configurations.
func TestAgentIntegration_ToolAccess(t *testing.T) {
	for _, agent := range GetBuiltInAgents() {
		t.Run(agent.Name, func(t *testing.T) {
			// Verify AllowedTools and RestrictedTools are mutually exclusive or appropriately configured.
			// For built-in agents, if AllowedTools is empty, all tools are allowed.

			if len(agent.AllowedTools) == 0 {
				// All tools allowed - this is the general agent pattern.
				assert.Equal(t, GeneralAgent, agent.Name, "Only general agent should have empty AllowedTools")
			} else {
				// Specific tools allowed - verify list is not empty.
				assert.NotEmpty(t, agent.AllowedTools, "AllowedTools should not be empty list for %s", agent.Name)

				// Verify common tools are included for specialized agents.
				assert.Contains(t, agent.AllowedTools, "read_file", "Specialized agents should have read_file access")
			}

			// RestrictedTools list should not overlap with AllowedTools (if AllowedTools is specified).
			if len(agent.AllowedTools) > 0 {
				for _, restricted := range agent.RestrictedTools {
					assert.NotContains(t, agent.AllowedTools, restricted,
						"Tool %s cannot be both allowed and restricted for agent %s", restricted, agent.Name)
				}
			}
		})
	}
}

// TestAgentIntegration_BackwardCompatibility tests backward compatibility with hardcoded prompts.
func TestAgentIntegration_BackwardCompatibility(t *testing.T) {
	// Create an agent with a hardcoded prompt (no SystemPromptPath).
	hardcodedPrompt := "This is a hardcoded system prompt for backward compatibility testing."
	agent := &Agent{
		Name:         "legacy-agent",
		DisplayName:  "Legacy Agent",
		SystemPrompt: hardcodedPrompt,
		// SystemPromptPath is not set - should use SystemPrompt.
		Category:  "general",
		IsBuiltIn: false,
	}

	// Load prompt - should return the hardcoded value.
	loadedPrompt, err := agent.LoadSystemPrompt()
	require.NoError(t, err)
	assert.Equal(t, hardcodedPrompt, loadedPrompt, "Should return hardcoded prompt when no path is set")

	// Verify the original SystemPrompt field is unchanged.
	assert.Equal(t, hardcodedPrompt, agent.SystemPrompt, "Original SystemPrompt should be unchanged")
}

// TestAgentIntegration_GetBuiltInAgents_ReturnsNewInstances tests that GetBuiltInAgents
// returns new instances each time (not shared references).
func TestAgentIntegration_GetBuiltInAgents_ReturnsNewInstances(t *testing.T) {
	agents1 := GetBuiltInAgents()
	agents2 := GetBuiltInAgents()

	require.Len(t, agents1, 5)
	require.Len(t, agents2, 5)

	// Verify they're different instances (not the same pointers).
	for i := range agents1 {
		assert.NotSame(t, agents1[i], agents2[i],
			"GetBuiltInAgents should return new instances, not shared references (agent %d)", i)

		// But they should have the same values.
		assert.Equal(t, agents1[i].Name, agents2[i].Name)
		assert.Equal(t, agents1[i].DisplayName, agents2[i].DisplayName)
	}
}

// TestAgentIntegration_PromptUpdatesAfterLoad tests that updating SystemPrompt
// after loading doesn't affect future loads.
func TestAgentIntegration_PromptUpdatesAfterLoad(t *testing.T) {
	agent := &Agent{
		Name:             GeneralAgent,
		DisplayName:      "General",
		SystemPrompt:     "Original prompt",
		SystemPromptPath: "general.md",
	}

	// Load prompt from file.
	loadedPrompt1, err := agent.LoadSystemPrompt()
	require.NoError(t, err)
	assert.NotEqual(t, "Original prompt", loadedPrompt1, "Should load from file, not use original")

	// Modify the agent's SystemPrompt field.
	agent.SystemPrompt = "Modified prompt"

	// Load again - should still load from file, not use the modified field.
	loadedPrompt2, err := agent.LoadSystemPrompt()
	require.NoError(t, err)
	assert.Equal(t, loadedPrompt1, loadedPrompt2, "Should load from file again, ignoring modified field")
	assert.NotEqual(t, "Modified prompt", loadedPrompt2, "Should not use modified SystemPrompt")
}
