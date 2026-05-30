package skills

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestFromConfig(t *testing.T) {
	tests := []struct {
		name           string
		skillName      string
		config         *schema.AISkillConfig
		checkAssertion func(*testing.T, *Skill)
	}{
		{
			name:      "complete config with all fields",
			skillName: "test-analyzer",
			config: &schema.AISkillConfig{
				DisplayName:     "Test Analyzer",
				Description:     "Analyzes test cases",
				SystemPrompt:    "You are a test analyzer assistant.",
				AllowedTools:    []string{"read", "write", "grep"},
				RestrictedTools: []string{"delete", "execute"},
				Category:        "analysis",
			},
			checkAssertion: func(t *testing.T, skill *Skill) {
				assert.Equal(t, "test-analyzer", skill.Name)
				assert.Equal(t, "Test Analyzer", skill.DisplayName)
				assert.Equal(t, "Analyzes test cases", skill.Description)
				assert.Equal(t, "You are a test analyzer assistant.", skill.SystemPrompt)
				assert.Equal(t, []string{"read", "write", "grep"}, skill.AllowedTools)
				assert.Equal(t, []string{"delete", "execute"}, skill.RestrictedTools)
				assert.Equal(t, "analysis", skill.Category)
				assert.False(t, skill.IsBuiltIn)
			},
		},
		{
			name:      "minimal config with only required fields",
			skillName: "minimal-skill",
			config: &schema.AISkillConfig{
				DisplayName: "Minimal Skill",
				Description: "A minimal skill",
			},
			checkAssertion: func(t *testing.T, skill *Skill) {
				assert.Equal(t, "minimal-skill", skill.Name)
				assert.Equal(t, "Minimal Skill", skill.DisplayName)
				assert.Equal(t, "A minimal skill", skill.Description)
				assert.Empty(t, skill.SystemPrompt)
				assert.Nil(t, skill.AllowedTools)
				assert.Nil(t, skill.RestrictedTools)
				assert.Empty(t, skill.Category)
				assert.False(t, skill.IsBuiltIn)
			},
		},
		{
			name:      "config with empty tool lists",
			skillName: "empty-tools-skill",
			config: &schema.AISkillConfig{
				DisplayName:     "Empty Tools Skill",
				Description:     "Skill with empty tool lists",
				AllowedTools:    []string{},
				RestrictedTools: []string{},
			},
			checkAssertion: func(t *testing.T, skill *Skill) {
				assert.Equal(t, "empty-tools-skill", skill.Name)
				assert.NotNil(t, skill.AllowedTools)
				assert.Empty(t, skill.AllowedTools)
				assert.NotNil(t, skill.RestrictedTools)
				assert.Empty(t, skill.RestrictedTools)
			},
		},
		{
			name:      "config with only allowed tools",
			skillName: "allowed-only",
			config: &schema.AISkillConfig{
				DisplayName:  "Allowed Only",
				Description:  "Only allows specific tools",
				AllowedTools: []string{"read", "grep"},
			},
			checkAssertion: func(t *testing.T, skill *Skill) {
				assert.Equal(t, []string{"read", "grep"}, skill.AllowedTools)
				assert.Nil(t, skill.RestrictedTools)
			},
		},
		{
			name:      "config with only restricted tools",
			skillName: "restricted-only",
			config: &schema.AISkillConfig{
				DisplayName:     "Restricted Only",
				Description:     "Only restricts specific tools",
				RestrictedTools: []string{"delete", "execute"},
			},
			checkAssertion: func(t *testing.T, skill *Skill) {
				assert.Nil(t, skill.AllowedTools)
				assert.Equal(t, []string{"delete", "execute"}, skill.RestrictedTools)
			},
		},
		{
			name:      "config with special characters in name",
			skillName: "special-skill_v1.0",
			config: &schema.AISkillConfig{
				DisplayName: "Special Skill v1.0",
				Description: "Skill with special characters",
			},
			checkAssertion: func(t *testing.T, skill *Skill) {
				assert.Equal(t, "special-skill_v1.0", skill.Name)
				assert.Equal(t, "Special Skill v1.0", skill.DisplayName)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skill := FromConfig(tt.skillName, tt.config)
			require.NotNil(t, skill)
			tt.checkAssertion(t, skill)
		})
	}
}

func TestFromConfig_NilConfig(t *testing.T) {
	// Test that FromConfig panics with nil config (expected behavior).
	// In production, nil should never be passed due to config validation.
	defer func() {
		if r := recover(); r == nil {
			t.Error("FromConfig should panic with nil config")
		}
	}()

	FromConfig("test-skill", nil)
}

func TestSkill_IsToolAllowed(t *testing.T) {
	tests := []struct {
		name          string
		skill         *Skill
		toolName      string
		expectAllowed bool
	}{
		{
			name: "empty allowed list allows all tools",
			skill: &Skill{
				Name:         "open-skill",
				AllowedTools: []string{},
			},
			toolName:      "any-tool",
			expectAllowed: true,
		},
		{
			name: "nil allowed list allows all tools",
			skill: &Skill{
				Name:         "open-skill",
				AllowedTools: nil,
			},
			toolName:      "any-tool",
			expectAllowed: true,
		},
		{
			name: "tool in allowed list is allowed",
			skill: &Skill{
				Name:         "restricted-skill",
				AllowedTools: []string{"read", "write", "grep"},
			},
			toolName:      "read",
			expectAllowed: true,
		},
		{
			name: "tool not in allowed list is not allowed",
			skill: &Skill{
				Name:         "restricted-skill",
				AllowedTools: []string{"read", "write", "grep"},
			},
			toolName:      "delete",
			expectAllowed: false,
		},
		{
			name: "exact match required (case sensitive)",
			skill: &Skill{
				Name:         "case-sensitive-skill",
				AllowedTools: []string{"Read", "Write"},
			},
			toolName:      "read",
			expectAllowed: false,
		},
		{
			name: "exact match works (case sensitive)",
			skill: &Skill{
				Name:         "case-sensitive-skill",
				AllowedTools: []string{"Read", "Write"},
			},
			toolName:      "Read",
			expectAllowed: true,
		},
		{
			name: "empty tool name not allowed when list is restricted",
			skill: &Skill{
				Name:         "restricted-skill",
				AllowedTools: []string{"read", "write"},
			},
			toolName:      "",
			expectAllowed: false,
		},
		{
			name: "empty tool name allowed when list is empty",
			skill: &Skill{
				Name:         "open-skill",
				AllowedTools: []string{},
			},
			toolName:      "",
			expectAllowed: true,
		},
		{
			name: "multiple tools with same name in list (edge case)",
			skill: &Skill{
				Name:         "duplicate-skill",
				AllowedTools: []string{"read", "read", "write"},
			},
			toolName:      "read",
			expectAllowed: true,
		},
		{
			name: "tool with special characters",
			skill: &Skill{
				Name:         "special-chars-skill",
				AllowedTools: []string{"read-file", "write_file", "grep.tool"},
			},
			toolName:      "read-file",
			expectAllowed: true,
		},
		{
			name: "single allowed tool",
			skill: &Skill{
				Name:         "single-tool-skill",
				AllowedTools: []string{"read"},
			},
			toolName:      "read",
			expectAllowed: true,
		},
		{
			name: "single allowed tool - checking different tool",
			skill: &Skill{
				Name:         "single-tool-skill",
				AllowedTools: []string{"read"},
			},
			toolName:      "write",
			expectAllowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.skill.IsToolAllowed(tt.toolName)
			assert.Equal(t, tt.expectAllowed, result)
		})
	}
}

func TestSkill_IsToolRestricted(t *testing.T) {
	tests := []struct {
		name             string
		skill            *Skill
		toolName         string
		expectRestricted bool
	}{
		{
			name: "tool in restricted list is restricted",
			skill: &Skill{
				Name:            "safe-skill",
				RestrictedTools: []string{"delete", "execute", "format"},
			},
			toolName:         "delete",
			expectRestricted: true,
		},
		{
			name: "tool not in restricted list is not restricted",
			skill: &Skill{
				Name:            "safe-skill",
				RestrictedTools: []string{"delete", "execute"},
			},
			toolName:         "read",
			expectRestricted: false,
		},
		{
			name: "empty restricted list means no restrictions",
			skill: &Skill{
				Name:            "unrestricted-skill",
				RestrictedTools: []string{},
			},
			toolName:         "delete",
			expectRestricted: false,
		},
		{
			name: "nil restricted list means no restrictions",
			skill: &Skill{
				Name:            "unrestricted-skill",
				RestrictedTools: nil,
			},
			toolName:         "delete",
			expectRestricted: false,
		},
		{
			name: "exact match required (case sensitive)",
			skill: &Skill{
				Name:            "case-sensitive-skill",
				RestrictedTools: []string{"Delete", "Execute"},
			},
			toolName:         "delete",
			expectRestricted: false,
		},
		{
			name: "exact match works (case sensitive)",
			skill: &Skill{
				Name:            "case-sensitive-skill",
				RestrictedTools: []string{"Delete", "Execute"},
			},
			toolName:         "Delete",
			expectRestricted: true,
		},
		{
			name: "empty tool name not in restricted list",
			skill: &Skill{
				Name:            "safe-skill",
				RestrictedTools: []string{"delete"},
			},
			toolName:         "",
			expectRestricted: false,
		},
		{
			name: "tool with special characters",
			skill: &Skill{
				Name:            "special-chars-skill",
				RestrictedTools: []string{"delete-all", "format_disk", "rm.force"},
			},
			toolName:         "delete-all",
			expectRestricted: true,
		},
		{
			name: "multiple restricted tools",
			skill: &Skill{
				Name:            "multi-restricted-skill",
				RestrictedTools: []string{"delete", "execute", "format", "truncate", "drop"},
			},
			toolName:         "format",
			expectRestricted: true,
		},
		{
			name: "single restricted tool - match",
			skill: &Skill{
				Name:            "single-restricted-skill",
				RestrictedTools: []string{"delete"},
			},
			toolName:         "delete",
			expectRestricted: true,
		},
		{
			name: "single restricted tool - no match",
			skill: &Skill{
				Name:            "single-restricted-skill",
				RestrictedTools: []string{"delete"},
			},
			toolName:         "execute",
			expectRestricted: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.skill.IsToolRestricted(tt.toolName)
			assert.Equal(t, tt.expectRestricted, result)
		})
	}
}

func TestSkill_LoadSystemPrompt(t *testing.T) {
	tests := []struct {
		name         string
		skill        *Skill
		contentCheck func(*testing.T, string)
	}{
		{
			name: "returns hardcoded prompt",
			skill: &Skill{
				Name:         "hardcoded",
				SystemPrompt: "This is a hardcoded system prompt.",
			},
			contentCheck: func(t *testing.T, content string) {
				assert.Equal(t, "This is a hardcoded system prompt.", content)
			},
		},
		{
			name: "returns empty when prompt is empty",
			skill: &Skill{
				Name:         "empty",
				SystemPrompt: "",
			},
			contentCheck: func(t *testing.T, content string) {
				assert.Empty(t, content)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := tt.skill.LoadSystemPrompt()
			require.NoError(t, err)
			if tt.contentCheck != nil {
				tt.contentCheck(t, content)
			}
		})
	}
}

func TestNewFallbackSkill(t *testing.T) {
	skill := NewFallbackSkill()

	assert.Equal(t, "general", skill.Name)
	assert.Equal(t, "General", skill.DisplayName)
	assert.NotEmpty(t, skill.Description)
	assert.NotEmpty(t, skill.SystemPrompt)
	assert.Contains(t, skill.SystemPrompt, "Atmos AI")
	assert.Contains(t, skill.SystemPrompt, "atmos ai skill install")
	assert.Equal(t, "general", skill.Category)
	assert.False(t, skill.IsBuiltIn)

	// Fallback skill should allow all tools.
	assert.True(t, skill.IsToolAllowed("any-tool"))
	assert.False(t, skill.IsToolRestricted("any-tool"))
}

func TestSkill_AllFields(t *testing.T) {
	// Test that all Skill struct fields can be set and retrieved.
	skill := &Skill{
		Name:            "complete-skill",
		DisplayName:     "Complete Skill",
		Description:     "A skill with all fields set",
		SystemPrompt:    "System prompt content",
		AllowedTools:    []string{"tool1", "tool2"},
		RestrictedTools: []string{"tool3", "tool4"},
		Category:        "testing",
		IsBuiltIn:       true,
	}

	assert.Equal(t, "complete-skill", skill.Name)
	assert.Equal(t, "Complete Skill", skill.DisplayName)
	assert.Equal(t, "A skill with all fields set", skill.Description)
	assert.Equal(t, "System prompt content", skill.SystemPrompt)
	assert.Equal(t, []string{"tool1", "tool2"}, skill.AllowedTools)
	assert.Equal(t, []string{"tool3", "tool4"}, skill.RestrictedTools)
	assert.Equal(t, "testing", skill.Category)
	assert.True(t, skill.IsBuiltIn)
}

func TestSkill_EmptySkill(t *testing.T) {
	// Test zero value of Skill struct.
	skill := &Skill{}

	assert.Empty(t, skill.Name)
	assert.Empty(t, skill.DisplayName)
	assert.Empty(t, skill.Description)
	assert.Empty(t, skill.SystemPrompt)
	assert.Nil(t, skill.AllowedTools)
	assert.Nil(t, skill.RestrictedTools)
	assert.Empty(t, skill.Category)
	assert.False(t, skill.IsBuiltIn)

	// Empty skill should allow all tools.
	assert.True(t, skill.IsToolAllowed("any-tool"))

	// Empty skill should not restrict any tools.
	assert.False(t, skill.IsToolRestricted("any-tool"))
}

func TestSkill_ToolInteraction(t *testing.T) {
	// Test that a tool can be both in allowed and restricted lists.
	// This is a valid configuration where a tool is allowed but requires confirmation.
	skill := &Skill{
		Name:            "careful-skill",
		AllowedTools:    []string{"delete", "read", "write"},
		RestrictedTools: []string{"delete"},
	}

	// Tool should be both allowed and restricted.
	assert.True(t, skill.IsToolAllowed("delete"))
	assert.True(t, skill.IsToolRestricted("delete"))

	// Other tools should behave normally.
	assert.True(t, skill.IsToolAllowed("read"))
	assert.False(t, skill.IsToolRestricted("read"))

	assert.False(t, skill.IsToolAllowed("execute"))
	assert.False(t, skill.IsToolRestricted("execute"))
}

func TestSkill_BuiltInStatus(t *testing.T) {
	// Test that IsBuiltIn flag is correctly set by FromConfig.
	config := &schema.AISkillConfig{
		DisplayName: "Test Skill",
		Description: "Test description",
	}

	skill := FromConfig("test", config)
	assert.False(t, skill.IsBuiltIn, "Skills created from config should not be built-in")

	// Manually creating a built-in skill.
	builtInSkill := &Skill{
		Name:      "built-in",
		IsBuiltIn: true,
	}
	assert.True(t, builtInSkill.IsBuiltIn)
}

func TestFromConfig_PreservesAllFields(t *testing.T) {
	// Verify that FromConfig doesn't lose any data from the config.
	config := &schema.AISkillConfig{
		DisplayName:     "Display Name",
		Description:     "Description text",
		SystemPrompt:    "System prompt text",
		AllowedTools:    []string{"a", "b", "c"},
		RestrictedTools: []string{"x", "y", "z"},
		Category:        "category-name",
	}

	skill := FromConfig("test-skill", config)

	// Verify all fields are preserved.
	assert.Equal(t, "test-skill", skill.Name)
	assert.Equal(t, config.DisplayName, skill.DisplayName)
	assert.Equal(t, config.Description, skill.Description)
	assert.Equal(t, config.SystemPrompt, skill.SystemPrompt)
	assert.Equal(t, config.AllowedTools, skill.AllowedTools)
	assert.Equal(t, config.RestrictedTools, skill.RestrictedTools)
	assert.Equal(t, config.Category, skill.Category)
	assert.False(t, skill.IsBuiltIn)
}
