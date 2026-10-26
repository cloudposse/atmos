package skills

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_NewRegistry(t *testing.T) {
	registry := NewRegistry()
	require.NotNil(t, registry)
	assert.Equal(t, 0, registry.Count())
}

func TestRegistry_Register(t *testing.T) {
	t.Run("register valid skill", func(t *testing.T) {
		registry := NewRegistry()
		err := registry.Register(&Skill{Name: "test", DisplayName: "Test"})
		assert.NoError(t, err)
		assert.Equal(t, 1, registry.Count())
	})

	t.Run("register nil skill", func(t *testing.T) {
		registry := NewRegistry()
		err := registry.Register(nil)
		assert.Error(t, err)
	})

	t.Run("register skill with empty name", func(t *testing.T) {
		registry := NewRegistry()
		err := registry.Register(&Skill{Name: ""})
		assert.Error(t, err)
	})

	t.Run("register duplicate skill", func(t *testing.T) {
		registry := NewRegistry()
		err := registry.Register(&Skill{Name: "test"})
		require.NoError(t, err)

		err = registry.Register(&Skill{Name: "test"})
		assert.Error(t, err)
	})
}

func TestRegistry_Get(t *testing.T) {
	registry := NewRegistry()
	_ = registry.Register(&Skill{Name: "test", DisplayName: "Test"})

	t.Run("get existing skill", func(t *testing.T) {
		skill, err := registry.Get("test")
		require.NoError(t, err)
		assert.Equal(t, "Test", skill.DisplayName)
	})

	t.Run("get non-existing skill", func(t *testing.T) {
		_, err := registry.Get("nonexistent")
		assert.Error(t, err)
	})
}

func TestRegistry_List(t *testing.T) {
	registry := NewRegistry()
	_ = registry.Register(&Skill{Name: "bravo"})
	_ = registry.Register(&Skill{Name: "alpha"})
	_ = registry.Register(&Skill{Name: "charlie"})

	skills := registry.List()
	assert.Equal(t, 3, len(skills))
	// Verify alphabetical sorting.
	assert.Equal(t, "alpha", skills[0].Name)
	assert.Equal(t, "bravo", skills[1].Name)
	assert.Equal(t, "charlie", skills[2].Name)
}

func TestRegistry_ListByCategory(t *testing.T) {
	registry := NewRegistry()
	_ = registry.Register(&Skill{Name: "skill-a", Category: "security"})
	_ = registry.Register(&Skill{Name: "skill-b", Category: "analysis"})
	_ = registry.Register(&Skill{Name: "skill-c", Category: "security"})
	_ = registry.Register(&Skill{Name: "skill-d", Category: "general"})

	t.Run("list security skills", func(t *testing.T) {
		skills := registry.ListByCategory("security")
		assert.Equal(t, 2, len(skills))
		assert.Equal(t, "skill-a", skills[0].Name)
		assert.Equal(t, "skill-c", skills[1].Name)
	})

	t.Run("list analysis skills", func(t *testing.T) {
		skills := registry.ListByCategory("analysis")
		assert.Equal(t, 1, len(skills))
		assert.Equal(t, "skill-b", skills[0].Name)
	})

	t.Run("list nonexistent category", func(t *testing.T) {
		skills := registry.ListByCategory("nonexistent")
		assert.Equal(t, 0, len(skills))
	})

	t.Run("list empty category", func(t *testing.T) {
		skills := registry.ListByCategory("")
		assert.Equal(t, 0, len(skills))
	})
}

func TestRegistry_ListBuiltIn(t *testing.T) {
	registry := NewRegistry()
	_ = registry.Register(&Skill{Name: "builtin-a", IsBuiltIn: true})
	_ = registry.Register(&Skill{Name: "custom-a", IsBuiltIn: false})
	_ = registry.Register(&Skill{Name: "builtin-b", IsBuiltIn: true})

	skills := registry.ListBuiltIn()
	assert.Equal(t, 2, len(skills))
	assert.Equal(t, "builtin-a", skills[0].Name)
	assert.Equal(t, "builtin-b", skills[1].Name)
}

func TestRegistry_ListCustom(t *testing.T) {
	registry := NewRegistry()
	_ = registry.Register(&Skill{Name: "builtin-a", IsBuiltIn: true})
	_ = registry.Register(&Skill{Name: "custom-a", IsBuiltIn: false})
	_ = registry.Register(&Skill{Name: "custom-b", IsBuiltIn: false})

	skills := registry.ListCustom()
	assert.Equal(t, 2, len(skills))
	assert.Equal(t, "custom-a", skills[0].Name)
	assert.Equal(t, "custom-b", skills[1].Name)
}

func TestRegistry_Unregister(t *testing.T) {
	t.Run("unregister existing skill", func(t *testing.T) {
		registry := NewRegistry()
		_ = registry.Register(&Skill{Name: "test"})
		assert.Equal(t, 1, registry.Count())

		err := registry.Unregister("test")
		assert.NoError(t, err)
		assert.Equal(t, 0, registry.Count())
		assert.False(t, registry.Has("test"))
	})

	t.Run("unregister nonexistent skill", func(t *testing.T) {
		registry := NewRegistry()
		err := registry.Unregister("nonexistent")
		assert.Error(t, err)
	})
}

func TestRegistry_Has(t *testing.T) {
	registry := NewRegistry()
	_ = registry.Register(&Skill{Name: "exists"})

	assert.True(t, registry.Has("exists"))
	assert.False(t, registry.Has("does-not-exist"))
}

func TestRegistry_ToPromptXML(t *testing.T) {
	t.Run("generates XML with skills", func(t *testing.T) {
		registry := NewRegistry()
		_ = registry.Register(&Skill{Name: "atmos-terraform", Description: "Terraform ops", Category: "terraform"})
		_ = registry.Register(&Skill{Name: "atmos-security", Description: "Security review", Category: "security"})

		xml := registry.ToPromptXML("atmos-terraform")
		assert.Contains(t, xml, "<available_skills>")
		assert.Contains(t, xml, "<current_skill>atmos-terraform</current_skill>")
		assert.Contains(t, xml, "<name>atmos-terraform</name>")
		assert.Contains(t, xml, "<description>Terraform ops</description>")
		assert.Contains(t, xml, "<category>terraform</category>")
		assert.Contains(t, xml, "<name>atmos-security</name>")
		assert.Contains(t, xml, "<description>Security review</description>")
		assert.Contains(t, xml, "Ctrl+A")
		assert.Contains(t, xml, "</available_skills>")
	})

	t.Run("skill without category omits category tag", func(t *testing.T) {
		registry := NewRegistry()
		_ = registry.Register(&Skill{Name: "no-cat", Description: "No category"})

		xml := registry.ToPromptXML("no-cat")
		assert.Contains(t, xml, "<name>no-cat</name>")
		assert.NotContains(t, xml, "<category>")
	})

	t.Run("empty registry returns empty string", func(t *testing.T) {
		registry := NewRegistry()
		xml := registry.ToPromptXML("any")
		assert.Equal(t, "", xml)
	})
}
