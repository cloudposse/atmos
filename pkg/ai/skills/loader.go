package skills

import (
	"github.com/cloudposse/atmos/pkg/schema"
)

// SkillLoader is the interface for loading marketplace-installed skills into a registry.
// This allows the loader to be decoupled from the marketplace package.
type SkillLoader interface {
	LoadInstalledSkills(registry *Registry) error
}

// LoadSkills loads all skills (marketplace-installed and custom) from configuration.
// If a marketplaceLoader is provided, it loads marketplace-installed skills first.
func LoadSkills(atmosConfig *schema.AtmosConfiguration, marketplaceLoader ...SkillLoader) (*Registry, error) {
	registry := NewRegistry()

	// 1. Load marketplace-installed skills.
	if len(marketplaceLoader) > 0 && marketplaceLoader[0] != nil {
		_ = marketplaceLoader[0].LoadInstalledSkills(registry)
	}

	// 2. Load custom skills from configuration if available.
	if atmosConfig != nil && len(atmosConfig.AI.Skills) > 0 {
		for name, config := range atmosConfig.AI.Skills {
			skill := FromConfig(name, config)
			if err := registry.Register(skill); err != nil {
				// Log warning but continue - don't fail if custom skill is invalid.
				continue
			}
		}
	}

	return registry, nil
}

// GetDefaultSkill returns the name of the default skill from configuration.
// Returns empty string if not specified (caller should handle fallback).
func GetDefaultSkill(atmosConfig *schema.AtmosConfiguration) string {
	if atmosConfig != nil && atmosConfig.AI.DefaultSkill != "" {
		return atmosConfig.AI.DefaultSkill
	}
	return ""
}
