package skills

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/ai/skills/builtin"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Skill represents a specialized AI assistant with specific expertise and tool access.
// Skills follow the Agent Skills open standard (https://agentskills.io).
type Skill struct {
	// Name is the unique identifier for the skill (e.g., "stack-analyzer").
	Name string

	// DisplayName is the user-facing name (e.g., "Stack Analyzer").
	DisplayName string

	// Description explains what this skill does.
	Description string

	// SystemPrompt contains specialized instructions for the AI.
	// This is either set directly or loaded from SystemPromptPath.
	SystemPrompt string

	// SystemPromptPath is the path to the SKILL.md file in the embedded filesystem.
	// If set, the prompt will be loaded from this file when LoadSystemPrompt() is called.
	// For built-in skills, use the directory format (e.g., "general/SKILL.md").
	SystemPromptPath string

	// AllowedTools lists tool names this skill can use.
	// Empty list means all tools are allowed.
	AllowedTools []string

	// RestrictedTools lists tools requiring extra confirmation.
	RestrictedTools []string

	// Category groups skills by purpose (e.g., "analysis", "refactor", "security").
	Category string

	// IsBuiltIn indicates if this is a built-in skill.
	IsBuiltIn bool
}

// FromConfig creates a Skill from configuration.
func FromConfig(name string, config *schema.AISkillConfig) *Skill {
	return &Skill{
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

// IsToolAllowed checks if a tool is allowed for this skill.
func (s *Skill) IsToolAllowed(toolName string) bool {
	// If AllowedTools is empty, all tools are allowed.
	if len(s.AllowedTools) == 0 {
		return true
	}

	// Check if tool is in allowed list.
	for _, allowed := range s.AllowedTools {
		if allowed == toolName {
			return true
		}
	}

	return false
}

// IsToolRestricted checks if a tool requires extra confirmation.
func (s *Skill) IsToolRestricted(toolName string) bool {
	for _, restricted := range s.RestrictedTools {
		if restricted == toolName {
			return true
		}
	}
	return false
}

// LoadSystemPrompt loads the skill's system prompt from the embedded filesystem.
// If SystemPromptPath is empty, it returns the existing SystemPrompt value.
// This allows skills to use either hardcoded prompts or file-based prompts.
func (s *Skill) LoadSystemPrompt() (string, error) {
	// If no path specified, use hardcoded prompt (backward compatibility).
	if s.SystemPromptPath == "" {
		return s.SystemPrompt, nil
	}

	// Load prompt from embedded filesystem (parses SKILL.md frontmatter).
	content, err := builtin.Read(s.SystemPromptPath)
	if err != nil {
		return "", fmt.Errorf("failed to load system prompt for skill %q: %w", s.Name, err)
	}

	return content, nil
}
