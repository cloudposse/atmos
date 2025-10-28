package agents

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/schema"
)

// LoadAgents loads all agents (built-in and custom) from configuration.
func LoadAgents(atmosConfig *schema.AtmosConfiguration) (*Registry, error) {
	registry := NewRegistry()

	// Register all built-in agents.
	builtinAgents := GetBuiltInAgents()
	for _, agent := range builtinAgents {
		if err := registry.Register(agent); err != nil {
			return nil, fmt.Errorf("failed to register built-in agent %s: %w", agent.Name, err)
		}
	}

	// Load custom agents from configuration if available.
	if atmosConfig != nil && len(atmosConfig.Settings.AI.Agents) > 0 {
		for name, config := range atmosConfig.Settings.AI.Agents {
			agent := FromConfig(name, config)
			if err := registry.Register(agent); err != nil {
				// Log warning but continue - don't fail if custom agent is invalid.
				// This allows the system to still work with built-in agents.
				continue
			}
		}
	}

	return registry, nil
}

// GetDefaultAgent returns the name of the default agent from configuration.
// Returns "general" if not specified.
func GetDefaultAgent(atmosConfig *schema.AtmosConfiguration) string {
	if atmosConfig != nil && atmosConfig.Settings.AI.DefaultAgent != "" {
		return atmosConfig.Settings.AI.DefaultAgent
	}
	return GeneralAgent
}
