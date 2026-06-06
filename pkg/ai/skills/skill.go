package skills

import (
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
	SystemPrompt string

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

// NewFallbackSkill returns a minimal fallback skill for when no skills are installed.
func NewFallbackSkill() *Skill {
	return &Skill{
		Name:         "general",
		DisplayName:  "General",
		Description:  "General-purpose assistant for Atmos operations",
		SystemPrompt: "You are Atmos AI, an assistant for cloud infrastructure orchestration using Atmos CLI. You help with Terraform, Helmfile, stack configurations, and infrastructure management. Install specialized skills with 'atmos ai skill install' for deeper domain expertise.",
		Category:     "general",
		IsBuiltIn:    false,
	}
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

// LoadSystemPrompt returns the skill's system prompt.
func (s *Skill) LoadSystemPrompt() (string, error) {
	return s.SystemPrompt, nil
}
