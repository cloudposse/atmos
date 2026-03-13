package skills

import (
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// LoadAndValidate loads the skill registry and validates that all specified skills exist.
// An optional SkillLoader can be provided to load marketplace-installed skills.
// Returns the validated skills or a helpful error listing invalid and available skills.
func LoadAndValidate(atmosConfig *schema.AtmosConfiguration, skillNames []string, loader SkillLoader) ([]*Skill, error) {
	defer perf.Track(nil, "skills.LoadAndValidate")()

	registry, err := LoadSkills(atmosConfig, loader)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrAISkillLoadFailed, err)
	}

	var validSkills []*Skill
	var invalidNames []string

	for _, name := range skillNames {
		skill, err := registry.Get(name)
		if err != nil {
			invalidNames = append(invalidNames, name)
		} else {
			validSkills = append(validSkills, skill)
		}
	}

	if len(invalidNames) > 0 {
		available := registry.List()
		names := make([]string, 0, len(available))
		for _, s := range available {
			names = append(names, s.Name)
		}

		builder := errUtils.Build(errUtils.ErrAISkillNotFound).
			WithExplanationf("The following skills are not installed or configured: %s", strings.Join(invalidNames, ", "))

		if len(names) > 0 {
			builder = builder.WithHintf("Available skills: %s", strings.Join(names, ", "))
		} else {
			builder = builder.WithHint("No skills are installed. Install skills with: atmos ai skill install cloudposse/atmos")
		}

		builder = builder.WithHint("See https://atmos.tools/ai/agent-skills for more information.")

		return nil, builder.Err()
	}

	return validSkills, nil
}

// BuildPrompt concatenates system prompts from multiple skills with a separator.
// Returns an empty string if no skills have system prompts.
func BuildPrompt(validSkills []*Skill) string {
	defer perf.Track(nil, "skills.BuildPrompt")()

	var prompts []string
	for _, s := range validSkills {
		if s.SystemPrompt != "" {
			prompts = append(prompts, s.SystemPrompt)
		}
	}
	return strings.Join(prompts, "\n\n---\n\n")
}
