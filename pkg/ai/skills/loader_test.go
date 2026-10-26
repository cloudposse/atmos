package skills

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// mockSkillLoader implements SkillLoader for testing.
type mockSkillLoader struct {
	skills []*Skill
	err    error
}

func (m *mockSkillLoader) LoadInstalledSkills(registry *Registry) error {
	if m.err != nil {
		return m.err
	}
	for _, skill := range m.skills {
		_ = registry.Register(skill)
	}
	return nil
}

func TestLoadSkills_WithNilConfig(t *testing.T) {
	// Arrange - nil config with no marketplace loader.
	var atmosConfig *schema.AtmosConfiguration = nil

	// Act.
	registry, err := LoadSkills(atmosConfig)

	// Assert - should return empty registry.
	require.NoError(t, err)
	require.NotNil(t, registry)
	assert.Equal(t, 0, registry.Count())
}

func TestLoadSkills_WithEmptyConfig(t *testing.T) {
	// Arrange - config with no custom skills, no marketplace loader.
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Skills: map[string]*schema.AISkillConfig{},
		},
	}

	// Act.
	registry, err := LoadSkills(atmosConfig)

	// Assert.
	require.NoError(t, err)
	require.NotNil(t, registry)
	assert.Equal(t, 0, registry.Count())
}

func TestLoadSkills_WithMarketplaceSkills(t *testing.T) {
	// Arrange - marketplace loader with skills.
	loader := &mockSkillLoader{
		skills: []*Skill{
			{Name: "atmos-terraform", DisplayName: "Terraform", Description: "Terraform skill"},
			{Name: "atmos-config", DisplayName: "Config", Description: "Config skill"},
		},
	}

	atmosConfig := &schema.AtmosConfiguration{}

	// Act.
	registry, err := LoadSkills(atmosConfig, loader)

	// Assert.
	require.NoError(t, err)
	require.NotNil(t, registry)
	assert.Equal(t, 2, registry.Count())
	assert.True(t, registry.Has("atmos-terraform"))
	assert.True(t, registry.Has("atmos-config"))
}

func TestLoadSkills_WithCustomSkills(t *testing.T) {
	// Arrange - config with custom skills.
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Skills: map[string]*schema.AISkillConfig{
				"custom-analyzer": {
					DisplayName:  "Custom Analyzer",
					Description:  "Custom skill for specific analysis",
					SystemPrompt: "You are a custom analyzer",
					AllowedTools: []string{"read_file", "search_files"},
					Category:     "analysis",
				},
				"custom-refactor": {
					DisplayName:     "Custom Refactor",
					Description:     "Custom refactoring skill",
					SystemPrompt:    "You are a custom refactoring assistant",
					AllowedTools:    []string{"read_file", "edit_file"},
					RestrictedTools: []string{"execute_command"},
					Category:        "refactor",
				},
			},
		},
	}

	// Act.
	registry, err := LoadSkills(atmosConfig)

	// Assert.
	require.NoError(t, err)
	require.NotNil(t, registry)
	assert.Equal(t, 2, registry.Count())

	// Verify custom skills are registered.
	customSkill1, err := registry.Get("custom-analyzer")
	require.NoError(t, err)
	assert.Equal(t, "Custom Analyzer", customSkill1.DisplayName)
	assert.Equal(t, "Custom skill for specific analysis", customSkill1.Description)
	assert.Equal(t, "analysis", customSkill1.Category)
	assert.False(t, customSkill1.IsBuiltIn)
	assert.Equal(t, []string{"read_file", "search_files"}, customSkill1.AllowedTools)

	customSkill2, err := registry.Get("custom-refactor")
	require.NoError(t, err)
	assert.Equal(t, "Custom Refactor", customSkill2.DisplayName)
	assert.Equal(t, "refactor", customSkill2.Category)
	assert.False(t, customSkill2.IsBuiltIn)
	assert.Equal(t, []string{"execute_command"}, customSkill2.RestrictedTools)
}

func TestLoadSkills_WithInvalidCustomSkill_ShouldContinue(t *testing.T) {
	// Arrange - config with invalid custom skills (empty name).
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Skills: map[string]*schema.AISkillConfig{
				"": { // Empty name - invalid.
					DisplayName:  "Invalid Skill",
					Description:  "This skill has an empty name",
					SystemPrompt: "This should be skipped",
				},
				"valid-skill": {
					DisplayName:  "Valid Skill",
					Description:  "This skill should be registered",
					SystemPrompt: "This should work",
					Category:     "test",
				},
			},
		},
	}

	// Act.
	registry, err := LoadSkills(atmosConfig)

	// Assert.
	require.NoError(t, err)
	require.NotNil(t, registry)

	// Should have only the valid custom skill (invalid one was skipped).
	assert.Equal(t, 1, registry.Count())

	// Verify valid custom skill is registered.
	validSkill, err := registry.Get("valid-skill")
	require.NoError(t, err)
	assert.Equal(t, "Valid Skill", validSkill.DisplayName)

	// Verify invalid skill is not registered.
	assert.False(t, registry.Has(""))
}

func TestLoadSkills_MarketplaceAndCustom(t *testing.T) {
	// Arrange - marketplace skills and custom skills together.
	loader := &mockSkillLoader{
		skills: []*Skill{
			{Name: "marketplace-skill", DisplayName: "Marketplace", Description: "From marketplace"},
		},
	}

	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Skills: map[string]*schema.AISkillConfig{
				"custom-skill": {
					DisplayName: "Custom",
					Description: "From config",
				},
			},
		},
	}

	// Act.
	registry, err := LoadSkills(atmosConfig, loader)

	// Assert.
	require.NoError(t, err)
	assert.Equal(t, 2, registry.Count())
	assert.True(t, registry.Has("marketplace-skill"))
	assert.True(t, registry.Has("custom-skill"))
}

func TestLoadSkills_DuplicateCustomSkillSkipped(t *testing.T) {
	// Arrange - marketplace skill and custom skill with same name.
	loader := &mockSkillLoader{
		skills: []*Skill{
			{Name: "my-skill", DisplayName: "Marketplace Version", Description: "From marketplace"},
		},
	}

	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Skills: map[string]*schema.AISkillConfig{
				"my-skill": { // Duplicate of marketplace skill.
					DisplayName: "Custom Version",
					Description: "This conflicts with marketplace skill",
				},
			},
		},
	}

	// Act.
	registry, err := LoadSkills(atmosConfig, loader)

	// Assert - marketplace version wins (registered first).
	require.NoError(t, err)
	assert.Equal(t, 1, registry.Count())
	skill, err := registry.Get("my-skill")
	require.NoError(t, err)
	assert.Equal(t, "Marketplace Version", skill.DisplayName)
}

func TestGetDefaultSkill_WithNilConfig(t *testing.T) {
	var atmosConfig *schema.AtmosConfiguration = nil
	defaultSkill := GetDefaultSkill(atmosConfig)
	assert.Equal(t, "", defaultSkill)
}

func TestGetDefaultSkill_WithEmptyDefaultSkill(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			DefaultSkill: "",
		},
	}
	defaultSkill := GetDefaultSkill(atmosConfig)
	assert.Equal(t, "", defaultSkill)
}

func TestGetDefaultSkill_WithCustomDefaultSkill(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			DefaultSkill: "atmos-terraform",
		},
	}
	defaultSkill := GetDefaultSkill(atmosConfig)
	assert.Equal(t, "atmos-terraform", defaultSkill)
}

func TestGetDefaultSkill_WithUserDefinedDefaultSkill(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			DefaultSkill: "my-custom-skill",
		},
	}
	defaultSkill := GetDefaultSkill(atmosConfig)
	assert.Equal(t, "my-custom-skill", defaultSkill)
}

func TestLoadSkills_RegistryIntegrity(t *testing.T) {
	// Arrange - config with multiple custom skills.
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Skills: map[string]*schema.AISkillConfig{
				"skill-1": {
					DisplayName: "Skill 1",
					Description: "First skill",
					Category:    "test",
				},
				"skill-2": {
					DisplayName: "Skill 2",
					Description: "Second skill",
					Category:    "test",
				},
				"skill-3": {
					DisplayName: "Skill 3",
					Description: "Third skill",
					Category:    "test",
				},
			},
		},
	}

	// Act.
	registry, err := LoadSkills(atmosConfig)

	// Assert.
	require.NoError(t, err)
	require.NotNil(t, registry)

	// Verify registry operations work correctly.
	assert.True(t, registry.Has("skill-1"))
	assert.True(t, registry.Has("skill-2"))
	assert.True(t, registry.Has("skill-3"))

	// Verify list operations.
	allSkills := registry.List()
	assert.Equal(t, 3, len(allSkills))

	customSkills := registry.ListCustom()
	assert.Equal(t, 3, len(customSkills))
	for _, skill := range customSkills {
		assert.False(t, skill.IsBuiltIn)
	}
}

func TestLoadSkills_NilSkillConfig(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Skills: map[string]*schema.AISkillConfig{
				"valid-skill": {
					DisplayName: "Valid Skill",
					Description: "This should work",
				},
			},
		},
	}

	registry, err := LoadSkills(atmosConfig)

	require.NoError(t, err)
	require.NotNil(t, registry)

	skill, err := registry.Get("valid-skill")
	require.NoError(t, err)
	assert.Equal(t, "Valid Skill", skill.DisplayName)
}

func TestLoadSkills_NilLoader(t *testing.T) {
	// Passing nil loader explicitly.
	atmosConfig := &schema.AtmosConfiguration{}
	registry, err := LoadSkills(atmosConfig, nil)

	require.NoError(t, err)
	require.NotNil(t, registry)
	assert.Equal(t, 0, registry.Count())
}
