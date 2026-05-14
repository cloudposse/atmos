package skills

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// mockLoader implements SkillLoader for testing.
type mockLoader struct {
	skills []*Skill
	err    error
}

func (m *mockLoader) LoadInstalledSkills(registry *Registry) error {
	if m.err != nil {
		return m.err
	}
	for _, s := range m.skills {
		_ = registry.Register(s)
	}
	return nil
}

// configWithSkills creates an AtmosConfiguration with custom AI skills.
func configWithSkills(skills map[string]*schema.AISkillConfig) *schema.AtmosConfiguration {
	return &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Skills: skills,
		},
	}
}

func TestLoadAndValidate_AllValid(t *testing.T) {
	cfg := configWithSkills(map[string]*schema.AISkillConfig{
		"skill-a": {DisplayName: "Skill A", Description: "First skill", SystemPrompt: "Prompt A"},
		"skill-b": {DisplayName: "Skill B", Description: "Second skill", SystemPrompt: "Prompt B"},
	})

	result, err := LoadAndValidate(cfg, []string{"skill-a", "skill-b"}, nil)
	require.NoError(t, err)
	assert.Len(t, result, 2)

	names := make(map[string]bool)
	for _, s := range result {
		names[s.Name] = true
	}
	assert.True(t, names["skill-a"])
	assert.True(t, names["skill-b"])
}

func TestLoadAndValidate_SingleValid(t *testing.T) {
	cfg := configWithSkills(map[string]*schema.AISkillConfig{
		"my-skill": {DisplayName: "My Skill", Description: "Test", SystemPrompt: "You are helpful."},
	})

	result, err := LoadAndValidate(cfg, []string{"my-skill"}, nil)
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "my-skill", result[0].Name)
}

func TestLoadAndValidate_InvalidSkill(t *testing.T) {
	cfg := configWithSkills(map[string]*schema.AISkillConfig{
		"real-skill": {DisplayName: "Real", Description: "Exists"},
	})

	_, err := LoadAndValidate(cfg, []string{"nonexistent"}, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrAISkillNotFound))
	formatted := errUtils.Format(err, errUtils.DefaultFormatterConfig())
	assert.Contains(t, formatted, "nonexistent")
}

func TestLoadAndValidate_MixedValidAndInvalid(t *testing.T) {
	cfg := configWithSkills(map[string]*schema.AISkillConfig{
		"valid": {DisplayName: "Valid", Description: "Exists"},
	})

	_, err := LoadAndValidate(cfg, []string{"valid", "invalid-a", "invalid-b"}, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrAISkillNotFound))
	formatted := errUtils.Format(err, errUtils.DefaultFormatterConfig())
	assert.Contains(t, formatted, "invalid-a")
	assert.Contains(t, formatted, "invalid-b")
}

func TestLoadAndValidate_InvalidSkillShowsAvailable(t *testing.T) {
	cfg := configWithSkills(map[string]*schema.AISkillConfig{
		"alpha": {DisplayName: "Alpha", Description: "Available"},
		"beta":  {DisplayName: "Beta", Description: "Available"},
	})

	_, err := LoadAndValidate(cfg, []string{"gamma"}, nil)
	require.Error(t, err)
	formatted := errUtils.Format(err, errUtils.DefaultFormatterConfig())
	assert.Contains(t, formatted, "alpha")
	assert.Contains(t, formatted, "beta")
}

func TestLoadAndValidate_EmptyRegistryShowsInstallHint(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}

	_, err := LoadAndValidate(cfg, []string{"anything"}, nil)
	require.Error(t, err)
	formatted := errUtils.Format(err, errUtils.DefaultFormatterConfig())
	assert.Contains(t, formatted, "atmos ai skill install")
}

func TestLoadAndValidate_WithMarketplaceLoader(t *testing.T) {
	loader := &mockLoader{
		skills: []*Skill{
			{Name: "marketplace-skill", DisplayName: "Marketplace", Description: "From marketplace", SystemPrompt: "MP prompt"},
		},
	}
	cfg := &schema.AtmosConfiguration{}

	result, err := LoadAndValidate(cfg, []string{"marketplace-skill"}, loader)
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "marketplace-skill", result[0].Name)
}

func TestLoadAndValidate_EmptySkillNames(t *testing.T) {
	cfg := configWithSkills(map[string]*schema.AISkillConfig{
		"skill": {DisplayName: "Skill", Description: "Test"},
	})

	result, err := LoadAndValidate(cfg, []string{}, nil)
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestLoadAndValidate_NilSkillNames(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}

	result, err := LoadAndValidate(cfg, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestBuildPrompt_SingleSkill(t *testing.T) {
	skills := []*Skill{
		{Name: "skill-a", SystemPrompt: "You are an expert in Terraform."},
	}

	prompt := BuildPrompt(skills)
	assert.Equal(t, "You are an expert in Terraform.", prompt)
}

func TestBuildPrompt_MultipleSkills(t *testing.T) {
	skills := []*Skill{
		{Name: "skill-a", SystemPrompt: "Terraform expert."},
		{Name: "skill-b", SystemPrompt: "Security reviewer."},
	}

	prompt := BuildPrompt(skills)
	assert.Equal(t, "Terraform expert.\n\n---\n\nSecurity reviewer.", prompt)
}

func TestBuildPrompt_SkipsEmptyPrompts(t *testing.T) {
	skills := []*Skill{
		{Name: "skill-a", SystemPrompt: "Terraform expert."},
		{Name: "skill-b", SystemPrompt: ""},
		{Name: "skill-c", SystemPrompt: "Security reviewer."},
	}

	prompt := BuildPrompt(skills)
	assert.Equal(t, "Terraform expert.\n\n---\n\nSecurity reviewer.", prompt)
}

func TestBuildPrompt_AllEmptyPrompts(t *testing.T) {
	skills := []*Skill{
		{Name: "skill-a", SystemPrompt: ""},
		{Name: "skill-b", SystemPrompt: ""},
	}

	prompt := BuildPrompt(skills)
	assert.Empty(t, prompt)
}

func TestBuildPrompt_NilSlice(t *testing.T) {
	prompt := BuildPrompt(nil)
	assert.Empty(t, prompt)
}

func TestBuildPrompt_EmptySlice(t *testing.T) {
	prompt := BuildPrompt([]*Skill{})
	assert.Empty(t, prompt)
}
