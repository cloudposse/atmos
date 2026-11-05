package config

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func validateSkillSources(config *schema.AtmosConfiguration) error {
	for label, d := range config.AI.Skills {
		if d == nil {
			continue
		}
		if d.Source == "" {
			if !d.Ref.IsZero() || d.Kind != "" || d.Subpath != "" || d.Scope != "" || len(d.Plugins)+len(d.Include)+len(d.Exclude)+len(d.Clients) > 0 {
				return fmt.Errorf("%w: %s requires source", errUtils.ErrAISkillSourceInvalid, label)
			}
			continue
		}
		if hasInlineSkillFields(d) {
			return fmt.Errorf("%w: %s mixes source and inline fields", errUtils.ErrAISkillSourceInvalid, label)
		}
	}
	return nil
}

func hasInlineSkillFields(d *schema.AISkillConfig) bool {
	return d.SystemPrompt != "" || d.DisplayName != "" || d.Description != "" || d.Category != "" || len(d.AllowedTools)+len(d.RestrictedTools) > 0
}
