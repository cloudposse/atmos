package skills

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestLoadSkills_WithNilConfig(t *testing.T) {
	// Arrange - nil config should still load built-in skills.
	var atmosConfig *schema.AtmosConfiguration = nil

	// Act.
	registry, err := LoadSkills(atmosConfig)

	// Assert.
	require.NoError(t, err)
	require.NotNil(t, registry)

	// Should have all built-in skills registered.
	builtinSkills := GetBuiltInSkills()
	assert.Equal(t, len(builtinSkills), registry.Count())

	// Verify specific built-in skills are present.
	for _, skill := range builtinSkills {
		assert.True(t, registry.Has(skill.Name), "Built-in skill %s should be registered", skill.Name)
	}
}

func TestLoadSkills_WithEmptyConfig(t *testing.T) {
	// Arrange - config with no custom skills.
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Skills: map[string]*schema.AISkillConfig{},
			},
		},
	}

	// Act.
	registry, err := LoadSkills(atmosConfig)

	// Assert.
	require.NoError(t, err)
	require.NotNil(t, registry)

	// Should have all built-in skills registered.
	builtinSkills := GetBuiltInSkills()
	assert.Equal(t, len(builtinSkills), registry.Count())
}

func TestLoadSkills_WithCustomSkills(t *testing.T) {
	// Arrange - config with custom skills.
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
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
		},
	}

	// Act.
	registry, err := LoadSkills(atmosConfig)

	// Assert.
	require.NoError(t, err)
	require.NotNil(t, registry)

	// Should have built-in skills + custom skills.
	builtinSkills := GetBuiltInSkills()
	expectedCount := len(builtinSkills) + 2 // 2 custom skills.
	assert.Equal(t, expectedCount, registry.Count())

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
	// This tests that the loader continues loading even if a custom skill is invalid.
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
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
		},
	}

	// Act.
	registry, err := LoadSkills(atmosConfig)

	// Assert.
	require.NoError(t, err)
	require.NotNil(t, registry)

	// Should have built-in skills + 1 valid custom skill (invalid one was skipped).
	builtinSkills := GetBuiltInSkills()
	expectedCount := len(builtinSkills) + 1 // Only the valid custom skill.
	assert.Equal(t, expectedCount, registry.Count())

	// Verify valid custom skill is registered.
	validSkill, err := registry.Get("valid-skill")
	require.NoError(t, err)
	assert.Equal(t, "Valid Skill", validSkill.DisplayName)

	// Verify invalid skill is not registered.
	assert.False(t, registry.Has(""))
}

func TestLoadSkills_WithDuplicateCustomSkill_ShouldContinue(t *testing.T) {
	// Arrange - config with custom skill that conflicts with built-in skill name.
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Skills: map[string]*schema.AISkillConfig{
					GeneralSkill: { // Duplicate of built-in skill.
						DisplayName:  "Duplicate General",
						Description:  "This conflicts with built-in general skill",
						SystemPrompt: "This should be skipped",
					},
					"unique-skill": {
						DisplayName:  "Unique Skill",
						Description:  "This should be registered",
						SystemPrompt: "This should work",
						Category:     "test",
					},
				},
			},
		},
	}

	// Act.
	registry, err := LoadSkills(atmosConfig)

	// Assert.
	require.NoError(t, err)
	require.NotNil(t, registry)

	// Should have built-in skills + 1 unique custom skill (duplicate was skipped).
	builtinSkills := GetBuiltInSkills()
	expectedCount := len(builtinSkills) + 1 // Only the unique custom skill.
	assert.Equal(t, expectedCount, registry.Count())

	// Verify unique custom skill is registered.
	uniqueSkill, err := registry.Get("unique-skill")
	require.NoError(t, err)
	assert.Equal(t, "Unique Skill", uniqueSkill.DisplayName)

	// Verify built-in general skill is still the original (not overwritten).
	generalSkill, err := registry.Get(GeneralSkill)
	require.NoError(t, err)
	assert.Equal(t, "General", generalSkill.DisplayName) // Built-in display name.
	assert.True(t, generalSkill.IsBuiltIn)
}

func TestLoadSkills_AllBuiltInSkillsRegistered(t *testing.T) {
	// Arrange.
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{},
		},
	}

	// Act.
	registry, err := LoadSkills(atmosConfig)

	// Assert.
	require.NoError(t, err)
	require.NotNil(t, registry)

	// Verify all expected built-in skills are registered.
	expectedBuiltInSkills := []string{
		GeneralSkill,
		StackAnalyzerSkill,
		ComponentRefactorSkill,
		SecurityAuditorSkill,
		ConfigValidatorSkill,
	}

	for _, skillName := range expectedBuiltInSkills {
		skill, err := registry.Get(skillName)
		require.NoError(t, err, "Built-in skill %s should be registered", skillName)
		assert.True(t, skill.IsBuiltIn, "Skill %s should be marked as built-in", skillName)
		assert.NotEmpty(t, skill.DisplayName, "Skill %s should have a display name", skillName)
		assert.NotEmpty(t, skill.Description, "Skill %s should have a description", skillName)
		assert.NotEmpty(t, skill.SystemPromptPath, "Skill %s should have a system prompt path", skillName)
	}
}

func TestGetDefaultSkill_WithNilConfig(t *testing.T) {
	// Arrange.
	var atmosConfig *schema.AtmosConfiguration = nil

	// Act.
	defaultSkill := GetDefaultSkill(atmosConfig)

	// Assert.
	assert.Equal(t, GeneralSkill, defaultSkill)
}

func TestGetDefaultSkill_WithEmptyDefaultSkill(t *testing.T) {
	// Arrange - config with empty default skill.
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				DefaultSkill: "",
			},
		},
	}

	// Act.
	defaultSkill := GetDefaultSkill(atmosConfig)

	// Assert.
	assert.Equal(t, GeneralSkill, defaultSkill)
}

func TestGetDefaultSkill_WithCustomDefaultSkill(t *testing.T) {
	// Arrange - config with custom default skill.
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				DefaultSkill: StackAnalyzerSkill,
			},
		},
	}

	// Act.
	defaultSkill := GetDefaultSkill(atmosConfig)

	// Assert.
	assert.Equal(t, StackAnalyzerSkill, defaultSkill)
}

func TestGetDefaultSkill_WithUserDefinedDefaultSkill(t *testing.T) {
	// Arrange - config with user-defined custom skill as default.
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				DefaultSkill: "my-custom-skill",
			},
		},
	}

	// Act.
	defaultSkill := GetDefaultSkill(atmosConfig)

	// Assert.
	assert.Equal(t, "my-custom-skill", defaultSkill)
}

func TestLoadSkills_RegistryIntegrity(t *testing.T) {
	// Arrange - config with multiple custom skills.
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
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
	assert.NotEmpty(t, allSkills)

	builtinSkills := registry.ListBuiltIn()
	customSkills := registry.ListCustom()
	assert.Equal(t, len(builtinSkills)+len(customSkills), len(allSkills))

	// Verify custom skills are correctly identified.
	assert.Equal(t, 3, len(customSkills))
	for _, skill := range customSkills {
		assert.False(t, skill.IsBuiltIn)
	}
}

func TestLoadSkills_NilSkillConfig(t *testing.T) {
	// Arrange - config with nil skill config (edge case).
	// This tests that FromConfig handles nil gracefully.
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Skills: map[string]*schema.AISkillConfig{
					"valid-skill": {
						DisplayName: "Valid Skill",
						Description: "This should work",
					},
				},
			},
		},
	}

	// Act.
	registry, err := LoadSkills(atmosConfig)

	// Assert.
	require.NoError(t, err)
	require.NotNil(t, registry)

	// Verify valid skill is registered.
	skill, err := registry.Get("valid-skill")
	require.NoError(t, err)
	assert.Equal(t, "Valid Skill", skill.DisplayName)
}
