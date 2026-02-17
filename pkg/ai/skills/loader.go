package skills

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/schema"
)

// LoadSkills loads all skills (built-in and custom) from configuration.
func LoadSkills(atmosConfig *schema.AtmosConfiguration) (*Registry, error) {
	registry := NewRegistry()

	// Register all built-in skills.
	builtinSkills := GetBuiltInSkills()
	for _, skill := range builtinSkills {
		if err := registry.Register(skill); err != nil {
			return nil, fmt.Errorf("failed to register built-in skill %s: %w", skill.Name, err)
		}
	}

	// Load custom skills from configuration if available.
	if atmosConfig != nil && len(atmosConfig.Settings.AI.Skills) > 0 {
		for name, config := range atmosConfig.Settings.AI.Skills {
			skill := FromConfig(name, config)
			if err := registry.Register(skill); err != nil {
				// Log warning but continue - don't fail if custom skill is invalid.
				// This allows the system to still work with built-in skills.
				continue
			}
		}
	}

	return registry, nil
}

// GetDefaultSkill returns the name of the default skill from configuration.
// Returns "general" if not specified.
func GetDefaultSkill(atmosConfig *schema.AtmosConfiguration) string {
	if atmosConfig != nil && atmosConfig.Settings.AI.DefaultSkill != "" {
		return atmosConfig.Settings.AI.DefaultSkill
	}
	return GeneralSkill
}
