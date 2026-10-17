package skills

import (
	log "github.com/charmbracelet/log"

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

	loadProjectSkills(atmosConfig, registry, marketplaceLoader)
	// 1. Load marketplace-installed skills.
	if len(marketplaceLoader) > 0 && marketplaceLoader[0] != nil {
		_ = marketplaceLoader[0].LoadInstalledSkills(registry)
	}

	// 2. Load custom skills from configuration if available.
	if atmosConfig != nil && len(atmosConfig.AI.Skills) > 0 {
		for name, config := range atmosConfig.AI.Skills {
			if config == nil || config.Source != "" {
				continue
			}
			skill := FromConfig(name, config)
			if err := registry.Register(skill); err != nil {
				log.Warnf("Shadowed or invalid inline skill %q: %v", name, err)
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

func loadProjectSkills(config *schema.AtmosConfiguration, registry *Registry, loaders []SkillLoader) {
	if config == nil || len(loaders) == 0 {
		return
	}
	loader, ok := loaders[0].(interface{ LoadProjectSkills(*Registry, string) error })
	if !ok {
		return
	}
	base := config.BasePath
	if base == "" {
		base = config.CliConfigPath
	}
	if err := loader.LoadProjectSkills(registry, base); err != nil {
		log.Warnf("Failed to load project skills: %v", err)
	}
}
