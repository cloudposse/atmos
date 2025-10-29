package agents

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/ai/agents/prompts"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Agent represents a specialized AI assistant with specific expertise and tool access.
type Agent struct {
	// Name is the unique identifier for the agent (e.g., "stack-analyzer").
	Name string

	// DisplayName is the user-facing name (e.g., "Stack Analyzer").
	DisplayName string

	// Description explains what this agent does.
	Description string

	// SystemPrompt contains specialized instructions for the AI.
	// This is either set directly or loaded from SystemPromptPath.
	SystemPrompt string

	// SystemPromptPath is the path to the prompt file in the embedded filesystem.
	// If set, the prompt will be loaded from this file when LoadSystemPrompt() is called.
	// For built-in agents, this should be just the filename (e.g., "general.md").
	SystemPromptPath string

	// AllowedTools lists tool names this agent can use.
	// Empty list means all tools are allowed.
	AllowedTools []string

	// RestrictedTools lists tools requiring extra confirmation.
	RestrictedTools []string

	// Category groups agents by purpose (e.g., "analysis", "refactor", "security").
	Category string

	// IsBuiltIn indicates if this is a built-in agent.
	IsBuiltIn bool
}

// FromConfig creates an Agent from configuration.
func FromConfig(name string, config *schema.AIAgentConfig) *Agent {
	return &Agent{
		Name:            name,
		DisplayName:     config.DisplayName,
		Description:     config.Description,
		SystemPrompt:    config.SystemPrompt,
		AllowedTools:    config.AllowedTools,
		RestrictedTools: config.RestrictedTools,
		Category:        config.Category,
		IsBuiltIn:       false,
	}
}

// IsToolAllowed checks if a tool is allowed for this agent.
func (a *Agent) IsToolAllowed(toolName string) bool {
	// If AllowedTools is empty, all tools are allowed.
	if len(a.AllowedTools) == 0 {
		return true
	}

	// Check if tool is in allowed list.
	for _, allowed := range a.AllowedTools {
		if allowed == toolName {
			return true
		}
	}

	return false
}

// IsToolRestricted checks if a tool requires extra confirmation.
func (a *Agent) IsToolRestricted(toolName string) bool {
	for _, restricted := range a.RestrictedTools {
		if restricted == toolName {
			return true
		}
	}
	return false
}

// LoadSystemPrompt loads the agent's system prompt from the embedded filesystem.
// If SystemPromptPath is empty, it returns the existing SystemPrompt value.
// This allows agents to use either hardcoded prompts or file-based prompts.
func (a *Agent) LoadSystemPrompt() (string, error) {
	// If no path specified, use hardcoded prompt (backward compatibility).
	if a.SystemPromptPath == "" {
		return a.SystemPrompt, nil
	}

	// Load prompt from embedded filesystem.
	content, err := prompts.Read(a.SystemPromptPath)
	if err != nil {
		return "", fmt.Errorf("failed to load system prompt for agent %q: %w", a.Name, err)
	}

	return content, nil
}
