package skills

import (
	"github.com/cloudposse/atmos/pkg/schema"
)

// SkillLoader is the interface for loading marketplace-installed skills into a registry.
// This allows the loader to be decoupled from the marketplace package.
type SkillLoader interface {
	LoadInstalledSkills(registry *Registry) error
}

// LoadSkills loads all skills (marketplace-installed, custom-configured, and
// built-in) into a fresh registry.
//
// Loaders are invoked in the order given. The registry's Register() is
// first-writer-wins, so earlier loaders override later ones. Callers that want
// marketplace-installed skills to override built-in embedded skills should pass
// the marketplace loader first and the embedded loader second (see the
// agent-skills/embedded package).
func LoadSkills(atmosConfig *schema.AtmosConfiguration, loaders ...SkillLoader) (*Registry, error) {
	registry := NewRegistry()

	// 1. Run every loader in order.
	for _, loader := range loaders {
		if loader == nil {
			continue
		}
		_ = loader.LoadInstalledSkills(registry)
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
